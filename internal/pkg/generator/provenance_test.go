package generator

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Educentr/go-project-starter/internal/pkg/meta"
)

// Functions allowed to drive the resolver. Both record the provenance of what
// they resolved, which is what makes provenance a property of resolving rather
// than of the channel that asked: a channel added later gets a record from the
// same code as the existing ones.
// The resolver itself is built in one place: the run has exactly one, so that
// every channel naming the same (repo, ref) reads one clone and one commit.
var (
	allowedResolveCallers   = map[string]bool{"resolveOne": true, "resolveInlineSpec": true}
	allowedNewResolverHosts = map[string]bool{"sourceResolver": true}
)

// TestEveryResolveRecordsProvenance is the guard behind "a new channel is
// covered by itself". A channel that resolved its source directly would copy a
// file into the project with no record of where it came from — and nothing
// about the manifest would say so, because the record would simply be missing
// its provenance fields.
//
// So the property is checked where it can be: every call that materializes a
// source goes through one of the two recording helpers.
func TestEveryResolveRecordsProvenance(t *testing.T) {
	fset := token.NewFileSet()

	pkg, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parse package: %v", err)
	}

	for _, p := range pkg {
		for name, file := range p.Files {
			checkResolveCalls(t, filepath.Base(name), file)
		}
	}
}

// checkResolveCalls walks one file and reports a Resolve / NewResolver call
// made from a function that does not record provenance.
func checkResolveCalls(t *testing.T, file string, f *ast.File) {
	t.Helper()

	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			// Matched by name rather than by type: the variable holding the
			// resolver is named differently in each helper, and a new channel
			// would name it however it likes. The package has no other Resolve
			// method to confuse this with.
			switch sel.Sel.Name {
			case "Resolve":
				if !allowedResolveCallers[fn.Name.Name] {
					t.Errorf("%s: %s calls Resolve directly — resolve through resolveOne "+
						"or resolveInlineSpec, which record where the file came from", file, fn.Name.Name)
				}
			case "NewResolver":
				if !allowedNewResolverHosts[fn.Name.Name] {
					t.Errorf("%s: %s builds its own resolver — use the resolve pre-pass or "+
						"resolveInlineSpec, which record where the files came from", file, fn.Name.Name)
				}
			}

			return true
		})
	}
}

// An inline input leaves no file in the project, so the `inputs` section of the
// manifest is the only trace of it — and the question it answers (which
// contracts the service was built from) is about it as much as about a copied
// spec.
func TestResolveInlineSpecRecordsInput(t *testing.T) {
	base := t.TempDir()
	body := "metrics: []\n"

	if err := os.WriteFile(filepath.Join(base, "metrics-map.yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	g := &Generator{BasePath: base}
	defer closeGenerator(t, g)

	path, err := g.resolveInlineSpec(t.Context(), channelMetricsPath, "./metrics-map.yaml")
	if err != nil {
		t.Fatalf("resolveInlineSpec() error = %v", err)
	}

	if want := filepath.Join(base, "metrics-map.yaml"); path != want {
		t.Errorf("path = %q, want %q", path, want)
	}

	inputs := g.inlineInputs()
	if len(inputs) != 1 {
		t.Fatalf("inputs = %+v, want one record", inputs)
	}

	rec := inputs[0]

	if rec.Channel != channelMetricsPath {
		t.Errorf("channel = %q, want %q", rec.Channel, channelMetricsPath)
	}

	if rec.SourceKind != meta.SourceKindLocal || rec.SourcePath != "./metrics-map.yaml" {
		t.Errorf("provenance = %+v, want the source as project.yaml writes it", rec.Provenance)
	}

	if rec.SourceSHA256 != meta.HashContent([]byte(body)) {
		t.Errorf("digest = %q, want the digest of the bytes read", rec.SourceSHA256)
	}
}

// The copy step knows only the staging path a spec was read from, so that is
// the key the provenance is looked up by when the manifest record is built.
func TestCopiedFileRecordCarriesProvenanceOfItsSource(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "api", "rest", "example", "v1", "api.yaml")

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}

	// The copy inside the project, already rewritten: its hash is not the
	// digest of what the source served, which is the whole reason the two are
	// recorded separately.
	if err := os.WriteFile(dest, []byte("rewritten\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	staged := filepath.Join(t.TempDir(), "api.yaml")

	g := &Generator{}
	g.sourceProvenance = map[string]meta.Provenance{
		staged: {
			SourceKind:   meta.SourceKindGit,
			SourceRepo:   "ssh://git@github.com/org/contracts.git",
			SourcePath:   "rest/api.yaml",
			Ref:          "main",
			Commit:       "0123456789abcdef",
			SourceSHA256: "sourcedigest",
		},
	}

	rec, err := g.copiedFileRecord(dir, copiedFile{dest: dest, source: staged})
	if err != nil {
		t.Fatalf("copiedFileRecord() error = %v", err)
	}

	if rec.Path != "api/rest/example/v1/api.yaml" || rec.Kind != meta.KindCopied {
		t.Errorf("record = %+v", rec)
	}

	if rec.Hash != meta.HashContent([]byte("rewritten\n")) {
		t.Errorf("hash = %q, want the hash of the copy on disk", rec.Hash)
	}

	if rec.SourceSHA256 != "sourcedigest" || rec.Commit != "0123456789abcdef" {
		t.Errorf("provenance = %+v, want the one recorded for the source", rec.Provenance)
	}
}

// A file the generator produced from a template has no source, and its record
// must keep exactly the shape it had before provenance existed.
func TestCopiedFileRecordWithoutProvenance(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "api.yaml")

	if err := os.WriteFile(dest, []byte("spec\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	g := &Generator{}

	rec, err := g.copiedFileRecord(dir, copiedFile{dest: dest, source: "/nowhere/api.yaml"})
	if err != nil {
		t.Fatalf("copiedFileRecord() error = %v", err)
	}

	if !rec.Provenance.IsZero() {
		t.Errorf("provenance = %+v, want none for a source that never went through the resolver", rec.Provenance)
	}
}
