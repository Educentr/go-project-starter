package templater

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/Educentr/go-project-starter/internal/pkg/ds"
	"github.com/Educentr/go-project-starter/internal/pkg/ockeymap"
)

// TestNoTemplateReadsOnlineConfDirectly is the guard of the generator repository
// that mirrors the one the generated project carries.
//
// A generated project may call the go-onlineconf library from exactly one place,
// the generated accessor package, and `make onlineconf-validate` enforces it. So
// a template that called the library itself would emit code the project's own
// gate rejects — and the place to find that out is here, not in a user's build.
func TestNoTemplateReadsOnlineConfDirectly(t *testing.T) {
	users, err := OnlineConfLibraryUsers()
	if err != nil {
		t.Fatalf("OnlineConfLibraryUsers() error: %v", err)
	}

	for _, template := range users {
		t.Errorf("%s uses the onlineconf library directly; read through the generated %s package "+
			"instead, and declare the key in %s%s", template, ds.OnlineConfPackage,
			template, ockeymap.GenDeclSuffix)
	}
}

// TestOnlineConfDeclarationsMatchAccessorCalls is the check that keeps the
// generator's own key declarations honest.
//
// Declaring the keys generated code reads is only worth anything if the
// declaration cannot fall behind the code. The first answer — a hand-kept list of
// reserved path prefixes — looked complete and was not: subtrees the generated
// code genuinely reads (`worker/<name>`, `settings`, `bot`) were simply missing,
// and nothing would ever have said so.
//
// The link is now mechanical in both directions. Generated code reads a key by
// calling the accessor of that key, and an accessor exists because some
// declaration asks for it — so an accessor a template calls must be one the
// template itself declares, and an accessor a template declares must be one it
// calls. A read added without a declaration fails to resolve a name; a read
// removed without its declaration leaves an accessor nobody calls.
func TestOnlineConfDeclarationsMatchAccessorCalls(t *testing.T) {
	calls, err := OnlineConfAccessorCalls()
	if err != nil {
		t.Fatalf("OnlineConfAccessorCalls() error: %v", err)
	}

	if len(calls) == 0 {
		t.Fatal("no template calls the accessor package: the scan is broken, not the templates")
	}

	decls, err := LoadOnlineConfKeyDecls()
	if err != nil {
		t.Fatalf("LoadOnlineConfKeyDecls() error: %v", err)
	}

	declared := make(map[string][]string, len(decls))
	for _, decl := range decls {
		declared[decl.Owner] = declaredAccessorNames(decl.Decl)
	}

	for template, names := range calls {
		own := declared[template]

		for _, name := range names {
			if declares(own, name) {
				continue
			}

			t.Errorf("%s calls %s.%s, which it does not declare: add the key to %s%s "+
				"(declared there: %s)", template, ds.OnlineConfPackage, name,
				template, ockeymap.GenDeclSuffix, strings.Join(own, ", "))
		}
	}

	for _, decl := range decls {
		for _, name := range declared[decl.Owner] {
			if isCalled(calls[decl.Owner], name) {
				continue
			}

			t.Errorf("%s%s declares %s, but %s never calls it or its path builder: a declaration "+
				"without a read keeps a path claimed against the service map for nothing",
				decl.Owner, ockeymap.GenDeclSuffix, name, decl.Owner)
		}
	}
}

// declares reports whether a declared name covers the called one. A declaration
// of a value key yields two functions — the accessor and the builder of its path,
// which a subscription needs — so a call of `<Name>Path` is a call of `<Name>`'s
// declaration.
func declares(own []string, called string) bool {
	return slices.Contains(own, called) ||
		slices.Contains(own, strings.TrimSuffix(called, "Path"))
}

// isCalled reports whether a declaration is used, by its accessor or by its path
// builder. Reading the path and reading the value are both reads of the key.
func isCalled(calls []string, declared string) bool {
	return slices.Contains(calls, declared) || slices.Contains(calls, declared+"Path")
}

