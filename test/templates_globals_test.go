package test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Educentr/go-project-starter/internal/pkg/meta"
	"github.com/Educentr/go-project-starter/internal/pkg/templater"
)

// generatedGlobalsConfigs are the test configs whose output carries the
// declarations TestNoUnjustifiedGlobalsInGeneratedProjects is about.
//
// Not every config: generation is the slow part, and the template scan already
// covers every template unconditionally. What these add is what a template scan
// cannot see — the expanded output, including a declaration a template emits
// conditionally or from a range — so the set is chosen to hit each template that
// declares something at package level.
var generatedGlobalsConfigs = []string{
	// telegram worker and driver: the hooks, the error-handler registry, the
	// WebApp signing key, the Unimplemented response.
	"worker-telegram",
	// daemon worker: the loop timers.
	"worker-daemon",
	// goat test environment (the singleton holder, the port table, the retry
	// parameters) plus the ogen client mocks.
	"rest-client-mocks",
	// the generated Prometheus collectors.
	"metrics-map",
	// the generated OnlineConf required-key tables.
	"onlineconf-map",
	// cmd_cli: the ldflags block of a CLI application.
	"cli-only",
}

// globalsNolintRx matches a nolint directive covering gochecknoglobals.
//
// Spelled out again rather than imported from the template scanner, the way
// assertNoInitFuncUnder repeats the init() guard's own pattern: this half
// answers for the generated output, and a guard that borrowed its rule from the
// code it checks would keep passing if that rule were loosened.
//
// The value and type classifiers below (allGlobalValuesAllowed,
// specDeclaresError) are not copies — they walk an AST where the template scan
// matches a line, which is the whole reason for having two guards. This pattern
// and looksLikeErrorName are the two places where the second implementation is
// the same rule written out twice, deliberately.
var globalsNolintRx = regexp.MustCompile(`//\s*nolint:[\w,]*\bgochecknoglobals\b`)

// TestNoUnjustifiedGlobalsInTemplates is the broad guard that keeps mutable
// package-level state out of every generated project.
//
// `gochecknoglobals` is enabled in the generated project's lint config, but a
// linter never parses a `.tmpl` file — and even in the generated tree
// `exclusions.generated: lax` keeps it away from `psg_*_gen.go` entirely. So the
// only thing that can speak for the templates is a scan of the templates, and it
// is also the only guard independent of the project's own config: a project that
// takes ownership of `configs/golangci-lint.yml` (`tools.lint_config.own`) drops
// the linter altogether.
//
// Extension points of generated code are declared so that no project has to
// reassign a global: the telegram worker's hooks and error handlers are fields
// set from `customize()` (SetUpdateHandler, SetUnknownCommandHandler,
// AddErrorHandler), the daemon worker's loop timers are fields read in
// `Create()`, and the telegram driver's WebApp signing key is a field of the
// driver instance.
func TestNoUnjustifiedGlobalsInTemplates(t *testing.T) {
	found, err := templater.GlobalVarOccurrences()
	if err != nil {
		t.Fatalf("GlobalVarOccurrences() error: %v", err)
	}

	for _, o := range found {
		t.Errorf("%s:%d declares a package-level variable (%q); generated projects enable "+
			"gochecknoglobals — use a field with a setter, or add //nolint:gochecknoglobals "+
			"with the reason if it genuinely has to be package-level (see DEPRECATION.md)",
			o.Template, o.Line, o.Text)
	}
}

// TestNoUnjustifiedGlobalsInGeneratedProjects generates projects and applies the
// gochecknoglobals rules to the generator's own output, parsed as real Go.
//
// The template scan is the broad guard; this one checks what the scan cannot: a
// template is not valid Go, so there is no AST to ask, and a declaration that
// only appears after expansion (behind a `{{ if }}`, or emitted from a range) is
// invisible to a line scan of the template. It also catches a global put back by
// a post-generation step.
//
// Only the files the manifest attributes to the generator are examined. The
// generated tree also holds output of other generators — ogen, argen, mockgen —
// which is full of package-level state that is not this repository's to fix; the
// manifest is the authoritative answer to "did the generator write this file?".
func TestNoUnjustifiedGlobalsInGeneratedProjects(t *testing.T) {
	for _, configDir := range generatedGlobalsConfigs {
		t.Run(configDir, func(t *testing.T) {
			// Each subtest generates into its own t.TempDir and only reads it,
			// so the six generations have nothing to serialize on.
			t.Parallel()

			assertNoUnjustifiedGlobalsUnder(t, generateProject(t, configDir))
		})
	}
}

