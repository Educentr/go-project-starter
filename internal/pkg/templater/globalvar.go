package templater

import (
	"regexp"
	"sort"
	"strings"

	"github.com/pkg/errors"
)

// GlobalVarOccurrence is one package-level `var` in a template that
// `gochecknoglobals` would reject.
type GlobalVarOccurrence struct {
	// Template is the path relative to the templates root.
	Template string
	// Line is the 1-indexed line number within the template.
	Line int
	// Text is the matching line, trimmed.
	Text string
}

// varKeyword is the keyword both shapes this scan reads start with: `var name =
// …` and `var (`.
const varKeyword = "var"

// nolintGlobalsRx matches a nolint directive that covers gochecknoglobals.
//
// The directive is the reviewed escape hatch: what stays package-level — a
// linker-filled ldflags variable, a Prometheus collector, a table generated in
// full from a declaration — carries one with its reason, and this scan honors
// it exactly as golangci-lint does. The list is comma-separated and the name may
// sit anywhere in it, hence the alternation around the word.
var nolintGlobalsRx = regexp.MustCompile(`//\s*nolint:[\w,]*\bgochecknoglobals\b`)

// goIdentRx matches a complete Go identifier, used to reject a name this scan
// cannot read (a template action in the name position).
var goIdentRx = regexp.MustCompile(`^[A-Za-z_]\w*$`)

// errorValueRx matches a value that constructs an error.
//
// The linter accepts an `err`/`Err`-prefixed name only if its type really
// implements error. A template has no types, so the stand-in is the value — and
// it is a stand-in worth having rather than trusting the prefix alone:
// `ErrorTimeout`, one of the declarations this task removed, is a
// `time.Duration` that the prefix rule would have waved straight through.
//
// The consequence is that a sentinel built some other way (`var ErrX =
// &myError{}`) is reported and needs a directive. That is the safe direction for
// a guard, and every sentinel in the templates is an errors.New.
var errorValueRx = regexp.MustCompile(`\b(errors\.(New|Wrap|Wrapf|Join)|fmt\.Errorf)\(`)

// GlobalVarOccurrences returns every package-level `var` in the embedded
// templates that is not one of the shapes gochecknoglobals accepts.
//
// golangci-lint never parses a `.tmpl` file, so nothing in the generated
// project's own lint run can speak for the templates — and even in the generated
// tree `exclusions.generated: lax` keeps the linter away from `psg_*_gen.go`
// entirely. Without this scan a mutable global reintroduced into a template
// would ship to every generated project and surface, if ever, in a user's own
// lint run.
//
// The accepted shapes are the ones the linter itself whitelists (see
// 4d63.com/gochecknoglobals): the blank identifier, a name of exactly `version`,
// a name prefixed `err`/`Err`, a `regexp.MustCompile` value and a `//go:embed`
// declaration — plus a `//nolint:gochecknoglobals` directive, which is how a
// justified exception is recorded.
//
// This is a line scan, not a parse — a template is not valid Go, so there is no
// AST to ask. It is deliberately the coarser of the two guards: the precise one
// parses the generated output (see TestNoUnjustifiedGlobalsInGeneratedProjects),
// which is real Go. A declaration whose name is a template action
// (`var {{ $m.VarName }} = ...`) cannot be classified here at all, so it has to
// carry a nolint directive — which is the right outcome: a name the guard cannot
// read is a name a reviewer has to approve.
func GlobalVarOccurrences() ([]GlobalVarOccurrence, error) {
	var found []GlobalVarOccurrence

	err := walkTemplates(func(relative, body string) error {
		found = append(found, scanGlobalVars(relative, body)...)

		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "error while scanning templates for global variables")
	}

	sort.Slice(found, func(i, j int) bool {
		if found[i].Template != found[j].Template {
			return found[i].Template < found[j].Template
		}

		return found[i].Line < found[j].Line
	})

	return found, nil
}

