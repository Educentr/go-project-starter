package templater

import (
	"io/fs"
	"strings"
	"testing"
)

// workflowTemplatesPrefix is the embedded directory of the GitHub Actions
// workflow templates, the consumers of the shared workflow fragments.
const workflowTemplatesPrefix = "embedded/templates/main/.github/workflows"

// privateModulesCallSite is the fragment rendered the way the workflow
// templates render it: left-trimmed, on a line of its own, between two steps.
// Pinning the call site rather than the bare body is what makes the
// surrounding blank lines part of the assertion.
const privateModulesCallSite = "      - name: Setup Go\n" +
	"{{- template \"privateModulesAccess\" $ }}\n" +
	"\n" +
	"      - name: Run check\n"

// privateModulesStepMarkers are the two lines that only the shared fragment
// may carry: the step that rewrites the git credentials and the one that sets
// GOPRIVATE.
var privateModulesStepMarkers = []string{
	"- name: Configure Git for private repos",
	"- name: Set GOPRIVATE",
}

// TestPrivateModulesAccessIsDefinedOnce is the guard that replaces the copies.
//
// The block used to be pasted into every job that downloads modules, and the
// copies drifted: one of them kept a hardcoded secret name long after the name
// became configurable, and another lost the GOPRIVATE step. A test that
// counts occurrences in the output catches that after the fact; this one
// catches a new copy at the moment it is written.
func TestPrivateModulesAccessIsDefinedOnce(t *testing.T) {
	defs, err := sharedPartials()
	if err != nil {
		t.Fatalf("sharedPartials() error: %v", err)
	}

	for _, marker := range privateModulesStepMarkers {
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

		for _, marker := range privateModulesStepMarkers {
			if strings.Contains(string(body), marker) {
				t.Errorf("%s spells out %q; render the shared fragment with "+
					`{{ template "privateModulesAccess" $ }} instead`, path, marker)
			}
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", workflowTemplatesPrefix, err)
	}
}

// TestPrivateModulesAccessCallers checks that every workflow template that
// needs private-module access renders the fragment. Both do: ci_cd.yml in the
// jobs that run Go on the runner, go-version-check.yml in its only job.
func TestPrivateModulesAccessCallers(t *testing.T) {
	for _, name := range []string{"ci_cd.yml.tmpl", "go-version-check.yml.tmpl"} {
		body, err := templates.ReadFile(workflowTemplatesPrefix + "/" + name)
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}

		if !strings.Contains(string(body), `template "privateModulesAccess" $`) {
			t.Errorf("%s does not render the privateModulesAccess fragment", name)
		}
	}
}

// TestPrivateModulesAccessRender pins the rendered fragment, whitespace
// included.
//
// The block is YAML inside a list of steps, so its indentation is part of its
// meaning: trimming the define tag with `-}}` eats the leading spaces of the
// first step and produces a workflow GitHub Actions refuses to parse. The
// leading newlines and the missing trailing one are pinned for the same
// reason: they are what makes a left-trimmed call on a line of its own render
// as a blank-line separated block — and render nothing at all, not even a
// stray blank line, when the project declares no private modules.
func TestPrivateModulesAccessRender(t *testing.T) {
	const wantWithPrivateRepos = "      - name: Setup Go\n" +
		"\n" +
		"      - name: Configure Git for private repos\n" +
		"        env:\n" +
		"          GH_TOKEN: ${{ secrets.GO_MODULES_TOKEN }}\n" +
		"        run: |\n" +
		"          git config --global url.\"https://x-access-token:${GH_TOKEN}@github.com/\"" +
		".insteadOf \"https://github.com/\"\n" +
		"\n" +
		"      - name: Set GOPRIVATE\n" +
		"        run: |\n" +
		"          go env -w GOPRIVATE=\"github.com/myorg/*\"\n" +
		"\n" +
		"      - name: Run check\n"

	// Nothing to take off the proxy, so the two steps and the blank line that
	// separated them from the step above are all gone.
	const wantWithoutPrivateRepos = "      - name: Setup Go\n" +
		"\n" +
		"      - name: Run check\n"

	tests := []struct {
		name         string
		privateRepos string
		want         string
	}{
		{
			name:         "declared private repos",
			privateRepos: "github.com/myorg/*",
			want:         wantWithPrivateRepos,
		},
		{
			name:         "no private repos",
			privateRepos: "",
			want:         wantWithoutPrivateRepos,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := GeneratorParams{
				PrivateRepos:            tt.privateRepos,
				PrivateReposTokenSecret: "GO_MODULES_TOKEN",
			}

			// The fragment is rendered through the same path the workflow
			// templates take, minus the disclaimer blocks a .yml file does not
			// carry here.
			tmpl := Template{
				Name: "partial-render-test",
				Tmpl: privateModulesCallSite,
			}

			buf, err := GenerateByTmpl(tmpl, params, nil, "partial-render-test.yml")
			if err != nil {
				t.Fatalf("GenerateByTmpl() error: %v", err)
			}

			if got := buf.String(); !strings.Contains(got, tt.want) {
				t.Errorf("rendered fragment does not match.\n--- got ---\n%s\n--- want ---\n%s", got, tt.want)
			}
		})
	}
}

