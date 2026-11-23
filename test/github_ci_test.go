package test

import (
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
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
// Env is decoded because a job-level variable is part of the environment its
// steps run with: a step executed in a test without it runs a different
// script than the runner does.
type githubJob struct {
	RunsOn         any               `yaml:"runs-on"`
	TimeoutMinutes *int              `yaml:"timeout-minutes"`
	If             string            `yaml:"if"`
	Env            map[string]string `yaml:"env"`
	Steps          []githubStep      `yaml:"steps"`
}

// githubStep is the subset of a job step the github_ci parameters affect.
type githubStep struct {
	Name string            `yaml:"name"`
	Uses string            `yaml:"uses"`
	With map[string]any    `yaml:"with"`
	Env  map[string]string `yaml:"env"`
	Run  string            `yaml:"run"`
}

// exprStubs resolves a workflow expression to the value the runner would
// substitute for it. Exact matches come first; the patterns cover the
// expressions whose names depend on the project (one PORT_PREFIX per
// application and transport), which an exact table would have to repeat per
// config.
type exprStubs struct {
	exact    map[string]string
	patterns []exprStubPattern
}

// exprStubPattern is a stub for every expression matching rx.
type exprStubPattern struct {
	rx    *regexp.Regexp
	value string
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

// notifyStepName is the step of the notify job that composes the Telegram
// message and sends it.
const notifyStepName = "Check conditions and set message"

// curlArgSeparator separates the arguments the curl stub of the notify step
// records, one marker line after each.
const curlArgSeparator = "===CURL-ARG==="

// markdownV2Specials are the characters Telegram reserves in the MarkdownV2
// parse mode the notify step sends the message with: an unescaped one makes
// Telegram answer 400 instead of delivering the message.
const markdownV2Specials = "_*[]()~`>#+-=|{}.!"

// branchSanitizePattern is the shell pattern both the `Get branch` step and
// the env file name derive from the branch name.
const branchSanitizePattern = `[^A-Za-z0-9_]`

// injectionMarkerFile is the file the hostile branch names of the injection
// tests try to create. Its absence after the step is what says the branch name
// stayed a value instead of becoming a command.
const injectionMarkerFile = "pwned"

// envTypeStub is the deploy environment type the stubbed `Create env file`
// step resolves, which the step appends to the env file name.
const envTypeStub = "prod"

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

// githubVarNameRx matches the characters GitHub allows in the name of a
// repository variable or secret. A name outside this set cannot exist, so a
// step that builds one sends the whole run down the "branch not enabled" path.
var githubVarNameRx = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

// notifyBranchRx captures the branch name as the notify step rendered it into
// the message. The greedy group stops at the last `*` of the line, which is
// the one closing the bold branch name.
var notifyBranchRx = regexp.MustCompile(`(?m)на ветке \*(.*)\*\s*$`)

// workflowExprRx matches a GitHub Actions expression, which the runner
// substitutes before the shell ever sees the recipe.
var workflowExprRx = regexp.MustCompile(`\$\{\{\s*([^{}]+?)\s*\}\}`)

// injectableRunExprRx matches the workflow contexts whose value carries text
// an outside party chooses: the branch or tag name, the pull request head, the
// event payload. In `run:` the runner substitutes them before the shell
// starts, so the value becomes part of the script source — `release/$(cmd)`
// runs a command in a job that holds the repository secrets, and a name with a
// backtick or a quote kills the step with a syntax error. Such a value belongs
// in `env:`, where it stays a value.
var injectableRunExprRx = regexp.MustCompile(`github\.(ref_name|head_ref|ref|event)\b`)

// branchSanitizeRx is branchSanitizePattern as the test applies it, to derive
// the name the steps build out of a branch name.
var branchSanitizeRx = regexp.MustCompile(branchSanitizePattern)

// hostileBranchNames are branch names `git check-ref-format` accepts and the
// shell reads as code: a command substitution, a backtick command and the
// unbalanced backtick and quote that break the script outright.
var hostileBranchNames = []string{
	"x$(touch " + injectionMarkerFile + ")",
	"x`touch " + injectionMarkerFile + "`",
	"a`b",
	`a"b`,
	"a;b",
}

// lookup returns the stubbed value of an expression.
func (s exprStubs) lookup(expr string) (string, bool) {
	if value, ok := s.exact[expr]; ok {
		return value, true
	}

	for _, p := range s.patterns {
		if p.rx.MatchString(expr) {
			return p.value, true
		}
	}

	return "", false
}

// runnerStubs are the values a runner would substitute for the workflow
// expressions of the executed steps, for a push of branch with the build
// ending in buildResult.
func runnerStubs(branch, buildResult string) exprStubs {
	return exprStubs{
		exact: map[string]string{
			"github.ref_name":                  branch,
			"github.ref":                       "refs/heads/" + branch,
			"github.repository":                "Educentr/github-ci-test",
			"github.workflow":                  "CI Pipeline",
			"github.run_id":                    "42",
			"github.event.head_commit.message": "fix: a commit",
			"secrets.TELEGRAM_BOT_TOKEN":       "stub-token",
			"secrets.TELEGRAM_CHAT_ID":         "1",
			"secrets.REGISTRY_LOGIN_SERVER":    "registry.test",
			"vars.REGISTRY_CONTAINER":          "github-ci-test",
			"needs.build.result":               buildResult,
			"needs.build-packages.result":      buildResult,

			"needs.prepare-env.outputs.ARTIFACTS":                       `["registry/github-ci-test:1.0"]`,
			"needs.prepare-env.outputs.PACKAGES":                        `["github-ci-test-1.0.rpm"]`,
			"needs.prepare-env.outputs.OC_HOST":                         "oc.test",
			"needs.prepare-env.outputs.OC_PORT":                         "8080",
			"needs.prepare-env.outputs.RELEASE":                         "1.0.0",
			"needs.prepare-env.outputs.INTERNAL_SUBNET":                 "10.1.0.0/24",
			"fromJSON(needs.prepare-env.outputs.ENV_TYPE)[matrix.envs]": envTypeStub,
		},
		patterns: []exprStubPattern{
			{
				rx:    regexp.MustCompile(`^fromJson\(needs\.prepare-env\.outputs\.PORT_PREFIX_[A-Za-z0-9_]+\)\[matrix\.envs\]$`),
				value: "31",
			},
			{
				rx:    regexp.MustCompile(`^fromJson\(needs\.prepare-env\.outputs\.DOMAIN_[A-Za-z0-9_]+\)\[matrix\.envs\]$`),
				value: "api.test",
			},
			{
				rx:    regexp.MustCompile(`^fromJson\(secrets\[needs\.prepare-env\.outputs\.OC_(USER|PASSWORD)\]\)\[matrix\.envs\]$`),
				value: "oc-secret",
			},
		},
	}
}

// substituteWorkflowExprs replaces every workflow expression in text with the
// value the runner would substitute, so the text can be used as what it
// becomes on a runner. An expression without a stub fails the test: a silently
// empty substitution would hide whatever the step does with that value.
func substituteWorkflowExprs(t *testing.T, what, text string, stubs exprStubs) string {
	t.Helper()

	var unknown []string

	substituted := workflowExprRx.ReplaceAllStringFunc(text, func(match string) string {
		expr := workflowExprRx.FindStringSubmatch(match)[1]

		value, ok := stubs.lookup(expr)
		if !ok {
			unknown = append(unknown, expr)
		}

		return value
	})

	if len(unknown) != 0 {
		t.Fatalf("%s uses the workflow expressions %v the test has no stub for", what, unknown)
	}

	return substituted
}

// stepShellEnv renders the environment a step runs with — the `env:` of its
// job plus its own, the step's winning — into the KEY=VALUE entries the shell
// gets, with the same expression substitution the runner does.
//
// Reading the `env:` blocks is what keeps the executed script the one the
// runner runs: a value moved out of `run:` into `env:` disappears from the
// recipe, so a test that only executed the recipe would run it with the
// variable unset and pass without asserting anything.
func stepShellEnv(t *testing.T, what string, job githubJob, step githubStep, stubs exprStubs) []string {
	t.Helper()

	vars := map[string]string{}

	for name, value := range job.Env {
		vars[name] = substituteWorkflowExprs(t, what+" job env "+name, value, stubs)
	}

	for name, value := range step.Env {
		vars[name] = substituteWorkflowExprs(t, what+" step env "+name, value, stubs)
	}

	env := make([]string, 0, len(vars))

	for name, value := range vars {
		env = append(env, name+"="+value)
	}

	slices.Sort(env)

	return env
}

// findStepByName returns the job that carries the named step and the step
// itself. The step is looked up by name across jobs because the job it lives
// in is itself conditional on the project's artifacts.
func findStepByName(t *testing.T, targetDir, stepName string) (string, githubJob, githubStep) {
	t.Helper()

	wf := parseWorkflow(t, targetDir, ciCDWorkflow)

	for _, jobName := range slices.Sorted(maps.Keys(wf.Jobs)) {
		job := wf.Jobs[jobName]

		for _, step := range job.Steps {
			if step.Name == stepName {
				return jobName, job, step
			}
		}
	}

	t.Fatalf("ci_cd.yml has no %q step, got jobs %v", stepName, slices.Sorted(maps.Keys(wf.Jobs)))

	return "", githubJob{}, githubStep{}
}

// sanitizeBranch is what both the `Get branch` step and the env file name do
// to a branch name before using it.
func sanitizeBranch(branch string) string {
	return branchSanitizeRx.ReplaceAllString(branch, "_")
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

	// Mirrors push_branches in github-ci/project.yaml, which carries a glob
	// pattern, an exclusion pattern and a branch name YAML resolves as a
	// float: each needs quoting in the generated YAML for a different reason,
	// so a round trip through the parser proves every filter survived as the
	// string it was configured as.
	assertPushBranches(t, tmpDir, []string{"main", "release/**", "!release/**-alpha", "1.10"})

	assertNotifyFollowsEnabledBuild(t, tmpDir)

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

// branchVarNames runs the `Get branch` step of the generated prepare-env job
// with GITHUB_REF set to ref and returns the variables it exported, keyed by
// name. The step is plain shell without workflow expressions, so executing it
// is what proves the suffix it derives for a branch name GitHub does not admit
// in a variable name (`release/1.2`); reading the YAML only shows the recipe.
func branchVarNames(t *testing.T, targetDir, ref string) map[string]string {
	t.Helper()

	wf := parseWorkflow(t, targetDir, ciCDWorkflow)

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

	wf := parseWorkflow(t, targetDir, ciCDWorkflow)

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

	wf := parseWorkflow(t, targetDir, ciCDWorkflow)

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

// escapeMarkdownV2 is what the generated step's sed does to a value before
// pasting it into the message.
func escapeMarkdownV2(s string) string {
	var b strings.Builder

	for _, r := range s {
		if strings.ContainsRune(markdownV2Specials, r) {
			b.WriteRune('\\')
		}

		b.WriteRune(r)
	}

	return b.String()
}

// notifyStepScript returns the recipe of the notify step and the environment
// it runs with, both with every workflow expression replaced by the value the
// runner would substitute, so the step can be executed the way a runner
// executes it.
func notifyStepScript(t *testing.T, targetDir, branch, buildResult string) (string, []string) {
	t.Helper()

	jobName, job, step := findStepByName(t, targetDir, notifyStepName)

	if jobName != notifyJobName {
		t.Errorf("the %q step is in job %q, want it in %q", notifyStepName, jobName, notifyJobName)
	}

	if step.Run == "" {
		t.Fatalf("job %s has no recipe in its %q step, got %#v", jobName, notifyStepName, step)
	}

	stubs := runnerStubs(branch, buildResult)
	what := "the " + notifyStepName + " step"

	return substituteWorkflowExprs(t, what, step.Run, stubs), stepShellEnv(t, what, job, step, stubs)
}

// notifyMessage runs the notify step for a branch and returns the message text
// it handed to curl. Executing the step is what proves the message Telegram
// receives: reading the YAML only shows the recipe, and the recipe is where a
// value gets pasted into MarkdownV2 unescaped.
func notifyMessage(t *testing.T, targetDir, branch, buildResult string) string {
	t.Helper()

	script, stepEnv := notifyStepScript(t, targetDir, branch, buildResult)

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "notify.sh")
	argsPath := filepath.Join(dir, "curl-args")
	binDir := filepath.Join(dir, "bin")

	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatalf("Error creating the stub bin directory: %v", err)
	}

	if err := os.WriteFile(scriptPath, []byte(script), 0o600); err != nil {
		t.Fatalf("Error writing the step script: %v", err)
	}

	// The stub stands in for curl so the step's request is captured instead of
	// sent. The arguments are separated by a marker line rather than by a
	// newline: the message is itself multi-line.
	curlStub := "#!/bin/sh\nfor arg in \"$@\"; do printf '%s\\n" + curlArgSeparator + "\\n' \"$arg\" >> \"$CURL_ARGS_FILE\"; done\n"
	if err := os.WriteFile(filepath.Join(binDir, "curl"), []byte(curlStub), 0o700); err != nil {
		t.Fatalf("Error writing the curl stub: %v", err)
	}

	cmd := exec.Command("bash", scriptPath)

	// The script runs in its own directory so that anything a hostile branch
	// name manages to create lands where the test looks for it.
	cmd.Dir = dir

	// The `env:` of the step and its job carry everything the step itself
	// reads — including the secrets it gives up early without, substituted by
	// runnerStubs — so the test adds only what the runner provides around the
	// step.
	stepEnv = append(stepEnv,
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"CURL_ARGS_FILE="+argsPath,
		"GITHUB_ENV="+filepath.Join(dir, "github_env"),
	)

	cmd.Env = append(os.Environ(), stepEnv...)

	out, err := cmd.CombinedOutput()

	// Checked before the exit status: a branch name that executed is the worse
	// failure of the two, and it can make the step fail as well.
	assertNoInjectionMarker(t, dir, notifyStepName, branch)

	if err != nil {
		t.Fatalf("Error running the %q step for %s: %v\n%s", notifyStepName, branch, err, out)
	}

	args, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("the %q step sent no request for %s: %v\n%s", notifyStepName, branch, err, out)
	}

	for _, arg := range strings.Split(string(args), "\n"+curlArgSeparator+"\n") {
		if text, found := strings.CutPrefix(arg, "text="); found {
			return text
		}
	}

	t.Fatalf("the %q step sent no text for %s, got the arguments:\n%s", notifyStepName, branch, args)

	return ""
}

// TestGenerateGithubCINotifyEscapesBranch verifies that the branch name
// reaches the Telegram message escaped for MarkdownV2, the parse mode the
// notify step sends it with.
//
// Without it a branch carrying a reserved character — which `release/1.2`,
// `hotfix-x` and `feature_x` all do, and which only became reachable once the
// notify job stopped being limited to main and staging — made Telegram answer
// 400 and drop the message, invisibly: `curl -s` does not see that error, so
// the job stayed green with no notification sent.
func TestGenerateGithubCINotifyEscapesBranch(t *testing.T) {
	tmpDir := generateProject(t, "github-ci")

	// Both outcomes are checked because the message is composed twice, once
	// per branch of the status check, and each pastes the branch name itself.
	for _, buildResult := range []string{"success", "failure"} {
		for _, branch := range []string{"main", "release/1.2", "hotfix-x", "feature_x", "1.10"} {
			t.Run(buildResult+"/"+branch, func(t *testing.T) {
				message := notifyMessage(t, tmpDir, branch, buildResult)

				match := notifyBranchRx.FindStringSubmatch(message)
				if match == nil {
					t.Fatalf("the message names no branch, got:\n%s", message)
				}

				if got, want := match[1], escapeMarkdownV2(branch); got != want {
					t.Errorf("the message names the branch as %q, want %q (MarkdownV2 escaped)\n%s",
						got, want, message)
				}
			})
		}
	}
}

// assertNoInjectionMarker checks that an executed step left no marker file
// behind, which is what a branch name reaching the shell as code creates.
func assertNoInjectionMarker(t *testing.T, dir, stepName, branch string) {
	t.Helper()

	if _, err := os.Stat(filepath.Join(dir, injectionMarkerFile)); err == nil {
		t.Fatalf("the %q step executed the branch name %q: it created %s",
			stepName, branch, injectionMarkerFile)
	}
}

// envFileName runs the `Create env file` step for a branch and returns the
// name of the env file it wrote, read back from the step output the upload
// step takes the path from.
//
// The step is executed, not read, because the branch name is only dangerous
// once a shell sees it, and only an executed step shows whether the name
// reached the script at all.
func envFileName(t *testing.T, targetDir, branch string) string {
	t.Helper()

	jobName, job, step := findStepByName(t, targetDir, envFileStepName)

	if step.Run == "" {
		t.Fatalf("job %s has no recipe in its %q step, got %#v", jobName, envFileStepName, step)
	}

	stubs := runnerStubs(branch, "success")
	what := "the " + envFileStepName + " step"
	script := substituteWorkflowExprs(t, what, step.Run, stubs)
	stepEnv := stepShellEnv(t, what, job, step, stubs)

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "create-env-file.sh")
	outputPath := filepath.Join(dir, "github_output")

	if err := os.WriteFile(scriptPath, []byte(script), 0o600); err != nil {
		t.Fatalf("Error writing the step script: %v", err)
	}

	if err := os.WriteFile(outputPath, nil, 0o600); err != nil {
		t.Fatalf("Error creating the GITHUB_OUTPUT file: %v", err)
	}

	cmd := exec.Command("bash", scriptPath)

	// The step writes the env file into the working directory, which is also
	// where anything a hostile branch name creates lands.
	cmd.Dir = dir

	stepEnv = append(stepEnv, "GITHUB_OUTPUT="+outputPath)
	cmd.Env = append(os.Environ(), stepEnv...)

	out, err := cmd.CombinedOutput()

	assertNoInjectionMarker(t, dir, envFileStepName, branch)

	if err != nil {
		t.Fatalf("Error running the %q step for %s: %v\n%s", envFileStepName, branch, err, out)
	}

	output, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Error reading the GITHUB_OUTPUT file: %v", err)
	}

	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if fname, found := strings.CutPrefix(line, "FNAME="); found {
			if _, err := os.Stat(filepath.Join(dir, fname)); err != nil {
				t.Fatalf("the %q step reported %s but wrote no such file: %v\n%s",
					envFileStepName, fname, err, out)
			}

			return fname
		}
	}

	t.Fatalf("the %q step reported no FNAME for %s, got the output:\n%s\n%s",
		envFileStepName, branch, output, out)

	return ""
}

