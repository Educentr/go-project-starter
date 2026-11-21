package test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// githubWorkflow is the subset of a GitHub Actions workflow the github_ci
// parameters affect.
type githubWorkflow struct {
	Jobs map[string]githubJob `yaml:"jobs"`
}

// githubJob is the subset of a workflow job the github_ci parameters affect.
type githubJob struct {
	RunsOn         any          `yaml:"runs-on"`
	TimeoutMinutes *int         `yaml:"timeout-minutes"`
	Steps          []githubStep `yaml:"steps"`
}

// githubStep is the subset of a job step the github_ci parameters affect.
type githubStep struct {
	Name string            `yaml:"name"`
	Uses string            `yaml:"uses"`
	With map[string]any    `yaml:"with"`
	Env  map[string]string `yaml:"env"`
	Run  string            `yaml:"run"`
}

// doRegistryLoginStepName is the name of the step that logs in to the
// DigitalOcean Container Registry in both jobs that push images.
const doRegistryLoginStepName = "Log in to DO Container Registry"

// The GitHub Actions workflows the generator produces.
const (
	ciCDWorkflow           = ".github/workflows/ci_cd.yml"
	goVersionCheckWorkflow = ".github/workflows/go-version-check.yml"
)

// generatedWorkflowNames lists the GitHub Actions workflows the generator
// produces; the github_ci parameters apply to every job of both.
var generatedWorkflowNames = []string{
	ciCDWorkflow,
	goVersionCheckWorkflow,
}

// privateReposStepNames are the two steps that give a job access to private Go
// modules: the git credential rewrite and GOPRIVATE. Every job that downloads
// modules needs both.
var privateReposStepNames = []string{
	"Configure Git for private repos",
	"Set GOPRIVATE",
}

// stepsByName indexes the steps of a job by step name.
func stepsByName(job githubJob) map[string]githubStep {
	steps := map[string]githubStep{}

	for _, step := range job.Steps {
		steps[step.Name] = step
	}

	return steps
}

// generateProject runs the generator for a config dir from
// test/docker-integration/configs and returns the target directory.
func generateProject(t *testing.T, configName string) string {
	t.Helper()

	curDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting current directory: %v", err)
	}

	configDir := filepath.Join(curDir, "..", "test", "docker-integration", "configs", configName)
	tmpDir := t.TempDir()

	out, err := ExecCommand(filepath.Join(curDir, ".."), "go", []string{
		"run", filepath.Join(curDir, "..", "cmd", "go-project-starter", "main.go"),
		"--target", tmpDir,
		"--configDir", configDir,
		"--config", "project.yaml",
	}, "Generate "+configName+" project ("+tmpDir+")")
	if err != nil {
		t.Fatalf("Error creating project: %s\n%s", err, out)
	}

	return tmpDir
}

// parseWorkflow reads a generated workflow and unmarshals it, which also
// asserts that the generated YAML is valid.
func parseWorkflow(t *testing.T, targetDir, relPath string) githubWorkflow {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(targetDir, relPath))
	if err != nil {
		t.Fatalf("Error reading %s: %v", relPath, err)
	}

	var wf githubWorkflow
	if err := yaml.Unmarshal(content, &wf); err != nil {
		t.Fatalf("%s is not valid YAML: %v\n%s", relPath, err, content)
	}

	if len(wf.Jobs) == 0 {
		t.Fatalf("%s has no jobs", relPath)
	}

	return wf
}

// doctlVersions collects the version input of every digitalocean/action-doctl
// step of a workflow. A nil entry means the step has no version input.
func doctlVersions(wf githubWorkflow) []any {
	versions := []any{}

	for _, job := range wf.Jobs {
		for _, step := range job.Steps {
			if strings.HasPrefix(step.Uses, "digitalocean/action-doctl@") {
				versions = append(versions, step.With["version"])
			}
		}
	}

	return versions
}

// doRegistryLoginSteps collects the DigitalOcean registry login steps of a
// workflow, keyed by job name so a missing job is visible in the failure.
func doRegistryLoginSteps(wf githubWorkflow) map[string]githubStep {
	steps := map[string]githubStep{}

	for jobName, job := range wf.Jobs {
		for _, step := range job.Steps {
			if step.Name == doRegistryLoginStepName {
				steps[jobName] = step
			}
		}
	}

	return steps
}

