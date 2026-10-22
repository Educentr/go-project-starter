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

// mustSourceRefs builds an override set that the test expects to be valid.
func mustSourceRefs(t *testing.T, entries ...string) *SourceRefs {
	t.Helper()

	refs, err := ParseSourceRefs(entries)
	if err != nil {
		t.Fatalf("ParseSourceRefs(%q): %v", entries, err)
	}

	return refs
}

func TestParseSourceRefs_Forms(t *testing.T) {
	tests := []struct {
		name        string
		entries     []string
		repo        string
		wantRef     string
		wantIsEmpty bool
	}{
		{
			name:        "nothing passed means no override",
			entries:     nil,
			repo:        "https://github.com/org/specs.git",
			wantIsEmpty: true,
		},
		{
			name:    "blanket ref applies to any repository",
			entries: []string{"release/2026.10"},
			repo:    "https://github.com/org/specs.git",
			wantRef: "release/2026.10",
		},
		{
			name:    "scoped ref applies to the repository it names",
			entries: []string{"github.com/org/specs:release/a"},
			repo:    "ssh://git@github.com/org/specs.git",
			wantRef: "release/a",
		},
		{
			name:    "scoped ref does not leak to another repository",
			entries: []string{"github.com/org/specs:release/a"},
			repo:    "ssh://git@github.com/org/proto.git",
			wantRef: "",
		},
		{
			name:    "repeated entries: each repository gets its own ref",
			entries: []string{"github.com/org/specs:release/a", "github.com/org/proto:release/b"},
			repo:    "https://github.com/org/proto.git",
			wantRef: "release/b",
		},
		{
			name:    "one value may carry several entries",
			entries: []string{"org/specs:release/a,org/proto:release/b"},
			repo:    "https://github.com/org/proto.git",
			wantRef: "release/b",
		},
		{
			name:    "whitespace separates entries too",
			entries: []string{"org/specs:release/a org/proto:release/b"},
			repo:    "https://github.com/org/specs.git",
			wantRef: "release/a",
		},
		{
			name:    "blanket is the fallback for repositories nobody named",
			entries: []string{"org/specs:release/a", "main"},
			repo:    "https://github.com/org/proto.git",
			wantRef: "main",
		},
		{
			name:    "the most specific selector wins",
			entries: []string{"specs:release/short", "github.com/org/specs:release/long"},
			repo:    "https://github.com/org/specs.git",
			wantRef: "release/long",
		},
		{
			name:    "same host, different owners are different repositories",
			entries: []string{"org-a/specs:release/a", "org-b/specs:release/b"},
			repo:    "https://github.com/org-b/specs.git",
			wantRef: "release/b",
		},
		{
			name:    "different hosts, same owner and name",
			entries: []string{"github.com/org/specs:release/gh", "gitlab.example.com/org/specs:release/gl"},
			repo:    "git@gitlab.example.com:org/specs.git",
			wantRef: "release/gl",
		},
		{
			name:    "selector written as a full URI",
			entries: []string{"git+ssh://git@github.com/org/specs.git:release/a"},
			repo:    "ssh://git@github.com/org/specs.git",
			wantRef: "release/a",
		},
		{
			name:    "segment boundary is respected",
			entries: []string{"specs:release/a"},
			repo:    "https://github.com/org/my-specs.git",
			wantRef: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			refs := mustSourceRefs(t, tc.entries...)

			if got := refs.IsEmpty(); got != tc.wantIsEmpty {
				t.Fatalf("IsEmpty: got %v, want %v", got, tc.wantIsEmpty)
			}

			if got := refs.refFor(tc.repo); got != tc.wantRef {
				t.Errorf("refFor(%q): got %q, want %q", tc.repo, got, tc.wantRef)
			}
		})
	}
}

func TestParseSourceRefs_Rejects(t *testing.T) {
	tests := []struct {
		name    string
		entries []string
		wantErr error
	}{
		{"unusable ref", []string{"--upload-pack=evil"}, ErrInvalidRef},
		{"unusable scoped ref", []string{"org/specs:-x"}, ErrInvalidRef},
		{"no ref after the colon", []string{"org/specs:"}, ErrInvalidSourceRef},
		{"empty selector", []string{":release/a"}, ErrInvalidSourceRef},
		{"two blanket refs", []string{"release/a", "release/b"}, ErrInvalidSourceRef},
		{
			"one repository named twice",
			[]string{"github.com/org/specs:release/a", "git@github.com:org/specs.git:release/b"},
			ErrInvalidSourceRef,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			refs, err := ParseSourceRefs(tc.entries)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("got %v, want %v", err, tc.wantErr)
			}

			if refs != nil {
				t.Errorf("a rejected set must not be returned: %v", refs)
			}
		})
	}
}

