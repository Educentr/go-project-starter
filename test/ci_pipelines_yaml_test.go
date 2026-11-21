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

// prepareEnvJob is the GitHub job that resolves the deployment variables and
// hands them to the jobs below it as outputs.
const prepareEnvJob = "prepare-env"

// sharedDomainConfig is the config fixture whose public REST transport (api)
// is served by two applications (api and worker) — the shape that made the
// GitHub workflow invalid. Only two fixtures have it (this one and
// metrics-map-apps); the rest give each public transport an application of
// its own, so a sweep over projects of their shape would not have caught the
// duplicate key.
const sharedDomainConfig = "metrics-map"

// sharedDomainOutput is the output the domain of that shared transport is
// declared under, and sharedDomainPortPrefixes are the per-application
// outputs of the same transport. The port prefixes are asserted next to the
// domain to pin the property the fixture is chosen for: lose one of the two
// applications and the domain stops being shared.
const sharedDomainOutput = "DOMAIN_API"

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

var sharedDomainPortPrefixes = []string{"PORT_PREFIX_API_API", "PORT_PREFIX_WORKER_API"}

// domainEnvWrite matches the line a pipeline script writes a
// DOMAIN_<transport> variable with, in each of the three forms the templates
// use: the GitHub job output, the GitHub env file and the GitLab env file
// heredoc.
var domainEnvWrite = regexp.MustCompile(`(?m)^\s*(?:echo ')?(DOMAIN_[A-Za-z0-9_]+)=`)

// yamlErrorLine matches the line number a yaml.v3 syntax error reports.
var yamlErrorLine = regexp.MustCompile(`line (\d+)`)

