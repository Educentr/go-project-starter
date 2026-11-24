package test

import (
	"maps"
	"regexp"
	"slices"
	"testing"
)

// deployJob is the GitHub job that ships the docker artifacts, and
// uploadPackagesJob is the one that pushes the built packages to their
// storage. deploy waits for the upload where a project has both artifacts,
// which is what ties the two together in TestDeployIsNotSkippedByAnUnreachableUpload.
const (
	deployJob         = "deploy"
	uploadPackagesJob = "upload-packages"
)

// dockerUploadConfig is the fixture carrying both a docker artifact and a
// package upload — the combination where an unreachable upload-packages took
// the whole deployment down with it (TOOLS-130). The package-upload-* fixtures
// ship a deb alone, so they render no deploy job to lose.
const dockerUploadConfig = "package-upload-docker"

// needsRefRx matches a read of the needs context. Only the job name is
// captured: the context holds nothing but the jobs listed in the reading
// job's own `needs:`, so whether the expression goes on to ask for `.outputs`,
// `.result` or the whole object makes no difference to whether it resolves.
var needsRefRx = regexp.MustCompile(`needs\.([A-Za-z0-9_-]+)`)

// needsSweepConfigs are the fixtures the needs sweep runs over. Every `needs:`
// list in the template is conditional, so the shape being checked exists only
// per combination of artifacts and enabled CI features, and one project shows
// one combination:
//
//   - package-upload-{minio,aws,rsync}: a deb with an upload target, the three
//     branches of upload-packages. The branches differ in their steps, and a
//     step reads the needs context the same way a job-level `if:` does.
//   - package-upload-docker: a docker artifact next to the upload, where
//     deploy joins the graph and waits for upload-packages.
//   - ci-checks: docker and deb with declared checks, which puts the checks
//     job into the needs of build and build-packages.
//   - github-ci: a docker artifact with goat tests, which adds a
//     run-goat-tests-<app> entry per application.
//   - deploy-env-files: the per-application create_<app>_env_files entries,
//     the only needs entries naming jobs the generator does not emit.
//   - docs-s3: deploy-docs, a job outside the build/deploy chain.
var needsSweepConfigs = []string{
	"package-upload-minio",
	"package-upload-aws",
	"package-upload-rsync",
	dockerUploadConfig,
	"ci-checks",
	"github-ci",
	"deploy-env-files",
	"docs-s3",
}

// TestWorkflowJobsOnlyReadNeedsTheyDeclare verifies that every read of the
// needs context in the generated workflow names a job the reading job waits
// for.
//
// The needs context of a job holds only the jobs listed in that job's own
// `needs:`. A read of anything else is not an error GitHub reports: the
// expression resolves to null, so a condition built on it is simply false and
// the job is skipped. The run stays green, which is how upload-packages went
// unnoticed reading needs.prepare-env.outputs.ENABLE_BUILD while waiting for
// build-packages alone (TOOLS-130).
//
// All of the job is scanned, not just its `if:`. A null in strategy.matrix, in
// a job- or step-level `env:` or in a step recipe is just as silent, and only
// the condition is a place where being skipped is the intended outcome of a
// null.
func TestWorkflowJobsOnlyReadNeedsTheyDeclare(t *testing.T) {
	for _, name := range needsSweepConfigs {
		t.Run(name, func(t *testing.T) {
			jobs := workflowJobs(t, parsePipeline(t, generateProject(t, name), ciCDWorkflow))

			for _, jobName := range slices.Sorted(maps.Keys(jobs)) {
				declared := declaredNeeds(t, jobName, jobs[jobName])

				for _, ref := range slices.Sorted(maps.Keys(needsReads(jobs[jobName]))) {
					if !declared[ref] {
						t.Errorf("%s: job %q reads needs.%s but waits for %v — the needs context holds "+
							"only the jobs listed in needs:, so the expression resolves to null and the "+
							"job is skipped on a green run",
							ciCDWorkflow, jobName, ref, slices.Sorted(maps.Keys(declared)))
					}
				}
			}
		})
	}
}

