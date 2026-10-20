package ds

import (
	"path/filepath"
	"strings"

	"github.com/Educentr/go-project-starter/internal/pkg/ockeymap"
)

// OnlineConfConfig carries the resolved OnlineConf key declarations into
// templates.
//
// Enabled mirrors the presence of `onlineconf.path` in project.yaml. When it is
// false the whole feature is off and Keys is empty: no accessor package is
// generated, the validator is not wired into the lint config, and no startup
// check is added to the generated main.
type OnlineConfConfig struct {
	Enabled bool
	Keys    []ockeymap.Key
}

const (
	// OnlineConfPackage is the Go package name of the generated accessor
	// package. Deliberately not "onlineconf": the generated package wraps the
	// library of that name, and the two must be distinguishable at a call
	// site.
	OnlineConfPackage = "occonf"

	// onlineConfValidatorMsg is the common tail of every validator message.
	// Kept in one place so the two generated lint configs cannot drift.
	onlineConfValidatorMsg = "declare the key in the OnlineConf key map instead — " +
		"the generated accessor package is the only place OnlineConf is read"
)

// onlineConfDir is the project-relative directory of the generated accessor
// package. It sits under internal/pkg/ because it is generated core logic with
// no config dependency, following the repository's three-layer split.
var onlineConfDir = filepath.Join("internal", "pkg", OnlineConfPackage)

// IsEnabled reports whether an OnlineConf key map is configured.
func (o OnlineConfConfig) IsEnabled() bool {
	return o.Enabled
}

// HasKeys reports whether at least one accessor will be generated.
func (o OnlineConfConfig) HasKeys() bool {
	return len(o.Keys) > 0
}

// Required returns the declarations the startup check verifies.
func (o OnlineConfConfig) Required() []ockeymap.Key {
	out := make([]ockeymap.Key, 0, len(o.Keys))

	for _, key := range o.Keys {
		if key.Required {
			out = append(out, key)
		}
	}

	return out
}

// HasRequired reports whether the generated package gets a CheckRequired body
// with anything in it.
func (o OnlineConfConfig) HasRequired() bool {
	for _, key := range o.Keys {
		if key.Required {
			return true
		}
	}

	return false
}

// NeedsTimeImport reports whether the generated package needs "time" — that is,
// whether any declaration is a duration.
func (o OnlineConfConfig) NeedsTimeImport() bool {
	for _, key := range o.Keys {
		if key.NeedsTimeImport() {
			return true
		}
	}

	return false
}

// TreeRoot returns the service tree every declared key lives in. The map is one
// per service, so there is exactly one.
func (o OnlineConfConfig) TreeRoot() string {
	if len(o.Keys) == 0 {
		return ""
	}

	return o.Keys[0].TreeRoot()
}

// ForbiddenCalls returns the validator's rules: every call into the
// go-onlineconf package is forbidden outside the generated accessor package and
// the generated readers of the generator's own subtrees.
//
// A blanket rule rather than a list of readers, deliberately. The map exists
// because a path assembled at the call site is invisible to every external
// tool; banning only onlineconf.Get* would leave onlineconf.MakePath free to
// build a path for some other reader, and banning only MakePath would leave a
// bare string literal path free. One rule over the package closes both.
func (o OnlineConfConfig) ForbiddenCalls() []ForbidRule {
	return []ForbidRule{
		{
			Pattern: `^onlineconf\.`,
			Msg:     "reading OnlineConf directly is not allowed here: " + onlineConfValidatorMsg,
		},
	}
}

// ExemptPathPattern returns the regexp of the paths the validator does not
// check, as a forbidigo `path:` value.
//
// Two groups are exempt. The generated accessor package is where OnlineConf IS
// read. And every generated Go file (psg_*_gen.go) reads the generator's own
// subtrees — transport, security, kafka, logger, dev-stand — which the service
// map may not declare at all (see ockeymap.ReservedSegments). Those readers are
// generated in full from project.yaml; they are not hand-written escape
// hatches.
func (o OnlineConfConfig) ExemptPathPattern() string {
	return "(" + strings.Join([]string{
		o.GetPackagePath() + "/",
		`psg_[^/]*_gen\.go$`,
	}, "|") + ")"
}

// GetTargetGeneratePath returns the directory of the generated accessor
// package.
func (o OnlineConfConfig) GetTargetGeneratePath(targetDir string) string {
	return filepath.Join(targetDir, onlineConfDir)
}

// GetPackagePath returns the project-relative directory of the generated
// accessor package, with forward slashes — suitable for import paths and for
// lint-config path patterns.
func (o OnlineConfConfig) GetPackagePath() string {
	return filepath.ToSlash(onlineConfDir)
}

// GetPackageName returns the Go package name of the generated accessor package.
func (o OnlineConfConfig) GetPackageName() string {
	return OnlineConfPackage
}
