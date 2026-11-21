package templater

import (
	"io/fs"
	"strings"
	"text/template"

	"github.com/pkg/errors"
)

// partialsPrefix is the embedded directory of shared template fragments.
//
// It sits next to embedded/templates rather than inside it because a fragment
// is generator input, not project output: every GetTemplates walk emits one
// file per template it finds, so a fragment placed under embedded/templates
// would be written into the generated project as a file of its own.
const partialsPrefix = "embedded/partials"

// partialsSuffix is the extension a shared fragment file carries.
const partialsSuffix = ".tmpl"

// sharedPartials returns the concatenated text of every shared fragment file.
//
// The files hold nothing but {{ define }} blocks and comments, so the result
// can be parsed into a template set that already carries a body: text/template
// allows many Parse calls on one template as long as only one of them carries
// text other than space, comments and definitions.
func sharedPartials() (string, error) {
	var defs strings.Builder

	err := fs.WalkDir(templates, partialsPrefix, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() || !strings.HasSuffix(path, partialsSuffix) {
			return nil
		}

		body, err := templates.ReadFile(path)
		if err != nil {
			return err
		}

		defs.Write(body)
		defs.WriteString("\n")

		return nil
	})
	if err != nil {
		return "", errors.Wrapf(err, "error while reading shared partials from `%s`", partialsPrefix)
	}

	return defs.String(), nil
}

// addSharedPartials makes the shared fragments available to tmpl, so that its
// body can render one with {{ template "<name>" $ }}.
//
// Associated templates are resolved when the body executes, so this may be
// called after the body has been parsed; keeping it a separate step leaves the
// line numbers a body parse error reports untouched.
func addSharedPartials(tmpl *template.Template) error {
	defs, err := sharedPartials()
	if err != nil {
		return err
	}

	if _, err := tmpl.Parse(defs); err != nil {
		return errors.Wrap(err, "error parse shared partials")
	}

	return nil
}
