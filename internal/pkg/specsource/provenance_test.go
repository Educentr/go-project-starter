package specsource

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const specBody = "openapi: 3.0.0\npaths: {}\n"

// wantDigest is the digest the provenance of specBody must carry: it is taken
// from the bytes the source served, which is what makes it comparable with the
// blob in the source repository.
var wantDigest = hashBytes([]byte(specBody))

func TestProvenanceLocal(t *testing.T) {
	base := t.TempDir()

	if err := os.WriteFile(filepath.Join(base, "api.yaml"), []byte(specBody), 0o600); err != nil {
		t.Fatal(err)
	}

	r, err := NewResolver(base, t.TempDir(), ResolverOptions{SkipGitCheck: true})
	if err != nil {
		t.Fatal(err)
	}

	defer r.Close()

	res, err := r.Resolve(t.Context(), LocalSource{RawPath: "./api.yaml"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	p := res.Provenance

	if p.Kind != KindLocal || p.Repo != "" || p.Path != "./api.yaml" {
		t.Errorf("provenance = %+v, want the path as written and no repository", p)
	}

	if p.Ref != "" || p.Commit != "" {
		t.Errorf("provenance = %+v, want no ref and no commit for a local source", p)
	}

	if p.SHA256 != wantDigest {
		t.Errorf("digest = %q, want %q", p.SHA256, wantDigest)
	}
}

// A local source that is not there yet is reported by the copy step, which
// names the config entry; resolving must not fail earlier with a bare read
// error, so the digest is simply absent.
func TestProvenanceLocalMissingFile(t *testing.T) {
	r, err := NewResolver(t.TempDir(), t.TempDir(), ResolverOptions{SkipGitCheck: true})
	if err != nil {
		t.Fatal(err)
	}

	defer r.Close()

	res, err := r.Resolve(t.Context(), LocalSource{RawPath: "./gone.yaml"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	if res.Provenance.SHA256 != "" {
		t.Errorf("digest = %q, want empty for a file that is not there", res.Provenance.SHA256)
	}

	if res.Provenance.Path != "./gone.yaml" {
		t.Errorf("provenance = %+v, want the source still identified", res.Provenance)
	}
}

func TestProvenanceHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := io.WriteString(w, specBody); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer srv.Close()

	r, err := NewResolver(t.TempDir(), t.TempDir(), ResolverOptions{SkipGitCheck: true})
	if err != nil {
		t.Fatal(err)
	}

	defer r.Close()

	src, err := ParseSpecSource(srv.URL + "/specs/api.yaml?v=2")
	if err != nil {
		t.Fatal(err)
	}

	res, err := r.Resolve(t.Context(), src)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	p := res.Provenance

	if p.Kind != KindHTTP || p.Repo != srv.URL {
		t.Errorf("provenance = %+v, want the origin in Repo", p)
	}

	// The query stays on the path side: it can select which content the URL
	// serves, so it belongs to the identity of the file.
	if p.Path != "/specs/api.yaml?v=2" {
		t.Errorf("path = %q, want the URL path with its query", p.Path)
	}

	if p.SHA256 != wantDigest {
		t.Errorf("digest = %q, want %q", p.SHA256, wantDigest)
	}
}

func TestProvenanceGit(t *testing.T) {
	repo := newTestRepo(t)

	r, err := NewResolver(t.TempDir(), t.TempDir(), ResolverOptions{LogStderr: io.Discard})
	if err != nil {
		t.Fatal(err)
	}

	defer r.Close()

	res, err := r.Resolve(t.Context(), GitSource{Repo: repo.path, Ref: "HEAD", Subpath: "api.yaml"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	p := res.Provenance

	if p.Kind != KindGit || p.Repo != repo.path || p.Path != "api.yaml" {
		t.Errorf("provenance = %+v, want the repository and the subpath separately", p)
	}

	if p.Ref != "HEAD" {
		t.Errorf("ref = %q, want the effective ref", p.Ref)
	}

	if len(p.Commit) != 40 {
		t.Errorf("commit = %q, want the resolved SHA", p.Commit)
	}

	if p.Commit != res.Commit {
		t.Errorf("commit = %q, want the same SHA ResolvedSpec reports (%q)", p.Commit, res.Commit)
	}

	if p.SHA256 != hashBytes([]byte("from-default")) {
		t.Errorf("digest = %q, want the digest of the blob in the clone", p.SHA256)
	}
}

// The effective ref is recorded, not the one written in the config: a release
// built with --sourceRef has to be provable from the manifest alone.
func TestProvenanceGitRecordsEffectiveRef(t *testing.T) {
	repo := newTestRepo(t)

	r, err := NewResolver(t.TempDir(), t.TempDir(), ResolverOptions{
		LogStderr:  io.Discard,
		SourceRefs: mustSourceRefs(t, branchA),
	})
	if err != nil {
		t.Fatal(err)
	}

	defer r.Close()

	res, err := r.Resolve(t.Context(), GitSource{Repo: repo.path, Ref: "HEAD", Subpath: "api.yaml"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	if res.Provenance.Ref != branchA {
		t.Errorf("ref = %q, want the override %q", res.Provenance.Ref, branchA)
	}

	if res.Provenance.SHA256 != hashBytes([]byte("from-"+branchA)) {
		t.Errorf("digest = %q, want the digest of the file on %s", res.Provenance.SHA256, branchA)
	}
}

func TestRedactRepoURL(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "ssh user is kept: it is not a secret and the URL does not clone without it",
			in:   "ssh://git@github.com/org/repo.git",
			want: "ssh://git@github.com/org/repo.git",
		},
		{
			name: "credentials written into the URL are dropped whole",
			in:   "https://oauth2:ghp_secret@github.com/org/repo.git",
			want: "https://github.com/org/repo.git",
		},
		{
			name: "plain https URL is untouched",
			in:   "https://github.com/org/repo.git",
			want: "https://github.com/org/repo.git",
		},
		{
			name: "a path that is no URL is returned as is",
			in:   "/tmp/repo",
			want: "/tmp/repo",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := redactRepoURL(tc.in); got != tc.want {
				t.Errorf("redactRepoURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// No provenance field may carry a token, whichever way one reaches the
// resolver: the value only ever lives in the environment variable named by
// token_env and in the in-memory clone URL.
func TestProvenanceNeverCarriesToken(t *testing.T) {
	const token = "ghp_thisMustNotBeRecorded"

	src := GitSource{
		Repo:      "https://oauth2:" + token + "@github.com/org/repo.git",
		Ref:       "main",
		Subpath:   "api.yaml",
		Transport: GitTransportHTTPS,
		TokenEnv:  "GPS_TOKEN",
	}

	p := provenanceOf(src, "abc", "def")

	if strings.Contains(p.Repo+p.Path+p.Ref+p.Commit+p.SHA256, token) {
		t.Errorf("provenance = %+v, want no token in any field", p)
	}
}

func TestProvenanceDescribe(t *testing.T) {
	cases := []struct {
		name string
		p    Provenance
		want string
	}{
		{
			name: "git",
			p:    Provenance{Kind: KindGit, Repo: "ssh://git@host/org/repo.git", Ref: "main", Path: "api.yaml"},
			want: "ssh://git@host/org/repo.git@main#api.yaml",
		},
		{
			name: "http",
			p:    Provenance{Kind: KindHTTP, Repo: "https://host", Path: "/api.yaml"},
			want: "https://host/api.yaml",
		},
		{
			name: "local",
			p:    Provenance{Kind: KindLocal, Path: "./api.yaml"},
			want: "./api.yaml",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.p.Describe(); got != tc.want {
				t.Errorf("Describe() = %q, want %q", got, tc.want)
			}
		})
	}
}
