package test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// TestGenerateRESTClientWithoutAuthParams is the regression test for the
// SecuritySource wrapper of a client to an API that needs no authentication.
//
// The wrapper's import block used to be emitted whatever auth_params said,
// while its body is emitted per auth type. A client without auth_params has no
// auth type, so the file came out as a package clause plus four imports
// nothing below them used — and a Go file with unused imports does not
// compile. Projects whose post_generate runs goimports had the imports swept
// away afterwards; the fixture here, like any project with an empty
// post_generate, kept the broken file.
func TestGenerateRESTClientWithoutAuthParams(t *testing.T) {
	tmpDir := generateFromConfig(t, "rest-client-only", "Generate project with a REST client without auth_params")

	authPath := filepath.Join("internal", "app", "transport", "rest", "billing", "v1", "psg_auth_gen.go")
	authFile := readGenerated(t, tmpDir, strings.Split(authPath, string(filepath.Separator))...)

	parsed, err := parser.ParseFile(token.NewFileSet(), authPath, authFile, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("%s does not parse as Go: %v\n%s", authPath, err, authFile)
	}

	if len(parsed.Imports) != 0 {
		t.Errorf("%s has %d imports, but no auth type is configured so nothing uses them:\n%s",
			authPath, len(parsed.Imports), authFile)
	}

	// Nothing but the package clause: a SecuritySource declared here is a
	// type the generated client never hands to ogen's NewClient.
	for _, decl := range parsed.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			t.Errorf("%s declares %s, but the client needs no SecuritySource:\n%s", authPath, d.Name.Name, authFile)
		case *ast.GenDecl:
			t.Errorf("%s declares a %s, but the client needs no SecuritySource:\n%s", authPath, d.Tok, authFile)
		}
	}
}
