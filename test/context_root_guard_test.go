package test

import (
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// lintConfigFile is the shape of the generated lint config this file reads: the
// forbidigo settings and the exclusion rules, and nothing else.
type lintConfigFile struct {
	Linters struct {
		Enable   []string `yaml:"enable"`
		Settings struct {
			Forbidigo struct {
				AnalyzeTypes bool `yaml:"analyze-types"`
				Forbid       []struct {
					Pattern string `yaml:"pattern"`
					Msg     string `yaml:"msg"`
				} `yaml:"forbid"`
			} `yaml:"forbidigo"`
		} `yaml:"settings"`
		Exclusions struct {
			Generated string `yaml:"generated"`
			Rules     []struct {
				Linters []string `yaml:"linters"`
				Path    string   `yaml:"path"`
			} `yaml:"rules"`
		} `yaml:"exclusions"`
	} `yaml:"linters"`
}

// TestGeneratedProjectForbidsNewContextRoots covers the guard side of TOOLS-132
// (issue #158 item 1): hand-written code in a generated project may not start a
// new context root.
//
// The rule is read out of the generated config and exercised as a regexp here,
// rather than asserted as a line of text: the pattern is the whole mechanism,
// and "the file contains this string" would pass just as happily for a pattern
// that matches nothing. What the regexp cannot say — that golangci-lint resolves
// the call through type information, so an import alias does not defeat the rule
// — is what analyze-types is checked for, and what
// TestGeneratedLintConfigRejectsContextRoots runs the real linter to confirm.
func TestGeneratedProjectForbidsNewContextRoots(t *testing.T) {
	tmpDir := generateFromConfig(t, "grpc-only", "Generate project for the context-root guard")

	raw := readGenerated(t, tmpDir, "configs", "golangci-lint.yml")

	var cfg lintConfigFile
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("generated configs/golangci-lint.yml does not parse: %v", err)
	}

	if !slices.Contains(cfg.Linters.Enable, "forbidigo") {
		t.Fatal("configs/golangci-lint.yml should enable forbidigo: it carries the context-root guard")
	}

	if !cfg.Linters.Settings.Forbidigo.AnalyzeTypes {
		t.Error("forbidigo should resolve types: without it `ctx \"context\"` + ctx.Background() " +
			"walks past a rule written for context.Background, and a gate an import alias " +
			"defeats is not a gate")
	}

	// The rule has to match both roots and nothing else: context.With* over the
	// context the caller was given is the answer, not the offense.
	var rule *regexp.Regexp

	var ruleMsg string

	for _, forbid := range cfg.Linters.Settings.Forbidigo.Forbid {
		compiled, err := regexp.Compile(forbid.Pattern)
		if err != nil {
			t.Fatalf("forbidigo pattern %q does not compile: %v", forbid.Pattern, err)
		}

		if compiled.MatchString("context.Background") {
			rule = compiled
			ruleMsg = forbid.Msg

			break
		}
	}

	if rule == nil {
		t.Fatal("configs/golangci-lint.yml should forbid context.Background: " +
			"the only context root of a generated project is the generated main")
	}

	for _, forbidden := range []string{"context.Background", "context.TODO"} {
		if !rule.MatchString(forbidden) {
			t.Errorf("the context-root rule should match %s", forbidden)
		}
	}

	// Deriving is the answer the rule points at, so it must not be caught by it.
	for _, allowed := range []string{
		"context.WithoutCancel", "context.WithCancel", "context.WithTimeout",
		"context.WithValue", "context.Context", "appctx.Detach",
	} {
		if rule.MatchString(allowed) {
			t.Errorf("the context-root rule must not match %s: deriving a context is what it asks for", allowed)
		}
	}

	// The message has to name the way out. A forbidding rule with no answer in
	// it is the reason people reach for a root in the first place.
	for _, want := range []string{"internal/pkg/appctx.Detach", "t.Context()", "occonftest"} {
		if !strings.Contains(ruleMsg, want) {
			t.Errorf("the context-root message should point at %q, it is %q", want, ruleMsg)
		}
	}

	// Generated files are exempt as a class, which is how the two legitimate
	// roots — mainCtx in each generated main — stay legal without an exemption
	// list that has to be maintained. Every Go file the generator writes is
	// named psg_*, and every one of them carries the generated header.
	if cfg.Linters.Exclusions.Generated != "lax" {
		t.Errorf("generated files should be exempt (`generated: lax`), it is %q: "+
			"the generated main is the one legal context root",
			cfg.Linters.Exclusions.Generated)
	}

	// _test.go is deliberately NOT exempt from forbidigo: a test takes its
	// context from the harness or from the generated fixture, and a test
	// building a bare root is where a read silently answering from a default is
	// least likely to be noticed.
	for _, exclusion := range cfg.Linters.Exclusions.Rules {
		if !strings.Contains(exclusion.Path, "_test") {
			continue
		}

		if slices.Contains(exclusion.Linters, "forbidigo") {
			t.Errorf("forbidigo must not be excluded for %q: the guard covers test files too", exclusion.Path)
		}
	}

	// And nothing the generator itself writes breaks the rule it just wrote:
	// every root in the output is in a psg_* file.
	for _, path := range goFilesWithContextRoot(t, tmpDir) {
		if !strings.HasPrefix(filepath.Base(path), "psg_") {
			t.Errorf("%s starts a context root and is not a generated file: "+
				"the project would be red on its own `make lint`", path)
		}
	}
}

