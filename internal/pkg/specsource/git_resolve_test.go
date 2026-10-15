package specsource

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// testRepo is a throwaway git repository with one file (api.yaml) whose
// content differs per branch, so a resolved file identifies the branch it came
// from.
type testRepo struct {
	path          string
	defaultBranch string
}

const (
	branchA = "release/a"
	branchB = "release/b"
)

func newTestRepo(t *testing.T) testRepo {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	dir := t.TempDir()

	git := func(args ...string) string {
		t.Helper()

		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
		)

		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}

		return strings.TrimSpace(string(out))
	}

	commit := func(content string) {
		t.Helper()

		if err := os.WriteFile(filepath.Join(dir, "api.yaml"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}

		git("add", "api.yaml")
		git("commit", "-m", content)
	}

	git("init")
	commit("from-default")

	defaultBranch := git("rev-parse", "--abbrev-ref", "HEAD")

	// Branches are created with checkout because each one needs its own
	// content; this is a scratch repo under t.TempDir().
	git("checkout", "-b", branchA)
	commit("from-" + branchA)
	git("checkout", "-b", branchB)
	commit("from-" + branchB)
	git("checkout", defaultBranch)

	return testRepo{path: dir, defaultBranch: defaultBranch}
}

// resolveFromRepo resolves src with the given override entries (see
// ParseSourceRefs) and returns the content of the resolved file, which names
// the branch it came from.
func resolveFromRepo(t *testing.T, src GitSource, refEntries ...string) (string, error) {
	t.Helper()

	r, err := NewResolver(t.TempDir(), t.TempDir(), ResolverOptions{
		SourceRefs: mustSourceRefs(t, refEntries...),
		LogStderr:  io.Discard,
	})
	if err != nil {
		return "", err
	}

	defer r.Close()

	res, err := r.Resolve(t.Context(), src)
	if err != nil {
		return "", err
	}

	content, err := os.ReadFile(res.LocalPath)
	if err != nil {
		t.Fatalf("read resolved file: %v", err)
	}

	return string(content), nil
}

func TestResolveGit_SourceRefOverridesUnpinned(t *testing.T) {
	repo := newTestRepo(t)
	src := GitSource{Repo: repo.path, Ref: "HEAD", Subpath: "api.yaml"}

	got, err := resolveFromRepo(t, src, branchA)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	if want := "from-" + branchA; got != want {
		t.Errorf("content: got %q, want %q", got, want)
	}
}

func TestResolveGit_SourceRefLeavesPinnedAlone(t *testing.T) {
	repo := newTestRepo(t)
	src := GitSource{Repo: repo.path, Ref: branchB, RefPinned: true, Subpath: "api.yaml"}

	got, err := resolveFromRepo(t, src, branchA)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	if want := "from-" + branchB; got != want {
		t.Errorf("pin was overridden: got %q, want %q", got, want)
	}
}

// Regression guard: without an override, resolving is byte-for-byte what it
// was before the knob existed — an unpinned source reads the default branch.
func TestResolveGit_NoSourceRefKeepsHEAD(t *testing.T) {
	repo := newTestRepo(t)
	src := GitSource{Repo: repo.path, Ref: "HEAD", Subpath: "api.yaml"}

	got, err := resolveFromRepo(t, src)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	if got != "from-default" {
		t.Errorf("content: got %q, want %q", got, "from-default")
	}
}

func TestResolveGit_MissingSourceRefFailsLoudly(t *testing.T) {
	repo := newTestRepo(t)
	src := GitSource{Repo: repo.path, Ref: "HEAD", Subpath: "api.yaml"}

	_, err := resolveFromRepo(t, src, "release/does-not-exist")
	if !errors.Is(err, ErrCloneFailed) {
		t.Fatalf("got %v, want ErrCloneFailed (no silent fallback to HEAD)", err)
	}

	// The message must name both the source and the ref that failed.
	if !strings.Contains(err.Error(), "release/does-not-exist") || !strings.Contains(err.Error(), repo.path) {
		t.Errorf("error must name source and ref: %v", err)
	}
}

// The point of the per-repository form: one build, two contract repositories,
// each taken from its own branch.
func TestResolveGit_ScopedRefsPerRepository(t *testing.T) {
	specs := newTestRepo(t)
	proto := newTestRepo(t)

	r, err := NewResolver(t.TempDir(), t.TempDir(), ResolverOptions{
		SourceRefs: mustSourceRefs(t, specs.path+":"+branchA, proto.path+":"+branchB),
		LogStderr:  io.Discard,
	})
	if err != nil {
		t.Fatal(err)
	}

	defer r.Close()

	want := map[string]string{
		specs.path: "from-" + branchA,
		proto.path: "from-" + branchB,
	}

	for repo, wantContent := range want {
		res, resErr := r.Resolve(t.Context(), GitSource{Repo: repo, Ref: "HEAD", Subpath: "api.yaml"})
		if resErr != nil {
			t.Fatalf("resolve %s: %v", repo, resErr)
		}

		content, readErr := os.ReadFile(res.LocalPath)
		if readErr != nil {
			t.Fatal(readErr)
		}

		if string(content) != wantContent {
			t.Errorf("%s: got %q, want %q", repo, content, wantContent)
		}
	}

	if err := r.sourceRefs.CheckUsed(); err != nil {
		t.Errorf("both selectors matched, got %v", err)
	}
}

// A repository nobody named keeps its own ref even when other repositories are
// overridden — the scoped form must not leak.
func TestResolveGit_ScopedRefLeavesOtherRepositoriesAlone(t *testing.T) {
	specs := newTestRepo(t)
	proto := newTestRepo(t)

	got, err := resolveFromRepo(t,
		GitSource{Repo: proto.path, Ref: "HEAD", Subpath: "api.yaml"},
		specs.path+":"+branchA,
	)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	if got != "from-default" {
		t.Errorf("content: got %q, want %q", got, "from-default")
	}
}

// Two sources from the same repo — one unpinned, one pinned to the ref the
// override names — must result in a single clone.
func TestResolveGit_OverriddenAndPinnedShareOneClone(t *testing.T) {
	repo := newTestRepo(t)

	r, err := NewResolver(t.TempDir(), t.TempDir(), ResolverOptions{
		SourceRefs: mustSourceRefs(t, branchA),
		LogStderr:  io.Discard,
	})
	if err != nil {
		t.Fatal(err)
	}

	defer r.Close()

	sources := []GitSource{
		{Repo: repo.path, Ref: "HEAD", Subpath: "api.yaml"},
		{Repo: repo.path, Ref: branchA, RefPinned: true, Subpath: "api.yaml"},
	}

	for i, src := range sources {
		res, resErr := r.Resolve(t.Context(), src)
		if resErr != nil {
			t.Fatalf("source %d: %v", i, resErr)
		}

		content, readErr := os.ReadFile(res.LocalPath)
		if readErr != nil {
			t.Fatal(readErr)
		}

		if want := "from-" + branchA; string(content) != want {
			t.Errorf("source %d: got %q, want %q", i, content, want)
		}
	}

	if len(r.cache.entries) != 1 {
		t.Errorf("expected 1 cached clone, got %d", len(r.cache.entries))
	}
}
