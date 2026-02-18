package migrate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name       string
		configPath string
		dryRun     bool
	}{
		{
			name:       "create migrator with path",
			configPath: "/path/to/config.yaml",
			dryRun:     false,
		},
		{
			name:       "create migrator with dry run",
			configPath: "/path/to/config.yaml",
			dryRun:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(tt.configPath, tt.dryRun)

			if m == nil {
				t.Error("New() returned nil")
				return
			}

			if m.configPath != tt.configPath {
				t.Errorf("New() configPath = %q, want %q", m.configPath, tt.configPath)
			}

			if m.dryRun != tt.dryRun {
				t.Errorf("New() dryRun = %v, want %v", m.dryRun, tt.dryRun)
			}
		})
	}
}

func TestMigrator_Migrate(t *testing.T) {
	m := New("/path/to/config.yaml", false)

	result, err := m.Migrate()

	if err != nil {
		t.Errorf("Migrate() error = %v, want nil", err)
	}

	if result == nil {
		t.Error("Migrate() result = nil, want non-nil")
		return
	}

	if result.OriginalPath != "/path/to/config.yaml" {
		t.Errorf("Migrate() OriginalPath = %q, want %q", result.OriginalPath, "/path/to/config.yaml")
	}

	// Currently no migrations, so Modified should be false
	if result.Modified {
		t.Error("Migrate() Modified = true, want false (no migrations)")
	}
}

func TestCheckDeprecations(t *testing.T) {
	// Create temp directory for test files
	tempDir := t.TempDir()

	tests := []struct {
		name           string
		configContent  string
		wantWarnings   int
		wantFeature    string
		wantErrContain string
	}{
		{
			name: "no deprecations",
			configContent: `
main:
  name: myproject
rest:
  - name: api
    generator_type: ogen
`,
			wantWarnings: 0,
		},
		{
			name: "empty_config_available is ignored (removed in 0.13.0)",
			configContent: `
main:
  name: myproject
rest:
  - name: api
    generator_type: ogen
    empty_config_available: true
`,
			wantWarnings: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Write config file
			configPath := filepath.Join(tempDir, tt.name+".yaml")

			err := os.WriteFile(configPath, []byte(tt.configContent), 0o644)
			if err != nil {
				t.Fatalf("Failed to write test config: %v", err)
			}

			warnings, err := CheckDeprecations(configPath)
			if tt.wantErrContain != "" {
				if err == nil {
					t.Errorf("CheckDeprecations() error = nil, want error containing %q", tt.wantErrContain)

					return
				}

				if !strings.Contains(err.Error(), tt.wantErrContain) {
					t.Errorf("CheckDeprecations() error = %q, want containing %q", err.Error(), tt.wantErrContain)
				}

				return
			}

			if err != nil {
				t.Errorf("CheckDeprecations() error = %v, want nil", err)
				return
			}

			if len(warnings) != tt.wantWarnings {
				t.Errorf("CheckDeprecations() returned %d warnings, want %d", len(warnings), tt.wantWarnings)
			}

			if tt.wantWarnings > 0 && tt.wantFeature != "" {
				for _, w := range warnings {
					if w.Feature != tt.wantFeature {
						t.Errorf("CheckDeprecations() warning feature = %q, want %q", w.Feature, tt.wantFeature)
					}

					if w.RemovalVer == "" {
						t.Error("CheckDeprecations() warning RemovalVer is empty")
					}

					if w.MigrationHint == "" {
						t.Error("CheckDeprecations() warning MigrationHint is empty")
					}
				}
			}
		})
	}
}

func TestCheckDeprecations_FileErrors(t *testing.T) {
	t.Run("file not found", func(t *testing.T) {
		_, err := CheckDeprecations("/nonexistent/path/config.yaml")

		if err == nil {
			t.Error("CheckDeprecations() error = nil, want error")
			return
		}

		if !strings.Contains(err.Error(), "failed to read config file") {
			t.Errorf("CheckDeprecations() error = %q, want containing %q", err.Error(), "failed to read config file")
		}
	})

	t.Run("invalid yaml", func(t *testing.T) {
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "invalid.yaml")

		err := os.WriteFile(configPath, []byte("invalid: yaml: content: ["), 0o644)
		if err != nil {
			t.Fatalf("Failed to write test config: %v", err)
		}

		_, err = CheckDeprecations(configPath)

		if err == nil {
			t.Error("CheckDeprecations() error = nil, want error")
			return
		}

		if !strings.Contains(err.Error(), "failed to parse config file") {
			t.Errorf("CheckDeprecations() error = %q, want containing %q", err.Error(), "failed to parse config file")
		}
	})
}

