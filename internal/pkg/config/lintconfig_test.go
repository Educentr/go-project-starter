package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLintConfig_HasOverrides(t *testing.T) {
	if (LintConfig{}).HasOverrides() {
		t.Error("an absent section must not declare an override layer")
	}

	if !(LintConfig{Overrides: "./golangci-overrides.yml"}).HasOverrides() {
		t.Error("a configured path must switch the override layer on")
	}
}

func TestLintConfig_IsValid(t *testing.T) {
	baseDir := t.TempDir()

	localOverrides := filepath.Join(baseDir, "golangci-overrides.yml")
	if err := os.WriteFile(localOverrides, []byte("linters:\n  disable:\n    - dupl\n"), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	tests := []struct {
		name    string
		lint    LintConfig
		wantOK  bool
		wantMsg string
	}{
		{
			name:   "absent section is valid",
			lint:   LintConfig{},
			wantOK: true,
		},
		{
			name:   "own alone is valid",
			lint:   LintConfig{Own: true},
			wantOK: true,
		},
		{
			name:   "existing local override document",
			lint:   LintConfig{Overrides: "./golangci-overrides.yml"},
			wantOK: true,
		},
		{
			name:   "remote override document is not fetched here",
			lint:   LintConfig{Overrides: "git+https://github.com/org/contracts.git@v1#lint/golangci.yml"},
			wantOK: true,
		},
		{
			// Owning the whole file and overriding the generated one are two
			// different answers to the same question; silently picking one
			// would hide a misconfiguration behind a working build.
			name:    "own together with overrides is refused",
			lint:    LintConfig{Own: true, Overrides: "./golangci-overrides.yml"},
			wantOK:  false,
			wantMsg: "mutually exclusive",
		},
		{
			name:    "missing local override document",
			lint:    LintConfig{Overrides: "./nope.yml"},
			wantOK:  false,
			wantMsg: "tools.lint_config.overrides",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, msg := tt.lint.IsValid(baseDir)

			if ok != tt.wantOK {
				t.Fatalf("IsValid() ok = %v, want %v (msg %q)", ok, tt.wantOK, msg)
			}

			if tt.wantMsg != "" && !strings.Contains(msg, tt.wantMsg) {
				t.Errorf("IsValid() msg = %q, want it to contain %q", msg, tt.wantMsg)
			}

			if tt.wantOK && msg != "" {
				t.Errorf("IsValid() returned a message for a valid config: %q", msg)
			}
		})
	}
}
