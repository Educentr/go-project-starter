package lintconfig_test

import (
	"strings"
	"testing"

	"github.com/Educentr/go-project-starter/internal/pkg/lintconfig"
	"gopkg.in/yaml.v3"
)

// generatedBase is a trimmed stand-in for the rendered golangci-lint.yml
// template: an explicitly enabled linter set, nested settings and a comment,
// which is everything the merge has to deal with.
const generatedBase = `version: "2"
linters:
  enable:
    - dupl
    - lll
    - misspell
    - wsl
  settings:
    # Keeping a comment here: the merge must not drop it.
    dupl:
      threshold: 150
    lll:
      line-length: 160
      tab-width: 8
    misspell:
      locale: US
    wsl:
      strict-append: true
  exclusions:
    paths:
      - third_party$
formatters:
  enable:
    - goimports
`

// get walks a decoded YAML tree by key, returning nil when the path is absent.
func get(t *testing.T, doc []byte, path ...string) any {
	t.Helper()

	var tree any
	if err := yaml.Unmarshal(doc, &tree); err != nil {
		t.Fatalf("unmarshal result: %v\n%s", err, doc)
	}

	cur := tree

	for _, key := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}

		cur, ok = m[key]
		if !ok {
			return nil
		}
	}

	return cur
}

// strs reads a YAML path expected to hold a list of names.
func strs(t *testing.T, doc []byte, path ...string) []string {
	t.Helper()

	raw := get(t, doc, path...)
	if raw == nil {
		return nil
	}

	items, ok := raw.([]any)
	if !ok {
		t.Fatalf("%s is not a list: %#v", strings.Join(path, "."), raw)
	}

	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.(string))
	}

	return out
}

func mustMerge(t *testing.T, base, overrides string) []byte {
	t.Helper()

	got, err := lintconfig.Merge([]byte(base), []byte(overrides))
	if err != nil {
		t.Fatalf("Merge() error = %v", err)
	}

	return got
}

// Rule 9 of the request: with no override layer the result is the generator's
// own output, byte for byte. Merge is never even asked to reformat.
func TestMergeNoOverridesIsByteIdentical(t *testing.T) {
	for name, overrides := range map[string]string{
		"empty":            "",
		"whitespace":       "   \n\n\t\n",
		"comments only":    "# nothing to override here\n",
		"explicit nothing": "--- {}\n",
	} {
		t.Run(name, func(t *testing.T) {
			got, err := lintconfig.Merge([]byte(generatedBase), []byte(overrides))
			if err != nil {
				t.Fatalf("Merge() error = %v", err)
			}

			if name == "explicit nothing" {
				// An explicit empty mapping is a real document, so the result is
				// re-encoded; it just must not change anything semantically.
				if len(strs(t, got, "linters", "enable")) != 4 {
					t.Errorf("empty mapping override changed the linter set:\n%s", got)
				}

				return
			}

			if string(got) != generatedBase {
				t.Errorf("Merge() reformatted the config without overrides:\n%s", got)
			}
		})
	}
}

// Rule 1: mappings merge recursively, so a project declaring one setting of a
// linter keeps the generator's other settings for it. This is the case the
// request is actually about — misspell.ignore-rules next to locale: US.
//
//nolint:misspell // "cancelled" is the literal under test
func TestMergeDeepMergesSettings(t *testing.T) {
	got := mustMerge(t, generatedBase, `
linters:
  settings:
    misspell:
      ignore-rules:
        - cancelled
`)

	if locale := get(t, got, "linters", "settings", "misspell", "locale"); locale != "US" {
		t.Errorf("generator's misspell.locale lost: %#v", locale)
	}

	if rules := strs(t, got, "linters", "settings", "misspell", "ignore-rules"); len(rules) != 1 || rules[0] != "cancelled" {
		t.Errorf("misspell.ignore-rules = %v, want the project's single entry", rules)
	}

	// Untouched neighbors survive.
	if th := get(t, got, "linters", "settings", "dupl", "threshold"); th != 150 {
		t.Errorf("dupl.threshold = %#v, want 150", th)
	}
}

// Rule 2: scalars and sequences replace rather than merge.
func TestMergeReplacesScalarsAndSequences(t *testing.T) {
	got := mustMerge(t, generatedBase, `
linters:
  settings:
    lll:
      line-length: 200
  exclusions:
    paths:
      - vendored$
`)

	if l := get(t, got, "linters", "settings", "lll", "line-length"); l != 200 {
		t.Errorf("lll.line-length = %#v, want 200", l)
	}

	if tw := get(t, got, "linters", "settings", "lll", "tab-width"); tw != 8 {
		t.Errorf("lll.tab-width = %#v, want the generator's 8 (sibling keys merge)", tw)
	}

	if paths := strs(t, got, "linters", "exclusions", "paths"); len(paths) != 1 || paths[0] != "vendored$" {
		t.Errorf("exclusions.paths = %v, want the project's list to replace the generator's", paths)
	}
}

