package specsource

import (
	"bytes"
	"errors"
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
