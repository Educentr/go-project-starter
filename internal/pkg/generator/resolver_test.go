package generator

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	cfg "github.com/Educentr/go-project-starter/internal/pkg/config"
	"github.com/Educentr/go-project-starter/internal/pkg/ds"
	"github.com/Educentr/go-project-starter/internal/pkg/meta"
)

// contractsRepo is a local git repository standing in for the contract
// repository, reachable under contractsRepoURL.
type contractsRepo struct {
	t    *testing.T
	path string
}

// The repository every channel in these tests names. It is an ordinary
// git+ssh URI with an explicit ref, so the resolve path exercised here is the
// production one down to `git clone`; what makes it work without a network is
// a git `insteadOf` rewrite pointing the URL at a local scratch repository
// (see newContractsRepo).
const (
	contractsRepoURL = "ssh://git@contracts.test/org/contracts.git"
	contractsRef     = "contracts"
	contractsURI     = "git+" + contractsRepoURL + "@" + contractsRef

	specSubpath = "#rest/api.yaml"
	mapSubpath  = "#metrics/map.yaml"
)

// newContractsRepo creates the repository and rewrites contractsRepoURL onto
// it for every `git` process this test starts (the resolver passes its own
// environment through to git, so GIT_CONFIG_GLOBAL reaches the clone).
func newContractsRepo(t *testing.T) *contractsRepo {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	repo := &contractsRepo{t: t, path: t.TempDir()}

	config := filepath.Join(t.TempDir(), "gitconfig")
	body := "[url \"" + repo.path + "\"]\n\tinsteadOf = " + contractsRepoURL + "\n"

	if err := os.WriteFile(config, []byte(body), 0o600); err != nil {
		t.Fatalf("write git config: %v", err)
	}

	t.Setenv("GIT_CONFIG_GLOBAL", config)

	repo.git("init", "-q", "-b", contractsRef)
	repo.write("rest/api.yaml", "openapi: 3.0.0\n")
	repo.write("metrics/map.yaml", "metrics: []\n")
	repo.commit()

	return repo
}

// git runs one git command in the repository. The identity comes from the
// environment because GIT_CONFIG_GLOBAL now points at the rewrite file, which
// declares no user.
func (r *contractsRepo) git(args ...string) string {
	r.t.Helper()

	cmd := exec.Command("git", append([]string{"-C", r.path}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}

	return strings.TrimSpace(string(out))
}

func (r *contractsRepo) write(rel, content string) {
	r.t.Helper()

	full := filepath.Join(r.path, filepath.FromSlash(rel))

	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		r.t.Fatalf("prepare %s: %v", rel, err)
	}

	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		r.t.Fatalf("write %s: %v", rel, err)
	}
}

// commit records everything in the worktree and returns the new HEAD.
func (r *contractsRepo) commit() string {
	r.t.Helper()

	r.git("add", "-A")
	r.git("commit", "-q", "-m", "contracts")

	return r.git("rev-parse", "HEAD")
}