// TestGenerateGithubCIEnvFileBranchNotInjected verifies that the deploy step
// takes the branch name as a value: it names the env file after every branch
// `git check-ref-format` accepts, including the names that read as shell code.
//
// With the name pasted into the recipe through a workflow expression, a branch
// like `x$(touch pwned)` ran a command in the job and one with an unbalanced
// backtick killed the step with a syntax error (GitHub Security hardening →
// script injection).
func TestGenerateGithubCIEnvFileBranchNotInjected(t *testing.T) {
	tmpDir := generateProject(t, "github-ci")

	// The ordinary names come first: the same assertion also says the file
	// name of an existing deploy did not change.
	branches := append([]string{"main", "release/1.2", "hotfix-x", "feature_x"}, hostileBranchNames...)

	for _, branch := range branches {
		t.Run(branch, func(t *testing.T) {
			want := ".env." + sanitizeBranch(branch) + "-" + envTypeStub

			if got := envFileName(t, tmpDir, branch); got != want {
				t.Errorf("the %q step named the env file %q, want %q", envFileStepName, got, want)
			}
		})
	}
}

// TestGenerateGithubCINotifyBranchNotInjected verifies that the notify step
// takes the branch name as a value: it reports every branch name
// `git check-ref-format` accepts, including the names that read as shell code,
// and runs none of them.
func TestGenerateGithubCINotifyBranchNotInjected(t *testing.T) {
	tmpDir := generateProject(t, "github-ci")

	// Both outcomes are checked because the message is composed twice, once
	// per branch of the status check, and each pastes the branch name itself.
	for _, buildResult := range []string{"success", "failure"} {
		for _, branch := range hostileBranchNames {
			t.Run(buildResult+"/"+branch, func(t *testing.T) {
				message := notifyMessage(t, tmpDir, branch, buildResult)

				match := notifyBranchRx.FindStringSubmatch(message)
				if match == nil {
					t.Fatalf("the message names no branch, got:\n%s", message)
				}

				if got, want := match[1], escapeMarkdownV2(branch); got != want {
					t.Errorf("the message names the branch as %q, want %q (MarkdownV2 escaped)\n%s",
						got, want, message)
				}
			})
		}
	}
}

