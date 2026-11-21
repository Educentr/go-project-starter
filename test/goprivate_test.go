package test

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// GOPRIVATE is read by four independent consumers of a generated project, and
// each of them used to assemble the value itself: the Makefile and the GitHub
// workflows prepended github.com/Educentr, .gitlab-ci.yml prepended
// gitlab.com, and the builder stage of the Dockerfile prepended nothing. A
// project that depended on a private module one of those prefixes covered
// therefore built on the host and in CI and failed inside `docker build`,
// which is the hardest shape of this defect to diagnose.
//
// The tests below assert the property that replaces those prefixes: every
// consumer renders one value, and that value is git.private_repos.

// makefileGoPrivate is the GOPRIVATE assignment of the generated Makefile.
var makefileGoPrivate = regexp.MustCompile(`(?m)^GOPRIVATE \?=(.*)$`)

// dockerfileGoPrivate is the builder-stage ENV of a generated Dockerfile.
var dockerfileGoPrivate = regexp.MustCompile(`(?m)^ENV GOPRIVATE="(.*)"$`)

// shellGoPrivate is the `go env -w` call the CI pipelines use. It covers both
// the GitHub Actions step and the .gitlab-ci.yml before_script entry.
var shellGoPrivate = regexp.MustCompile(`go env -w GOPRIVATE="(.*)"`)

// goPrivateValues collects every GOPRIVATE value a generated project declares,
// keyed by the file and consumer it came from so a disagreement names the two
// sides. Files that declare none contribute no entry: whether a consumer
// should carry the value at all is asserted separately, by the caller that
// knows whether the project declares private modules.
func goPrivateValues(t *testing.T, targetDir string) map[string]string {
	t.Helper()

	// The Makefile and the Dockerfile each hold at most one assignment; the CI
	// files hold one per job that fetches modules, and all of them must agree,
	// so they are keyed by occurrence.
	single := []struct {
		relPath string
		re      *regexp.Regexp
	}{
		{"Makefile", makefileGoPrivate},
		{"Dockerfile-api", dockerfileGoPrivate},
	}

	values := map[string]string{}

	for _, src := range single {
		content := readGenerated(t, targetDir, src.relPath)

		if m := src.re.FindStringSubmatch(content); m != nil {
			values[src.relPath] = m[1]
		}
	}

	for _, relPath := range []string{ciCDWorkflow, goVersionCheckWorkflow, ".gitlab-ci.yml"} {
		content := readGenerated(t, targetDir, relPath)

		for i, m := range shellGoPrivate.FindAllStringSubmatch(content, -1) {
			values[relPath+" #"+strconv.Itoa(i+1)] = m[1]
		}
	}

	return values
}

// TestGenerateGoPrivateIsOneValue checks that the Makefile, the builder stage
// of the Dockerfile, every GitHub Actions job and the GitLab pipeline of one
// generated project declare the same GOPRIVATE, and that it is exactly
// git.private_repos.
func TestGenerateGoPrivateIsOneValue(t *testing.T) {
	// private-token-secret declares git.private_repos and renders every
	// consumer: a Makefile, a Dockerfile, both workflows (with the checks,
	// goat-tests and build-packages jobs) and .gitlab-ci.yml.
	tmpDir := generateProject(t, "private-token-secret")

	// The value declared by test/docker-integration/configs/private-token-secret.
	const want = "github.com/myorg/*"

	values := goPrivateValues(t, tmpDir)

	// A consumer that stopped rendering the value would otherwise pass by
	// being absent from the map.
	wantSources := []string{
		"Makefile",
		"Dockerfile-api",
		ciCDWorkflow + " #1",
		ciCDWorkflow + " #2",
		ciCDWorkflow + " #3",
		goVersionCheckWorkflow + " #1",
		".gitlab-ci.yml #1",
	}

	for _, source := range wantSources {
		if _, ok := values[source]; !ok {
			t.Errorf("%s declares no GOPRIVATE, want %q; got %v", source, want, values)
		}
	}

	if len(values) != len(wantSources) {
		t.Errorf("GOPRIVATE is declared in %d places, want %d: %v", len(values), len(wantSources), values)
	}

	for source, got := range values {
		if got != want {
			t.Errorf("%s: GOPRIVATE = %q, want %q (all consumers must agree)", source, got, want)
		}
	}
}

// TestGenerateGoPrivateWithoutPrivateRepos checks the other half of the
// decision: without git.private_repos nothing is taken off the module proxy.
//
// The generator used to put github.com/Educentr in GOPRIVATE unconditionally,
// so every generated project — including every project of an external user of
// the generator — fetched all Educentr modules straight from git and without
// verifying them against sum.golang.org. All of them are public, so the only
// effect was losing that verification.
func TestGenerateGoPrivateWithoutPrivateRepos(t *testing.T) {
	tmpDir := generateProject(t, "github-ci-defaults")

	if values := goPrivateValues(t, tmpDir); len(values) != 1 {
		t.Errorf("project without git.private_repos declares GOPRIVATE in %v, want only the empty Makefile default", values)
	}

	// The Makefile keeps the (empty) assignment: it is a `?=` default a
	// developer can override from the environment, not a value.
	if got := makefileGoPrivate.FindStringSubmatch(readGenerated(t, tmpDir, "Makefile")); got == nil || got[1] != "" {
		t.Errorf("Makefile GOPRIVATE = %v, want an empty default", got)
	}

	// A bare `ENV GOPRIVATE=""` in the image would be a value too, so the
	// Dockerfile drops the line instead.
	if content := readGenerated(t, tmpDir, "Dockerfile-api"); strings.Contains(content, "GOPRIVATE") {
		t.Error("Dockerfile sets GOPRIVATE without git.private_repos")
	}
}
