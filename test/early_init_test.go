package test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// callOffset returns the byte offset of the single call to want in src, where
// want is written as it appears in the source ("godotenv.Load",
// "occonf.Initialize", "earlyFunc.earlyInit").
//
// Calls are found through the AST rather than with strings.Index because the
// generated main documents the hook in a comment that names
// occonf.Initialize — a text search would match the comment and report an
// order that is not the order of the statements.
func callOffset(t *testing.T, fset *token.FileSet, file *ast.File, want string) int {
	t.Helper()

	found := []int{}

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		recv, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}

		if recv.Name+"."+sel.Sel.Name == want {
			found = append(found, fset.Position(call.Pos()).Offset)
		}

		return true
	})

	if len(found) != 1 {
		t.Fatalf("expected exactly one call to %s in the generated main, found %d", want, len(found))
	}

	return found[0]
}

// assertEarlyInitPoint checks the early extension point in one generated main:
// the declarations that make it optional, the call, and the position of that
// call — after the .env load and before occonf.Initialize.
//
// The position is the whole point of the feature (TOOLS-123, issue #142): a
// hook called after Initialize cannot change the environment the configuration
// tree is built from, which is what projects were using init() for.
func assertEarlyInitPoint(t *testing.T, targetDir, app, exitStmt string) {
	t.Helper()

	path := filepath.Join(targetDir, "cmd", app, "psg_main_gen.go")
	src := readGenerated(t, targetDir, "cmd", app, "psg_main_gen.go")

	// The default is a method promoted from an embedded type: a method the
	// project declares on *UserEarlyInit shadows it, and that is what lets a
	// project leave the hook alone.
	for _, want := range []string{
		"type EmptyEarlyInit struct{}",
		"func (e *EmptyEarlyInit) earlyInit() error { return nil }",
		"type UserEarlyInit struct {",
		"earlyFunc := UserEarlyInit{}",
		"err = earlyFunc.earlyInit()",
		// A failing hook stops the process with its own exit code, so a
		// caller can tell it apart from a broken configuration tree.
		"ExitCodeErrorEarlyInit",
		exitStmt,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("cmd/%s/psg_main_gen.go should contain %q", app, want)
		}
	}

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, path, src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing cmd/%s/psg_main_gen.go: %v", app, err)
	}

	envLoad := callOffset(t, fset, file, "godotenv.Load")
	early := callOffset(t, fset, file, "earlyFunc.earlyInit")
	ocInit := callOffset(t, fset, file, "occonf.Initialize")

	if early < envLoad {
		t.Errorf("cmd/%s: the hook is called before godotenv.Load; .env adds variables that are "+
			"not in the environment yet, so a hook running earlier judges an environment the "+
			"process never sees", app)
	}

	if early > ocInit {
		t.Errorf("cmd/%s: the hook is called after occonf.Initialize; the point of it is to run "+
			"while the configuration tree is not built yet", app)
	}
}

// TestGenerateEarlyInitPointInServiceMain covers the service main (cmd).
func TestGenerateEarlyInitPointInServiceMain(t *testing.T) {
	tmpDir := generateFromConfig(t, "rest-only", "Generate REST project to check the early init point")

	assertEarlyInitPoint(t, tmpDir, "api", "os.Exit(ExitCodeErrorEarlyInit)")
}

// TestGenerateEarlyInitPointInCLIMain covers the CLI main (cmd_cli), where the
// hook has to sit in the same place: a CLI had no extension point at all, and
// its os.Args and stdout checks are among the things that have to happen before
// anything is read.
func TestGenerateEarlyInitPointInCLIMain(t *testing.T) {
	tmpDir := generateFromConfig(t, "cli-only", "Generate CLI project to check the early init point")

	assertEarlyInitPoint(t, tmpDir, "admin-cli", "return ExitCodeErrorEarlyInit")
}

// TestGenerateEarlyInitExitCodesAppended pins the exit codes that were there
// before the hook.
//
// The codes come from one iota block, so declaring the new one anywhere but at
// the end renumbers the codes above it — and those are the process's observable
// contract: the scripts and units of a generated project already branch on
// them.
func TestGenerateEarlyInitExitCodesAppended(t *testing.T) {
	for _, tc := range []struct {
		configDir string
		app       string
		want      []string
	}{
		{
			configDir: "rest-only",
			app:       "api",
			want: []string{
				"ExitCodeOK = iota",
				"ExitCodeErrorConfig",
				"ExitCodeErrorApp",
				"ExitCodeErrorDriver",
				"ExitCodeErrorTransport",
				"ExitCodeErrorService",
				"ExitCodeInit",
				"ExitCodeErrorRun",
				"ExitCodeErrorEarlyInit",
			},
		},
		{
			configDir: "cli-only",
			app:       "admin-cli",
			want: []string{
				"ExitCodeOK = iota",
				"ExitCodeErrorConfig",
				"ExitCodeErrorDriver",
				"ExitCodeErrorService",
				"ExitCodeErrorRun",
				"ExitCodeErrorEarlyInit",
			},
		},
	} {
		t.Run(tc.configDir, func(t *testing.T) {
			tmpDir := generateFromConfig(t, tc.configDir, "Generate "+tc.configDir+" to check exit codes")
			src := readGenerated(t, tmpDir, "cmd", tc.app, "psg_main_gen.go")

			prev := -1

			for _, name := range tc.want {
				at := strings.Index(src, name)
				if at < 0 {
					t.Fatalf("cmd/%s/psg_main_gen.go should declare %s", tc.app, name)
				}

				if at < prev {
					t.Errorf("cmd/%s/psg_main_gen.go: %s is declared out of order — the iota block "+
						"numbers the exit codes by position", tc.app, name)
				}

				prev = at
			}
		})
	}
}
