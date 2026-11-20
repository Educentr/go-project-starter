package test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// githubWorkflow is the subset of a GitHub Actions workflow the github_ci
// parameters affect.
type githubWorkflow struct {
	// On carries the workflow triggers. The key is quoted in the tag because
	// `on` is also a YAML 1.1 boolean; yaml.v3 reads it as the string key the
	// generated workflow means.
	On   githubTriggers       `yaml:"on"`
	Jobs map[string]githubJob `yaml:"jobs"`
}

// githubTriggers is the subset of the `on:` section the github_ci parameters
// affect.
type githubTriggers struct {
	Push githubPushTrigger `yaml:"push"`
}

// githubPushTrigger is the subset of the `on.push` trigger the github_ci
// parameters affect. The filters are decoded as []any, not []string, because
// GitHub wants them as strings and a filter the generator emitted unquoted
// resolves to whatever YAML makes of it (`1.10` to the float 1.1). Decoding
// into []string would convert such a value back to a string and hide it.
type githubPushTrigger struct {
	Branches []any `yaml:"branches"`
}

// githubJob is the subset of a workflow job the github_ci parameters affect.
type githubJob struct {
	RunsOn         any          `yaml:"runs-on"`
	TimeoutMinutes *int         `yaml:"timeout-minutes"`
	If             string       `yaml:"if"`
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

// branchVarsStepName is the name of the prepare-env step that resolves the
// per-branch names of the repository variables and secrets the build reads.
const branchVarsStepName = "Get branch"

// envFileStepName is the name of the deploy step that writes the env file of
// the branch being deployed.
const envFileStepName = "Create env file"

// notifyJobName is the job of ci_cd.yml that sends the build notification.
const notifyJobName = "artifact_version"

// branchSanitizePattern is the shell pattern both the `Get branch` step and
// the env file name derive from the branch name. Asserting on the pattern,
// rather than on a name produced by it, is what the env file step allows: its
// recipe is full of workflow expressions, so it cannot be executed in a test
// the way `Get branch` can.
const branchSanitizePattern = `[^A-Za-z0-9_]`

// generatedWorkflowNames lists the GitHub Actions workflows the generator
// produces; the github_ci parameters apply to every job of both.
var generatedWorkflowNames = []string{
	".github/workflows/ci_cd.yml",
	".github/workflows/go-version-check.yml",
}

// githubVarNameRx matches the characters GitHub allows in the name of a
// repository variable or secret. A name outside this set cannot exist, so a
// step that builds one sends the whole run down the "branch not enabled" path.
var githubVarNameRx = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

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

// assertPushBranches checks the `on.push.branches` filters of both generated
// workflows against want, each as the string it was configured as. The two
// templates render the list in different YAML styles (block in ci_cd.yml, flow
// in go-version-check.yml), so both are parsed back rather than compared as
// text.
func assertPushBranches(t *testing.T, targetDir string, want []string) {
	t.Helper()

	for _, name := range generatedWorkflowNames {
		got := parseWorkflow(t, targetDir, name).On.Push.Branches

		if len(got) != len(want) {
			t.Errorf("%s: on.push.branches = %#v, want %#v", name, got, want)

			continue
		}

		for i, wantBranch := range want {
			gotBranch, isStr := got[i].(string)
			if !isStr {
				t.Errorf("%s: on.push.branches[%d] = %#v (%T), want the string %q",
					name, i, got[i], got[i], wantBranch)

				continue
			}

			if gotBranch != wantBranch {
				t.Errorf("%s: on.push.branches[%d] = %q, want %q", name, i, gotBranch, wantBranch)
			}
		}
	}
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

	// Both workflows must keep the historical push trigger.
	assertPushBranches(t, tmpDir, []string{"main", "staging"})

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

	// Mirrors push_branches in github-ci/project.yaml, which carries a glob
	// pattern, an exclusion pattern and a branch name YAML resolves as a
	// float: each needs quoting in the generated YAML for a different reason,
	// so a round trip through the parser proves every filter survived as the
	// string it was configured as.
	assertPushBranches(t, tmpDir, []string{"main", "release/**", "!release/**-alpha", "1.10"})

	assertNotifyFollowsEnabledBuild(t, tmpDir)

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

// branchVarNames runs the `Get branch` step of the generated prepare-env job
// with GITHUB_REF set to ref and returns the variables it exported, keyed by
// name. The step is plain shell without workflow expressions, so executing it
// is what proves the suffix it derives for a branch name GitHub does not admit
// in a variable name (`release/1.2`); reading the YAML only shows the recipe.
func branchVarNames(t *testing.T, targetDir, ref string) map[string]string {
	t.Helper()

	wf := parseWorkflow(t, targetDir, ".github/workflows/ci_cd.yml")

	job, ok := wf.Jobs["prepare-env"]
	if !ok {
		t.Fatalf("ci_cd.yml has no prepare-env job, got jobs %v", wf.Jobs)
	}

	var step githubStep

	for _, s := range job.Steps {
		if s.Name == branchVarsStepName {
			step = s

			break
		}
	}

	// A `uses:` step cannot be it: the action used before
	// (noliran/branch-based-secrets) is exactly what pasted the branch name
	// into the variable name unchanged.
	if step.Run == "" || step.Uses != "" {
		t.Fatalf("prepare-env step %q = %#v, want a run step resolving the branch variable names",
			branchVarsStepName, step)
	}

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "get-branch.sh")
	envPath := filepath.Join(dir, "github_env")

	if err := os.WriteFile(scriptPath, []byte(step.Run), 0o600); err != nil {
		t.Fatalf("Error writing the step script: %v", err)
	}

	if err := os.WriteFile(envPath, nil, 0o600); err != nil {
		t.Fatalf("Error creating the GITHUB_ENV file: %v", err)
	}

	cmd := exec.Command("bash", scriptPath)

	cmd.Env = append(os.Environ(), "GITHUB_REF="+ref, "GITHUB_ENV="+envPath)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Error running the %q step for %s: %v\n%s", branchVarsStepName, ref, err, out)
	}

	content, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("Error reading the GITHUB_ENV file: %v", err)
	}

	vars := map[string]string{}

	for _, line := range strings.Split(strings.TrimSpace(string(content)), "\n") {
		if line == "" {
			continue
		}

		name, value, found := strings.Cut(line, "=")
		if !found {
			t.Fatalf("the %q step wrote %q to GITHUB_ENV, want NAME=VALUE", branchVarsStepName, line)
		}

		vars[name] = value
	}

	if len(vars) == 0 {
		t.Fatalf("the %q step exported nothing for %s\n%s", branchVarsStepName, ref, out)
	}

	return vars
}

