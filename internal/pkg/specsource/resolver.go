package specsource

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// ResolverOptions tweak Resolver behavior. Zero value is fine for production
// (uses os.Getenv, http.DefaultClient with 60s timeout, real `git`).
type ResolverOptions struct {
	// EnvLookup is consulted when a source needs an environment variable
	// (token_env). Defaults to os.Getenv.
	EnvLookup func(string) string
	// HTTPClient is used for HTTPSource. Defaults to a new client with 60s
	// timeout.
	HTTPClient *http.Client
	// LogStderr is the writer that receives git's stderr (token-redacted).
	// Defaults to os.Stderr.
	LogStderr io.Writer
	// SourceRefs overrides the ref of GitSources whose URI did not pin one
	// explicitly (see GitSource.RefPinned), per repository or for all of them
	// at once. Nil means "no override": each source keeps the ref it was
	// parsed with. Build it with ParseSourceRefs, which validates every ref.
	SourceRefs *SourceRefs
}

// stagingDirPrefix is the os.MkdirTemp prefix of the staging directory, so a
// leftover is recognizable as ours.
const stagingDirPrefix = "gps-specs-"

// Resolver materializes SpecSources into local files under a per-run staging
// directory. Construct with NewResolver; release resources with Close.
//
// One Resolver is meant to serve a whole run: the clone cache lives for its
// lifetime, so every channel that names the same (repo, ref) is answered from
// one clone — which is also what makes them agree on the commit when the
// branch moves while the run is in progress.
type Resolver struct {
	basePath string

	// stagingDir is created on first use rather than when the Resolver is
	// built: a run whose sources are all local stages nothing and must not
	// leave a temporary directory behind at all.
	stagingMu  sync.Mutex
	stagingDir string

	cache *cloneCache

	// gitOnce guards the `git` probe, which is deferred to the first git
	// source for the same reason: whether a run needs git is answered by the
	// sources it actually resolves, not by the subset of channels any one
	// caller knows about.
	gitOnce sync.Once
	gitErr  error

	envLookup  func(string) string
	httpClient *http.Client
	logStderr  io.Writer
	sourceRefs *SourceRefs
}

// NewResolver constructs a Resolver. basePath is the directory of project.yaml
// (used to resolve LocalSource). The staging directory is created under
// os.TempDir() when the first non-local source is resolved and removed by
// Close. Refs in opts.SourceRefs are validated where they enter the process
// (ParseSourceRefs), not here.
func NewResolver(basePath string, opts ResolverOptions) *Resolver {
	r := &Resolver{
		basePath:   basePath,
		cache:      newCloneCache(),
		envLookup:  opts.EnvLookup,
		httpClient: opts.HTTPClient,
		logStderr:  opts.LogStderr,
		sourceRefs: opts.SourceRefs,
	}

	if r.envLookup == nil {
		r.envLookup = os.Getenv
	}
	if r.httpClient == nil {
		r.httpClient = &http.Client{Timeout: 60 * time.Second}
	}
	if r.logStderr == nil {
		r.logStderr = os.Stderr
	}

	return r
}

// Resolve returns the local filesystem path for src. For LocalSource the
// returned path is basePath joined with the raw path. For HTTPSource and
// GitSource the file is materialized under stagingDir.
func (r *Resolver) Resolve(ctx context.Context, src SpecSource) (ResolvedSpec, error) {
	switch s := src.(type) {
	case LocalSource:
		return r.resolveLocal(s)
	case HTTPSource:
		return r.resolveHTTP(ctx, s)
	case GitSource:
		return r.resolveGit(ctx, s)
	default:
		return ResolvedSpec{}, fmt.Errorf("specsource: unsupported source type %T", src)
	}
}

// Close removes the staging directory and the clones under it. Safe to call
// multiple times, and on a Resolver that never staged anything. A Resolver is
// not meant to be used after Close.
func (r *Resolver) Close() error {
	r.stagingMu.Lock()
	dir := r.stagingDir
	r.stagingDir = ""
	r.stagingMu.Unlock()

	if dir == "" {
		return nil
	}

	return os.RemoveAll(dir)
}

// stagingRoot returns this Resolver's staging directory, creating it on first
// use.
func (r *Resolver) stagingRoot() (string, error) {
	r.stagingMu.Lock()
	defer r.stagingMu.Unlock()

	if r.stagingDir != "" {
		return r.stagingDir, nil
	}

	dir, err := os.MkdirTemp("", stagingDirPrefix)
	if err != nil {
		return "", fmt.Errorf("specsource: create staging dir: %w", err)
	}

	r.stagingDir = dir

	return dir, nil
}

// ensureGit verifies, once per Resolver, that `git` is on PATH. It runs when a
// git source is resolved instead of when the Resolver is built: the Resolver
// serves every channel of the run, so "does this project need git" cannot be
// decided up front from the channels one caller happens to see — a project
// whose only git source sits in an inline channel would lose the check, and a
// project with no git source at all would gain a dependency on git.
func (r *Resolver) ensureGit() error {
	r.gitOnce.Do(func() {
		if _, err := exec.LookPath("git"); err != nil {
			r.gitErr = fmt.Errorf("%w: install git or set PATH (lookup error: %w)", ErrGitNotInstalled, err)
		}
	})

	return r.gitErr
}