// scanGlobalVars reports the package-level `var` declarations of one template
// that no accepted shape covers.
func scanGlobalVars(template, body string) []GlobalVarOccurrence {
	lines := strings.Split(body, "\n")

	// Not pre-allocated to the line count: the healthy answer is an empty slice,
	// and a template with hundreds of lines declares at most a handful.
	found := make([]GlobalVarOccurrence, 0)

	// entryIndent is the leading whitespace of a block's entries, learned from
	// the first one. Measured rather than assumed to be one tab: these templates
	// are not gofmt'd (the generator formats its output, not its sources), and
	// several indent with spaces. Deeper indentation is a composite literal's
	// insides, which are not declarations.
	inVarBlock, blockExempt, entryIndent := false, false, ""

	for i, line := range lines {
		code := strings.TrimRight(stripLineComment(line), " \t\r")

		if !inVarBlock {
			if isVarBlockOpener(code) {
				// A directive on the opener covers the whole declaration, which
				// is how golangci-lint reads it too.
				inVarBlock, blockExempt, entryIndent = true, hasGlobalsNolint(line, lines, i), ""

				continue
			}

			spec, ok := topLevelVarSpec(code)
			if !ok {
				continue
			}

			if globalVarAllowed(spec, line, lines, i) {
				continue
			}

			found = append(found, occurrence(template, lines, i))

			continue
		}

		if strings.TrimSpace(code) == ")" {
			inVarBlock = false

			continue
		}

		spec, indent, ok := blockEntrySpec(code, entryIndent)
		if !ok {
			continue
		}

		if entryIndent == "" {
			entryIndent = indent
		}

		if blockExempt || globalVarAllowed(spec, line, lines, i) {
			continue
		}

		found = append(found, occurrence(template, lines, i))
	}

	return found
}

func occurrence(template string, lines []string, i int) GlobalVarOccurrence {
	return GlobalVarOccurrence{
		Template: template,
		Line:     i + 1,
		Text:     strings.TrimSpace(lines[i]),
	}
}

// stripLineComment removes a trailing `//` comment so that the structural
// checks see code only. Crude on purpose: a `//` inside a string literal (a URL
// in a default value) would be cut too, but the checks that run on the result
// only ask about leading keywords and a trailing paren, which a truncated
// string literal cannot fake.
func stripLineComment(line string) string {
	if i := strings.Index(line, "//"); i >= 0 {
		return line[:i]
	}

	return line
}

// isVarBlockOpener recognizes a package-level `var (`, including one carrying a
// trailing comment — which stripLineComment has already removed.
//
// Column 0 only, like topLevelVarSpec: a `var (…)` block inside a function body
// is indented, and gochecknoglobals has nothing to say about its entries. The
// ogen error-response template and both logger templates open such a block, so
// trimming the indentation here would report a handful of ordinary local
// variables.
func isVarBlockOpener(code string) bool {
	if !strings.HasPrefix(code, varKeyword) {
		return false
	}

	return strings.TrimSpace(strings.TrimPrefix(code, varKeyword)) == "("
}

// topLevelVarSpec returns the declaration body of a `var` at column 0.
//
// Only column 0 counts as package scope: `var` inside a function is indented,
// and so is the worked example a generated helper panics with (a raw string
// literal showing the project how to register its test environment). That
// example is the shape the init() guard had to be widened for — here the
// opposite holds, because `gochecknoglobals` has nothing to say about a
// variable that is not in package scope.
// A leading template action does not change the scope of what follows it, so it
// is stepped over first: `{{ if .X }}var mutable = …{{ end }}` on one line is a
// package-level declaration.
func topLevelVarSpec(code string) (string, bool) {
	if strings.HasPrefix(code, "{{") {
		if closeIdx := strings.Index(code, "}}"); closeIdx >= 0 {
			code = strings.TrimLeft(code[closeIdx+len("}}"):], " \t")
		}
	}

	// The space matters: without it `variable = 1` would read as a `var`
	// declaration named `iable`.
	if !strings.HasPrefix(code, varKeyword+" ") {
		return "", false
	}

	return strings.TrimPrefix(code, varKeyword+" "), true
}

// blockEntrySpec returns the declaration body of one entry of a `var (…)` block,
// along with the entry's indentation.
//
// What it must not mistake for an entry: a comment (already stripped), a
// template action controlling the block (`{{- range … }}`), and the inside of a
// composite literal belonging to the previous entry. The first two are
// recognized by shape; the third by indentation, since a literal's fields are
// indented deeper than the entry that opens it.
//
// A line it cannot classify is returned as an entry, not skipped. An
// unclassifiable line is then reported and needs a directive — noisy, but a
// guard whose healthy state is "nothing found" must not have a path that
// silently drops declarations.
func blockEntrySpec(code, entryIndent string) (spec, indent string, ok bool) {
	indent = code[:len(code)-len(strings.TrimLeft(code, " \t"))]
	entry := strings.TrimSpace(code)

	if entry == "" {
		return "", "", false
	}

	// Deeper than the entries: inside a composite literal of the entry above.
	if entryIndent != "" && len(indent) > len(entryIndent) {
		return "", "", false
	}

	// A literal's closing brace, or a closing brace plus a comma.
	if strings.HasPrefix(entry, "}") || strings.HasPrefix(entry, ")") {
		return "", "", false
	}

	// A template action is control flow over the block (`{{- range … }}`,
	// `{{- end }}`) unless something is assigned *after* it closes, in which case
	// it is a declaration whose name this scan cannot read — it then falls
	// through and gets reported, because a name the guard cannot read is a name a
	// reviewer has to approve.
	//
	// The `=` has to be looked for past the closing `}}`: `{{- range $_, $c :=
	// .Checks }}` carries one inside the action.
	if strings.HasPrefix(entry, "{{") && !assignsAfterAction(entry) {
		return "", "", false
	}

	return entry, indent, true
}