// TestGenerateGithubCIDefaults verifies that without a github_ci section the
// generated workflows keep the historical output: every job runs on
// ubuntu-latest, no job declares a timeout and action-doctl installs the
// latest release.
func TestGenerateGithubCIDefaults(t *testing.T) {
	// github-ci-defaults is github-ci without the github_ci block, so the two
	// configs differ only in the parameters under test.
	tmpDir := generateProject(t, "github-ci-defaults")

	for _, name := range generatedWorkflowNames {
		wf := parseWorkflow(t, tmpDir, name)

		for jobName, job := range wf.Jobs {
			if job.RunsOn != "ubuntu-latest" {
				t.Errorf("%s: job %q runs-on = %#v, want \"ubuntu-latest\"", name, jobName, job.RunsOn)
			}

			if job.TimeoutMinutes != nil {
				t.Errorf("%s: job %q should not declare timeout-minutes, got %d", name, jobName, *job.TimeoutMinutes)
			}
		}
	}

	// action-doctl must keep installing the latest release.
	wf := parseWorkflow(t, tmpDir, ciCDWorkflow)

	versions := doctlVersions(wf)
	if len(versions) == 0 {
		t.Fatal("ci_cd.yml has no digitalocean/action-doctl steps, expected the digitalocean registry branch")
	}

	for i, v := range versions {
		if v != nil {
			t.Errorf("action-doctl step %d: version = %#v, want no version input", i, v)
		}
	}

	// The login step keeps taking the token from the action, not from its own
	// environment: that shape belongs to the image mode only.
	for jobName, step := range doRegistryLoginSteps(wf) {
		if _, ex := step.Env["DIGITALOCEAN_ACCESS_TOKEN"]; ex {
			t.Errorf("job %q: login step should not declare DIGITALOCEAN_ACCESS_TOKEN in the install mode", jobName)
		}
	}
}

// TestGenerateGithubCIDoctlImage verifies that github_ci.doctl: image drops
// the doctl install step from every job and makes the login step take the
// token from its own environment, after printing the version of the doctl the
// runner image provides.
func TestGenerateGithubCIDoctlImage(t *testing.T) {
	tmpDir := generateProject(t, "github-ci-doctl-image")

	wf := parseWorkflow(t, tmpDir, ciCDWorkflow)

	// No doctl may be installed: neither by action-doctl nor by a hand-rolled
	// download. A runner image without doctl has to turn the job red.
	if versions := doctlVersions(wf); len(versions) != 0 {
		t.Errorf("ci_cd.yml has %d digitalocean/action-doctl steps, want none in the image mode", len(versions))
	}

	for jobName, job := range wf.Jobs {
		for _, step := range job.Steps {
			if strings.Contains(step.Name, "Install doctl") || strings.Contains(step.Run, "doctl-") {
				t.Errorf("job %q: step %q installs doctl, want none in the image mode", jobName, step.Name)
			}
		}
	}

	// Every job that logs in to the registry does so in the image mode: the
	// two that push images (the GOAT tests job and the build matrix) plus the
	// checks job, which the config gives a check with registry_login: true.
	// The checks job arrived alongside this mode, so it is the one place where
	// a stale install step can survive unnoticed.
	loginSteps := doRegistryLoginSteps(wf)
	if len(loginSteps) != 3 {
		t.Fatalf("ci_cd.yml has %d %q steps (%v), want 3", len(loginSteps), doRegistryLoginStepName, loginSteps)
	}

	if _, ok := loginSteps["checks"]; !ok {
		t.Errorf("the checks job must log in to the registry for a check with registry_login, got jobs %v", loginSteps)
	}

	for jobName, step := range loginSteps {
		if step.Uses != "" {
			t.Errorf("job %q: login step uses %q, want a plain run step", jobName, step.Uses)
		}

		if got, want := step.Env["DIGITALOCEAN_ACCESS_TOKEN"], "${{ secrets.REGISTRY_PASSWORD }}"; got != want {
			t.Errorf("job %q: login step env DIGITALOCEAN_ACCESS_TOKEN = %q, want %q", jobName, got, want)
		}

		versionIdx := strings.Index(step.Run, "doctl version")
		loginIdx := strings.Index(step.Run, "doctl registry login")

		if versionIdx < 0 || loginIdx < 0 {
			t.Errorf("job %q: login step run = %q, want it to call `doctl version` and `doctl registry login`", jobName, step.Run)

			continue
		}

		if versionIdx > loginIdx {
			t.Errorf("job %q: login step runs `doctl version` after the login, want it first as the loud stop on a missing binary", jobName)
		}
	}
}