// Rule 3: an explicit null drops the key, in all three YAML spellings.
func TestMergeNullDeletesKey(t *testing.T) {
	for name, overrides := range map[string]string{
		"bare":    "linters:\n  settings:\n    wsl:\n",
		"tilde":   "linters:\n  settings:\n    wsl: ~\n",
		"keyword": "linters:\n  settings:\n    wsl: null\n",
	} {
		t.Run(name, func(t *testing.T) {
			got := mustMerge(t, generatedBase, overrides)

			if wsl := get(t, got, "linters", "settings", "wsl"); wsl != nil {
				t.Errorf("wsl settings survived a null override: %#v", wsl)
			}

			// Deleting the settings must not delete the linter itself.
			if enable := strs(t, got, "linters", "enable"); !contains(enable, "wsl") {
				t.Errorf("linters.enable = %v, want wsl still enabled", enable)
			}
		})
	}
}

// Rule 4: enable adds, disable removes — and a linter the generator enables
// explicitly is dropped from enable rather than listed under disable, because
// golangci-lint rejects a config that does both.
func TestMergeLinterSetOperations(t *testing.T) {
	got := mustMerge(t, generatedBase, `
linters:
  enable:
    - gochecknoinits
  disable:
    - dupl
`)

	enable := strs(t, got, "linters", "enable")

	if contains(enable, "dupl") {
		t.Errorf("linters.enable = %v, want dupl removed", enable)
	}

	if !contains(enable, "gochecknoinits") {
		t.Errorf("linters.enable = %v, want gochecknoinits added", enable)
	}

	if !contains(enable, "lll") || !contains(enable, "misspell") || !contains(enable, "wsl") {
		t.Errorf("linters.enable = %v, want the untouched generator linters kept", enable)
	}

	// dupl was explicitly enabled, so subtracting it is enough: emitting a
	// disable entry too would make golangci-lint reject the config.
	if disable := strs(t, got, "linters", "disable"); len(disable) != 0 {
		t.Errorf("linters.disable = %v, want empty", disable)
	}

	// The enable list stays sorted, so regenerating is deterministic and the
	// diff against the generator's alphabetical list stays readable.
	if !sorted(enable) {
		t.Errorf("linters.enable = %v, want sorted", enable)
	}
}

// A disabled name the generator never enabled explicitly comes from
// golangci-lint's own default set (linters.default: standard), so subtracting
// it from enable would be a no-op. It has to reach golangci-lint as a real
// disable entry — this is the case a naive "remove from enable" merge loses.
func TestMergeDisablePassesThroughDefaultLinters(t *testing.T) {
	got := mustMerge(t, generatedBase, `
linters:
  disable:
    - errcheck
    - dupl
`)

	if disable := strs(t, got, "linters", "disable"); len(disable) != 1 || disable[0] != "errcheck" {
		t.Errorf("linters.disable = %v, want [errcheck] only", disable)
	}

	if enable := strs(t, got, "linters", "enable"); contains(enable, "dupl") || contains(enable, "errcheck") {
		t.Errorf("linters.enable = %v, want neither dupl nor errcheck", enable)
	}
}

// Rule 4 applies to formatters too — a project must be able to switch the
// generator's formatter off.
func TestMergeFormatterSetOperations(t *testing.T) {
	got := mustMerge(t, generatedBase, `
formatters:
  enable:
    - gofumpt
  disable:
    - goimports
`)

	enable := strs(t, got, "formatters", "enable")
	if len(enable) != 1 || enable[0] != "gofumpt" {
		t.Errorf("formatters.enable = %v, want [gofumpt]", enable)
	}

	if disable := strs(t, got, "formatters", "disable"); len(disable) != 0 {
		t.Errorf("formatters.disable = %v, want empty", disable)
	}
}

// A section the generator does not emit at all is added wholesale, including
// its enable list.
func TestMergeAddsMissingSection(t *testing.T) {
	got := mustMerge(t, "version: \"2\"\n", `
linters:
  enable:
    - misspell
run:
  concurrency: 8
`)

	if enable := strs(t, got, "linters", "enable"); len(enable) != 1 || enable[0] != "misspell" {
		t.Errorf("linters.enable = %v, want [misspell]", enable)
	}

	if c := get(t, got, "run", "concurrency"); c != 8 {
		t.Errorf("run.concurrency = %#v, want 8", c)
	}
}

