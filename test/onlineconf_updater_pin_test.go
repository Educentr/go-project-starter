package test

import (
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// composeService carries the fields of a dev stand service the tests here ask
// about. Everything else in the compose file is left unparsed on purpose:
// resolving `extends` is compose's job, and these two keys are written by the
// template itself.
type composeService struct {
	Image      string `yaml:"image"`
	PullPolicy string `yaml:"pull_policy"`
}

const (
	// devStandProjectName is main.name of the config the subtests generate
	// from (test/docker-integration/configs/rest-only), the prefix the stand
	// puts on the images it builds.
	devStandProjectName = "resttest"

	// onlineConfAdminService is the name of the admin service in the generated
	// dev stand compose file.
	onlineConfAdminService = "onlineconf-admin"
)

// updaterImageRx matches the updater image reference a generated compose file
// runs, capturing repository and tag separately.
var updaterImageRx = regexp.MustCompile(`image:\s+(\S+/onlineconf-updater):(\S+)`)

// makefileUpdaterVersionRx captures the value of ONLINECONF_UPDATER_VERSION in
// the generated Makefile.
var makefileUpdaterVersionRx = regexp.MustCompile(`(?m)^ONLINECONF_UPDATER_VERSION = (\S+)$`)

// semverTagRx is the shape the pin has to have: a released version, not
// `latest` and not a commit-sha tag of a build nobody can name.
var semverTagRx = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

// TestOnlineConfSourcesPointAtTheFork covers where a generated project takes
// OnlineConf from (TOOLS-127): the updater module and image, and the dev
// stand's submodule. All of it is our fork Educentr/onlineconf — the old
// Nikolo/nikolo namespace gets no new versions and the module path there
// survives only as a GitHub redirect.
func TestOnlineConfSourcesPointAtTheFork(t *testing.T) {
	// One tree for all subtests: the dev stand clones the submodule during
	// generation, so generating per subtest would pay for it again.
	tmpDir := generateFromConfig(t, "rest-only", "Generate project to check its OnlineConf sources")

	// The updater pin used to be two independent literals in three templates
	// and had drifted — the Makefile installed v3.6.4 while compose ran
	// ghcr.io/nikolo/onlineconf-updater:5ee228a, a 09.2024 build — so the
	// binary a developer got from `make` and the one the stand ran were
	// different versions of the updater.
	t.Run("the updater module and every compose image name one version", func(t *testing.T) {
		makefile := readGenerated(t, tmpDir, "Makefile")

		m := makefileUpdaterVersionRx.FindStringSubmatch(makefile)
		if m == nil {
			t.Fatal("generated Makefile must define ONLINECONF_UPDATER_VERSION")
		}

		version := m[1]

		if !semverTagRx.MatchString(version) {
			t.Errorf("ONLINECONF_UPDATER_VERSION = %q, want a released semver version (vX.Y.Z)", version)
		}

		wantInstall := "go install github.com/Educentr/onlineconf/onlineconf-updater/v3@$(ONLINECONF_UPDATER_VERSION)"
		if !strings.Contains(makefile, wantInstall) {
			t.Errorf("generated Makefile must install the updater as %q", wantInstall)
		}

		// Both compose files — the dev stand's and the application's — run the
		// image of exactly the version the Makefile installs.
		for _, compose := range rootComposeFiles(t, tmpDir) {
			body := readGenerated(t, tmpDir, compose)

			img := updaterImageRx.FindStringSubmatch(body)
			if img == nil {
				t.Errorf("%s: no onlineconf-updater image found", compose)

				continue
			}

			if img[1] != "ghcr.io/educentr/onlineconf-updater" {
				t.Errorf("%s: updater image is %q, want ghcr.io/educentr/onlineconf-updater", compose, img[1])
			}

			if img[2] != version {
				t.Errorf("%s: updater image tag is %q, but the Makefile installs %q — the pins must name one version",
					compose, img[2], version)
			}
		}
	})

	// The stand takes the MySQL schema, the database service and the admin
	// sources from this submodule, so it has to be the fork the admin image is
	// built from — not upstream onlineconf/onlineconf.
	t.Run("the dev stand submodule is the fork", func(t *testing.T) {
		gitmodules := readGenerated(t, tmpDir, ".gitmodules")

		if !strings.Contains(gitmodules, "https://github.com/Educentr/onlineconf") {
			t.Errorf(".gitmodules must point etc/repo-oc at the fork, got:\n%s", gitmodules)
		}
	})

	// The stand builds the admin out of the submodule, so the image it writes
	// must carry a name of this stand. Inheriting the name from the extended
	// service tags a build of the submodule's current master with the release
	// tag of the published admin, and then any other compose on the machine
	// asking for that tag with the default pull policy runs this build
	// instead of the release (TOOLS-131). This subtest shares the tree above
	// because generating a second one re-clones the submodule.
	t.Run("the stand tags its own admin build, not the release", func(t *testing.T) {
		svc := devStandService(t, tmpDir, onlineConfAdminService)

		// The project prefix is what keeps two stands on one machine from
		// overwriting each other's admin build, so the whole name is checked,
		// not just the absence of a registry. The released tag is deliberately
		// not spelled out here: it moves with every admin release, while the
		// defect is the registry-qualified name, not the version.
		if want := devStandProjectName + "-onlineconf-admin:dev"; svc.Image != want {
			t.Errorf("docker-compose-dev.yaml: %s image is %q, want %q",
				onlineConfAdminService, svc.Image, want)
		}

		if strings.Contains(svc.Image, "/") {
			t.Errorf("docker-compose-dev.yaml: %s image %q is registry-qualified — a local build must not claim a published name",
				onlineConfAdminService, svc.Image)
		}

		// Without this the override above would be moot: compose would pick
		// between the inherited image and our build by the local cache.
		if svc.PullPolicy != "build" {
			t.Errorf("docker-compose-dev.yaml: %s pull_policy is %q, want \"build\"",
				onlineConfAdminService, svc.PullPolicy)
		}
	})

	// The acceptance criterion of TOOLS-127 read off the output.
	t.Run("nothing in the tree references the old namespace", func(t *testing.T) {
		assertNoNikoloReferences(t, tmpDir)
	})
}

// devStandService returns the named service of the generated dev stand compose
// file.
func devStandService(t *testing.T, targetDir, name string) composeService {
	t.Helper()

	body := readGenerated(t, targetDir, "docker-compose-dev.yaml")

	var compose struct {
		Services map[string]composeService `yaml:"services"`
	}

	if err := yaml.Unmarshal([]byte(body), &compose); err != nil {
		t.Fatalf("Error parsing generated docker-compose-dev.yaml: %v", err)
	}

	svc, ok := compose.Services[name]
	if !ok {
		t.Fatalf("docker-compose-dev.yaml has no service %q, got %v", name, slices.Sorted(maps.Keys(compose.Services)))
	}

	return svc
}

// rootComposeFiles returns the generated compose files at the project root, so
// that a compose file added later is covered without editing the test.
func rootComposeFiles(t *testing.T, targetDir string) []string {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(targetDir, "docker-compose*.yaml"))
	if err != nil {
		t.Fatalf("Error globbing compose files: %v", err)
	}

	names := make([]string, 0, len(matches))

	for _, match := range matches {
		names = append(names, filepath.Base(match))
	}

	// The dev stand's compose plus the application's — both carry an updater.
	if len(names) < 2 {
		t.Fatalf("expected both the dev stand and the application compose file, got %v", names)
	}

	return names
}

// assertNoNikoloReferences reports every generated file still pointing at the
// old Nikolo/nikolo namespace. etc/repo-oc is skipped — it is the OnlineConf
// submodule, somebody else's code, not generator output.
func assertNoNikoloReferences(t *testing.T, root string) {
	t.Helper()

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return rerr
		}

		if info.IsDir() {
			switch rel {
			case filepath.Join("etc", "repo-oc"), ".git":
				return filepath.SkipDir
			}

			return nil
		}

		if !info.Mode().IsRegular() {
			return nil
		}

		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}

		if strings.Contains(strings.ToLower(string(data)), "nikolo") {
			t.Errorf("%s references the old Nikolo namespace", rel)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("Error walking generated tree: %v", err)
	}
}