// TestGenerateGithubCI verifies that the github_ci parameters reach every job
// of both generated workflows and the version input of both action-doctl
// steps.
func TestGenerateGithubCI(t *testing.T) {
	tmpDir := generateProject(t, "github-ci")

	// Mirrors github-ci/project.yaml: runs_on as a runner group narrowed by
	// labels, a 30 minute job timeout and a pinned doctl version.
	wantRunsOn := map[string]any{
		"group":  "my-runners",
		"labels": []any{"linux", "x64"},
	}

	for _, name := range generatedWorkflowNames {
		wf := parseWorkflow(t, tmpDir, name)

		for jobName, job := range wf.Jobs {
			gotRunsOn, ok := job.RunsOn.(map[string]any)
			if !ok {
				t.Errorf("%s: job %q runs-on = %#v, want a group mapping", name, jobName, job.RunsOn)

				continue
			}

			if gotRunsOn["group"] != wantRunsOn["group"] {
				t.Errorf("%s: job %q runs-on.group = %#v, want %#v", name, jobName, gotRunsOn["group"], wantRunsOn["group"])
			}

			gotLabels, _ := gotRunsOn["labels"].([]any)
			if len(gotLabels) != 2 || gotLabels[0] != "linux" || gotLabels[1] != "x64" {
				t.Errorf("%s: job %q runs-on.labels = %#v, want [linux x64]", name, jobName, gotRunsOn["labels"])
			}

			if job.TimeoutMinutes == nil || *job.TimeoutMinutes != 30 {
				t.Errorf("%s: job %q timeout-minutes = %v, want 30", name, jobName, job.TimeoutMinutes)
			}
		}
	}

	// Both action-doctl steps live in ci_cd.yml and must carry the pinned
	// version.
	versions := doctlVersions(parseWorkflow(t, tmpDir, ciCDWorkflow))
	if len(versions) == 0 {
		t.Fatal("ci_cd.yml has no digitalocean/action-doctl steps, expected the digitalocean registry branch")
	}

	for i, v := range versions {
		if v != "1.170.0" {
			t.Errorf("action-doctl step %d: version = %#v, want \"1.170.0\"", i, v)
		}
	}
}

// TestGenerateGoVersionCheckPrivateRepos verifies that the Go version policy
// workflow gets private-module access when git.private_repos is declared.
// Its script downloads the direct dependencies (`go list -m all`, `go mod
// download`), so without those steps the job fails on the first private
// dependency and the check is red on every push.
func TestGenerateGoVersionCheckPrivateRepos(t *testing.T) {
	// private-token-secret declares git.private_repos plus a non-default
	// private_repos_token_secret, so the steps must also read the configured
	// secret name rather than the GH_PAT default.
	tmpDir := generateProject(t, "private-token-secret")

	wf := parseWorkflow(t, tmpDir, goVersionCheckWorkflow)

	const jobName = "check-go-mod-version"

	job, ok := wf.Jobs[jobName]
	if !ok {
		t.Fatalf("%s has no %q job, got %v", goVersionCheckWorkflow, jobName, wf.Jobs)
	}

	steps := stepsByName(job)

	for _, name := range privateReposStepNames {
		if _, ok := steps[name]; !ok {
			t.Errorf("job %q has no %q step, got steps %v", jobName, name, job.Steps)
		}
	}

	if got, want := steps[privateReposStepNames[0]].Env["GH_TOKEN"], "${{ secrets.GO_MODULES_TOKEN }}"; got != want {
		t.Errorf("job %q: git config step env GH_TOKEN = %q, want %q", jobName, got, want)
	}

	// Mirrors private-token-secret/project.yaml: the declared glob has to reach
	// GOPRIVATE, otherwise the proxy is still asked for the private modules.
	if run := steps[privateReposStepNames[1]].Run; !strings.Contains(run, "github.com/myorg/*") {
		t.Errorf("job %q: GOPRIVATE step run = %q, want it to carry the declared private repos", jobName, run)
	}

	// Both steps have to precede the check itself: configuring access after the
	// download would not help it.
	checkIdx := -1
	lastSetupIdx := -1

	for i, step := range job.Steps {
		switch {
		case strings.HasPrefix(step.Name, "Check go.mod version"):
			checkIdx = i
		case step.Name == privateReposStepNames[0], step.Name == privateReposStepNames[1]:
			lastSetupIdx = i
		}
	}

	if checkIdx < 0 {
		t.Fatalf("job %q has no go.mod version check step, got steps %v", jobName, job.Steps)
	}

	if lastSetupIdx > checkIdx {
		t.Errorf("job %q configures private-module access after the check (step %d vs %d)", jobName, lastSetupIdx, checkIdx)
	}
}