// TestGeneratedDerivedContextHelper covers the answer side of TOOLS-132 (issue
// #158 item 2): the one generated way to get a context that outlives its
// parent.
//
// What the helper does with a live context is asserted where it can be run —
// assertDerivedContextBehavior in test/docker-integration, inside the generated
// project. Here: that it is generated at all, in every project, with one entry
// point and the shape the rule's message promises.
func TestGeneratedDerivedContextHelper(t *testing.T) {
	tmpDir := generateFromConfig(t, "grpc-only", "Generate project for the derived-context helper")

	helper := readGenerated(t, tmpDir, "internal", "pkg", "appctx", "psg_appctx_gen.go")

	for _, want := range []string{
		"package appctx",
		"func Detach(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc)",
		// WithoutCancel is the whole point: it keeps the values — the
		// OnlineConf tree, the logger, the request's trace id — and drops only
		// the cancellation. Copying values out by hand is a list that goes
		// stale the moment the wiring gains one.
		"context.WithoutCancel(parent)",
		// A positive timeout is a deadline on the detached context; zero or
		// less leaves it to the returned cancel. One entry point, because
		// every project needs the same two things of a derived context.
		"context.WithTimeout(detached, timeout)",
		"context.WithCancel(detached)",
	} {
		if !strings.Contains(helper, want) {
			t.Errorf("generated derived-context helper should contain %q", want)
		}
	}

	if _, err := parser.ParseFile(token.NewFileSet(), "psg_appctx_gen.go", helper,
		parser.SkipObjectResolution); err != nil {
		t.Errorf("generated derived-context helper is not valid Go: %v", err)
	}

	// The helper is not a way to get a context out of nothing — that is the
	// thing being removed, so it must not contain one.
	for _, forbidden := range []string{"context.Background()", "context.TODO()"} {
		if strings.Contains(stripComments(t, helper), forbidden) {
			t.Errorf("the derived-context helper must not build a root itself (%s)", forbidden)
		}
	}
}

