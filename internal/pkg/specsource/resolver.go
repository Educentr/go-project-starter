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
	// SkipGitCheck disables the up-front `git --version` probe. Used by tests
	// that don't exercise GitSource.
	SkipGitCheck bool
}

// Resolver materializes SpecSources into local files under a per-run staging
// directory. Construct with NewResolver; release resources with Close.
type Resolver struct {
	basePath   string
	stagingDir string

	cache *cloneCache

	envLookup  func(string) string
	httpClient *http.Client
	logStderr  io.Writer
}

// NewResolver constructs a Resolver. basePath is the directory of project.yaml
// (used to resolve LocalSource). stagingDir must already exist and be
// writable; the caller is responsible for creating it (typically via
// os.MkdirTemp). The Resolver verifies that `git` is installed unless
// opts.SkipGitCheck is set.
func NewResolver(basePath, stagingDir string, opts ResolverOptions) (*Resolver, error) {
	if !opts.SkipGitCheck {
		if _, err := exec.LookPath("git"); err != nil {
			return nil, fmt.Errorf("%w: install git or set PATH (lookup error: %w)", ErrGitNotInstalled, err)
		}
	}

	r := &Resolver{
		basePath:   basePath,
		stagingDir: stagingDir,
		cache:      newCloneCache(),
		envLookup:  opts.EnvLookup,
		httpClient: opts.HTTPClient,
		logStderr:  opts.LogStderr,
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
	return r, nil
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

// Close removes the staging directory and any per-run clones. Safe to call
// multiple times.
func (r *Resolver) Close() error {
	if r.stagingDir == "" {
		return nil
	}
	dir := r.stagingDir
	r.stagingDir = ""
	return os.RemoveAll(dir)
}

func (r *Resolver) resolveLocal(s LocalSource) (ResolvedSpec, error) {
	path := s.RawPath
	if !filepath.IsAbs(path) {
		path = filepath.Join(r.basePath, path)
	}
	return ResolvedSpec{LocalPath: path, Source: s}, nil
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

	if err := s.fetch(ctx, r.httpClient, r.envLookup, f); err != nil {
		_ = os.Remove(dst)
		return ResolvedSpec{}, err
	}
	return ResolvedSpec{LocalPath: dst, Source: s}, nil
}

func (r *Resolver) resolveGit(ctx context.Context, s GitSource) (ResolvedSpec, error) {
	cloneDir, err := r.cache.getOrClone(s, func() (string, error) {
		target := filepath.Join(r.stagingDir, "git", cacheKey(s))
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
	return ResolvedSpec{LocalPath: dst, Source: s}, nil
}

// uniqueStagingPath builds a path under stagingDir/files/ that does not
// collide with previously-resolved files of the same basename.
func (r *Resolver) uniqueStagingPath(filename string) (string, error) {
	if filename == "" {
		filename = "spec"
	}
	dir := filepath.Join(r.stagingDir, "files")
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