// TestGenerateGoVersionCheckWithoutPrivateRepos verifies that a project
// without git.private_repos keeps the previous workflow: no token is read and
// no GOPRIVATE is set where there is nothing private to fetch.
func TestGenerateGoVersionCheckWithoutPrivateRepos(t *testing.T) {
	tmpDir := generateProject(t, "github-ci-defaults")

	wf := parseWorkflow(t, tmpDir, goVersionCheckWorkflow)

	for jobName, job := range wf.Jobs {
		for _, name := range privateReposStepNames {
			if _, ok := stepsByName(job)[name]; ok {
				t.Errorf("job %q has a %q step without git.private_repos", jobName, name)
			}
		}
	}
}

// TestGeneratePrivateModulesAccessPerJob pins which jobs of ci_cd.yml get
// private-module access on the runner, for a project that declares private
// modules.
//
// The block is rendered from one shared fragment, so the question it used to
// answer per copy — does this job need it — is now answered once per call
// site, and this is where that list lives. Every job that runs Go on the
// runner has both steps; `build` has neither, because it runs no Go there: it
// calls `make docker-<app>`, and the modules are downloaded inside the image
// from the BuildKit secret, which the Build-and-push step passes on its own.
//
// Whether the block is rendered at all is the second axis, and
// TestGeneratePrivateModulesAccessWithoutPrivateRepos covers it: the steps
// used to be unconditional here because GOPRIVATE always covered
// github.com/Educentr.
func TestGeneratePrivateModulesAccessPerJob(t *testing.T) {
	// private-token-secret builds a container and a deb package, declares a CI
	// check and runs goat tests, so every job that can carry the block is
	// rendered, and it configures a non-default secret name, so a job reading
	// the wrong one is visible here too.
	tmpDir := generateProject(t, "private-token-secret")

	wf := parseWorkflow(t, tmpDir, ciCDWorkflow)

	// jobs that fetch modules on the runner.
	wantAccess := []string{"build-packages", "checks", "run-goat-tests-api"}
	// jobs that do not, listed explicitly so that a new job cannot join the
	// workflow without a decision being made here.
	wantNoAccess := []string{"build", "deploy", "prepare-env"}

	for _, jobName := range append(append([]string{}, wantAccess...), wantNoAccess...) {
		if _, ok := wf.Jobs[jobName]; !ok {
			t.Fatalf("%s has no %q job, got %v", ciCDWorkflow, jobName, wf.Jobs)
		}
	}

	for jobName, job := range wf.Jobs {
		steps := stepsByName(job)
		want := slices.Contains(wantAccess, jobName)

		for _, name := range privateReposStepNames {
			if _, ok := steps[name]; ok != want {
				t.Errorf("job %q: has %q step = %v, want %v", jobName, name, ok, want)
			}
		}

		if !want {
			continue
		}

		if got, w := steps[privateReposStepNames[0]].Env["GH_TOKEN"], "${{ secrets.GO_MODULES_TOKEN }}"; got != w {
			t.Errorf("job %q: git config step env GH_TOKEN = %q, want %q", jobName, got, w)
		}

		if run := steps[privateReposStepNames[1]].Run; !strings.Contains(run, "github.com/myorg/*") {
			t.Errorf("job %q: GOPRIVATE step run = %q, want it to carry the declared private repos", jobName, run)
		}
	}

	// The image still gets the token: the build step hands it to `docker build
	// --secret`, which is what the removal of the runner-side step relies on.
	buildSteps := stepsByName(wf.Jobs["build"])

	step, ok := buildSteps["Build and push"]
	if !ok {
		t.Fatalf("job %q has no %q step, got steps %v", "build", "Build and push", wf.Jobs["build"].Steps)
	}

	if got, w := step.Env["GITHUB_TOKEN"], "${{ secrets.GO_MODULES_TOKEN }}"; got != w {
		t.Errorf("job \"build\": Build and push env GITHUB_TOKEN = %q, want %q", got, w)
	}
}

// TestGeneratePrivateModulesAccessWithoutPrivateRepos checks that a project
// without git.private_repos gets neither step, in either workflow.
//
// In ci_cd.yml they used to be unconditional, justified by GOPRIVATE always
// covering github.com/Educentr: the credential rewrite was installed in every
// generated project, so a project that depended on no private module still
// needed a token secret for its own CI to fetch the public Educentr modules
// through that rewrite. With nothing taken off the module proxy there is no
// such fetch and no such secret.
func TestGeneratePrivateModulesAccessWithoutPrivateRepos(t *testing.T) {
	tmpDir := generateProject(t, "github-ci-defaults")

	for _, relPath := range generatedWorkflowNames {
		wf := parseWorkflow(t, tmpDir, relPath)

		for jobName, job := range wf.Jobs {
			for _, name := range privateReposStepNames {
				if _, ok := stepsByName(job)[name]; ok {
					t.Errorf("%s: job %q has a %q step without git.private_repos", relPath, jobName, name)
				}
			}
		}
	}
}
