package selfversion

import (
	"runtime/debug"
	"testing"
)

func buildInfo(mainVersion string) func() (*debug.BuildInfo, bool) {
	return func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Main: debug.Module{Version: mainVersion}}, true
	}
}

func noBuildInfo() (*debug.BuildInfo, bool) {
	return nil, false
}

func TestResolve(t *testing.T) {
	tests := []struct {
		name      string
		ldflags   string
		buildInfo func() (*debug.BuildInfo, bool)
		want      string
	}{
		{
			// A release build: `make build` stamps the tag it was cut from.
			name:      "ldflags tag wins",
			ldflags:   "v1.4.0",
			buildInfo: buildInfo("v1.3.0"),
			want:      "v1.4.0",
		},
		{
			// `go run <pkg>@v1.4.0`: nothing is stamped, the module version is
			// the only record of what is running — and the common path, since
			// the generated Makefile invokes the generator exactly this way.
			name:      "module version when nothing is stamped",
			ldflags:   "dev",
			buildInfo: buildInfo("v1.4.0"),
			want:      "v1.4.0",
		},
		{
			name:      "prerelease tag is a published version",
			ldflags:   "v1.4.0-rc.1",
			buildInfo: noBuildInfo,
			want:      "v1.4.0-rc.1",
		},
		{
			// `go get <pkg>@<commit>` of an untagged commit — the proxy serves
			// this form, so it is a usable pin.
			name:      "pseudo-version is a published version",
			ldflags:   "dev",
			buildInfo: buildInfo("v0.0.0-20260101120000-abcdef123456"),
			want:      "v0.0.0-20260101120000-abcdef123456",
		},
		{
			// `git describe` past the tag: parses as semver, names nothing the
			// proxy can serve.
			name:      "git describe of an untagged commit is unknown",
			ldflags:   "v1.4.0-7-gabc1234",
			buildInfo: noBuildInfo,
			want:      "",
		},
		{
			name:      "dirty tree is unknown",
			ldflags:   "v1.4.0-dirty",
			buildInfo: noBuildInfo,
			want:      "",
		},
		{
			name:      "dirty tree past the tag is unknown",
			ldflags:   "v1.4.0-7-gabc1234-dirty",
			buildInfo: noBuildInfo,
			want:      "",
		},
		{
			// `git describe --always` with no tags in the repository.
			name:      "bare commit hash is unknown",
			ldflags:   "abc1234",
			buildInfo: noBuildInfo,
			want:      "",
		},
		{
			// `go build` / `go run .` of a working copy.
			name:      "devel placeholders are unknown",
			ldflags:   "dev",
			buildInfo: buildInfo("(devel)"),
			want:      "",
		},
		{
			name:      "unstamped build without build info is unknown",
			ldflags:   "",
			buildInfo: noBuildInfo,
			want:      "",
		},
		{
			name:      "unusable stamp falls through to the module version",
			ldflags:   "v1.4.0-7-gabc1234",
			buildInfo: buildInfo("v1.4.0"),
			want:      "v1.4.0",
		},
		{
			name:      "surrounding whitespace is trimmed",
			ldflags:   " v1.4.0\n",
			buildInfo: noBuildInfo,
			want:      "v1.4.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolve(tt.ldflags, tt.buildInfo); got != tt.want {
				t.Errorf("resolve(%q, …) = %q, want %q", tt.ldflags, got, tt.want)
			}
		})
	}
}

// TestResolveReadsRealBuildInfo guards the wiring of the exported entry point:
// the test binary is built from this module, so build info is present and the
// stamped value must still win over it.
func TestResolveReadsRealBuildInfo(t *testing.T) {
	if got := Resolve("v9.9.9"); got != "v9.9.9" {
		t.Errorf("Resolve(%q) = %q, want %q", "v9.9.9", got, "v9.9.9")
	}

	// The test binary's own module version is "(devel)", i.e. unknown.
	if got := Resolve("dev"); got != "" {
		t.Errorf("Resolve(%q) = %q, want empty", "dev", got)
	}
}
