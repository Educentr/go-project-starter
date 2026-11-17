package ockeymap

import (
	"fmt"
	"go/token"
	"go/types"
	"slices"
	"strings"
)

// occonfIdents is the identifier surface of the generated accessor package: the
// local names of its imports, its package-level declarations, the parameters of
// the functions the template writes itself and the locals its bodies declare
// (templater/embedded/templates/occonf/occonf.go.tmpl).
//
// The name of a runtime segment becomes a parameter of the accessor generated
// for the key, so a name taken here yields a package that does not compile:
// `path := AccountQuotaPath(path)` declares nothing new, the `v` of the read is
// not the string the caller passed, and a shadowed `fmt` has no Errorf. The
// name is rejected rather than the locals renamed, because the imports and the
// package's own functions cannot be renamed out of the way.
//
// The list covers the whole package, not just the identifiers one accessor body
// happens to reference: the name of a runtime segment names the parameter and
// nothing else — the path is composed from the argument — so renaming the
// segment costs the declaration nothing.
//
// assertAccessorsShadowNothing (test/onlineconf_map_test.go, called from
// TestGenerateOnlineConfMap) reads the identifiers back out of a generated
// package and fails when one of them is missing here.
var occonfIdents = []string{
	// Imports the bodies call.
	"context", "errors", "fmt", "onlineconf", "strings", "time",
	// Package-level declarations the bodies reference, and the fields of the
	// startup check.
	"generatedRequired", "path", "read", "requiredCheck", "serviceRequired", "serviceTree",
	// Parameters of the functions the template writes itself. Two of them —
	// ctx and dst — sit in the signature of a generated accessor, where a
	// parameter of the same name is a redeclaration.
	"app", "cb", "configDir", "ctx", "dst", "paths",
	// Locals the bodies declare.
	"check", "checks", "err", "fallback", "ok", "problems", "v",
}

// ReservedParamNames returns every name a runtime segment of a path may not
// carry, because the generated accessor package already uses it. Go keywords and
// the predeclared identifiers are reserved too, and come from the language
// rather than from this list.
//
// Exported for the test that keeps the list and the accessor template in step.
func ReservedParamNames() []string {
	return slices.Clone(occonfIdents)
}

// runtimeParamProblem reports why the name of a runtime segment cannot be a
// parameter of the generated accessor, or returns the empty string when it can.
//
// The map comes from a contract repository, so a name that cannot be an
// identifier has to be refused while the map is read: the alternative is a
// generation that reports `done` and a project that does not build.
func runtimeParamProblem(path, segment string) string {
	name := strings.Trim(segment, "<>")

	var reason string

	switch {
	case token.IsKeyword(name):
		reason = "it is a Go keyword"
	case types.Universe.Lookup(name) != nil:
		// The predeclared identifiers: shadowing one breaks a body that writes
		// it out — the declared default of a `strings` key is a []string
		// literal, and a parameter named `string` leaves it without a type.
		reason = "it is a predeclared Go identifier"
	case slices.Contains(occonfIdents, name):
		reason = "the generated accessor package uses it itself"
	default:
		return ""
	}

	return fmt.Sprintf(
		"path %q has the runtime segment %q, whose name cannot be a parameter of the generated "+
			"accessor: %s. The name names the parameter and nothing else — the path is composed "+
			"from the argument the caller passes — so rename the segment", path, segment, reason)
}