// TestGenerateGithubCINoInjectableRunExpr verifies that no `run:` recipe of
// the generated workflows substitutes a workflow context whose value an
// outside party chooses. Such a value has to travel through `env:`, where the
// shell reads it as data; in a recipe it is source code (GitHub Security
// hardening → script injection).
//
// The test is the one that catches the pattern coming back: every future step
// that pastes a branch name, a tag name or an event payload into a script
// fails it, whether or not the test suite executes that step.
func TestGenerateGithubCINoInjectableRunExpr(t *testing.T) {
	// github-ci enables the most jobs of ci_cd.yml (build, deploy, env files,
	// goat tests, notification), so it renders every recipe the other configs
	// render a subset of.
	tmpDir := generateProject(t, "github-ci")

	for _, name := range generatedWorkflowNames {
		wf := parseWorkflow(t, tmpDir, name)

		for _, jobName := range slices.Sorted(maps.Keys(wf.Jobs)) {
			for _, step := range wf.Jobs[jobName].Steps {
				for _, match := range workflowExprRx.FindAllStringSubmatch(step.Run, -1) {
					if ctx := injectableRunExprRx.FindString(match[1]); ctx != "" {
						t.Errorf("%s: job %s step %q runs a script with ${{ %s }} in it (%s): pass the value through env: instead",
							name, jobName, step.Name, match[1], ctx)
					}
				}
			}
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
