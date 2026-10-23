package test

import (
	"os"
	"path/filepath"
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

// generatedWorkflowNames lists the GitHub Actions workflows the generator
// produces; the github_ci parameters apply to every job of both.
var generatedWorkflowNames = []string{
	".github/workflows/ci_cd.yml",
	".github/workflows/go-version-check.yml",
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
	wf := parseWorkflow(t, tmpDir, ".github/workflows/ci_cd.yml")

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

	wf := parseWorkflow(t, tmpDir, ".github/workflows/ci_cd.yml")

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
	versions := doctlVersions(parseWorkflow(t, tmpDir, ".github/workflows/ci_cd.yml"))
	if len(versions) == 0 {
		t.Fatal("ci_cd.yml has no digitalocean/action-doctl steps, expected the digitalocean registry branch")
	}

	for i, v := range versions {
		if v != "1.170.0" {
			t.Errorf("action-doctl step %d: version = %#v, want \"1.170.0\"", i, v)
		}
	}
}
