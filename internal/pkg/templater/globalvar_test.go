package templater

import (
	"io/fs"
	"strings"
	"testing"
)

// TestGlobalVarScanCatchesReintroduction checks the classifier against the
// shapes it has to catch and the ones it must let through — including each shape
// removed from the templates when gochecknoglobals was turned on (TOOLS-31).
func TestGlobalVarScanCatchesReintroduction(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want int
	}{
		{
			name: "the mutable registry this task removed",
			body: "package p\n\nvar CustomError = map[error]func(){}\n",
			want: 1,
		},
		{
			name: "a reassignable hook",
			body: "package p\n\nvar updateHandler = (*Worker).handleUpdate\n",
			want: 1,
		},
		{
			name: "env read in a var initializer",
			body: "package p\n\nvar (\n\tErrorTimeout    = durationFromEnv(\"X\", 0)\n\tNewCycleTimeout = durationFromEnv(\"Y\", 0)\n)\n",
			want: 2,
		},
		{
			name: "a declaration without a value",
			body: "package p\n\nvar tgSHA []byte\n",
			want: 1,
		},
		{
			name: "sentinel errors are fine",
			body: "package p\n\nvar (\n\tErrOne = errors.New(\"one\")\n\terrTwo = errors.New(\"two\")\n)\n",
			want: 0,
		},
		{
			name: "a compile-time interface check is fine",
			body: "package p\n\nvar _ oas.Handler = (*Handler)(nil)\n",
			want: 0,
		},
		{
			name: "regexp.MustCompile is fine",
			body: "package p\n\nvar rx = regexp.MustCompile(`a+`)\n",
			want: 0,
		},
		{
			name: "a go:embed declaration is fine",
			body: "package p\n\n//go:embed static\nvar static embed.FS\n",
			want: 0,
		},
		{
			name: "a trailing nolint is honored",
			body: "package p\n\nvar Version string //nolint:gochecknoglobals // -X main.Version\n",
			want: 0,
		},
		{
			name: "a nolint on the line above is honored",
			body: "package p\n\n//nolint:gochecknoglobals // reason\nvar table = map[string]int{}\n",
			want: 0,
		},
		{
			name: "a nolint listing several linters is honored",
			body: "package p\n\n//nolint:gochecknoglobals,decorder // reason\nvar table = map[string]int{}\n",
			want: 0,
		},
		{
			name: "a nolint for another linter is not",
			body: "package p\n\nvar table = map[string]int{} //nolint:lll // reason\n",
			want: 1,
		},
		{
			name: "a name the scan cannot read needs a directive",
			body: "package p\n\nvar {{ $m.VarName }} = prometheus.NewCounter(\n\tOpts{},\n)\n",
			want: 1,
		},
		{
			name: "a function-local var is not package scope",
			body: "package p\n\nfunc f() {\n\tvar funcs = template.FuncMap{}\n\t_ = funcs\n}\n",
			want: 0,
		},
		{
			// Both logger templates and the ogen error-response template open
			// one of these inside a function body.
			name: "a function-local var block is not package scope",
			body: "package p\n\nfunc f() {\n\tvar (\n\t\tlevelGlob = zlog.InfoLevel\n\t\tzlogInst  = zlog.New(os.Stdout)\n\t)\n\t_ = levelGlob\n}\n",
			want: 0,
		},
		{
			// The opposite of the init() guard, which had to be widened to see
			// exactly this shape: there `func init(` inside a raw string was
			// still a worked example of the forbidden pattern. gochecknoglobals
			// has nothing to say about a variable outside package scope, so
			// neither does this.
			name: "a worked example inside a raw string literal is not a declaration",
			body: "package p\n\nfunc f() {\n\tpanic(`implement it:\n\n    var _ = SetTestEnvInitializer(&impl{})\n`)\n}\n",
			want: 0,
		},
		{
			name: "the nested lines of a composite literal are not declarations",
			body: "package p\n\nvar (\n\tErrX = errors.New(\"x\")\n\tresp = Response{\n\t\tText: \"hi\",\n\t}\n)\n",
			want: 1,
		},
		{
			// The closing brace of a composite literal sits at the same one-tab
			// indent as the entries, so only "starts with an identifier" tells
			// them apart. Without that it reaches the name reader, fails to
			// parse and is reported — a false positive on every grouped
			// declaration holding a literal.
			name: "the closing brace of a composite literal is not a declaration",
			body: "package p\n\nvar (\n\tErrX = errors.New(\"x\")\n\ttable = map[string]int{\n\t\t\"a\": 1,\n\t}\n)\n",
			want: 1,
		},
		{
			name: "a comment inside a var block is not a declaration",
			body: "package p\n\nvar (\n\t// importErrRx = regexp.MustCompile(`x`)\n\tErrX = errors.New(\"x\")\n)\n",
			want: 0,
		},
		{
			name: "a template action inside a var block is not a declaration",
			body: "package p\n\nvar (\n{{- range $_, $c := .Checks }}\n\tErrX = errors.New(\"x\")\n{{- end }}\n)\n",
			want: 0,
		},
		{
			name: "several names in one specification are all checked",
			body: "package p\n\nvar a, ErrB = f()\n",
			want: 1,
		},
		// The cases below are the holes a review found in the first version of
		// this scan. Each one was a silent pass: the classifier failed to parse
		// the shape and skipped the line instead of reporting it, so a mutable
		// global in that shape would have shipped to every generated project
		// while the guard stayed green.
		{
			// `Err`-prefixed name whose `error` is a *return* type. The previous
			// rule accepted any spec ending in " error".
			name: "an err-prefixed handler is not a sentinel error",
			body: "package p\n\nvar errorHandler func(ctx context.Context) error\n",
			want: 1,
		},
		{
			name: "an Err-prefixed mutable func global is not a sentinel error",
			body: "package p\n\nvar ErrHandler = func(error) error { return nil }\n",
			want: 1,
		},
		{
			// A trailing directive belongs to the entry it sits on; it used to
			// leak to the entry below.
			name: "a directive on one block entry does not exempt the next",
			body: "package p\n\nvar (\n\tBuildCommit string //nolint:gochecknoglobals // -X main.BuildCommit\n\tregistry = map[string]int{}\n)\n",
			want: 1,
		},
		{
			// Space-indented blocks exist in this template tree; the previous
			// rule required exactly one tab and skipped the whole block.
			name: "a space-indented block entry is still a declaration",
			body: "package p\n\nvar (\n    registry = map[string]int{}\n)\n",
			want: 1,
		},
		{
			// A trailing comment on the opener used to defeat block detection,
			// hiding every entry.
			name: "a block opener with a trailing comment still opens a block",
			body: "package p\n\nvar ( // lookup tables\n\tregistry = map[string]int{}\n\thooks = map[string]func(){}\n)\n",
			want: 2,
		},
		{
			// golangci-lint honors a directive on the whole declaration, so this
			// scan does too — the exemption is recorded, not silent.
			name: "a directive above a block exempts the block",
			body: "package p\n\n//nolint:gochecknoglobals // reason\nvar (\n\tregistry = map[string]int{}\n)\n",
			want: 0,
		},
		{
			// An unreadable name inside a block used to be skipped, so moving
			// the metrics collectors into a block would have dropped their
			// reviewed exception.
			name: "an unreadable name inside a block still needs a directive",
			body: "package p\n\nvar (\n\t{{ $m.VarName }} = prometheus.NewCounter(Opts{})\n)\n",
			want: 1,
		},
		{
			// The linter allows a value that *is* a MustCompile call, not a
			// mutable container holding one.
			name: "a map of compiled patterns is not a MustCompile value",
			body: "package p\n\nvar patterns = map[string]*regexp.Regexp{\"a\": regexp.MustCompile(\"^/a\")}\n",
			want: 1,
		},
		{
			name: "a slice of compiled patterns is not a MustCompile value",
			body: "package p\n\nvar patterns = []*regexp.Regexp{regexp.MustCompile(\"a\")}\n",
			want: 1,
		},
		{
			// A leading action does not change the scope of what follows it.
			name: "a declaration behind a one-line template action is package scope",
			body: "package p\n\n{{ if .X }}var mutable = map[string]string{}{{ end }}\n",
			want: 1,
		},
		{
			// `var` is a prefix of plenty of identifiers.
			name: "an identifier starting with var is not a declaration",
			body: "package p\n\nfunc f() { variableName := 1; _ = variableName }\n",
			want: 0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := scanGlobalVars("t.tmpl", tc.body)
			if len(got) != tc.want {
				t.Errorf("scan reported %d declarations, want %d: %+v", len(got), tc.want, got)
			}
		})
	}
}

// TestGlobalVarScanReachesTemplates fails if the walk behind
// GlobalVarOccurrences stops reading templates.
//
// A guard whose healthy state is "nothing reported" is indistinguishable from a
// guard whose walk broke — a renamed templates root, an embed pattern that
// stopped matching. The classifier is covered above; what is checked here is
// that it is handed real content, and specifically the Go templates, which are
// the ones that can declare a variable at all.
func TestGlobalVarScanReachesTemplates(t *testing.T) {
	// Far below the real count (78+ templates, dozens of them *.go.tmpl); high
	// enough that an embed pattern matching only a subdirectory fails here.
	const minGoTemplates = 20

	goTemplates := 0

	err := fs.WalkDir(templates, templatesRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() && strings.HasSuffix(path, ".go.tmpl") {
			goTemplates++
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", templatesRoot, err)
	}

	if goTemplates < minGoTemplates {
		t.Fatalf("the walk reached only %d *.go.tmpl templates, expected at least %d: "+
			"the scan is broken, not the templates", goTemplates, minGoTemplates)
	}
}
