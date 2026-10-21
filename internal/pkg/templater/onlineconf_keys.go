package templater

import (
	"io/fs"
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

// onlineConfReadMarkers are the calls that mean "this template's generated code
// reads a key". Deliberately not every mention of the package: the generated
// mains call Initialize, StartWatcher, StopWatcher and FromContext, which
// manage the instance rather than read any key, and demanding a declaration for
// them would make the rule mean nothing.
var onlineConfReadMarkers = []string{
	"onlineconf.MakePath(",
	"onlineconf.Get",
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

// OnlineConfReadingTemplates returns the Go templates whose generated code reads
// an OnlineConf key, relative to the templates root.
//
// It is what ties a declaration to a read: every template listed here must carry
// a declaration file, and TestOnlineConfKeyDeclarations fails when one does not.
// Without that check the declarations would be just another hand-maintained
// list, and a read added to a template would fall out of the surface silently.
func OnlineConfReadingTemplates() ([]string, error) {
	var reading []string

	err := fs.WalkDir(templates, templatesRoot, func(path string, d fs.DirEntry, err error) error {
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

		if readsOnlineConf(string(data)) {
			reading = append(reading, relative)
		}

		return nil
	})

	if err != nil {
		return nil, errors.Wrap(err, "error while scanning templates for onlineconf reads")
	}

	sort.Strings(reading)

	return reading, nil
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

// readsOnlineConf reports whether a template's generated code reads a key.
func readsOnlineConf(source string) bool {
	for _, marker := range onlineConfReadMarkers {
		if strings.Contains(source, marker) {
			return true
		}
	}

	return false
}

// templateRelPath strips the embed prefix from a template path.
func templateRelPath(path string) string {
	return strings.TrimPrefix(path, templatesRoot+"/")
}
