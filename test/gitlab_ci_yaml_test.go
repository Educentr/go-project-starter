package test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// gitlabCIPath is the pipeline file the generator writes for GitLab.
const gitlabCIPath = ".gitlab-ci.yml"

// yamlErrorContextLines is how many lines a parse failure prints on each side
// of the line the error points at.
const yamlErrorContextLines = 3

// registryMatrixConfig is the fixture the registry_type matrix is built on:
// it is the one carrying both places where a registry login branch is
// followed by another key — the before_script of goat-tests-<app> (followed
// by script:) and the one of .build-template (followed by rules:).
const registryMatrixConfig = "rest-envs-goat"

// matrixPrivateRepos is the git.private_repos value of the matrix runs that
// render the GOPRIVATE step of the goat job.
const matrixPrivateRepos = "github.com/myorg/*"

// registryTypes lists every value config validation accepts for
// main.registry_type. Each one renders its own branch of the registry login
// blocks of the pipeline template, so only a matrix over all four covers
// every branch.
var registryTypes = []string{"github", "digitalocean", "aws", "selfhosted"}

// registryMatrixChecks names, per job of registryMatrixConfig, the key that
// follows the registry login branch. A trimmer that swallows a newline moves
// such a key into the before_script list above it instead of leaving it at
// job level.
var registryMatrixChecks = map[string]string{
	"goat-tests-api":  "script",
	".build-template": "rules",
}

// yamlErrorLine matches the line number a yaml.v3 syntax error reports.
var yamlErrorLine = regexp.MustCompile(`line (\d+)`)

// TestGitLabCIIsValidYAML verifies that the generated pipeline parses as YAML
// for every config fixture. Until this check existed the generated file was
// invalid YAML in every project with a Docker artifact (TOOLS-109): the
// template closed its registry login branches with a right-trimming
// `{{ end -}}` that left the indentation of the action behind, which pulled
// the next key into the before_script list above it.
func TestGitLabCIIsValidYAML(t *testing.T) {
	for _, name := range configFixtureNames(t) {
		t.Run(name, func(t *testing.T) {
			parseGitLabCI(t, generateProject(t, name))
		})
	}
}

// TestGitLabCIIsValidYAMLForEveryRegistryType verifies the same for all four
// registry types, with and without private Go modules. The fixtures only
// cover github and digitalocean, and each registry type renders a branch of
// its own, so the fixture sweep alone leaves aws and selfhosted unchecked.
// private_repos is the second axis because its branch is the one that ends
// the before_script of goat-tests-<app>, and which of the two closes that
// list depends on it.
func TestGitLabCIIsValidYAMLForEveryRegistryType(t *testing.T) {
	for _, registryType := range registryTypes {
		for _, privateRepos := range []string{"", matrixPrivateRepos} {
			name := registryType
			if privateRepos != "" {
				name += "/private-repos"
			}

			t.Run(name, func(t *testing.T) {
				configDir := writeMatrixConfig(t, registryMatrixConfig, registryType, privateRepos)
				targetDir := t.TempDir()

				out, err := generateFromConfigDir(t, configDir, targetDir,
					"Generate "+registryMatrixConfig+" as "+name)
				if err != nil {
					t.Fatalf("Error creating project %s: %v\n%s", name, err, out)
				}

				pipeline := parseGitLabCI(t, targetDir)

				for jobName, key := range registryMatrixChecks {
					job, ok := pipeline[jobName].(map[string]any)
					if !ok {
						t.Fatalf("%s has no job %q, got %#v", gitlabCIPath, jobName, pipeline[jobName])
					}

					if _, ok := job[key]; !ok {
						t.Errorf("job %q has no %q key — the branch above it swallowed the key: %#v",
							jobName, key, job)
					}
				}
			})
		}
	}
}

// configFixturesDir returns the directory with the generator configs the
// tests generate from.
func configFixturesDir(t *testing.T) string {
	t.Helper()

	curDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting current directory: %v", err)
	}

	return filepath.Join(curDir, "..", "test", "docker-integration", "configs")
}

// configFixtureNames lists every config directory under
// test/docker-integration/configs.
func configFixtureNames(t *testing.T) []string {
	t.Helper()

	entries, err := os.ReadDir(configFixturesDir(t))
	if err != nil {
		t.Fatalf("Error reading configs directory: %v", err)
	}

	names := make([]string, 0, len(entries))

	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}

	if len(names) == 0 {
		t.Fatal("no config fixtures found")
	}

	return names
}

