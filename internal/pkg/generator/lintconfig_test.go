package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	cfg "github.com/Educentr/go-project-starter/internal/pkg/config"
	"github.com/Educentr/go-project-starter/internal/pkg/ds"
	"github.com/Educentr/go-project-starter/internal/pkg/lintconfig"
	"github.com/Educentr/go-project-starter/internal/pkg/templater"
)

// lintTarget is the absolute destination the generator uses for the lint
// config in these tests.
func lintTarget(targetPath string) string {
	return filepath.Join(targetPath, filepath.FromSlash(lintconfig.ConfigPath))
}

func TestGenerator_UserOwnedPaths(t *testing.T) {
	t.Run("default run does not claim the lint config", func(t *testing.T) {
		g := Generator{}

		for _, p := range g.userOwnedPaths() {
			if p == lintconfig.ConfigPath {
				t.Fatalf("userOwnedPaths() = %v, want the lint config generator-owned by default", g.userOwnedPaths())
			}
		}
	})

	t.Run("own declares the lint config user-owned", func(t *testing.T) {
		g := Generator{LintConfigOwn: true}

		var found bool

		for _, p := range g.userOwnedPaths() {
			if p == lintconfig.ConfigPath {
				found = true
			}
		}

		if !found {
			t.Errorf("userOwnedPaths() = %v, want it to contain %s", g.userOwnedPaths(), lintconfig.ConfigPath)
		}
	})

	// userOwnedPaths adds to staticIgnorePaths rather than replacing it:
	// claiming the lint config must not stop protecting a project's static
	// assets.
	t.Run("static asset dirs are still listed", func(t *testing.T) {
		g := Generator{
			LintConfigOwn: true,
			Applications: ds.Apps{
				{
					Name: "web",
					Transports: ds.Transports{
						"assets": {
							GeneratorType:     "template",
							GeneratorTemplate: "static",
							GeneratorParams:   map[string]string{"dir": "public"},
						},
					},
				},
			},
		}

		got := g.userOwnedPaths()

		var hasStatic, hasLint bool

		for _, p := range got {
			switch p {
			case "public/":
				hasStatic = true
			case lintconfig.ConfigPath:
				hasLint = true
			}
		}

		if !hasStatic {
			t.Errorf("userOwnedPaths() = %v, want it to keep the static dir public/", got)
		}

		if !hasLint {
			t.Errorf("userOwnedPaths() = %v, want it to contain %s", got, lintconfig.ConfigPath)
		}
	})
}

func TestGenerator_LintBodyTransform(t *testing.T) {
	targetPath := filepath.FromSlash("/tmp/project")

	t.Run("nil without an override layer", func(t *testing.T) {
		g := Generator{}

		if g.lintBodyTransform(targetPath, lintTarget(targetPath)) != nil {
			t.Error("lintBodyTransform() must be nil when no overrides are configured")
		}
	})

	t.Run("nil for every other file", func(t *testing.T) {
		g := Generator{LintOverrides: []byte("linters:\n  disable:\n    - dupl\n")}

		for _, dest := range []string{
			filepath.Join(targetPath, "Makefile"),
			filepath.Join(targetPath, filepath.FromSlash("configs/golangci-metrics.yml")),
			filepath.Join(targetPath, filepath.FromSlash("cmd/api/psg_main_gen.go")),
		} {
			if g.lintBodyTransform(targetPath, dest) != nil {
				t.Errorf("lintBodyTransform() must be nil for %s", dest)
			}
		}
	})

	t.Run("merges the overrides into the lint config body", func(t *testing.T) {
		g := Generator{LintOverrides: []byte("linters:\n  disable:\n    - dupl\n")}

		transform := g.lintBodyTransform(targetPath, lintTarget(targetPath))
		if transform == nil {
			t.Fatal("lintBodyTransform() = nil, want a transform for the lint config")
		}

		got, err := transform([]byte("version: \"2\"\nlinters:\n  enable:\n    - dupl\n    - misspell\n"))
		if err != nil {
			t.Fatalf("transform() error = %v", err)
		}

		if strings.Contains(string(got), "- dupl") {
			t.Errorf("merged body still enables dupl:\n%s", got)
		}

		if !strings.Contains(string(got), "- misspell") {
			t.Errorf("merged body lost misspell:\n%s", got)
		}
	})
}

