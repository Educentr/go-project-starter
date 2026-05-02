package specsource

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHTTPSource_Resolve_Basic(t *testing.T) {
	body := "openapi: 3.0.0\ninfo:\n  title: Test\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("unexpected Authorization header: %q", got)
		}
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()

	r := newTestResolver(t, nil)
	defer r.Close()

	src := HTTPSource{URL: srv.URL + "/api.yaml"}
	res, err := r.Resolve(t.Context(), src)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	got, err := os.ReadFile(res.LocalPath)
	if err != nil {
		t.Fatalf("read resolved file: %v", err)
	}
	if string(got) != body {
		t.Errorf("body: got %q, want %q", string(got), body)
	}
	if filepath.Base(res.LocalPath) != "api.yaml" {
		t.Errorf("filename: got %q, want api.yaml", filepath.Base(res.LocalPath))
	}
}

func TestHTTPSource_Resolve_BearerToken(t *testing.T) {
	var seenAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		_, _ = io.WriteString(w, "ok")
	}))
	defer srv.Close()

	env := map[string]string{"MY_TOKEN": "secret-value"}
	r := newTestResolver(t, env)
	defer r.Close()

	src := HTTPSource{URL: srv.URL + "/x", TokenEnv: "MY_TOKEN"}
	if _, err := r.Resolve(t.Context(), src); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if seenAuth != "Bearer secret-value" {
		t.Errorf("Authorization: got %q, want %q", seenAuth, "Bearer secret-value")
	}
}

func TestHTTPSource_Resolve_TokenEnvMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("server should not be hit when token env is empty")
	}))
	defer srv.Close()

	r := newTestResolver(t, map[string]string{}) // MY_TOKEN is unset
	defer r.Close()

	src := HTTPSource{URL: srv.URL + "/x", TokenEnv: "MY_TOKEN"}
	_, err := r.Resolve(t.Context(), src)
	if !errors.Is(err, ErrTokenEnvMissing) {
		t.Fatalf("expected ErrTokenEnvMissing, got %v", err)
	}
}

func TestHTTPSource_Resolve_Non2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	r := newTestResolver(t, nil)
	defer r.Close()

	src := HTTPSource{URL: srv.URL + "/x"}
	_, err := r.Resolve(t.Context(), src)
	if !errors.Is(err, ErrHTTPStatus) {
		t.Fatalf("expected ErrHTTPStatus, got %v", err)
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected 404 in error, got %v", err)
	}
}

func TestHTTPSource_Resolve_UniqueFilenamesOnCollision(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "x")
	}))
	defer srv.Close()

	r := newTestResolver(t, nil)
	defer r.Close()

	src := HTTPSource{URL: srv.URL + "/api.yaml"}
	a, err := r.Resolve(t.Context(), src)
	if err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	b, err := r.Resolve(t.Context(), src)
	if err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	if a.LocalPath == b.LocalPath {
		t.Errorf("expected distinct staging paths, got %q twice", a.LocalPath)
	}
}

func TestLocalSource_Resolve_AbsolutePath(t *testing.T) {
	dir := t.TempDir()
	abs := filepath.Join(dir, "api.yaml")
	if err := os.WriteFile(abs, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := newTestResolver(t, nil)
	defer r.Close()

	res, err := r.Resolve(t.Context(), LocalSource{RawPath: abs})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.LocalPath != abs {
		t.Errorf("LocalPath: got %q, want %q", res.LocalPath, abs)
	}
}

func TestLocalSource_Resolve_RelativeToBase(t *testing.T) {
	base := t.TempDir()
	stagingDir := t.TempDir()

	r, err := NewResolver(base, stagingDir, ResolverOptions{
		EnvLookup:    func(string) string { return "" },
		SkipGitCheck: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	res, err := r.Resolve(t.Context(), LocalSource{RawPath: "./api.yaml"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := filepath.Join(base, "./api.yaml")
	if res.LocalPath != want {
		t.Errorf("LocalPath: got %q, want %q", res.LocalPath, want)
	}
}

// newTestResolver builds a resolver with deterministic env lookup and a fresh
// staging directory cleaned up via t.Cleanup.
func newTestResolver(t *testing.T, env map[string]string) *Resolver {
	t.Helper()
	stagingDir := t.TempDir()
	r, err := NewResolver(t.TempDir(), stagingDir, ResolverOptions{
		EnvLookup: func(k string) string {
			if env == nil {
				return ""
			}
			return env[k]
		},
		SkipGitCheck: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return r
}