// closeGenerator releases the staging directory of a generator built by hand.
func closeGenerator(t *testing.T, g *Generator) {
	t.Helper()

	if err := g.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

// stagingLeftovers lists the generator's staging directories under dir.
func stagingLeftovers(t *testing.T, dir string) []string {
	t.Helper()

	found, err := filepath.Glob(filepath.Join(dir, "gps-*"))
	if err != nil {
		t.Fatalf("glob staging dirs: %v", err)
	}

	return found
}

// TestOneResolverPerRunAgreesOnTheCommit is the defect this file is about: the
// inline channels (resolved while the config is loaded) and the resolve
// pre-pass (inside Generate) used to build a resolver each, so each cloned the
// repository separately. A branch that moved in between — seconds, but the
// window is real for a busy contract repository — left the tree built from two
// states of the contracts, and nothing said so.
//
// The branch here moves at exactly that moment. Both channels must report the
// commit the run started on, in the same records that become meta.yaml: the
// manifest entry of the copied spec (sourceProvenance, keyed by the staged
// path the copy step reads) and the `inputs` entry of the metrics map.
func TestOneResolverPerRunAgreesOnTheCommit(t *testing.T) {
	repo := newContractsRepo(t)
	started := repo.git("rev-parse", "HEAD")

	g := &Generator{
		BasePath: t.TempDir(),
		Transports: ds.Transports{
			"users": ds.Transport{Name: "users", SpecPath: []string{contractsURI + specSubpath}},
		},
		JSONSchemas: ds.JSONSchemas{},
	}

	defer closeGenerator(t, g)

	// Config load: the metrics map comes from the contract repository.
	mapPath, err := g.resolveInlineSpec(t.Context(), channelMetricsPath, contractsURI+mapSubpath)
	if err != nil {
		t.Fatalf("resolveInlineSpec() error = %v", err)
	}

	repo.write("rest/api.yaml", "openapi: 3.0.0\n# moved\n")

	moved := repo.commit()
	if moved == started {
		t.Fatal("the branch did not move: the test would pass without the fix")
	}

	// Generate: the pre-pass stages the REST spec from the same repo@ref.
	if err = g.resolveAllSources(); err != nil {
		t.Fatalf("resolveAllSources() error = %v", err)
	}

	specPath := g.Transports["users"].SpecPath[0]

	if got := g.provenanceOf(specPath).Commit; got != started {
		t.Errorf("pre-pass commit = %q, want %q (the state the run started on, not %q)", got, started, moved)
	}

	inputs := g.inlineInputs()
	if len(inputs) != 1 {
		t.Fatalf("inputs = %+v, want the metrics map", inputs)
	}

	if got := inputs[0].Commit; got != started {
		t.Errorf("inline commit = %q, want %q", got, started)
	}

	// The staged spec is the one that commit carries — a record naming a
	// commit whose content is not what was staged would be worse than none.
	content := readFileT(t, specPath)
	if strings.Contains(content, "moved") {
		t.Errorf("staged spec is the post-move revision:\n%s", content)
	}

	// Both channels stage into one directory because they share one resolver,
	// which is what shares the clone cache that pinned the commit above.
	if filepath.Dir(mapPath) != filepath.Dir(specPath) {
		t.Errorf("inline staged into %q and the pre-pass into %q, want one staging directory per run",
			filepath.Dir(mapPath), filepath.Dir(specPath))
	}

	// And the cache is what it is for: one clone of a (repo, ref) per run,
	// however many channels name it.
	root := filepath.Dir(filepath.Dir(specPath))

	clones, err := filepath.Glob(filepath.Join(root, "git", "*"))
	if err != nil {
		t.Fatalf("glob clones: %v", err)
	}

	if len(clones) != 1 {
		t.Errorf("clones of one repo@ref = %v, want exactly one", clones)
	}
}

// Close releases the staging directory of the run — the inline channels no
// longer wipe their own right after parsing, so this is the only thing that
// does. It has to tolerate being called twice and on a generator that
// resolved nothing, since the owner defers it unconditionally.
func TestCloseReleasesStagingDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)

	newContractsRepo(t)

	g := &Generator{BasePath: t.TempDir()}

	if _, err := g.resolveInlineSpec(t.Context(), channelMetricsPath, contractsURI+mapSubpath); err != nil {
		t.Fatalf("resolveInlineSpec() error = %v", err)
	}

	if got := stagingLeftovers(t, tmp); len(got) != 1 {
		t.Fatalf("staging dirs after resolving = %v, want exactly one", got)
	}

	closeGenerator(t, g)

	if got := stagingLeftovers(t, tmp); len(got) != 0 {
		t.Errorf("staging dirs after Close = %v, want none", got)
	}

	closeGenerator(t, g)
}

// New owns the resolver from the moment it resolves an inline channel, so a
// failure after that point has to release it itself: it returns nil, leaving
// the caller nothing to Close.
func TestNewReleasesStagingDirOnFailure(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)

	repo := newContractsRepo(t)

	// Resolving the map succeeds (so the run owns a staging directory by
	// then); reading it as a metrics map does not.
	repo.write("metrics/map.yaml", "not a metrics map\n")
	repo.commit()

	var config cfg.Config
	config.Metrics.Path = contractsURI + mapSubpath

	g, err := New("v1.0.0", config, meta.Meta{}, false, nil)
	if err == nil {
		closeGenerator(t, g)
		t.Fatal("New() must fail on a metrics map that cannot be read")
	}

	if g != nil {
		t.Errorf("New() returned a generator alongside error %v", err)
	}

	if got := stagingLeftovers(t, tmp); len(got) != 0 {
		t.Errorf("staging dirs after a failed New = %v, want none", got)
	}
}

// A project whose sources are all local must neither stage anything nor need
// git. The staging directory is created on first use and the `git` probe runs
// on the first git source, so both answers follow from the sources the run
// actually resolves — with one resolver serving every channel, neither can be
// decided up front from the channels one caller sees.
func TestLocalSourcesNeedNoStagingAndNoGit(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	t.Setenv("PATH", "")

	base := t.TempDir()

	if err := os.WriteFile(filepath.Join(base, "map.yaml"), []byte("metrics: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(base, "api.yaml"), []byte("openapi: 3.0.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	g := &Generator{
		BasePath: base,
		Transports: ds.Transports{
			"users": ds.Transport{Name: "users", SpecPath: []string{"./api.yaml"}},
		},
		JSONSchemas: ds.JSONSchemas{},
	}

	defer closeGenerator(t, g)

	if _, err := g.resolveInlineSpec(t.Context(), channelMetricsPath, "./map.yaml"); err != nil {
		t.Fatalf("resolveInlineSpec() error = %v", err)
	}

	if err := g.resolveAllSources(); err != nil {
		t.Fatalf("resolveAllSources() error = %v", err)
	}

	if got := stagingLeftovers(t, tmp); len(got) != 0 {
		t.Errorf("staging dirs for a local-only run = %v, want none", got)
	}
}

func readFileT(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	return string(data)
}