func TestGenerator_ProcessLintConfig(t *testing.T) {
	t.Run("absent section leaves both knobs off", func(t *testing.T) {
		g := Generator{}

		if err := g.processLintConfig(cfg.Config{}); err != nil {
			t.Fatalf("processLintConfig() error = %v", err)
		}

		if g.LintConfigOwn {
			t.Error("LintConfigOwn must stay false without the section")
		}

		if g.LintOverrides != nil {
			t.Errorf("LintOverrides = %q, want nil", g.LintOverrides)
		}
	})

	t.Run("own is carried through without reading any file", func(t *testing.T) {
		var g Generator

		config := cfg.Config{}
		config.Tools.LintConfig.Own = true

		if err := g.processLintConfig(config); err != nil {
			t.Fatalf("processLintConfig() error = %v", err)
		}

		if !g.LintConfigOwn {
			t.Error("LintConfigOwn = false, want true")
		}

		if g.LintOverrides != nil {
			t.Errorf("LintOverrides = %q, want nil — own mode has no override layer", g.LintOverrides)
		}
	})

	t.Run("local override document is read and kept", func(t *testing.T) {
		baseDir := t.TempDir()
		doc := "linters:\n  disable:\n    - dupl\n"
		writeTempFile(t, filepath.Join(baseDir, "golangci-overrides.yml"), doc)

		var g Generator

		config := cfg.Config{BasePath: baseDir}
		config.Tools.LintConfig.Overrides = "./golangci-overrides.yml"

		if err := g.processLintConfig(config); err != nil {
			t.Fatalf("processLintConfig() error = %v", err)
		}

		if string(g.LintOverrides) != doc {
			t.Errorf("LintOverrides = %q, want %q", g.LintOverrides, doc)
		}
	})

	// Parsing at config time rather than at merge time is the point: a broken
	// override document must stop the run before any file is written.
	t.Run("malformed override document fails before generation", func(t *testing.T) {
		baseDir := t.TempDir()
		writeTempFile(t, filepath.Join(baseDir, "broken.yml"), "linters: [\n")

		var g Generator

		config := cfg.Config{BasePath: baseDir}
		config.Tools.LintConfig.Overrides = "./broken.yml"

		err := g.processLintConfig(config)
		if err == nil {
			t.Fatal("processLintConfig() error = nil, want a parse failure")
		}

		if !strings.Contains(err.Error(), "tools.lint_config.overrides") {
			t.Errorf("error = %v, want it to name the offending config key", err)
		}
	})

	t.Run("override document that is not a mapping is refused", func(t *testing.T) {
		baseDir := t.TempDir()
		writeTempFile(t, filepath.Join(baseDir, "list.yml"), "- dupl\n")

		var g Generator

		config := cfg.Config{BasePath: baseDir}
		config.Tools.LintConfig.Overrides = "./list.yml"

		if err := g.processLintConfig(config); err == nil {
			t.Fatal("processLintConfig() error = nil, want a shape failure")
		}
	})

	t.Run("missing override document is reported", func(t *testing.T) {
		var g Generator

		config := cfg.Config{BasePath: t.TempDir()}
		config.Tools.LintConfig.Overrides = "./absent.yml"

		if err := g.processLintConfig(config); err == nil {
			t.Fatal("processLintConfig() error = nil, want a read failure")
		}
	})
}

// GetMainTemplates owns the "do not write this file at all" decision, so the
// own-mode filter is tested where it lives.
func TestGetMainTemplatesLintConfigOwn(t *testing.T) {
	lintFile := filepath.FromSlash(lintconfig.ConfigPath)
	metricsFile := filepath.FromSlash("configs/golangci-metrics.yml")

	params := templater.GeneratorParams{
		AppInfo: "test",
		Metrics: ds.MetricsConfig{Enabled: true},
	}

	t.Run("generated by default", func(t *testing.T) {
		_, files, err := templater.GetMainTemplates(params)
		if err != nil {
			t.Fatalf("GetMainTemplates() error = %v", err)
		}

		if !hasDest(files, lintFile) {
			t.Errorf("%s must be generated by default", lintFile)
		}
	})

	t.Run("own drops the lint config but keeps the metrics gate", func(t *testing.T) {
		ownParams := params
		ownParams.LintConfigOwn = true

		_, files, err := templater.GetMainTemplates(ownParams)
		if err != nil {
			t.Fatalf("GetMainTemplates() error = %v", err)
		}

		if hasDest(files, lintFile) {
			t.Errorf("%s must not be generated when the project owns it", lintFile)
		}

		// Criterion 4 of the request: the metrics validator is a separate file
		// behind `make metrics-validate`, so the TOOLS-8 gate survives whoever
		// owns the main lint config.
		if !hasDest(files, metricsFile) {
			t.Errorf("%s must still be generated — the metrics gate is independent", metricsFile)
		}
	})
}

func hasDest(files []ds.Files, dest string) bool {
	for _, f := range files {
		if f.DestName == dest {
			return true
		}
	}

	return false
}

// writeTempFile creates path with content, failing the test on error.
func writeTempFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing fixture %s: %v", path, err)
	}
}