// TestCIPipelinesAreValidYAML verifies that both generated CI pipelines parse
// as YAML for every config fixture. Both were invalid YAML until the sweep
// existed, and for unrelated reasons, so both are checked from one generated
// project rather than from a sweep of their own:
//
//   - .gitlab-ci.yml, in every project with a Docker artifact (TOOLS-109): the
//     template closed its registry login branches with a right-trimming
//     `{{ end -}}` that left the indentation of the action behind, which pulled
//     the next key into the before_script list above it.
//   - .github/workflows/ci_cd.yml, in every project where one public REST
//     transport is served by more than one application (TOOLS-110): the
//     DOMAIN_<transport> output was emitted once per application, which gave
//     the outputs map of prepare-env a duplicate key.
func TestCIPipelinesAreValidYAML(t *testing.T) {
	for _, name := range configFixtureNames(t) {
		t.Run(name, func(t *testing.T) {
			targetDir := generateProject(t, name)

			parsePipeline(t, targetDir, gitlabCIPath)
			parsePipeline(t, targetDir, ciCDWorkflow)
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

				pipeline := parsePipeline(t, targetDir, gitlabCIPath)

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

// TestCIDeclaresSharedDomainOnce verifies that a public REST transport served
// by more than one application gets exactly one DOMAIN_<transport>
// declaration in either pipeline. The sweep above catches the duplicate
// mapping key the second application used to add to the GitHub workflow, but
// it says nothing about the repeats that are valid YAML — the GitLab env file
// got the same line twice — and a fix that dropped the declaration altogether
// would pass the sweep just as well, which would leave a public service
// without a domain.
func TestCIDeclaresSharedDomainOnce(t *testing.T) {
	targetDir := generateProject(t, sharedDomainConfig)

	// The outputs are read from the untyped parse because githubWorkflow has
	// no outputs field, so a decode into it would not even look at the
	// mapping that used to carry the duplicate.
	outputs := prepareEnvOutputs(t, parsePipeline(t, targetDir, ciCDWorkflow))

	for _, name := range append([]string{sharedDomainOutput}, sharedDomainPortPrefixes...) {
		if _, ok := outputs[name]; !ok {
			t.Errorf("job %q declares no output %q, got %v", prepareEnvJob, name, outputs)
		}
	}

	// A repeat in the remaining places is valid YAML, so the sweep says
	// nothing about it: the scripts would check and write the same variable
	// twice, and the secrets step would fetch it twice.
	for jobName, job := range parseWorkflow(t, targetDir, ciCDWorkflow).Jobs {
		for _, step := range job.Steps {
			for name, count := range domainWrites(step.Run) {
				if count > 1 {
					t.Errorf("%s: job %q step %q writes %s %d times, want once",
						ciCDWorkflow, jobName, step.Name, name, count)
				}
			}

			if count := secretsNamed(step, sharedDomainOutput); count > 1 {
				t.Errorf("%s: job %q step %q requests secret %s %d times, want once",
					ciCDWorkflow, jobName, step.Name, sharedDomainOutput, count)
			}
		}
	}

	for jobName, script := range gitlabScripts(t, parsePipeline(t, targetDir, gitlabCIPath)) {
		for name, count := range domainWrites(script) {
			if count > 1 {
				t.Errorf("%s: job %q writes %s %d times, want once",
					gitlabCIPath, jobName, name, count)
			}
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

// parsePipeline reads a generated CI pipeline and unmarshals the whole file,
// which asserts that the generated YAML is valid. The file is checked as a
// whole, and into a plain map rather than into a struct of the keys a test
// looks at, on purpose: a template trimmer that swallows a newline shifts the
// next key into the list above it, and a key emitted once per application
// lands in the same mapping twice. Substring assertions on the output still
// pass on such a file, and so does a decode that skips the mapping holding
// the duplicate, while the CI provider rejects the pipeline outright.
func parsePipeline(t *testing.T, targetDir, relPath string) map[string]any {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(targetDir, relPath))
	if err != nil {
		t.Fatalf("Error reading %s: %v", relPath, err)
	}

	pipeline := map[string]any{}
	if err := yaml.Unmarshal(content, &pipeline); err != nil {
		t.Fatalf("%s is not valid YAML: %v\n%s", relPath, err, yamlErrorContext(content, err))
	}

	if len(pipeline) == 0 {
		t.Fatalf("%s has no top-level keys", relPath)
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

// prepareEnvOutputs returns the outputs mapping of the job that resolves the
// deployment variables.
func prepareEnvOutputs(t *testing.T, pipeline map[string]any) map[string]any {
	t.Helper()

	jobs, ok := pipeline["jobs"].(map[string]any)
	if !ok {
		t.Fatalf("%s has no jobs mapping, got %#v", ciCDWorkflow, pipeline["jobs"])
	}

	job, ok := jobs[prepareEnvJob].(map[string]any)
	if !ok {
		t.Fatalf("%s has no job %q, got %#v", ciCDWorkflow, prepareEnvJob, jobs[prepareEnvJob])
	}

	outputs, ok := job["outputs"].(map[string]any)
	if !ok {
		t.Fatalf("job %q has no outputs mapping, got %#v", prepareEnvJob, job["outputs"])
	}

	return outputs
}

// domainWrites counts, per variable name, how many times a pipeline script
// writes a DOMAIN_<transport> variable.
func domainWrites(script string) map[string]int {
	counts := map[string]int{}

	for _, match := range domainEnvWrite.FindAllStringSubmatch(script, -1) {
		counts[match[1]]++
	}

	return counts
}

// secretsNamed counts how many times a step lists a variable in its secrets
// input. The branch-based-secrets action takes the names as one
// comma-separated string, so a repeat there is a repeated fetch rather than a
// YAML error.
func secretsNamed(step githubStep, name string) int {
	secrets, _ := step.With["secrets"].(string)

	count := 0

	for _, secret := range strings.Split(secrets, ",") {
		if strings.TrimSpace(secret) == name {
			count++
		}
	}

	return count
}

// gitlabScripts returns the script of every GitLab job that has one, joined
// into one string and keyed by job name. The env file the deploy jobs write is
// a heredoc inside such a script, so a line repeated in it is invisible to the
// YAML sweep.
func gitlabScripts(t *testing.T, pipeline map[string]any) map[string]string {
	t.Helper()

	scripts := map[string]string{}

	for jobName, raw := range pipeline {
		job, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		lines, ok := job["script"].([]any)
		if !ok {
			continue
		}

		for _, line := range lines {
			if text, ok := line.(string); ok {
				scripts[jobName] += text + "\n"
			}
		}
	}

	if len(scripts) == 0 {
		t.Fatalf("%s has no job with a script", gitlabCIPath)
	}

	return scripts
}
