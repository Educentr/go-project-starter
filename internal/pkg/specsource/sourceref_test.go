package specsource

import (
	"errors"
	"testing"
)

func TestParseSpecSource_RefPinned(t *testing.T) {
	tests := []struct {
		name       string
		in         string
		wantRef    string
		wantPinned bool
	}{
		{"explicit tag", "git+ssh://git@github.com/org/repo.git@v1.0.0#api.yaml", "v1.0.0", true},
		{"explicit branch", "git+ssh://git@github.com/org/repo.git@main#api.yaml", "main", true},
		{"explicit sha", "git+https://github.com/org/repo.git@0bf1c32#api.yaml", "0bf1c32", true},
		{"explicit HEAD", "git+ssh://git@github.com/org/repo.git@HEAD#api.yaml", "HEAD", true},
		{"no ref", "git+ssh://git@github.com/org/repo.git#api.yaml", "HEAD", false},
		{"no ref, https with token", "git+https://github.com/org/repo.git?token_env=T#api.yaml", "HEAD", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseSpecSource(tc.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			gs, ok := got.(GitSource)
			if !ok {
				t.Fatalf("expected GitSource, got %T", got)
			}

			if gs.Ref != tc.wantRef {
				t.Errorf("Ref: got %q, want %q", gs.Ref, tc.wantRef)
			}

			if gs.RefPinned != tc.wantPinned {
				t.Errorf("RefPinned: got %v, want %v", gs.RefPinned, tc.wantPinned)
			}
		})
	}
}

func TestGitSource_ApplySourceRef(t *testing.T) {
	unpinned := GitSource{Repo: "https://github.com/org/repo.git", Ref: "HEAD"}
	pinned := GitSource{Repo: "https://github.com/org/repo.git", Ref: "v1.0.0", RefPinned: true}

	tests := []struct {
		name    string
		src     GitSource
		ref     string
		wantRef string
	}{
		{"unpinned takes the override", unpinned, "release/2026.10", "release/2026.10"},
		{"unpinned without override keeps HEAD", unpinned, "", "HEAD"},
		{"pinned ignores the override", pinned, "release/2026.10", "v1.0.0"},
		{"pinned without override keeps its ref", pinned, "", "v1.0.0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.src.applySourceRef(tc.ref)
			if got.Ref != tc.wantRef {
				t.Errorf("Ref: got %q, want %q", got.Ref, tc.wantRef)
			}
			// The override must not touch anything but the ref.
			if got.Repo != tc.src.Repo || got.Subpath != tc.src.Subpath || got.RefPinned != tc.src.RefPinned {
				t.Errorf("source mutated beyond Ref: %+v (was %+v)", got, tc.src)
			}
		})
	}
}

// An overridden source and a source pinned to the same ref must share one
// clone: the cache key is built from the effective ref, not from the URI.
func TestGitSource_CloneKeyUsesEffectiveRef(t *testing.T) {
	overridden := GitSource{Repo: "https://github.com/org/repo.git", Ref: "HEAD"}.applySourceRef("release/a")
	pinned := GitSource{Repo: "https://github.com/org/repo.git", Ref: "release/a", RefPinned: true}

	if cacheKey(overridden) != cacheKey(pinned) {
		t.Errorf("same effective ref must share a clone: %q vs %q", cacheKey(overridden), cacheKey(pinned))
	}

	other := GitSource{Repo: "https://github.com/org/repo.git", Ref: "HEAD"}.applySourceRef("release/b")
	if cacheKey(overridden) == cacheKey(other) {
		t.Error("distinct effective refs must not share a clone")
	}
}

func TestGitSource_DescribeShowsEffectiveRef(t *testing.T) {
	src := GitSource{Repo: "https://github.com/org/repo.git", Ref: "HEAD", Subpath: "api.yaml"}

	d := src.applySourceRef("release/a").Describe()
	if !contains(d, "release/a") {
		t.Errorf("Describe must name the ref actually fetched: %s", d)
	}

	if contains(d, "HEAD") {
		t.Errorf("Describe must not name the overridden ref: %s", d)
	}
}

func TestValidateRef(t *testing.T) {
	valid := []string{"", "main", "release/2026.10", "v1.2.0", "0bf1c32d", "feature/TOOLS-22_knob"}
	for _, ref := range valid {
		if err := ValidateRef(ref); err != nil {
			t.Errorf("ValidateRef(%q): unexpected error %v", ref, err)
		}
	}

	invalid := []string{"-main", "--upload-pack=evil", "release 2026", "main\n", "main\tx"}
	for _, ref := range invalid {
		err := ValidateRef(ref)
		if !errors.Is(err, ErrInvalidRef) {
			t.Errorf("ValidateRef(%q): got %v, want ErrInvalidRef", ref, err)
		}
	}
}

func TestNewResolver_RejectsInvalidSourceRef(t *testing.T) {
	_, err := NewResolver(t.TempDir(), t.TempDir(), ResolverOptions{
		SkipGitCheck: true,
		SourceRef:    "-x",
	})
	if !errors.Is(err, ErrInvalidRef) {
		t.Fatalf("got %v, want ErrInvalidRef", err)
	}
}
