package config

import (
	"testing"
)

func TestCheck_IsValid(t *testing.T) {
	tests := []struct {
		name    string
		check   Check
		wantOK  bool
		wantMsg string
	}{
		{
			name:   "valid check",
			check:  Check{Name: "lint", Run: "make lint"},
			wantOK: true,
		},
		{
			name:   "valid check with registry login",
			check:  Check{Name: "guards", Run: "make lint-guards", RegistryLogin: true},
			wantOK: true,
		},
		{
			name:    "empty name",
			check:   Check{Run: "make lint"},
			wantOK:  false,
			wantMsg: "github_ci.checks: empty name",
		},
		{
			name:    "empty run command",
			check:   Check{Name: "lint"},
			wantOK:  false,
			wantMsg: "github_ci.checks: empty run command in check 'lint'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOK, gotMsg := tt.check.IsValid()

			if gotOK != tt.wantOK {
				t.Errorf("Check.IsValid() ok = %v, want %v (msg: %q)", gotOK, tt.wantOK, gotMsg)
			}

			if gotMsg != tt.wantMsg {
				t.Errorf("Check.IsValid() msg = %q, want %q", gotMsg, tt.wantMsg)
			}
		})
	}
}

// TestGithubCIConfig_ValidateChecks covers the checks part of the github_ci
// section through the exported entry point used on config load.
func TestGithubCIConfig_ValidateChecks(t *testing.T) {
	tests := []struct {
		name    string
		checks  CheckList
		wantOK  bool
		wantMsg string
	}{
		{
			name:   "no checks declared",
			checks: nil,
			wantOK: true,
		},
		{
			name: "several distinct checks",
			checks: CheckList{
				{Name: "lint", Run: "make lint"},
				{Name: "unit-tests", Run: "make unit-tests"},
				{Name: "guards", Run: "make lint-guards", RegistryLogin: true},
			},
			wantOK: true,
		},
		{
			name: "duplicate name",
			checks: CheckList{
				{Name: "lint", Run: "make lint"},
				{Name: "lint", Run: "make lint-full"},
			},
			wantOK:  false,
			wantMsg: "github_ci.checks: duplicate check name: lint",
		},
		{
			name:    "missing run command",
			checks:  CheckList{{Name: "lint"}},
			wantOK:  false,
			wantMsg: "github_ci.checks: empty run command in check 'lint'",
		},
		{
			name:    "missing name",
			checks:  CheckList{{Run: "make lint"}},
			wantOK:  false,
			wantMsg: "github_ci.checks: empty name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOK, gotMsg := GithubCIConfig{Checks: tt.checks}.IsValid()

			if gotOK != tt.wantOK {
				t.Errorf("GithubCIConfig.IsValid() ok = %v, want %v (msg: %q)", gotOK, tt.wantOK, gotMsg)
			}

			if gotMsg != tt.wantMsg {
				t.Errorf("GithubCIConfig.IsValid() msg = %q, want %q", gotMsg, tt.wantMsg)
			}
		})
	}
}