// TestGeneratedOnlineConfTestFixture covers the test-fixture side of TOOLS-132
// (issue #158 item 3): what replaces the probe that was withdrawn.
//
// A test used to ask `occonf.Initialized(ctx)` and fall back to a default when
// the answer was no — which is the silent degradation the key map exists to
// remove, performed by the code that was supposed to be checking. The
// replacement is a context that really carries a tree, so the read answers from
// it and a missing key is a missing key.
func TestGeneratedOnlineConfTestFixture(t *testing.T) {
	tmpDir := generateFromConfig(t, "grpc-only", "Generate project for the OnlineConf test fixture")

	fixture := readGenerated(t, tmpDir, "internal", "pkg", "occonf", "occonftest",
		"psg_occonftest_gen.go")

	for _, want := range []string{
		"package occonftest",
		"func Context(tb testing.TB, values map[string]string) context.Context",
		// A real CDB through the real library, the same way the dev stand
		// builds one: a fake instance answering from a map would prove nothing
		// about the accessors.
		"onlineconf_dev.GenerateCDB(dir, onlineconf.DefaultModule, tree)",
		"occonf.Initialize(tb.Context(), dir)",
		// tb.TempDir, so the tree is gone when the test is.
		"dir := tb.TempDir()",
	} {
		if !strings.Contains(fixture, want) {
			t.Errorf("generated OnlineConf test fixture should contain %q", want)
		}
	}

	if _, err := parser.ParseFile(token.NewFileSet(), "psg_occonftest_gen.go", fixture,
		parser.SkipObjectResolution); err != nil {
		t.Errorf("generated OnlineConf test fixture is not valid Go: %v", err)
	}

	// It takes tb, and that is the design: a fixture that did not would have to
	// build a context root of its own.
	if strings.Contains(stripComments(t, fixture), "context.Background()") {
		t.Error("the fixture must descend from tb.Context(), not build a root of its own")
	}

	// A separate package, so `testing` stays out of the service binary's import
	// graph — and inside the accessor package's directory, because building a
	// tree means calling the library, which `make onlineconf-validate` allows
	// nowhere else.
	occonfFile := readGenerated(t, tmpDir, "internal", "pkg", "occonf", "psg_occonf_gen.go")
	if strings.Contains(occonfFile, `"testing"`) {
		t.Error("the accessor package must not import testing: it is linked into the service binary")
	}

	onlineconfLint := readGenerated(t, tmpDir, "configs", "golangci-onlineconf.yml")
	if !strings.Contains(onlineconfLint, "path: internal/pkg/occonf/") {
		t.Error("the OnlineConf validator's exemption should cover the accessor package's directory, " +
			"which is what lets the fixture build a tree")
	}
}

// TestGeneratedLintConfigRejectsContextRoots runs the real linter with the real
// generated config over a module that contains one root in hand-written code,
// one in a hand-written test and one in a generated file.
//
// This is the acceptance criterion as written — "a hand-written .go and a
// hand-written _test.go turn `make lint` red, a psg_*_gen.go does not" — and
// nothing short of golangci-lint can answer it: the exemption of generated
// files comes from `generated: lax` matching the header the generator writes,
// and the rule's reach over an import alias comes from analyze-types. The
// scratch module is used rather than the generated project because the
// generated project needs its dependencies downloaded before it type-checks,
// and the guard has nothing to do with them.
//
// Skipped when no usable golangci-lint is on PATH: the repository's own gate
// pins a v1 binary, which cannot read the v2 config a project is generated
// with. The hermetic half of the criterion is the test above.
func TestGeneratedLintConfigRejectsContextRoots(t *testing.T) {
	linter := usableGolangciLint(t)

	tmpDir := generateFromConfig(t, "grpc-only", "Generate project for the lint-run assertion")
	config := filepath.Join(tmpDir, "configs", "golangci-lint.yml")

	module := t.TempDir()

	files := map[string]string{
		"go.mod": "module ctxguard\n\ngo 1.24\n",
		// Hand-written: forbidden, and through an import alias, which only
		// analyze-types catches.
		"service.go": `package ctxguard

import (
	"context"
	alias "context"
)

func handRoot() context.Context { return context.Background() }

func handAliasRoot() context.Context { return alias.TODO() }
`,
		// Hand-written test: forbidden too, deliberately.
		"service_test.go": `package ctxguard

import (
	"context"
	"testing"
)

func TestHandRoot(t *testing.T) { _ = context.Background() }
`,
		// Generated, with the header the generator actually writes — trailing
		// spaces included, because that is what `generated: lax` has to cope
		// with and what strict mode would not.
		"psg_main_gen.go": "// Code generated by projectStarter generator. DO NOT EDIT. \n" +
			"// This code was generated from a template. \n" +
			"\npackage ctxguard\n\nimport \"context\"\n\n" +
			"func generatedRoot() context.Context { return context.Background() }\n",
	}

	for name, body := range files {
		if err := os.WriteFile(filepath.Join(module, name), []byte(body), 0o600); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}

	// --enable-only: the config enables fifty linters and this module is not
	// written to satisfy any of them. The question is only what forbidigo says.
	cmd := exec.CommandContext(t.Context(), linter, "run",
		"--config="+config, "--enable-only=forbidigo", "./...")
	cmd.Dir = module

	out, err := cmd.CombinedOutput()
	report := string(out)

	if err == nil {
		t.Fatalf("the generated config should reject a new context root:\n%s", report)
	}

	for _, want := range []string{
		"service.go", "service_test.go",
		"new root contexts are not allowed",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("the lint report should mention %q:\n%s", want, report)
		}
	}

	if strings.Contains(report, "psg_main_gen.go") {
		t.Errorf("a generated file must not be reported: the generated main is the one legal root:\n%s", report)
	}
}