func TestFindConfigFile(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name       string
		setup      func() (configDir, configFile string)
		wantResult func(tempDir string) string
	}{
		{
			name: "file in configDir",
			setup: func() (string, string) {
				subDir := filepath.Join(tempDir, "subdir1")
				if err := os.MkdirAll(subDir, 0o755); err != nil {
					t.Fatalf("Failed to create subdir: %v", err)
				}

				configPath := filepath.Join(subDir, "project.yaml")
				if err := os.WriteFile(configPath, []byte("test"), 0o644); err != nil {
					t.Fatalf("Failed to write config: %v", err)
				}

				return subDir, "project.yaml"
			},
			wantResult: func(_ string) string {
				return filepath.Join(tempDir, "subdir1", "project.yaml")
			},
		},
		{
			name: "file exists as configFile directly",
			setup: func() (string, string) {
				configPath := filepath.Join(tempDir, "direct.yaml")
				if err := os.WriteFile(configPath, []byte("test"), 0o644); err != nil {
					t.Fatalf("Failed to write config: %v", err)
				}

				return "/nonexistent", configPath
			},
			wantResult: func(_ string) string {
				return filepath.Join(tempDir, "direct.yaml")
			},
		},
		{
			name: "file not found returns first attempt path",
			setup: func() (string, string) {
				return filepath.Join(tempDir, "missing"), "notfound.yaml"
			},
			wantResult: func(_ string) string {
				return filepath.Join(tempDir, "missing", "notfound.yaml")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configDir, configFile := tt.setup()
			want := tt.wantResult(tempDir)

			got := FindConfigFile(configDir, configFile)

			if got != want {
				t.Errorf("FindConfigFile() = %q, want %q", got, want)
			}
		})
	}
}

func TestFindConfigFile_DefaultPath(t *testing.T) {
	// This test needs to be run from a temp directory to test the default path
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	// Create default config path
	if err := os.MkdirAll(".project-config", 0o755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	defaultPath := filepath.Join(".project-config", "project.yaml")

	if err := os.WriteFile(defaultPath, []byte("test"), 0o644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	got := FindConfigFile("/nonexistent", "nonexistent.yaml")

	if got != defaultPath {
		t.Errorf("FindConfigFile() = %q, want %q (default path)", got, defaultPath)
	}
}

func TestDeprecationWarning_Fields(t *testing.T) {
	w := DeprecationWarning{
		Feature:       "test_feature",
		Description:   "Test description",
		RemovalVer:    "1.0.0",
		MigrationHint: "Do this instead",
	}

	if w.Feature != "test_feature" {
		t.Errorf("DeprecationWarning.Feature = %q, want %q", w.Feature, "test_feature")
	}

	if w.Description != "Test description" {
		t.Errorf("DeprecationWarning.Description = %q, want %q", w.Description, "Test description")
	}

	if w.RemovalVer != "1.0.0" {
		t.Errorf("DeprecationWarning.RemovalVer = %q, want %q", w.RemovalVer, "1.0.0")
	}

	if w.MigrationHint != "Do this instead" {
		t.Errorf("DeprecationWarning.MigrationHint = %q, want %q", w.MigrationHint, "Do this instead")
	}
}

func TestMigrationResult_Fields(t *testing.T) {
	r := MigrationResult{
		Modified:     true,
		Warnings:     []DeprecationWarning{{Feature: "test"}},
		OriginalPath: "/path/to/original.yaml",
		BackupPath:   "/path/to/backup.yaml",
	}

	if !r.Modified {
		t.Error("MigrationResult.Modified = false, want true")
	}

	if len(r.Warnings) != 1 {
		t.Errorf("MigrationResult.Warnings len = %d, want 1", len(r.Warnings))
	}

	if r.OriginalPath != "/path/to/original.yaml" {
		t.Errorf("MigrationResult.OriginalPath = %q, want %q", r.OriginalPath, "/path/to/original.yaml")
	}

	if r.BackupPath != "/path/to/backup.yaml" {
		t.Errorf("MigrationResult.BackupPath = %q, want %q", r.BackupPath, "/path/to/backup.yaml")
	}
}

func TestConstants(t *testing.T) {
	// Verify version constants are set
	if CurrentVersion == "" {
		t.Error("CurrentVersion is empty")
	}

	if RemovalVersionCreateContext == "" {
		t.Error("RemovalVersionCreateContext is empty")
	}
}
