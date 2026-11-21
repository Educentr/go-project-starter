package test

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// needsWorkflow is the subset of the generated workflow this test reads: which
// jobs exist and what each one waits for.
type needsWorkflow struct {
	Jobs map[string]struct {
		Needs []string `yaml:"needs"`
	} `yaml:"jobs"`
}

// TestDeployWaitsOnlyForEnvFilesItCanGet covers the deploy job's dependency on
// the per-application env-file jobs (TOOLS-96, issue #124 item 1).
//
// create_<app>_env_files is declared by the project itself, in the user section
// of the workflow — the generator emits the needs entry, not the job. So a needs
// entry the project has no reason to declare is not a harmless extra: GitHub
// rejects the whole workflow over a job name that resolves to nothing. deploy
// ships docker artifacts only, which is what makes HasDocker the condition: an
// application packaged as a deb has no image for deploy to ship and no env file
// for it to wait for.
func TestDeployWaitsOnlyForEnvFilesItCanGet(t *testing.T) {
	tmpDir := generateFromConfig(t, "deploy-env-files", "Generate project with use_envs applications")

	content := readGenerated(t, tmpDir, ".github", "workflows", "ci_cd.yml")

	var workflow needsWorkflow

	if err := yaml.Unmarshal([]byte(content), &workflow); err != nil {
		t.Fatalf("generated workflow is not valid YAML: %v", err)
	}

	deploy, ok := workflow.Jobs["deploy"]
	if !ok {
		t.Fatal("generated workflow should declare a deploy job: the fixture ships a docker artifact")
	}

	needs := make(map[string]bool, len(deploy.Needs))
	for _, need := range deploy.Needs {
		needs[need] = true
	}

	// The docker application: its env file is part of the deploy payload.
	if !needs["create_shipped_env_files"] {
		t.Errorf("deploy should wait for create_shipped_env_files, waits for %v", deploy.Needs)
	}

	// The deb-only application: nothing deploy does involves it.
	if needs["create_packaged_env_files"] {
		t.Errorf("deploy should not wait for create_packaged_env_files — the application ships no image, "+
			"and the job is one nobody declares; needs: %v", deploy.Needs)
	}

	// The other half of the contract, and the reason the entry above has to be
	// earned: the generator does not emit the job it names. A project with a
	// use_envs application has to declare it, which is now documented in
	// docs/configuration/applications.md.
	if _, generated := workflow.Jobs["create_shipped_env_files"]; generated {
		t.Error("the generator should not emit create_shipped_env_files — it is the project's job to declare, " +
			"and TestDeployWaitsOnlyForEnvFilesItCanGet exists because the needs entry is therefore a promise")
	}
}
