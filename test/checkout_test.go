package test

import (
	"sort"
	"strings"
	"testing"
)

// checkoutStep is a generated actions/checkout step together with the workflow
// and job it belongs to.
type checkoutStep struct {
	Workflow string
	Job      string
	Uses     string
	With     map[string]any
}

// checkoutSteps collects every actions/checkout step of both generated
// workflows. parseWorkflow also asserts that the generated YAML is valid, so
// the conditional `with:` block of deploy-docs is covered for every
// documentation variant it is called with.
func checkoutSteps(t *testing.T, targetDir string) []checkoutStep {
	t.Helper()

	steps := []checkoutStep{}

	for _, name := range generatedWorkflowNames {
		wf := parseWorkflow(t, targetDir, name)

		for jobName, job := range wf.Jobs {
			for _, step := range job.Steps {
				if !strings.HasPrefix(step.Uses, "actions/checkout@") {
					continue
				}

				steps = append(steps, checkoutStep{
					Workflow: name,
					Job:      jobName,
					Uses:     step.Uses,
					With:     step.With,
				})
			}
		}
	}

	return steps
}

// checkoutJobs returns the sorted names of the jobs that check out the
// repository. Job maps iterate in random order, so the names are sorted to
// make the coverage assertion deterministic.
func checkoutJobs(steps []checkoutStep) []string {
	jobs := make([]string, 0, len(steps))
	for _, step := range steps {
		jobs = append(jobs, step.Job)
	}

	sort.Strings(jobs)

	return jobs
}

// assertCheckoutHardened verifies that a checkout step pins @v4 and drops the
// job token from .git/config.
func assertCheckoutHardened(t *testing.T, step checkoutStep) {
	t.Helper()

	if step.Uses != "actions/checkout@v4" {
		t.Errorf("%s: job %q uses %q, want \"actions/checkout@v4\"", step.Workflow, step.Job, step.Uses)
	}

	if got := step.With["persist-credentials"]; got != false {
		t.Errorf("%s: job %q persist-credentials = %#v, want false", step.Workflow, step.Job, got)
	}
}

// TestGenerateCheckoutPersistCredentials verifies that every generated
// actions/checkout step pins @v4 and sets persist-credentials: false, so the
// job token is not left in .git/config for the build steps that follow.
// private-token-secret builds both a container and a deb package, which is the
// config that emits all four ci_cd.yml checkouts.
func TestGenerateCheckoutPersistCredentials(t *testing.T) {
	tmpDir := generateProject(t, "private-token-secret")

	steps := checkoutSteps(t, tmpDir)

	// Guards against a template change that stops emitting a checkout and
	// would otherwise make the assertions below pass vacuously.
	wantJobs := []string{"build", "build-packages", "check-go-mod-version", "prepare-env", "run-goat-tests-api"}

	if got := checkoutJobs(steps); !equalStrings(got, wantJobs) {
		t.Errorf("jobs with a checkout step = %v, want %v", got, wantJobs)
	}

	for _, step := range steps {
		assertCheckoutHardened(t, step)
	}
}

// TestGenerateCheckoutDocsS3 verifies that deploy-docs drops the token too
// when the documentation is published to S3: the job only uploads the built
// site and never pushes to git.
func TestGenerateCheckoutDocsS3(t *testing.T) {
	tmpDir := generateProject(t, "docs-s3")

	steps := checkoutSteps(t, tmpDir)

	var deployDocs *checkoutStep

	for i, step := range steps {
		assertCheckoutHardened(t, step)

		if step.Job == "deploy-docs" {
			deployDocs = &steps[i]
		}
	}

	if deployDocs == nil {
		t.Fatal("ci_cd.yml has no deploy-docs checkout step, expected the documentation branch")
	}

	// fetch-depth is only needed by gh-deploy, which pushes a branch.
	if got, ok := deployDocs.With["fetch-depth"]; ok {
		t.Errorf("deploy-docs fetch-depth = %#v, want no fetch-depth input for the S3 variant", got)
	}
}

// TestGenerateCheckoutDocsGitHubPages verifies the one documented exception:
// with GitHub Pages, deploy-docs runs `mkdocs gh-deploy`, which pushes to the
// gh-pages branch with the checkout token, so that single step keeps the
// default persist-credentials. Every other checkout stays hardened.
func TestGenerateCheckoutDocsGitHubPages(t *testing.T) {
	tmpDir := generateProject(t, "docs-ghpages")

	steps := checkoutSteps(t, tmpDir)

	var deployDocs *checkoutStep

	for i, step := range steps {
		if step.Job == "deploy-docs" {
			deployDocs = &steps[i]

			continue
		}

		assertCheckoutHardened(t, step)
	}

	if deployDocs == nil {
		t.Fatal("ci_cd.yml has no deploy-docs checkout step, expected the documentation branch")
	}

	if deployDocs.Uses != "actions/checkout@v4" {
		t.Errorf("deploy-docs uses %q, want \"actions/checkout@v4\"", deployDocs.Uses)
	}

	// Setting persist-credentials: false here would break `gh-deploy`.
	if got, ok := deployDocs.With["persist-credentials"]; ok {
		t.Errorf("deploy-docs persist-credentials = %#v, want the action default for the GitHub Pages variant", got)
	}

	if got := deployDocs.With["fetch-depth"]; got != 0 {
		t.Errorf("deploy-docs fetch-depth = %#v, want 0", got)
	}
}

// equalStrings reports whether two string slices hold the same elements in the
// same order.
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}
