package templater

import (
	"io/fs"
	"regexp"
	"sort"
	"strings"

	"github.com/pkg/errors"

	"github.com/Educentr/go-project-starter/internal/pkg/ockeymap"
)

// templatesRoot is the embed prefix every template path is relative to. The
// paths handed around below are relative to it, so that a declaration names its
// owner the way a developer would: transport/rest/files/.../config_oc.go.tmpl.
const templatesRoot = "embedded/templates"

// occonfTemplateDir holds the generated accessor package — the one place
// OnlineConf is meant to be read from, so its template declares nothing.
const occonfTemplateDir = "occonf"

// onlineConfPackageRx matches any use of the go-onlineconf package. Outside the
// accessor package's own template that is a defect: the generated project's
// validator forbids it, so a template that did it would emit code `make
// onlineconf-validate` rejects.
var onlineConfPackageRx = regexp.MustCompile(`\bonlineconf\.[A-Z]`)

// occonfCallRx matches a call into the generated accessor package in a template.
// The identifier is what the test checks against the declarations: an accessor a
// template calls has to be one the template declares.
var occonfCallRx = regexp.MustCompile(`\bocconf\.([A-Z][A-Za-z0-9]*)`)

// occonfFacade are the exported names of the accessor package that come from the
// package itself rather than from a declaration: the library's lifecycle, the
// subscription helper, the startup check and its sentinel error. A template may
// use them without declaring anything — they read no particular key.
var occonfFacade = map[string]struct{}{ //nolint:gochecknoglobals // fixed lookup table, written as a literal and read-only afterwards
	"Initialize": {}, "StartWatcher": {}, "StopWatcher": {}, "Instance": {},
	"Subscribe": {}, "CheckRequired": {}, "Paths": {}, "ErrKeyNotSet": {},
}

// LoadOnlineConfKeyDecls reads every OnlineConf key declaration shipped with the
// generator — one file per template whose generated code reads a key, sitting
// next to that template.
//
// The result is the generator's half of the key surface: expanded against a
// concrete project by ockeymap.ExpandGenDecls, it says which paths generated
// code reads, so that the service map cannot declare one of them and the owner
// of the contract can see the whole tree.
func LoadOnlineConfKeyDecls() ([]ockeymap.GenDeclFile, error) {
	var decls []ockeymap.GenDeclFile

	err := fs.WalkDir(templates, templatesRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() || !strings.HasSuffix(path, ockeymap.GenDeclSuffix) {
			return nil
		}

		data, rerr := templates.ReadFile(path)
		if rerr != nil {
			return rerr
		}

		relative := templateRelPath(path)

		decl, perr := ockeymap.ParseGenDecl(data, relative)
		if perr != nil {
			return perr
		}

		decls = append(decls, ockeymap.GenDeclFile{
			Owner: strings.TrimSuffix(relative, ockeymap.GenDeclSuffix),
			Decl:  decl,
		})

		return nil
	})

	if err != nil {
		return nil, errors.Wrap(err, "error while loading onlineconf key declarations")
	}

	sort.Slice(decls, func(i, j int) bool { return decls[i].Owner < decls[j].Owner })

	return decls, nil
}

// OnlineConfLibraryUsers returns the Go templates outside the accessor package
// that use the go-onlineconf library directly, relative to the templates root.
//
// The answer has to stay empty. The generated project's validator forbids every
// call into the library outside the generated accessor package, so a template
// that made one would emit code that cannot pass `make onlineconf-validate` —
// and the place to find that out is this repository's test suite, not a user's
// project.
func OnlineConfLibraryUsers() ([]string, error) {
	var users []string

	err := walkGoTemplates(func(relative, source string) error {
		if onlineConfPackageRx.MatchString(source) {
			users = append(users, relative)
		}

		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "error while scanning templates for onlineconf use")
	}

	sort.Strings(users)

	return users, nil
}

// OnlineConfAccessorCalls returns, per Go template outside the accessor package,
// the names of the accessor package's functions its generated code calls.
//
// It is what ties a declaration to a read, in both directions. An accessor a
// template calls has to be one the template declares, so that the declaration
// sits in the file a developer has open while adding the read; and an accessor a
// template declares has to be one it calls, so that a read removed from a
// template takes its declaration with it. The facade names (see occonfFacade)
// are excluded: they read no particular key.
func OnlineConfAccessorCalls() (map[string][]string, error) {
	calls := map[string][]string{}

	err := walkGoTemplates(func(relative, source string) error {
		seen := map[string]struct{}{}

		var names []string

		for _, match := range occonfCallRx.FindAllStringSubmatch(source, -1) {
			name := match[1]

			if _, facade := occonfFacade[name]; facade {
				continue
			}

			if _, dup := seen[name]; dup {
				continue
			}

			seen[name] = struct{}{}

			names = append(names, name)
		}

		if len(names) > 0 {
			sort.Strings(names)
			calls[relative] = names
		}

		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "error while scanning templates for accessor calls")
	}

	return calls, nil
}

// walkGoTemplates calls visit for every Go template outside the accessor
// package's own directory, with its path relative to the templates root and its
// source.
func walkGoTemplates(visit func(relative, source string) error) error {
	return fs.WalkDir(templates, templatesRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() || !strings.HasSuffix(path, ".go.tmpl") {
			return nil
		}

		relative := templateRelPath(path)

		if strings.HasPrefix(relative, occonfTemplateDir+"/") {
			return nil
		}

		data, rerr := templates.ReadFile(path)
		if rerr != nil {
			return rerr
		}

		return visit(relative, string(data))
	})
}

// ReadTemplateSource returns the raw text of a template, relative to the
// templates root.
func ReadTemplateSource(relative string) (string, error) {
	data, err := templates.ReadFile(embedJoin(templatesRoot, relative))
	if err != nil {
		return "", errors.Wrapf(err, "error while reading template %s", relative)
	}

	return string(data), nil
}

// templateRelPath strips the embed prefix from a template path.
func templateRelPath(path string) string {
	return strings.TrimPrefix(path, templatesRoot+"/")
}