// TestWorkflowTemplatesCallFragmentTrimmed guards the one thing a call site
// still has to get right: the call must be left-trimmed. An untrimmed
// `{{ template … }}` keeps the newline of the line above, so a project without
// private modules gets a stray blank line where the block used to be; see the
// fragment's own comment.
func TestWorkflowTemplatesCallFragmentTrimmed(t *testing.T) {
	err := fs.WalkDir(templates, workflowTemplatesPrefix, func(path string, d fs.DirEntry, err error) error {
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

		if strings.Contains(string(body), `{{ template "privateModulesAccess" $ }}`) {
			t.Errorf(`%s calls the fragment untrimmed; use {{- template "privateModulesAccess" $ }}`, path)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", workflowTemplatesPrefix, err)
	}
}

// TestNoTemplateReadsPrivateReposDirectly is the guard for the single source of
// the GOPRIVATE value. The value used to be assembled in each consumer — the
// Makefile and the workflows prepended github.com/Educentr, .gitlab-ci.yml
// prepended gitlab.com, the Dockerfile prepended nothing — so the host build,
// CI and `docker build` disagreed about which modules bypass the proxy.
// Templates read GeneratorParams.GoPrivate instead of PrivateRepos, which is
// what keeps them from drifting apart again.
func TestNoTemplateReadsPrivateReposDirectly(t *testing.T) {
	for _, root := range []string{templatesRoot, partialsPrefix} {
		err := fs.WalkDir(templates, root, func(path string, d fs.DirEntry, err error) error {
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

			// PrivateReposTokenSecretName is a different value (the CI secret
			// name) and has its own accessor, so only a bare .PrivateRepos is
			// a finding.
			for _, ref := range []string{".PrivateRepos }}", ".PrivateRepos)", ".PrivateRepos "} {
				if strings.Contains(string(body), ref) {
					t.Errorf("%s reads %q; render .GoPrivate so every consumer gets the same value", path, ref)
				}
			}

			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
}

// TestSharedPartialsAreNotCopiedIntoProjects checks that the fragments stay
// generator input. They are templates, so a walk that emitted them would write
// them into the generated project as files of their own.
func TestSharedPartialsAreNotCopiedIntoProjects(t *testing.T) {
	if strings.HasPrefix(partialsPrefix, templatesRoot) {
		t.Fatalf("partials live under %s, so every GetTemplates walk of it emits them as project files",
			templatesRoot)
	}

	_, files, err := GetMainTemplates(GeneratorParams{})
	if err != nil {
		t.Fatalf("GetMainTemplates() error: %v", err)
	}

	for _, f := range files {
		if strings.Contains(f.SourceName, "/partials/") {
			t.Errorf("main templates include the shared fragment %s", f.SourceName)
		}
	}
}
