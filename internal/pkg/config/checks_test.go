package config

import (
	"strings"
	"testing"

	"github.com/pkg/errors"
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
			wantMsg: "Empty name",
		},
		{
			name:    "empty run command",
			check:   Check{Name: "lint"},
			wantOK:  false,
			wantMsg: "Empty run command in check 'lint'",
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

func TestValidateChecks(t *testing.T) {
	tests := []struct {
		name    string
		checks  CheckList
		wantErr string
	}{
		{
			name:   "no checks declared",
			checks: nil,
		},
		{
			name: "several distinct checks",
			checks: CheckList{
				{Name: "lint", Run: "make lint"},
				{Name: "unit-tests", Run: "make unit-tests"},
				{Name: "guards", Run: "make lint-guards", RegistryLogin: true},
			},
		},
		{
			name: "duplicate name",
			checks: CheckList{
				{Name: "lint", Run: "make lint"},
				{Name: "lint", Run: "make lint-full"},
			},
			wantErr: "duplicate check name: lint",
		},
		{
			name:    "missing run command",
			checks:  CheckList{{Name: "lint"}},
			wantErr: "Empty run command in check 'lint'",
		},
		{
			name:    "missing name",
			checks:  CheckList{{Run: "make lint"}},
			wantErr: "Empty name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateChecks(tt.checks)

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateChecks() unexpected error: %v", err)
				}

				return
			}

			if err == nil {
				t.Fatalf("validateChecks() expected error containing %q, got nil", tt.wantErr)
			}

			if !errors.Is(err, ErrInvalidConfig) {
				t.Errorf("validateChecks() error should wrap ErrInvalidConfig, got %v", err)
			}

			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("validateChecks() error = %q, want it to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}
