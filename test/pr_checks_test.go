package test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// prChecksConfig is the fixture the pull-request workflow is asserted on: it
// declares three checks, one of them needing a registry login, plus a runner
// and a job timeout — so the workflow covers both the gate and the parts the
// github_ci parameters reach.
const prChecksConfig = "ci-checks"

// checksJobName is the job both workflows run the declared checks in.
const checksJobName = "checks"

// lintBaseRevEnv is the make variable `make lint` takes its comparison
// revision from, and which the pull-request job overrides.
const lintBaseRevEnv = "LINT_BASE_REV"

// TestGeneratePRChecksRunsTheChecks verifies that the generated pull-request
// workflow runs the declared checks, with the same matrix as the push one.
//
// Before it existed, github_ci.checks ran on push only, so a project learnt
// that its linter was red after the merge — which is why projects kept a
// hand-written pull-request workflow next to the generated CI.
func TestGeneratePRChecksRunsTheChecks(t *testing.T) {
	targetDir := generateProject(t, prChecksConfig)

	pr := parseWorkflow(t, targetDir, prChecksWorkflow)

	job, ok := pr.Jobs[checksJobName]
	if !ok {
		t.Fatalf("%s has no %s job, only %v", prChecksWorkflow, checksJobName, jobNames(pr))
	}

	// The whole file is the checks: nothing to build, publish or deploy on a
	// pull request, and therefore no job to switch off one `if:` at a time.
	if names := jobNames(pr); len(names) != 1 {
		t.Errorf("%s declares %v, want the %s job alone", prChecksWorkflow, names, checksJobName)
	}

	// The gate: vars.SKIP_CHECKS, the project's own switch, and nothing else.
	// A condition on needs.prepare-env.outputs.ENABLE_BUILD would resolve to
	// null here (GITHUB_REF is refs/pull/<N>/merge, so the branch variable it
	// comes from cannot exist) and the job would report `skipped` on a green
	// run — a pull request with no check in it.
	if !strings.Contains(job.If, "vars.SKIP_CHECKS != 'true'") {
		t.Errorf("%s: job %s if = %q, want the SKIP_CHECKS switch in it", prChecksWorkflow, checksJobName, job.If)
	}

	if strings.Contains(job.If, "needs.") {
		t.Errorf("%s: job %s if = %q, want no needs context: on a pull request it resolves to null "+
			"and the job is skipped on a green run", prChecksWorkflow, checksJobName, job.If)
	}

	// The matrix legs are the push ones, leg for leg: a pull-request check
	// that stops matching the push check reports green on code the merge then
	// rejects.
	pushJob, ok := parseWorkflow(t, targetDir, ciCDWorkflow).Jobs[checksJobName]
	if !ok {
		t.Fatalf("%s has no %s job", ciCDWorkflow, checksJobName)
	}

	if got, want := checkLegs(t, job), checkLegs(t, pushJob); !slices.Equal(got, want) {
		t.Errorf("%s runs %v, %s runs %v — the two must run the same checks",
			prChecksWorkflow, got, ciCDWorkflow, want)
	}

	// The github_ci parameters apply to this job like any other.
	if got := job.RunsOn; got != "self-hosted" {
		t.Errorf("%s: job %s runs-on = %#v, want the configured runner", prChecksWorkflow, checksJobName, got)
	}

	if job.TimeoutMinutes == nil || *job.TimeoutMinutes != 25 {
		t.Errorf("%s: job %s timeout-minutes = %v, want the configured timeout",
			prChecksWorkflow, checksJobName, job.TimeoutMinutes)
	}
}

// TestGeneratePRChecksTrigger verifies the trigger: pull requests into the
// branches whose CI the project declared (github_ci.push_branches), and no
// push trigger, which would duplicate every run of ci_cd.yml.
func TestGeneratePRChecksTrigger(t *testing.T) {
	content := readGenerated(t, generateProject(t, prChecksConfig), filepath.FromSlash(prChecksWorkflow))

	if !strings.Contains(content, "on:\n  pull_request:\n    branches:\n      - main\n      - staging\n") {
		t.Errorf("%s does not trigger on pull requests into the declared branches:\n%s", prChecksWorkflow, content)
	}

	if strings.Contains(content, "\n  push:\n") {
		t.Errorf("%s also triggers on push, which duplicates every run of %s", prChecksWorkflow, ciCDWorkflow)
	}
}

