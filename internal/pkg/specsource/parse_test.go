package specsource

import (
	"errors"
	"testing"
)

func TestParseSpecSource_Local(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"relative dot", "./api.yaml"},
		{"relative bare", "api.yaml"},
		{"relative subdir", "api/v1/openapi.yaml"},
		{"absolute unix", "/abs/path/to/api.yaml"},
		{"windows-like", "C:\\specs\\api.yaml"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseSpecSource(tc.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			ls, ok := got.(LocalSource)
			if !ok {
				t.Fatalf("expected LocalSource, got %T (%v)", got, got)
			}
			if ls.RawPath != tc.in {
				t.Errorf("RawPath: got %q, want %q", ls.RawPath, tc.in)
			}
			if got.Kind() != KindLocal {
				t.Errorf("Kind: got %v, want %v", got.Kind(), KindLocal)
			}
		})
	}
}

func TestParseSpecSource_GitSSH(t *testing.T) {
	got, err := ParseSpecSource("git+ssh://git@github.com/org/api-specs.git@v1.0.0#openapi/example.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	gs, ok := got.(GitSource)
	if !ok {
		t.Fatalf("expected GitSource, got %T", got)
	}
	if want := "ssh://git@github.com/org/api-specs.git"; gs.Repo != want {
		t.Errorf("Repo: got %q, want %q", gs.Repo, want)
	}
	if gs.Ref != "v1.0.0" {
		t.Errorf("Ref: got %q, want v1.0.0", gs.Ref)
	}
	if gs.Subpath != "openapi/example.yaml" {
		t.Errorf("Subpath: got %q, want openapi/example.yaml", gs.Subpath)
	}
	if gs.Transport != GitTransportSSH {
		t.Errorf("Transport: got %v, want ssh", gs.Transport)
	}
	if gs.TargetFilename() != "example.yaml" {
		t.Errorf("TargetFilename: got %q, want example.yaml", gs.TargetFilename())
	}
}

func TestParseSpecSource_GitHTTPS_WithToken(t *testing.T) {
	got, err := ParseSpecSource("git+https://github.com/org/repo.git@main?token_env=GITHUB_TOKEN#api/users.proto")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	gs, ok := got.(GitSource)
	if !ok {
		t.Fatalf("expected GitSource, got %T", got)
	}
	if want := "https://github.com/org/repo.git"; gs.Repo != want {
		t.Errorf("Repo: got %q, want %q", gs.Repo, want)
	}
	if gs.Ref != "main" {
		t.Errorf("Ref: got %q, want main", gs.Ref)
	}
	if gs.Transport != GitTransportHTTPS {
		t.Errorf("Transport: got %v, want https", gs.Transport)
	}
	if gs.TokenEnv != "GITHUB_TOKEN" {
		t.Errorf("TokenEnv: got %q, want GITHUB_TOKEN", gs.TokenEnv)
	}
	if gs.TargetFilename() != "users.proto" {
		t.Errorf("TargetFilename: got %q", gs.TargetFilename())
	}
}

func TestParseSpecSource_GitNoRef_DefaultsToHEAD(t *testing.T) {
	got, err := ParseSpecSource("git+ssh://git@github.com/org/repo.git#api.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	gs := got.(GitSource)
	if gs.Ref != "HEAD" {
		t.Errorf("Ref: got %q, want HEAD", gs.Ref)
	}
	if want := "ssh://git@github.com/org/repo.git"; gs.Repo != want {
		t.Errorf("Repo: got %q, want %q", gs.Repo, want)
	}
}

func TestParseSpecSource_GitMissingSubpath(t *testing.T) {
	_, err := ParseSpecSource("git+ssh://git@github.com/org/repo.git@v1.0.0")
	if !errors.Is(err, ErrSubpathRequired) {
		t.Fatalf("expected ErrSubpathRequired, got %v", err)
	}
}

func TestParseSpecSource_GitDirectorySubpath(t *testing.T) {
	_, err := ParseSpecSource("git+ssh://git@github.com/org/repo.git@v1.0.0#openapi/")
	if !errors.Is(err, ErrSubpathDirectory) {
		t.Fatalf("expected ErrSubpathDirectory, got %v", err)
	}
}

func TestParseSpecSource_GitSSHRejectsTokenEnv(t *testing.T) {
	_, err := ParseSpecSource("git+ssh://git@github.com/org/repo.git@v1?token_env=GITHUB_TOKEN#api.yaml")
	if err == nil {
		t.Fatalf("expected error for token_env on ssh, got nil")
	}
}

func TestParseSpecSource_HTTP(t *testing.T) {
	got, err := ParseSpecSource("https://raw.githubusercontent.com/org/specs/main/api.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	hs, ok := got.(HTTPSource)
	if !ok {
		t.Fatalf("expected HTTPSource, got %T", got)
	}
	if hs.URL != "https://raw.githubusercontent.com/org/specs/main/api.yaml" {
		t.Errorf("URL: got %q", hs.URL)
	}
	if hs.TokenEnv != "" {
		t.Errorf("TokenEnv: got %q, want empty", hs.TokenEnv)
	}
	if hs.TargetFilename() != "api.yaml" {
		t.Errorf("TargetFilename: got %q", hs.TargetFilename())
	}
}

func TestParseSpecSource_HTTP_WithTokenEnv(t *testing.T) {
	got, err := ParseSpecSource("https://api.example.com/spec.yaml?token_env=COMPANY_TOKEN")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	hs := got.(HTTPSource)
	if hs.URL != "https://api.example.com/spec.yaml" {
		t.Errorf("URL: got %q, want url without token_env", hs.URL)
	}
	if hs.TokenEnv != "COMPANY_TOKEN" {
		t.Errorf("TokenEnv: got %q, want COMPANY_TOKEN", hs.TokenEnv)
	}
}

func TestParseSpecSource_HTTP_PreservesOtherQuery(t *testing.T) {
	got, err := ParseSpecSource("https://api.example.com/spec.yaml?ref=main&token_env=T")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	hs := got.(HTTPSource)
	// token_env stripped, ref preserved
	if hs.URL != "https://api.example.com/spec.yaml?ref=main" {
		t.Errorf("URL: got %q, want token_env stripped, ref kept", hs.URL)
	}
	if hs.TokenEnv != "T" {
		t.Errorf("TokenEnv: got %q", hs.TokenEnv)
	}
}

func TestParseSpecSource_SCPStyleRejected(t *testing.T) {
	_, err := ParseSpecSource("git@github.com:org/repo.git")
	if !errors.Is(err, ErrSCPStyleURL) {
		t.Fatalf("expected ErrSCPStyleURL, got %v", err)
	}
}

func TestParseSpecSource_UnknownScheme(t *testing.T) {
	_, err := ParseSpecSource("ftp://example.com/api.yaml")
	if !errors.Is(err, ErrUnknownScheme) {
		t.Fatalf("expected ErrUnknownScheme, got %v", err)
	}
}

func TestParseSpecSource_Empty(t *testing.T) {
	if _, err := ParseSpecSource(""); err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestParseSpecSource_MixedCaseScheme(t *testing.T) {
	got, err := ParseSpecSource("GIT+SSH://git@github.com/org/repo.git@v1#api.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := got.(GitSource); !ok {
		t.Fatalf("expected GitSource for mixed-case scheme, got %T", got)
	}
}

func TestGitSource_Describe_StripsToken(t *testing.T) {
	// If a stray token ends up in the URL userinfo (shouldn't happen via
	// ParseSpecSource, but be defensive), Describe must still scrub it.
	gs := GitSource{
		Repo:      "https://oauth2:hunter2@github.com/org/repo.git",
		Ref:       "main",
		Subpath:   "api.yaml",
		Transport: GitTransportHTTPS,
		TokenEnv:  "GITHUB_TOKEN",
	}
	d := gs.Describe()
	if contains(d, "hunter2") {
		t.Errorf("Describe leaked token: %s", d)
	}
	if !contains(d, "GITHUB_TOKEN") {
		t.Errorf("Describe should mention token_env name: %s", d)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