// parseGitLabCI reads the generated pipeline and unmarshals the whole file,
// which asserts that the generated YAML is valid. The file is checked as a
// whole on purpose: a template trimmer that swallows a newline shifts the
// next key into the list above it, and substring assertions on the output
// still pass on such a file while GitLab rejects the pipeline outright.
func parseGitLabCI(t *testing.T, targetDir string) map[string]any {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(targetDir, gitlabCIPath))
	if err != nil {
		t.Fatalf("Error reading %s: %v", gitlabCIPath, err)
	}

	pipeline := map[string]any{}
	if err := yaml.Unmarshal(content, &pipeline); err != nil {
		t.Fatalf("%s is not valid YAML: %v\n%s", gitlabCIPath, err, yamlErrorContext(content, err))
	}

	if len(pipeline) == 0 {
		t.Fatalf("%s has no top-level keys", gitlabCIPath)
	}

	return pipeline
}

// writeMatrixConfig copies the config fixture into a temporary directory with
// main.registry_type replaced and git.private_repos set (an empty value
// leaves the fixture without private modules), and returns that directory.
// The copy keeps the spec files next to project.yaml, which the relative
// `path:` entries of the fixture need.
func writeMatrixConfig(t *testing.T, fixture, registryType, privateRepos string) string {
	t.Helper()

	srcDir := filepath.Join(configFixturesDir(t), fixture)
	dstDir := t.TempDir()

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		t.Fatalf("Error reading config %s: %v", fixture, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		content, err := os.ReadFile(filepath.Join(srcDir, entry.Name()))
		if err != nil {
			t.Fatalf("Error reading %s/%s: %v", fixture, entry.Name(), err)
		}

		if err := os.WriteFile(filepath.Join(dstDir, entry.Name()), content, 0o644); err != nil {
			t.Fatalf("Error writing %s: %v", entry.Name(), err)
		}
	}

	patchMatrixConfig(t, filepath.Join(dstDir, "project.yaml"), registryType, privateRepos)

	return dstDir
}

// patchMatrixConfig rewrites main.registry_type and git.private_repos of a
// project config, and cuts its post_generate steps down to git_install
// (which dev_stand requires). Generating the code and tidying the module cost
// seconds per run and the pipeline file does not depend on either, so the
// matrix skips them.
func patchMatrixConfig(t *testing.T, path, registryType, privateRepos string) {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Error reading %s: %v", path, err)
	}

	doc := map[string]any{}
	if err := yaml.Unmarshal(content, &doc); err != nil {
		t.Fatalf("Error parsing %s: %v", path, err)
	}

	main, ok := doc["main"].(map[string]any)
	if !ok {
		t.Fatalf("%s has no main section", path)
	}

	main["registry_type"] = registryType
	doc["post_generate"] = []any{"git_install"}

	if privateRepos != "" {
		gitSection, ok := doc["git"].(map[string]any)
		if !ok {
			t.Fatalf("%s has no git section", path)
		}

		gitSection["private_repos"] = privateRepos
	}

	patched, err := yaml.Marshal(doc)
	if err != nil {
		t.Fatalf("Error marshaling %s: %v", path, err)
	}

	if err := os.WriteFile(path, patched, 0o644); err != nil {
		t.Fatalf("Error writing %s: %v", path, err)
	}
}

// generateFromConfigDir runs the generator for a config directory the test
// prepared itself, which generateProject cannot do: it takes the name of a
// fixture under test/docker-integration/configs.
func generateFromConfigDir(t *testing.T, configDir, targetDir, msg string) (string, error) {
	t.Helper()

	curDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting current directory: %v", err)
	}

	return ExecCommand(filepath.Join(curDir, ".."), "go", []string{
		"run", filepath.Join(curDir, "..", "cmd", "go-project-starter", "main.go"),
		"--target", targetDir,
		"--configDir", configDir,
		"--config", "project.yaml",
	}, msg)
}

// yamlErrorContext returns the lines of content around the one the parse
// error points at. The pipeline file is a few hundred lines long and the
// sweep runs a subtest per config, so dumping the whole file of every failure
// buries the one line that matters.
func yamlErrorContext(content []byte, err error) string {
	match := yamlErrorLine.FindStringSubmatch(err.Error())
	if match == nil {
		return string(content)
	}

	num, convErr := strconv.Atoi(match[1])
	if convErr != nil {
		return string(content)
	}

	lines := strings.Split(string(content), "\n")

	from := max(num-yamlErrorContextLines-1, 0)
	to := min(num+yamlErrorContextLines, len(lines))

	numbered := make([]string, 0, max(to-from, 0))
	for i := from; i < to; i++ {
		numbered = append(numbered, fmt.Sprintf("%4d| %s", i+1, lines[i]))
	}

	return strings.Join(numbered, "\n")
}