// TestDeployIsNotSkippedByAnUnreachableUpload names the consequence the sweep
// above catches in a project with both a docker artifact and a package upload:
// deploy waits for upload-packages, and its `if:` has no always(), so the
// implicit success() applies and a skipped upload skips the deployment too.
// Nothing ships, and the run is green.
//
// The check is on deploy's whole dependency closure rather than on
// upload-packages alone: any job in it that never runs skips deploy the same
// way.
func TestDeployIsNotSkippedByAnUnreachableUpload(t *testing.T) {
	jobs := workflowJobs(t, parsePipeline(t, generateProject(t, dockerUploadConfig), ciCDWorkflow))

	deploy, ok := jobs[deployJob]
	if !ok {
		t.Fatalf("%s should declare a %s job: %s ships a docker artifact",
			ciCDWorkflow, deployJob, dockerUploadConfig)
	}

	if !declaredNeeds(t, deployJob, deploy)[uploadPackagesJob] {
		t.Fatalf("%s should wait for %s in %s — without that edge the fixture no longer covers the "+
			"combination it exists for", deployJob, uploadPackagesJob, dockerUploadConfig)
	}

	for _, jobName := range needsClosure(jobs, deployJob) {
		declared := declaredNeeds(t, jobName, jobs[jobName])

		for _, ref := range slices.Sorted(maps.Keys(needsReads(jobs[jobName]))) {
			if !declared[ref] {
				t.Errorf("%s: job %q reads needs.%s but waits for %v, so it is skipped — and %q waits "+
					"for it without always(), which skips the whole deployment on a green run",
					ciCDWorkflow, jobName, ref, slices.Sorted(maps.Keys(declared)), deployJob)
			}
		}
	}
}

// workflowJobs returns the jobs mapping of a parsed workflow.
func workflowJobs(t *testing.T, workflow map[string]any) map[string]any {
	t.Helper()

	jobs, ok := workflow["jobs"].(map[string]any)
	if !ok {
		t.Fatalf("%s has no jobs mapping, got %#v", ciCDWorkflow, workflow["jobs"])
	}

	return jobs
}

// declaredNeeds returns the jobs a job waits for. Both YAML forms of the key
// are read: GitHub accepts a single job name as a scalar, and a template that
// renders one entry would produce it.
func declaredNeeds(t *testing.T, jobName string, job any) map[string]bool {
	t.Helper()

	body, ok := job.(map[string]any)
	if !ok {
		t.Fatalf("%s: job %q is not a mapping, got %#v", ciCDWorkflow, jobName, job)
	}

	needs := map[string]bool{}

	switch declared := body["needs"].(type) {
	case nil:
	case string:
		needs[declared] = true
	case []any:
		for i, entry := range declared {
			name, isStr := entry.(string)
			if !isStr {
				t.Fatalf("%s: job %q needs[%d] is not a job name, got %#v",
					ciCDWorkflow, jobName, i, entry)
			}

			needs[name] = true
		}
	default:
		t.Fatalf("%s: job %q has a needs: key that is neither a name nor a list, got %#v",
			ciCDWorkflow, jobName, body["needs"])
	}

	return needs
}

// needsReads collects the jobs a job's body reads out of the needs context,
// anywhere in it. The whole subtree is walked rather than the keys a workflow
// is known to put expressions in: an expression is a string wherever it
// appears, and a `with:` input or a nested matrix value carries one as readily
// as an `if:` does. Entries of the job's own `needs:` list are job names, not
// reads, and do not match the expression pattern.
func needsReads(job any) map[string]bool {
	reads := map[string]bool{}

	collectNeedsReads(job, reads)

	return reads
}

func collectNeedsReads(node any, reads map[string]bool) {
	switch value := node.(type) {
	case string:
		for _, match := range needsRefRx.FindAllStringSubmatch(value, -1) {
			reads[match[1]] = true
		}
	case []any:
		for _, item := range value {
			collectNeedsReads(item, reads)
		}
	case map[string]any:
		for key, item := range value {
			collectNeedsReads(key, reads)
			collectNeedsReads(item, reads)
		}
	}
}

// needsClosure returns root together with every job it depends on, directly or
// transitively, sorted. Names without a job of their own are left out: the
// create_<app>_env_files entries are declared by the project itself, in the
// user section of the workflow (see TestDeployWaitsOnlyForEnvFilesItCanGet),
// so the generator has no body to check for them.
func needsClosure(jobs map[string]any, root string) []string {
	seen := map[string]bool{}
	queue := []string{root}

	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]

		if seen[name] {
			continue
		}

		job, generated := jobs[name]
		if !generated {
			continue
		}

		seen[name] = true

		body, isMap := job.(map[string]any)
		if !isMap {
			continue
		}

		switch declared := body["needs"].(type) {
		case string:
			queue = append(queue, declared)
		case []any:
			for _, entry := range declared {
				if dep, isStr := entry.(string); isStr {
					queue = append(queue, dep)
				}
			}
		}
	}

	return slices.Sorted(maps.Keys(seen))
}