// TestOnlineConfDeclarationsParse pins that every shipped declaration is valid:
// a defect here would surface as a failed generation in a user's project.
func TestOnlineConfDeclarationsParse(t *testing.T) {
	decls, err := LoadOnlineConfKeyDecls()
	if err != nil {
		t.Fatalf("LoadOnlineConfKeyDecls() error: %v", err)
	}

	if len(decls) == 0 {
		t.Fatal("no declarations loaded: generated code does read OnlineConf")
	}

	for i := 1; i < len(decls); i++ {
		if decls[i-1].Owner >= decls[i].Owner {
			t.Fatalf("LoadOnlineConfKeyDecls() is not sorted by owner: %q before %q",
				decls[i-1].Owner, decls[i].Owner)
		}
	}
}

// TestLoggerDeclarationsAgree pins the one declaration pair that is duplicated
// on purpose: the two logger implementations read the same keys, and a project
// generates one of them, so both must declare the same set. Nothing else would
// notice a divergence — the registry of a zerolog project would simply stop
// listing a key the logrus declaration still claims.
func TestLoggerDeclarationsAgree(t *testing.T) {
	decls, err := LoadOnlineConfKeyDecls()
	if err != nil {
		t.Fatalf("LoadOnlineConfKeyDecls() error: %v", err)
	}

	byOwner := make(map[string]*ockeymap.GenDecl, len(decls))
	for _, decl := range decls {
		byOwner[decl.Owner] = decl.Decl
	}

	zlog, okZlog := byOwner["logger/zlog/zlog.go.tmpl"]
	logrus, okLogrus := byOwner["logger/logrus/logrus.go.tmpl"]

	if !okZlog || !okLogrus {
		t.Fatal("both logger templates should carry a declaration")
	}

	zlogKeys := declaredKeySet(zlog)
	logrusKeys := declaredKeySet(logrus)

	for key := range zlogKeys {
		if _, ok := logrusKeys[key]; !ok {
			t.Errorf("zlog declares %s, logrus does not", key)
		}
	}

	for key := range logrusKeys {
		if _, ok := zlogKeys[key]; !ok {
			t.Errorf("logrus declares %s, zlog does not", key)
		}
	}
}

// TestOnlineConfDeclarationsAreNotCopiedIntoProjects pins that a declaration
// file stays generator input. It lives inside the templates tree, and the tree
// is walked wholesale — so without the skip in GetTemplates it would land in
// every generated project as a stray yaml file.
func TestOnlineConfDeclarationsAreNotCopiedIntoProjects(t *testing.T) {
	_, files, err := GetMainTemplates(GeneratorParams{})
	if err != nil {
		t.Fatalf("GetMainTemplates() error: %v", err)
	}

	for _, file := range files {
		if strings.HasSuffix(file.SourceName, ockeymap.GenDeclSuffix) {
			t.Errorf("%s would be copied into the generated project as %s", file.SourceName, file.DestName)
		}
	}
}

// declaredAccessorNames returns the accessor identifiers a declaration asks the
// generator for, sorted. An entry read through a shared accessor contributes
// none: it declares a concrete path for the registry, and the reader already has
// its accessor.
func declaredAccessorNames(decl *ockeymap.GenDecl) []string {
	var out []string

	for _, group := range decl.Groups {
		for _, key := range group.Keys {
			if key.Name == "" || slices.Contains(out, key.Name) {
				continue
			}

			out = append(out, key.Name)
		}
	}

	sort.Strings(out)

	return out
}

// declaredKeySet returns one line per declared key, carrying everything a
// divergence could hide in — including the prose, because the registry reports
// the default and the description of whichever declaration the expansion saw
// first, so a value changed in one file only would silently win.
func declaredKeySet(decl *ockeymap.GenDecl) map[string]struct{} {
	out := make(map[string]struct{})

	for _, group := range decl.Groups {
		for _, key := range group.Keys {
			out[fmt.Sprintf("%s %s name=%s type=%s required=%t shared=%t read_via=%s default=%s description=%s",
				group.Scope, key.Path, key.Name, key.Type,
				key.Required, key.Shared, key.ReadVia,
				key.Default, strings.Join(strings.Fields(key.Description), " "))] = struct{}{}
		}
	}

	return out
}