// usableGolangciLint returns the path of a golangci-lint that can read the
// config a project is generated with (schema version 2), or skips the test.
func usableGolangciLint(t *testing.T) string {
	t.Helper()

	path, err := exec.LookPath("golangci-lint")
	if err != nil {
		t.Skip("golangci-lint is not on PATH; the hermetic half of this criterion is " +
			"TestGeneratedProjectForbidsNewContextRoots")
	}

	out, err := exec.CommandContext(t.Context(), path, "--version").CombinedOutput()
	if err != nil {
		t.Skipf("golangci-lint at %s does not report a version: %v", path, err)
	}

	version := regexp.MustCompile(`\b(\d+)\.\d+`).FindStringSubmatch(string(out))
	if version == nil || version[1] == "1" {
		t.Skipf("golangci-lint at %s cannot read a v2 config (%s)", path, strings.TrimSpace(string(out)))
	}

	return path
}

// goFilesWithContextRoot returns every Go file under root whose code — not its
// comments — starts a new context root.
func goFilesWithContextRoot(t *testing.T, root string) []string {
	t.Helper()

	var found []string

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}

		// A cloned third-party tree inside the project (the dev stand clones
		// OnlineConf into etc/) is its own Go module, so the project's `./...`
		// never reaches it and its code is nobody's to lint here.
		if inNestedModule(path, root) {
			return nil
		}

		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		code := stripComments(t, string(body))
		if strings.Contains(code, "context.Background()") || strings.Contains(code, "context.TODO()") {
			relative, relErr := filepath.Rel(root, path)
			if relErr != nil {
				relative = path
			}

			found = append(found, relative)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walking the generated project: %v", err)
	}

	return found
}

// inNestedModule reports whether path belongs to a Go module declared below
// root — the project's own go.mod at root does not count.
func inNestedModule(path, root string) bool {
	for dir := filepath.Dir(path); dir != root && strings.HasPrefix(dir, root); dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return true
		}
	}

	return false
}

// stripComments returns body with its comments removed, so that a doc comment
// naming context.Background() does not read as a call to it.
func stripComments(t *testing.T, body string) string {
	t.Helper()

	fileSet := token.NewFileSet()

	file, err := parser.ParseFile(fileSet, "body.go", body, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		// Not parseable Go: the caller's own assertion reports that better than
		// a failure here would, so the text goes back unchanged.
		return body
	}

	var out strings.Builder

	previous := 0
	raw := []byte(body)
	base := fileSet.File(file.Pos()).Base()

	for _, group := range file.Comments {
		start := int(group.Pos()) - base
		end := int(group.End()) - base

		if start < previous || end > len(raw) {
			continue
		}

		out.Write(raw[previous:start])
		previous = end
	}

	out.Write(raw[previous:])

	return out.String()
}
