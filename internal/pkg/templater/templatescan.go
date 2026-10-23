package templater

import (
	"io/fs"
)

// walkTemplates hands every embedded template to fn as (path relative to the
// templates root, file body).
//
// Shared by the guards that answer for the templates themselves —
// InitFuncOccurrences and GlobalVarOccurrences — because what they have in
// common is exactly the part that must not drift: which files count as
// templates. Both report nothing in the healthy state, so a walk that quietly
// stopped covering part of the tree is indistinguishable from a clean tree.
// With one enumeration, a change to the embed pattern or to the root moves both
// guards at once.
//
// Every template is visited, not just `*.go.tmpl`: a Markdown template teaching
// a forbidden pattern (LLMS.md, docs) hands projects the same thing the linter
// then rejects.
func walkTemplates(fn func(relative, body string) error) error {
	return fs.WalkDir(templates, templatesRoot, func(path string, d fs.DirEntry, err error) error {
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

		return fn(templateRelPath(path), string(data))
	})
}