// A selector that matches no source is a typo: left silent, the build would
// take that repository from its default branch — the exact failure the knob
// exists to prevent.
func TestSourceRefs_CheckUsed(t *testing.T) {
	refs := mustSourceRefs(t, "org/specs:release/a", "org/proto:release/b")

	_ = refs.apply(GitSource{Repo: "https://github.com/org/specs.git", Ref: "HEAD"})

	err := refs.CheckUsed()
	if !errors.Is(err, ErrUnusedSourceRef) {
		t.Fatalf("got %v, want ErrUnusedSourceRef", err)
	}

	if !contains(err.Error(), "org/proto") {
		t.Errorf("the error must name the unused selector: %v", err)
	}

	if !contains(err.Error(), "github.com/org/specs") {
		t.Errorf("the error must list the repositories actually used: %v", err)
	}

	_ = refs.apply(GitSource{Repo: "git@github.com:org/proto.git", Ref: "HEAD"})

	if err := refs.CheckUsed(); err != nil {
		t.Errorf("every selector matched, got %v", err)
	}
}

// A pin wins over a selector, but it still proves the selector named a real
// repository — reporting it as unused would be a false alarm.
func TestSourceRefs_CheckUsed_PinnedSourceCountsAsMatch(t *testing.T) {
	refs := mustSourceRefs(t, "org/specs:release/a")

	got := refs.apply(GitSource{Repo: "https://github.com/org/specs.git", Ref: "v1.0.0", RefPinned: true})
	if got.Ref != "v1.0.0" {
		t.Errorf("pin was overridden: %q", got.Ref)
	}

	if err := refs.CheckUsed(); err != nil {
		t.Errorf("selector matched a pinned source, got %v", err)
	}
}

// The blanket entry is set for whole CI pipelines, so projects that have no
// git sources at all must not fail because of it.
func TestSourceRefs_CheckUsed_BlanketNeverUnused(t *testing.T) {
	if err := mustSourceRefs(t, "release/a").CheckUsed(); err != nil {
		t.Errorf("got %v, want nil", err)
	}

	var none *SourceRefs
	if err := none.CheckUsed(); err != nil {
		t.Errorf("no override at all: got %v, want nil", err)
	}
}

func TestCanonicalRepo(t *testing.T) {
	// Every spelling of one repository must reduce to the same identity.
	same := []string{
		"github.com/org/specs",
		"github.com/org/specs/",
		"github.com/org/specs.git",
		"https://github.com/org/specs.git",
		"ssh://git@github.com/org/specs.git",
		"git+ssh://git@github.com/org/specs.git",
		"git+https://github.com/org/specs.git",
		"https://oauth2:token@github.com/org/specs.git",
		"git@github.com:org/specs.git",
		"ssh://git@github.com:22/org/specs.git",
		"GitHub.com/Org/Specs",
	}

	want := canonicalRepo(same[0])
	if want != "github.com/org/specs" {
		t.Fatalf("canonical form: got %q", want)
	}

	for _, in := range same[1:] {
		if got := canonicalRepo(in); got != want {
			t.Errorf("canonicalRepo(%q): got %q, want %q", in, got, want)
		}
	}

	// Distinct repositories must not collapse into one.
	distinct := []string{
		"github.com/org/specs",
		"github.com/org/proto",
		"gitlab.example.com/org/specs",
		"github.com/other/specs",
	}

	seen := make(map[string]string, len(distinct))

	for _, in := range distinct {
		canon := canonicalRepo(in)
		if prev, dup := seen[canon]; dup {
			t.Errorf("%q and %q collapsed into %q", prev, in, canon)
		}

		seen[canon] = in
	}
}

func TestSourceRefs_String(t *testing.T) {
	refs := mustSourceRefs(t, "github.com/org/specs:release/a", "main")

	got := refs.String()
	for _, want := range []string{"github.com/org/specs", "release/a", "main"} {
		if !contains(got, want) {
			t.Errorf("String() must mention %q: %s", want, got)
		}
	}

	var none *SourceRefs
	if none.String() != "" {
		t.Errorf("no override must render empty, got %q", none.String())
	}
}
