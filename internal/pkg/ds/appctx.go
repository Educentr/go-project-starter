package ds

import "path/filepath"

// AppCtxConfig carries the generated derived-context helper into the templates:
// the one sanctioned way for project code to get a context that outlives the
// one it was handed without losing what that context carries.
//
// It holds nothing, because there is nothing to configure: the helper is
// generated in every project, with the same single entry point. It is a type
// rather than a set of template functions so that the package name, the
// package path and the entry point are written once — the lint rule that
// forbids a new context root names the helper in its message, and a message
// pointing at a package that does not exist is worse than no message.
type AppCtxConfig struct{}

const (
	// AppCtxPackage is the Go package name of the generated helper.
	AppCtxPackage = "appctx"

	// AppCtxEntryPoint is the one function the helper exports. One, not a
	// family: every project needs the same two things of a derived context —
	// keep the values, drop the parent's cancellation — and an optional
	// timeout is a parameter of that, not a second function.
	AppCtxEntryPoint = "Detach"
)

// appCtxDir is the project-relative directory of the generated helper. It sits
// under internal/pkg/ because it is generated core logic with no config
// dependency, following the repository's three-layer split — the same place the
// generated OnlineConf accessors live.
var appCtxDir = filepath.Join(internalDir, pkgDir, AppCtxPackage) //nolint:gochecknoglobals // filepath.Join of constants; read-only

// GetPackageName returns the Go package name of the generated helper.
func (a AppCtxConfig) GetPackageName() string {
	return AppCtxPackage
}

// GetPackagePath returns the project-relative directory of the generated
// helper, with forward slashes — suitable for import paths and for lint-config
// path patterns.
func (a AppCtxConfig) GetPackagePath() string {
	return filepath.ToSlash(appCtxDir)
}

// GetEntryPoint returns the name of the one function the helper exports.
func (a AppCtxConfig) GetEntryPoint() string {
	return AppCtxEntryPoint
}

// ForbiddenCalls returns the rules that keep a new context root out of
// hand-written code: context.Background and context.TODO.
//
// A context root is where the service's wiring enters the picture — the
// OnlineConf tree, the logger, the metrics registry, the process's own
// cancellation — and a project has exactly one, in the generated main. A
// context built next to it carries none of that: a configuration read
// dereferences the instance that is not there, a log line goes nowhere, a
// goroutine outlives the shutdown it was supposed to honor. Worse is the code
// that handles the first of those — a reader that answers from a default when
// the tree is missing turns a misconfiguration into a plausible answer, found
// weeks later in a consumer. Hence a rule rather than a convention.
//
// The two names are forbidden, not the shapes built from them: a derived
// context is the answer, and `context.With*` over the context the caller was
// given is exactly that. When the work has to outlive its parent, the
// generated helper is the sanctioned way — hence naming it here.
//
// `_test.go` is deliberately NOT exempt. A test gets its context from the test
// harness (`t.Context()`) or from the generated OnlineConf fixture, and a test
// that builds a bare root is the case the rule exists for: it is where a read
// silently answering from a default is least likely to be noticed.
func (a AppCtxConfig) ForbiddenCalls() []ForbidRule {
	return []ForbidRule{
		{
			Pattern: `^context\.(Background|TODO)$`,
			Msg: "new root contexts are not allowed: derive from the context you were given — " +
				a.GetPackagePath() + "." + AppCtxEntryPoint +
				" keeps OnlineConf, the logger and the rest while outliving the parent's cancellation, " +
				"and a test takes its context from t.Context() or from the generated occonftest fixture; " +
				"the only root is the generated main",
		},
	}
}
