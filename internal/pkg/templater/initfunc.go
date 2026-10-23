package templater

import (
	"io/fs"
	"regexp"
	"sort"
	"strings"

	"github.com/pkg/errors"
)

// InitFuncOccurrence is one place a template mentions `func init(`.
type InitFuncOccurrence struct {
	// Template is the path relative to the templates root.
	Template string
	// Line is the 1-indexed line number within the template.
	Line int
	// Text is the matching line, trimmed.
	Text string
}

// initFuncRx matches a Go init function declaration.
//
// Deliberately not anchored to the start of a line and not limited to the Go
// grammar: the declaration this guard exists for lived inside a raw string
// literal (the "create your init.go" instructions a generated test helper
// panicked with), indented by four spaces. A pattern that only matched real
// top-level declarations would have let the generator keep handing every
// project a worked example of the thing the guard forbids.
//
// `func init(x int)` is not a legal init function, but the Go compiler rejects
// it too, so matching it costs nothing.
var initFuncRx = regexp.MustCompile(`func\s+init\s*\(`)

// InitFuncOccurrences returns every `func init(` in the embedded templates.
//
// golangci-lint never sees a `.tmpl` file, so `gochecknoinits` — enabled both in
// this repository's config and in the one generated projects get — cannot say
// anything about the templates themselves. Without this scan an init function
// reintroduced into a template would ship to every generated project and only
// surface, if at all, in a user's lint run.
//
// Every template is scanned, not just `*.go.tmpl`: a Markdown template teaching
// the pattern (LLMS.md, docs) hands projects the same thing the linter then
// rejects.
func InitFuncOccurrences() ([]InitFuncOccurrence, error) {
	var found []InitFuncOccurrence

	err := fs.WalkDir(templates, templatesRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		data, rerr := templates.ReadFile(path)
		if rerr != nil {
			return rerr
		}

		relative := templateRelPath(path)

		for i, line := range strings.Split(string(data), "\n") {
			if initFuncRx.MatchString(line) {
				found = append(found, InitFuncOccurrence{
					Template: relative,
					Line:     i + 1,
					Text:     strings.TrimSpace(line),
				})
			}
		}

		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "error while scanning templates for init functions")
	}

	sort.Slice(found, func(i, j int) bool {
		if found[i].Template != found[j].Template {
			return found[i].Template < found[j].Template
		}

		return found[i].Line < found[j].Line
	})

	return found, nil
}
