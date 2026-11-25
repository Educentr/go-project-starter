package templater

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/Educentr/go-project-starter/internal/pkg/ds"
)

// checksJobMarkers are the lines only the shared checksJob fragment may carry:
// the job key and the step that runs one declared check.
var checksJobMarkers = []string{
	"  checks:",
	"- name: Run check ${{\"{{\"}} matrix.name {{\"}}\"}}",
}

// TestChecksJobIsDefinedOnce is the guard that keeps the two workflows running
// the same checks.
//
// The job is spelled out in neither template: ci_cd.yml renders it for push,
// pr-checks.yml for a pull request, and both call the fragment. A copy would
// drift the way the private-module block did — and here the drift is worse
// than a broken job, because a pull-request check that quietly stops matching
// the push one reports green on code the merge then rejects.
func TestChecksJobIsDefinedOnce(t *testing.T) {
	defs, err := sharedPartials()
	if err != nil {
		t.Fatalf("sharedPartials() error: %v", err)
	}

	for _, marker := range checksJobMarkers {
		if !strings.Contains(defs, marker) {
			t.Errorf("shared partials do not define %q; the fragment moved or was renamed", marker)
		}
	}

	err = fs.WalkDir(templates, workflowTemplatesPrefix, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		body, err := templates.ReadFile(path)
		if err != nil {
			return err
		}

		for _, marker := range checksJobMarkers {
			if strings.Contains(string(body), marker) {
				t.Errorf("%s spells out %q; render the shared fragment with "+
					`{{ template "checksJob" … }} instead`, path, marker)
			}
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", workflowTemplatesPrefix, err)
	}
}

// TestChecksJobCallers checks that both workflow templates render the
// fragment, each with the context of its own trigger. A template calling the
// other one's context would render the wrong gate — and both gates are
// conditions, so the job would be skipped rather than fail.
func TestChecksJobCallers(t *testing.T) {
	callers := map[string]string{
		"ci_cd.yml.tmpl":     `template "checksJob" $.ChecksOnPush`,
		"pr-checks.yml.tmpl": `template "checksJob" $.ChecksOnPullRequest`,
	}

	for name, call := range callers {
		body, err := templates.ReadFile(workflowTemplatesPrefix + "/" + name)
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}

		if !strings.Contains(string(body), call) {
			t.Errorf("%s does not render the checks fragment as %q", name, call)
		}
	}
}

// TestChecksRenderRegistryLoginIf pins the two conditions of the registry
// login steps. On push the matrix leg alone decides; on a pull request the
// credential has to be there as well, because a fork sees no secrets and
// docker/login-action with an empty password fails the leg before the check
// it was supposed to guard ever runs.
func TestChecksRenderRegistryLoginIf(t *testing.T) {
	if got := (GeneratorParams{}).ChecksOnPush().RegistryLoginIf(); got != "matrix.registry_login" {
		t.Errorf("ChecksOnPush().RegistryLoginIf() = %q, want the matrix leg alone", got)
	}

	got := (GeneratorParams{}).ChecksOnPullRequest().RegistryLoginIf()

	if !strings.Contains(got, "matrix.registry_login") {
		t.Errorf("ChecksOnPullRequest().RegistryLoginIf() = %q, want the matrix leg in it", got)
	}

	if !strings.Contains(got, "env.REGISTRY_CREDENTIAL != ''") {
		t.Errorf("ChecksOnPullRequest().RegistryLoginIf() = %q, want the credential presence check in it", got)
	}
}

// TestChecksRenderRegistryCredentialSecret pins the secret whose presence
// stands for "this run can log in", per registry type. Each one is a secret
// the login steps of that registry actually read — a name nothing reads would
// make every fork pull request skip a login it could have performed.
func TestChecksRenderRegistryCredentialSecret(t *testing.T) {
	want := map[string]string{
		"github":       ghcrTokenSecret,
		"aws":          awsAccessKeyIDSecret,
		"digitalocean": registryPasswordSecret,
		"selfhosted":   registryPasswordSecret,
	}

	for registryType, wantSecret := range want {
		render := GeneratorParams{RegistryType: registryType}.ChecksOnPullRequest()

		if got := render.RegistryCredentialSecret(); got != wantSecret {
			t.Errorf("registry_type %q: RegistryCredentialSecret() = %q, want %q", registryType, got, wantSecret)
		}
	}
}

// TestPRChecksWorkflowNeedsDeclaredChecks checks the output set: the
// pull-request workflow is the checks matrix and nothing else, so a project
// without declared checks must not get the file. An empty matrix is a workflow
// GitHub rejects.
func TestPRChecksWorkflowNeedsDeclaredChecks(t *testing.T) {
	_, files, err := GetMainTemplates(GeneratorParams{})
	if err != nil {
		t.Fatalf("GetMainTemplates() error: %v", err)
	}

	for _, f := range files {
		if f.DestName == prCheckWorkflowPath {
			t.Fatalf("a project with no declared checks gets %s", prCheckWorkflowPath)
		}
	}

	params := GeneratorParams{}
	params.GithubCI.Checks.Checks = []ds.Check{{Name: "lint", Run: "make lint"}}

	_, files, err = GetMainTemplates(params)
	if err != nil {
		t.Fatalf("GetMainTemplates() error: %v", err)
	}

	for _, f := range files {
		if f.DestName == prCheckWorkflowPath {
			return
		}
	}

	t.Fatalf("a project with declared checks does not get %s", prCheckWorkflowPath)
}