// assertNoUnjustifiedGlobalsUnder reports every package-level variable in the
// generator's output that gochecknoglobals would reject.
func assertNoUnjustifiedGlobalsUnder(t *testing.T, root string) {
	t.Helper()

	m, err := meta.GetMeta(root, filepath.Join(root, ".project-config", "meta.yaml"))
	if err != nil {
		t.Fatalf("reading the manifest of %s: %v", root, err)
	}

	checked := 0

	for _, rec := range m.Manifest {
		if rec.Kind != meta.KindGenerated || filepath.Ext(rec.Path) != ".go" {
			continue
		}

		// A generated _test.go is excluded from the linter by path in the
		// generated config, so the guard follows it.
		if strings.HasSuffix(rec.Path, "_test.go") {
			continue
		}

		checked++

		reportGlobalsInGeneratedFile(t, root, rec.Path)
	}

	if checked == 0 {
		t.Fatalf("the manifest of %s lists no generated Go file: the check has nothing to look at", root)
	}

	t.Logf("checked %d generated Go files under %s", checked, root)
}

// reportGlobalsInGeneratedFile parses one generated file and reports the
// package-level variables that no accepted shape covers.
func reportGlobalsInGeneratedFile(t *testing.T, root, rel string) {
	t.Helper()

	path := filepath.Join(root, rel)

	src, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("reading %s: %v", rel, err)

		return
	}

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, path, src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		t.Errorf("generated file %s is not valid Go: %v", rel, err)

		return
	}

	lines := strings.Split(string(src), "\n")

	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.VAR {
			continue
		}

		if docHasPrefix(genDecl.Doc, "//go:embed ") {
			continue
		}

		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}

			if allGlobalValuesAllowed(valueSpec.Values) {
				continue
			}

			declaresError := specDeclaresError(valueSpec)

			for _, name := range valueSpec.Names {
				if name.Name == "_" || name.Name == "version" {
					continue
				}

				if looksLikeErrorName(name.Name) && declaresError {
					continue
				}

				line := fset.Position(name.Pos()).Line
				if globalsNolintNear(lines, line) {
					continue
				}

				t.Errorf("%s:%d: generated project declares a package-level variable %q; "+
					"gochecknoglobals is enabled in the generated config — use a field with a "+
					"setter, or add //nolint:gochecknoglobals with the reason",
					rel, line, name.Name)
			}
		}
	}
}

// globalsNolintNear reports a nolint directive for gochecknoglobals on the
// 1-indexed line or on the line above it, which is where the templates put it.
func globalsNolintNear(lines []string, line int) bool {
	for _, i := range []int{line - 1, line - 2} {
		if i >= 0 && i < len(lines) && globalsNolintRx.MatchString(lines[i]) {
			return true
		}
	}

	return false
}

func docHasPrefix(doc *ast.CommentGroup, prefix string) bool {
	if doc == nil {
		return false
	}

	for _, c := range doc.List {
		if strings.HasPrefix(c.Text, prefix) {
			return true
		}
	}

	return false
}

// allGlobalValuesAllowed mirrors the linter's value whitelist: a declaration
// whose every value is a `regexp.MustCompile` call is accepted whatever its
// name.
func allGlobalValuesAllowed(values []ast.Expr) bool {
	if len(values) == 0 {
		return false
	}

	for _, v := range values {
		call, ok := v.(*ast.CallExpr)
		if !ok {
			return false
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return false
		}

		pkg, ok := sel.X.(*ast.Ident)
		if !ok || pkg.Name != "regexp" || sel.Sel.Name != "MustCompile" {
			return false
		}
	}

	return true
}

// looksLikeErrorName mirrors the linter's name rule for sentinel errors: an
// exported name prefixed `Err`, an unexported one prefixed `err`.
func looksLikeErrorName(name string) bool {
	prefix := "err"
	if name[0] >= 'A' && name[0] <= 'Z' {
		prefix = "Err"
	}

	return strings.HasPrefix(name, prefix)
}

// specDeclaresError reports whether a declaration really produces an error,
// which is what lets an `Err`-prefixed name through.
//
// The linter type-checks this; the scan is not a type checker, so it looks at
// the declared type (`var errX error`) or at the value constructing one. The
// prefix alone would not do: `ErrorTimeout` — a `time.Duration`, and one of the
// declarations this task removed — satisfies it.
func specDeclaresError(spec *ast.ValueSpec) bool {
	if ident, ok := spec.Type.(*ast.Ident); ok && ident.Name == "error" {
		return true
	}

	for _, v := range spec.Values {
		call, ok := v.(*ast.CallExpr)
		if !ok {
			return false
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return false
		}

		pkg, ok := sel.X.(*ast.Ident)
		if !ok {
			return false
		}

		switch {
		case pkg.Name == "errors" && (sel.Sel.Name == "New" || sel.Sel.Name == "Wrap" ||
			sel.Sel.Name == "Wrapf" || sel.Sel.Name == "Join"):
		case pkg.Name == "fmt" && sel.Sel.Name == "Errorf":
		default:
			return false
		}
	}

	return len(spec.Values) > 0
}
