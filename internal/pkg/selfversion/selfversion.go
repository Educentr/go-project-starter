// Package selfversion reports the version of the running go-project-starter
// binary.
//
// The value is provenance: it is pinned into the generated tree (GPSVERSION in
// the Makefile) so a project records which generator produced it and can call
// exactly that one again. Because the pin is fed to `go run <pkg>@<version>`,
// only a version the module proxy can resolve is usable — a `git describe`
// string of an untagged build names no published version and is reported as
// unknown instead.
package selfversion

import (
	"regexp"
	"runtime/debug"
	"strings"

	"golang.org/x/mod/semver"
)

// dirtySuffix is what `git describe --dirty` appends when the working tree has
// uncommitted changes. Such a build matches no published version.
const dirtySuffix = "-dirty"

// describeSuffixRe matches the "-<commits>-g<hash>" tail `git describe` adds to
// the closest tag when HEAD is not that tag. A module pseudo-version
// ("v0.0.0-<timestamp>-<hash>") carries no "g" prefix on its hash and is left
// alone — the proxy serves it.
var describeSuffixRe = regexp.MustCompile(`-[0-9]+-g[0-9a-f]{7,}$`)

// Resolve returns the version of the running binary, or an empty string when it
// cannot be determined (a local build from an untagged or dirty tree).
//
// ldflagsVersion is the value stamped at link time (-X 'main.version=…').
// It wins when it names a published version; otherwise the module version
// recorded in the binary is used, which is what `go install`/`go run` of a
// pinned version writes and link-time stamping does not.
func Resolve(ldflagsVersion string) string {
	return resolve(ldflagsVersion, debug.ReadBuildInfo)
}

// resolve is Resolve with the build info reader injected for tests.
func resolve(ldflagsVersion string, readBuildInfo func() (*debug.BuildInfo, bool)) string {
	if v := published(ldflagsVersion); v != "" {
		return v
	}

	if info, ok := readBuildInfo(); ok && info != nil {
		return published(info.Main.Version)
	}

	return ""
}

// published returns v when it names a version `go run <pkg>@<v>` can resolve,
// and an empty string otherwise.
//
// Rejected: the "dev"/"(devel)" placeholders of an unstamped build, anything
// that is not a semantic version, and the `git describe` forms of a commit that
// is not a tag — "v1.2.3-7-gabc1234" and "v1.2.3-dirty" parse as valid semver
// prereleases but name nothing the proxy can serve.
func published(v string) string {
	v = strings.TrimSpace(v)

	if !semver.IsValid(v) || strings.HasSuffix(v, dirtySuffix) || describeSuffixRe.MatchString(v) {
		return ""
	}

	return v
}