// Comments in the generated config are documentation of the generator's
// intent; the yaml.Node round-trip has to keep them.
func TestMergePreservesComments(t *testing.T) {
	got := mustMerge(t, generatedBase, "linters:\n  settings:\n    lll:\n      line-length: 200\n")

	if !strings.Contains(string(got), "Keeping a comment here") {
		t.Errorf("comment lost in the merged config:\n%s", got)
	}
}

func TestMergeErrors(t *testing.T) {
	tests := map[string]struct {
		base      string
		overrides string
		wantIn    string
	}{
		"overrides not a mapping": {
			base:      generatedBase,
			overrides: "- misspell\n",
			wantIn:    "expected a mapping at the top level",
		},
		"overrides unparseable": {
			base:      generatedBase,
			overrides: "linters: [\n",
			wantIn:    "parse lint overrides",
		},
		"enable not a list": {
			base:      generatedBase,
			overrides: "linters:\n  enable: misspell\n",
			wantIn:    "linters.enable must be a list of names",
		},
		"disable nested list": {
			base:      generatedBase,
			overrides: "linters:\n  disable:\n    - [misspell]\n",
			wantIn:    "linters.disable must be a list of names",
		},
		"base unparseable": {
			base:      "linters: [\n",
			overrides: "run:\n  concurrency: 8\n",
			wantIn:    "parse generated lint config",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := lintconfig.Merge([]byte(tt.base), []byte(tt.overrides))
			if err == nil {
				t.Fatalf("Merge() error = nil, want one containing %q", tt.wantIn)
			}

			if !strings.Contains(err.Error(), tt.wantIn) {
				t.Errorf("Merge() error = %q, want it to contain %q", err, tt.wantIn)
			}
		})
	}
}

// Validate is the config-load gate: a malformed override document must be
// rejected before the generator writes anything.
func TestValidate(t *testing.T) {
	tests := map[string]struct {
		overrides string
		wantErr   bool
	}{
		"empty":        {overrides: "", wantErr: false},
		"comment only": {overrides: "# nothing\n", wantErr: false},
		"mapping":      {overrides: "linters:\n  disable:\n    - dupl\n", wantErr: false},
		"sequence":     {overrides: "- dupl\n", wantErr: true},
		"scalar":       {overrides: "nope\n", wantErr: true},
		"broken yaml":  {overrides: "linters: [\n", wantErr: true},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			err := lintconfig.Validate([]byte(tt.overrides))
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// The merged file is written on every regeneration and lands in meta.yaml as
// a hash, so the same inputs have to produce the same bytes — set operations
// run over Go maps and must not leak iteration order into the output.
//
//nolint:misspell // "cancelled" is the literal under test
func TestMergeIsDeterministic(t *testing.T) {
	overrides := `
linters:
  enable:
    - gochecknoinits
    - gochecknoglobals
  disable:
    - dupl
    - errcheck
    - govet
  settings:
    misspell:
      ignore-rules:
        - cancelled
`

	want := mustMerge(t, generatedBase, overrides)

	for i := 0; i < 20; i++ {
		if got := mustMerge(t, generatedBase, overrides); string(got) != string(want) {
			t.Fatalf("run %d produced different bytes:\n--- want ---\n%s\n--- got ---\n%s", i, want, got)
		}
	}
}

// Today's template emits no linters.disable, but if it grows one its entries
// must not be dropped by the set-merge — except for the ones the project
// enables back, where the project's explicit enable wins.
func TestMergeKeepsGeneratorDisableList(t *testing.T) {
	base := `version: "2"
linters:
  enable:
    - misspell
  disable:
    - govet
    - unused
`

	got := mustMerge(t, base, `
linters:
  enable:
    - unused
  disable:
    - errcheck
`)

	disable := strs(t, got, "linters", "disable")
	if len(disable) != 2 || disable[0] != "errcheck" || disable[1] != "govet" {
		t.Errorf("linters.disable = %v, want [errcheck govet]", disable)
	}

	if enable := strs(t, got, "linters", "enable"); !contains(enable, "unused") {
		t.Errorf("linters.enable = %v, want unused enabled back by the project", enable)
	}
}

func contains(haystack []string, needle string) bool {
	for _, v := range haystack {
		if v == needle {
			return true
		}
	}

	return false
}

func sorted(values []string) bool {
	for i := 1; i < len(values); i++ {
		if values[i-1] > values[i] {
			return false
		}
	}

	return true
}