// assignsAfterAction reports whether a line beginning with a template action
// assigns something once the action closes — `{{ $m.VarName }} = …`, a
// declaration — as opposed to being the action alone.
func assignsAfterAction(entry string) bool {
	closeIdx := strings.Index(entry, "}}")
	if closeIdx < 0 {
		return false
	}

	return strings.Contains(entry[closeIdx+len("}}"):], "=")
}

// globalVarAllowed reports whether a declaration is one of the shapes
// gochecknoglobals accepts, or carries a nolint directive for it.
func globalVarAllowed(spec, line string, lines []string, i int) bool {
	if hasGlobalsNolint(line, lines, i) {
		return true
	}

	if commentAbove(lines, i, "//go:embed ") {
		return true
	}

	// Anchored to the start of the value, not matched anywhere in the line: the
	// linter allows a declaration whose value *is* a regexp.MustCompile call,
	// not a mutable map or slice that happens to hold one.
	if value, found := declValue(spec); found && strings.HasPrefix(value, "regexp.MustCompile(") {
		return true
	}

	names := globalVarNames(spec)
	if len(names) == 0 {
		return false
	}

	// A sentinel error has to look like one in both name and value. The name
	// alone is what the linter checks before type-checking that the type really
	// implements error, and a template has no types — but `ErrorTimeout`, one of
	// the declarations this task removed, is a time.Duration that passes the
	// name rule, so the value has to agree.
	constructsError := errorValueRx.MatchString(spec)

	for _, name := range names {
		if name == "_" || name == "version" {
			continue
		}

		if looksLikeErrorName(name) && constructsError {
			continue
		}

		return false
	}

	return true
}

// declValue returns the text to the right of the first `=`.
func declValue(spec string) (string, bool) {
	i := strings.Index(spec, "=")
	if i < 0 {
		return "", false
	}

	return strings.TrimSpace(stripLineComment(spec[i+1:])), true
}

// hasGlobalsNolint reports a nolint directive for gochecknoglobals on the
// declaration's own line, or on the line above it when that line is a comment.
//
// The "comment above" restriction is the point: a trailing directive belongs to
// the declaration it sits on. Accepting the previous line unconditionally let a
// directive on one entry of a `var (…)` block exempt the next, unrelated entry.
//
// golangci-lint is laxer still — it reads the whole doc comment group — so this
// scan can ask for a directive the linter would already have honored a few
// lines higher. The cost is moving one line; the gain is that a reader of the
// declaration sees the exception next to it. The error message says so.
func hasGlobalsNolint(line string, lines []string, i int) bool {
	if nolintGlobalsRx.MatchString(line) {
		return true
	}

	return commentAbove(lines, i, "//nolint") && nolintGlobalsRx.MatchString(lines[i-1])
}

// commentAbove reports whether the line above i is a comment starting with
// prefix.
func commentAbove(lines []string, i int, prefix string) bool {
	if i == 0 {
		return false
	}

	above := strings.TrimSpace(lines[i-1])

	return strings.HasPrefix(above, "//") && strings.HasPrefix(above, prefix)
}

// globalVarNames splits the declared names out of a var specification:
// everything left of the first `=`, or — for a declaration without a value —
// the names ahead of the type.
//
// Returns nil when the names cannot be read: a template action in the name
// position (`var {{ $m.VarName }} = ...`), for instance. The declaration is then
// reported rather than waved through, so a name the guard cannot read is a name
// a reviewer has to approve with a nolint directive.
func globalVarNames(spec string) []string {
	decl := spec
	if i := strings.Index(decl, "="); i >= 0 {
		decl = decl[:i]
	}

	fields := strings.Fields(decl)
	names := make([]string, 0, len(fields))

	// In `a, b int` every name but the last is comma-terminated, so the first
	// field without a comma ends the name list and whatever follows is the type.
	for _, field := range fields {
		name := strings.TrimSuffix(field, ",")
		if !goIdentRx.MatchString(name) {
			return nil
		}

		names = append(names, name)

		if !strings.HasSuffix(field, ",") {
			break
		}
	}

	return names
}

// looksLikeErrorName mirrors the linter's name rule for sentinel errors: an
// exported name prefixed `Err`, an unexported one prefixed `err`.
//
// On its own the name is not enough — see errorValueRx — so callers pair it with
// a look at the value.
func looksLikeErrorName(name string) bool {
	prefix := "err"
	if name[0] >= 'A' && name[0] <= 'Z' {
		prefix = "Err"
	}

	return strings.HasPrefix(name, prefix)
}