func (r *Resolver) resolveLocal(s LocalSource) (ResolvedSpec, error) {
	path := s.RawPath
	if !filepath.IsAbs(path) {
		path = filepath.Join(r.basePath, path)
	}

	// Best-effort digest: a local source that cannot be read is not this
	// function's error to report. The pre-feature behavior is that a missing
	// spec is reported by the copy step, which names the config entry and the
	// path the operator has to fix; failing here would replace that with a
	// bare read error.
	sum, err := hashFile(path)
	if err != nil {
		sum = ""
	}

	return ResolvedSpec{LocalPath: path, Source: s, Provenance: provenanceOf(s, "", sum)}, nil
}

func (r *Resolver) resolveHTTP(ctx context.Context, s HTTPSource) (ResolvedSpec, error) {
	dst, err := r.uniqueStagingPath(s.TargetFilename())
	if err != nil {
		return ResolvedSpec{}, err
	}

	f, err := os.Create(dst)
	if err != nil {
		return ResolvedSpec{}, fmt.Errorf("specsource: create staging file: %w", err)
	}
	defer f.Close()

	// Hashed as it is written: these are the bytes the source served, which is
	// what the provenance digest has to cover — the copy inside the project may
	// still be rewritten ($ref rewriting) before it is hashed for the manifest.
	hasher := sha256.New()

	if err := s.fetch(ctx, r.httpClient, r.envLookup, io.MultiWriter(f, hasher)); err != nil {
		_ = os.Remove(dst)
		return ResolvedSpec{}, err
	}

	sum := hex.EncodeToString(hasher.Sum(nil))

	return ResolvedSpec{LocalPath: dst, Source: s, Provenance: provenanceOf(s, "", sum)}, nil
}

func (r *Resolver) resolveGit(ctx context.Context, s GitSource) (ResolvedSpec, error) {
	if err := r.ensureGit(); err != nil {
		return ResolvedSpec{}, err
	}

	// From here on s carries the effective ref, so the clone cache key, the
	// clone itself and every error message speak about the ref we actually
	// fetched — not the one written in project.yaml.
	s = r.sourceRefs.apply(s)

	cloneDir, err := r.cache.getOrClone(s, func() (string, error) {
		root, rootErr := r.stagingRoot()
		if rootErr != nil {
			return "", rootErr
		}

		target := filepath.Join(root, "git", cacheKey(s))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return "", fmt.Errorf("specsource: prepare git clone dir: %w", err)
		}
		if err := gitClone(ctx, s, r.envLookup, target, r.logStderr); err != nil {
			return "", err
		}
		return target, nil
	})
	if err != nil {
		return ResolvedSpec{}, err
	}

	srcFile, err := findSubpath(cloneDir, s.Subpath)
	if err != nil {
		return ResolvedSpec{}, err
	}

	dst, err := r.uniqueStagingPath(s.TargetFilename())
	if err != nil {
		return ResolvedSpec{}, err
	}
	if err := copyFile(srcFile, dst); err != nil {
		return ResolvedSpec{}, err
	}

	// The digest is taken from the file in the clone: that is the blob of the
	// commit below, so a consumer can compare the two without having to know
	// what the generator did with the copy afterwards.
	sum, err := hashFile(srcFile)
	if err != nil {
		return ResolvedSpec{}, fmt.Errorf("specsource: %s: hash fetched file: %w", s.Describe(), err)
	}

	commit, err := gitHeadCommit(ctx, cloneDir, r.logStderr)
	if err != nil {
		return ResolvedSpec{}, fmt.Errorf("specsource: %s: %w", s.Describe(), err)
	}

	return ResolvedSpec{
		LocalPath:  dst,
		Source:     s,
		Commit:     commit,
		Provenance: provenanceOf(s, commit, sum),
	}, nil
}

// uniqueStagingPath builds a path under stagingDir/files/ that does not
// collide with previously-resolved files of the same basename.
func (r *Resolver) uniqueStagingPath(filename string) (string, error) {
	if filename == "" {
		filename = "spec"
	}

	root, err := r.stagingRoot()
	if err != nil {
		return "", err
	}

	dir := filepath.Join(root, "files")

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("specsource: prepare staging dir: %w", err)
	}
	candidate := filepath.Join(dir, filename)
	if _, err := os.Stat(candidate); os.IsNotExist(err) {
		return candidate, nil
	}
	for i := 1; i < 10000; i++ {
		alt := filepath.Join(dir, fmt.Sprintf("%d-%s", i, filename))
		if _, err := os.Stat(alt); os.IsNotExist(err) {
			return alt, nil
		}
	}
	return "", fmt.Errorf("specsource: cannot allocate unique staging path for %q", filename)
}

func cacheKey(s GitSource) string {
	sum := sha256.Sum256([]byte(s.cloneKey()))
	return hex.EncodeToString(sum[:])[:16]
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("specsource: open source: %w", err)
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("specsource: create dest: %w", err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("specsource: copy: %w", err)
	}
	return nil
}

// cloneCache memoises (repo, ref) -> cloneDir for a single Resolver lifetime.
type cloneCache struct {
	mu      sync.Mutex
	entries map[string]cloneEntry
}

type cloneEntry struct {
	dir string
	err error
}

func newCloneCache() *cloneCache {
	return &cloneCache{entries: map[string]cloneEntry{}}
}

func (c *cloneCache) getOrClone(s GitSource, doClone func() (string, error)) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := cacheKey(s)
	if e, ok := c.entries[key]; ok {
		return e.dir, e.err
	}
	dir, err := doClone()
	c.entries[key] = cloneEntry{dir: dir, err: err}
	return dir, err
}