// TestGenerateGithubCIBranchVarNames verifies that the names of the per-branch
// repository variables and secrets the generated prepare-env job reads are
// legal GitHub names for every branch github_ci.push_branches admits, and that
// main and staging keep the names existing projects already configured.
//
// Without it a branch with `/` or `-` in the name (`release/**`, the pattern
// the key exists for) asked for a variable that cannot exist, so the push
// triggered the workflow, every job was skipped and the run went green with
// nothing built.
func TestGenerateGithubCIBranchVarNames(t *testing.T) {
	tmpDir := generateProject(t, "github-ci")

	tests := []struct {
		ref    string
		suffix string
	}{
		{ref: "refs/heads/main", suffix: "MAIN"},
		{ref: "refs/heads/staging", suffix: "STAGING"},
		{ref: "refs/heads/release/1.2", suffix: "RELEASE_1_2"},
		{ref: "refs/heads/hotfix-x", suffix: "HOTFIX_X"},
		{ref: "refs/heads/feature/ABC-1_x", suffix: "FEATURE_ABC_1_X"},
	}

	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			vars := branchVarNames(t, tmpDir, tt.ref)

			// ENABLED is the variable that gates every job of ci_cd.yml, so it
			// is the one the whole run hangs on.
			if got, want := vars["ENABLED_NAME"], "ENABLED_"+tt.suffix; got != want {
				t.Errorf("ENABLED_NAME = %q, want %q", got, want)
			}

			for name, value := range vars {
				if !githubVarNameRx.MatchString(value) {
					t.Errorf("%s = %q, which GitHub cannot have as a variable name", name, value)
				}

				if !strings.HasSuffix(value, "_"+tt.suffix) {
					t.Errorf("%s = %q, want the branch suffix %q", name, value, tt.suffix)
				}
			}
		})
	}

	assertEnvFileNameSanitized(t, tmpDir)
}

// assertNotifyFollowsEnabledBuild checks that the notification job reports
// every branch whose build is enabled instead of a hardcoded pair of branch
// names: it is gated on the same ENABLE_BUILD output as the build itself, and
// no step narrows that down to a branch literal. A literal `main`/`staging`
// check would leave a branch added through github_ci.push_branches building
// without any notification.
func assertNotifyFollowsEnabledBuild(t *testing.T, targetDir string) {
	t.Helper()

	wf := parseWorkflow(t, targetDir, ".github/workflows/ci_cd.yml")

	job, ok := wf.Jobs[notifyJobName]
	if !ok {
		t.Fatalf("ci_cd.yml has no %s job, got jobs %v", notifyJobName, wf.Jobs)
	}

	if !strings.Contains(job.If, "ENABLE_BUILD") {
		t.Errorf("job %s if = %q, want it gated on the ENABLE_BUILD output", notifyJobName, job.If)
	}

	for _, step := range job.Steps {
		for _, branch := range []string{"main", "staging"} {
			if strings.Contains(step.Run, `"`+branch+`"`) {
				t.Errorf("job %s step %q compares the branch to the literal %q, want it to follow ENABLE_BUILD",
					notifyJobName, step.Name, branch)
			}
		}
	}
}

// assertEnvFileNameSanitized checks that the deploy step builds the env file
// name out of a sanitized branch name. With the raw name a branch containing
// `/` turned `.env.` into a path to a directory that does not exist, and the
// step died on its first redirect.
func assertEnvFileNameSanitized(t *testing.T, targetDir string) {
	t.Helper()

	wf := parseWorkflow(t, targetDir, ".github/workflows/ci_cd.yml")

	for jobName, job := range wf.Jobs {
		for _, step := range job.Steps {
			if step.Name != envFileStepName {
				continue
			}

			if !strings.Contains(step.Run, branchSanitizePattern) {
				t.Errorf("job %q step %q builds the env file name without sanitizing the branch (%s): %s",
					jobName, envFileStepName, branchSanitizePattern, step.Run)
			}

			return
		}
	}

	t.Fatalf("ci_cd.yml has no %q step, expected it in the deploy job", envFileStepName)
}