// TestGeneratePRChecksLintBase verifies the revision `make lint` compares
// against on a pull request.
//
// The Makefile default is origin/main, that is main as it is at the moment of
// the run: once main moves, a re-run of an open pull request reports lines the
// pull request never touched. The base of the pull request is the commit it is
// merged into, so that is what the job hands the Makefile — through the
// environment, which is both where a value belongs and what make reads with
// `?=`.
func TestGeneratePRChecksLintBase(t *testing.T) {
	targetDir := generateProject(t, prChecksConfig)

	job := parseWorkflow(t, targetDir, prChecksWorkflow).Jobs[checksJobName]

	if got, want := job.Env[lintBaseRevEnv], "${{ github.event.pull_request.base.sha }}"; got != want {
		t.Errorf("%s: job %s env.%s = %q, want %q", prChecksWorkflow, checksJobName, lintBaseRevEnv, got, want)
	}

	// The variable has to be the one the recipe reads, and overridable from
	// the environment (`?=`), or the job sets something nothing uses.
	makefile := readGenerated(t, targetDir, "Makefile")

	if !strings.Contains(makefile, lintBaseRevEnv+" ?= origin/main") {
		t.Errorf("the generated Makefile does not default %s to origin/main", lintBaseRevEnv)
	}

	if !strings.Contains(makefile, "--new-from-rev=$("+lintBaseRevEnv+")") {
		t.Errorf("the generated `make lint` does not compare against $(%s)", lintBaseRevEnv)
	}

	// The push workflow keeps the default, so its output is the one it had.
	if env := parseWorkflow(t, targetDir, ciCDWorkflow).Jobs[checksJobName].Env; env[lintBaseRevEnv] != "" {
		t.Errorf("%s: job %s sets %s = %q; on push the Makefile default is the right base",
			ciCDWorkflow, checksJobName, lintBaseRevEnv, env[lintBaseRevEnv])
	}

	// And the base has to be reachable: --new-from-rev resolves the commit in
	// the checked-out history.
	for _, step := range job.Steps {
		if strings.HasPrefix(step.Uses, "actions/checkout@") {
			if got := step.With["fetch-depth"]; got != 0 {
				t.Errorf("%s: job %s checks out with fetch-depth %#v, want the full history so the "+
					"base commit of the pull request resolves", prChecksWorkflow, checksJobName, got)
			}
		}
	}
}

// TestGeneratePRChecksRegistryLoginSurvivesAFork verifies that the registry
// login steps of the pull-request job require the credential to be present.
//
// A pull request from a fork gets empty secrets, and docker/login-action with
// an empty password fails the leg before the check it was supposed to guard
// ever runs. With the guard, a leg that genuinely needs the registry still
// fails — on its own command, with its own message.
func TestGeneratePRChecksRegistryLoginSurvivesAFork(t *testing.T) {
	targetDir := generateProject(t, prChecksConfig)

	prLogins := registryLoginConditions(parseWorkflow(t, targetDir, prChecksWorkflow).Jobs[checksJobName])
	if len(prLogins) == 0 {
		t.Fatalf("%s renders no registry login step; %s declares a check that needs one",
			prChecksWorkflow, prChecksConfig)
	}

	for _, condition := range prLogins {
		if !strings.Contains(condition, "env.REGISTRY_CREDENTIAL != ''") {
			t.Errorf("%s: a registry login step runs when %q, which fails the whole leg on a "+
				"pull request from a fork", prChecksWorkflow, condition)
		}
	}

	// On push the condition is the matrix leg alone: a push carries the
	// repository's secrets by definition, and a login silently skipped there
	// would turn a missing secret into a red check with a puzzling message.
	for _, condition := range registryLoginConditions(parseWorkflow(t, targetDir, ciCDWorkflow).Jobs[checksJobName]) {
		if strings.Contains(condition, "REGISTRY_CREDENTIAL") {
			t.Errorf("%s: a registry login step runs when %q; on push the matrix leg decides alone",
				ciCDWorkflow, condition)
		}
	}
}

// TestGeneratePRChecksAbsentWithoutChecks verifies the other half: a project
// that declares no checks gets no pull-request workflow at all, so its output
// is unchanged.
func TestGeneratePRChecksAbsentWithoutChecks(t *testing.T) {
	targetDir := generateFromConfig(t, "rest-only", "Generate project without declared checks")

	switch _, err := os.Stat(filepath.Join(targetDir, prChecksWorkflow)); {
	case err == nil:
		t.Errorf("a project that declares no checks gets %s, a workflow with an empty matrix",
			prChecksWorkflow)
	case !os.IsNotExist(err):
		t.Fatalf("Error looking for %s: %v", prChecksWorkflow, err)
	}
}

// jobNames returns the sorted job names of a workflow.
func jobNames(wf githubWorkflow) []string {
	names := make([]string, 0, len(wf.Jobs))
	for name := range wf.Jobs {
		names = append(names, name)
	}

	slices.Sort(names)

	return names
}

// checkLegs returns the declared checks of a checks job as "<name>: <command>"
// lines, sorted, so the two workflows can be compared leg for leg.
func checkLegs(t *testing.T, job githubJob) []string {
	t.Helper()

	legs := make([]string, 0, len(job.Strategy.Matrix.Include))

	for _, leg := range job.Strategy.Matrix.Include {
		legs = append(legs, leg.Name+": "+leg.Run)
	}

	slices.Sort(legs)

	return legs
}

// registryLoginConditions returns the `if:` of every registry login step of a
// job — the steps conditioned on the matrix leg needing the registry.
func registryLoginConditions(job githubJob) []string {
	conditions := []string{}

	for _, step := range job.Steps {
		if strings.Contains(step.If, "matrix.registry_login") {
			conditions = append(conditions, step.If)
		}
	}

	return conditions
}
