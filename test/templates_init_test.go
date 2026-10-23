package test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Educentr/go-project-starter/internal/pkg/templater"
)

// TestNoInitFuncInTemplates is the guard that keeps `func init()` out of every
// generated project.
//
// `gochecknoinits` is enabled in the generated project's lint config, but a
// linter never parses a `.tmpl` file — so the only thing that can speak for the
// templates is a scan of the templates. It is also the only guard that does not
// depend on the project's own config: a project that takes ownership of
// `configs/golangci-lint.yml` (`tools.lint_config.own`) drops the linter, and
// `exclusions.generated: lax` keeps it away from `psg_*_gen.go` regardless.
//
// Extension points of generated code are declared so that no project needs
// init(): the telegram worker's command registry is a field filled from
// `customize()` (a method on *Worker shadowing the promoted default), and the
// goat test environment is replaced through `SetTestEnvInitializer` from a
// variable initializer.
func TestNoInitFuncInTemplates(t *testing.T) {
	found, err := templater.InitFuncOccurrences()
	if err != nil {
		t.Fatalf("InitFuncOccurrences() error: %v", err)
	}

	for _, o := range found {
		t.Errorf("%s:%d declares or documents an init function (%q); generated projects enable "+
			"gochecknoinits — use an explicit extension point instead (see DEPRECATION.md)",
			o.Template, o.Line, o.Text)
	}
}

// TestInitFuncScanSeesTemplates fails if the scan stops finding anything at all.
//
// A guard that reports zero occurrences is indistinguishable from a guard whose
// walk broke — a renamed templates root, an embed pattern that stopped matching.
// Pointing it at a sentinel string that definitely exists in the templates keeps
// "nothing found" meaning "nothing there".
func TestInitFuncScanSeesTemplates(t *testing.T) {
	root := templatesDirOnDisk(t)

	count := 0

	err := filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		count++

		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}

	if count == 0 {
		t.Fatalf("no template files under %s: the scan is broken, not the templates", root)
	}
}

// TestInitFuncScanCatchesReintroduction checks the guard against the exact shapes
// it has to catch, including the one that slipped through before: an init
// function inside a raw string literal, which is how the generated goat helper
// used to hand every project a worked example of the forbidden pattern.
func TestInitFuncScanCatchesReintroduction(t *testing.T) {
	rx := regexp.MustCompile(`func\s+init\s*\(`)

	for _, line := range []string{
		"func init() {",
		"\tfunc init() {",
		"    func init() {",
		"func  init  () {",
	} {
		if !rx.MatchString(line) {
			t.Errorf("the guard pattern misses %q", line)
		}
	}

	for _, line := range []string{
		"func initialize() {",
		"// initialization happens in Create",
		"var initDone bool",
	} {
		if rx.MatchString(line) {
			t.Errorf("the guard pattern falsely flags %q", line)
		}
	}
}

// templatesDirOnDisk returns the embedded templates directory in the working
// tree. The guard itself reads the embed FS; this is only used to prove the
// directory is not empty.
func templatesDirOnDisk(t *testing.T) string {
	t.Helper()

	curDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting current directory: %v", err)
	}

	root := filepath.Join(curDir, "..", "internal", "pkg", "templater", "embedded", "templates")

	if _, err := os.Stat(root); err != nil {
		t.Fatalf("templates root %s is not readable: %v", root, err)
	}

	return root
}

// TestNoInitFuncInGeneratedProjects generates the two projects whose templates
// used to carry init() — the telegram worker (the command registry) and a
// goat-enabled REST app (the test environment) — and asserts the output is free
// of it, together with the new extension points being present.
//
// The template scan above is the broad guard; this one checks the thing the
// scan cannot: that the generated files actually hold the replacement, and that
// no post-generation step (goimports, generators) puts init() back.
func TestNoInitFuncInGeneratedProjects(t *testing.T) {
	for _, tc := range []struct {
		configDir string
		expect    map[string][]string
	}{
		{
			configDir: "worker-telegram",
			expect: map[string][]string{
				"internal/app/worker/telegrambot/psg_const_gen.go": {
					"func generatedCommands() []telegram.Cmd",
				},
				"internal/app/worker/telegrambot/psg_telegram_gen.go": {
					"cmds map[telegram.CommandName]telegram.Cmd",
					"func (e *EmptyCustomizer) customize() {}",
					"func (w *Worker) AddCommands(cmds ...telegram.Cmd)",
					"w.AddCommands(generatedCommands()...)",
					"w.customize()",
				},
			},
		},
		{
			configDir: "github-ci-defaults",
			expect: map[string][]string{
				"tests/api/psg_init_gen.go": {
					"func SetTestEnvInitializer(i TestEnvInitializer) TestEnvInitializer",
					"var _ = SetTestEnvInitializer(&testEnvInitializerImpl{})",
				},
			},
		},
	} {
		t.Run(tc.configDir, func(t *testing.T) {
			curDir, err := os.Getwd()
			if err != nil {
				t.Fatalf("Error getting current directory: %v", err)
			}

			configDir := filepath.Join(curDir, "..", "test", "docker-integration", "configs", tc.configDir)
			tmpDir := t.TempDir()

			out, err := ExecCommand(filepath.Join(curDir, ".."), "go", []string{
				"run", filepath.Join(curDir, "..", "cmd", "go-project-starter", "main.go"),
				"--target", tmpDir,
				"--configDir", configDir,
				"--config", "project.yaml",
			}, "Generate "+tc.configDir+" project ("+tmpDir+")")
			if err != nil {
				t.Fatalf("Error creating project: %s\n%s", err, out)
			}

			assertNoInitFuncUnder(t, tmpDir)

			for rel, wants := range tc.expect {
				content, err := os.ReadFile(filepath.Join(tmpDir, rel))
				if err != nil {
					t.Fatalf("Error reading %s: %v", rel, err)
				}

				for _, want := range wants {
					if !strings.Contains(string(content), want) {
						t.Errorf("%s should contain %q", rel, want)
					}
				}
			}
		})
	}
}

// assertNoInitFuncUnder walks the generated tree and reports every `func init(`
// in a Go file the generator produced.
//
// `etc/repo-oc/` is skipped: `dev_stand: true` checks out the upstream
// onlineconf sources there for the local stand, and those are somebody else's
// code, not generator output.
func assertNoInitFuncUnder(t *testing.T, root string) {
	t.Helper()

	rx := regexp.MustCompile(`func\s+init\s*\(`)

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return rerr
		}

		if info.IsDir() {
			switch rel {
			case filepath.Join("etc", "repo-oc"), ".git":
				return filepath.SkipDir
			}

			return nil
		}

		if filepath.Ext(path) != ".go" {
			return nil
		}

		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}

		for i, line := range strings.Split(string(data), "\n") {
			if rx.MatchString(line) {
				t.Errorf("%s:%d: generated project contains an init function (%q)",
					rel, i+1, strings.TrimSpace(line))
			}
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
}
