package specsource

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestCloneCache_DedupesSameRepoRef(t *testing.T) {
	c := newCloneCache()
	src := GitSource{Repo: "https://github.com/org/repo.git", Ref: "v1.0.0"}

	calls := 0
	doClone := func() (string, error) {
		calls++
		return "/tmp/clone-1", nil
	}

	for i := 0; i < 5; i++ {
		dir, err := c.getOrClone(src, doClone)
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if dir != "/tmp/clone-1" {
			t.Errorf("call %d: dir=%q", i, dir)
		}
	}
	if calls != 1 {
		t.Errorf("expected 1 clone, got %d", calls)
	}
}

func TestCloneCache_DistinctRefsClonedSeparately(t *testing.T) {
	c := newCloneCache()
	a := GitSource{Repo: "https://github.com/org/repo.git", Ref: "v1.0.0"}
	b := GitSource{Repo: "https://github.com/org/repo.git", Ref: "v2.0.0"}

	calls := 0
	doClone := func() (string, error) {
		calls++
		return "", nil
	}
	if _, err := c.getOrClone(a, doClone); err != nil {
		t.Fatal(err)
	}
	if _, err := c.getOrClone(b, doClone); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Errorf("expected 2 clones for distinct refs, got %d", calls)
	}
}

func TestCloneCache_CachesErrors(t *testing.T) {
	c := newCloneCache()
	src := GitSource{Repo: "https://x", Ref: "main"}
	cloneErr := errors.New("boom")

	calls := 0
	doClone := func() (string, error) {
		calls++
		return "", cloneErr
	}
	for i := 0; i < 3; i++ {
		_, err := c.getOrClone(src, doClone)
		if !errors.Is(err, cloneErr) {
			t.Fatalf("call %d: got %v, want cloneErr", i, err)
		}
	}
	if calls != 1 {
		t.Errorf("expected error to be cached after first call, got %d invocations", calls)
	}
}

func TestRedactingWriter_ReplacesToken(t *testing.T) {
	var buf bytes.Buffer
	w := redactingWriter(&buf, "hunter2")
	if _, err := w.Write([]byte("error: cloning https://oauth2:hunter2@host fail")); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if got != "error: cloning https://oauth2:<redacted>@host fail" {
		t.Errorf("got %q", got)
	}
}

func TestRedactingWriter_NoTokenPassesThrough(t *testing.T) {
	var buf bytes.Buffer
	w := redactingWriter(&buf, "")
	if _, err := w.Write([]byte("plain")); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "plain" {
		t.Errorf("got %q", buf.String())
	}
}

func TestInjectUserInfo_HTTPS(t *testing.T) {
	got, err := injectUserInfo("https://github.com/org/repo.git", "oauth2", "tok")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://oauth2:tok@github.com/org/repo.git" {
		t.Errorf("got %q", got)
	}
}

func TestInjectUserInfo_LeavesNonHTTP(t *testing.T) {
	in := "ssh://git@github.com/org/repo.git"
	got, err := injectUserInfo(in, "oauth2", "tok")
	if err != nil {
		t.Fatal(err)
	}
	if got != in {
		t.Errorf("got %q, want unchanged", got)
	}
}

// The `git` probe belongs to resolving a git source, not to building the
// Resolver: one Resolver now serves every channel of a run, so whether the run
// needs git is answered by the sources it actually resolves. A project on
// local specs alone must not require git at all.
func TestResolver_GitProbeOnlyForGitSources(t *testing.T) {
	base := t.TempDir()

	if err := os.WriteFile(filepath.Join(base, "api.yaml"), []byte(specBody), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", "")

	r := NewResolver(base, ResolverOptions{LogStderr: io.Discard})
	defer r.Close()

	if _, err := r.Resolve(t.Context(), LocalSource{RawPath: "./api.yaml"}); err != nil {
		t.Errorf("resolving a local source must not need git, got %v", err)
	}

	_, err := r.Resolve(t.Context(), GitSource{Repo: "ssh://git@host/org/repo.git", Ref: "main", Subpath: "api.yaml"})
	if !errors.Is(err, ErrGitNotInstalled) {
		t.Errorf("resolving a git source without git = %v, want ErrGitNotInstalled", err)
	}
}

// A Resolver that only ever saw local sources leaves nothing behind: the
// staging directory is created on first use, so a local-only run has none to
// remove — and nothing for a later run to mistake for its own.
func TestResolver_LocalSourceStagesNothing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)

	base := t.TempDir()

	if err := os.WriteFile(filepath.Join(base, "api.yaml"), []byte(specBody), 0o600); err != nil {
		t.Fatal(err)
	}

	r := NewResolver(base, ResolverOptions{})

	if _, err := r.Resolve(t.Context(), LocalSource{RawPath: "./api.yaml"}); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	staged, err := filepath.Glob(filepath.Join(tmp, stagingDirPrefix+"*"))
	if err != nil {
		t.Fatalf("glob staging dirs: %v", err)
	}

	if len(staged) != 0 {
		t.Errorf("staging dirs = %v, want none", staged)
	}

	if err := r.Close(); err != nil {
		t.Errorf("Close() on a resolver that staged nothing = %v", err)
	}
}
