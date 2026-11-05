package test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// scripts/check-skills-plugin.sh is the release-time gate on the Claude skills
// plugin (Educentr/claude, plugins/go-project-starter): every skill in it names
// the generator version its instructions cover, and the commands it hands to
// users pin that version, so a release that leaves the plugin behind ships
// instructions for the previous version.
//
// The checks below run the script against a local plugin tree (PLUGIN_SRC),
// which is also how it is run when reviewing a plugin branch. That keeps the
// test offline — the default path downloads the plugin repository.

const (
	skillCheckOK       = 0
	skillCheckProblem  = 1
	skillCheckInternal = 2
)

const (
	skillCovered = "v0.31.0"
	skillNext    = "v0.32.0"
)

// mainSkill renders skills/go-project-starter/SKILL.md: the version statement
// the script reads as the version the plugin claims to cover, plus an install
// command pinning a version.
func mainSkill(covered, pinned string) string {
	return strings.Join([]string{
		"---",
		"name: go-project-starter",
		"---",
		"",
		"## Generator version",
		"",
		"These instructions cover **go-project-starter " + covered + "**.",
		"",
		"```bash",
		"go install github.com/Educentr/go-project-starter/cmd/go-project-starter@" + pinned,
		"```",
		"",
	}, "\n")
}

// taskSkill renders one of the per-task skills (add-rest-api, create-grpc, …).
// Each is read on its own, so each carries its own version statement.
func taskSkill(covered, pinned string) string {
	return strings.Join([]string{
		"---",
		"name: add-rest-api",
		"---",
		"",
		"These instructions cover **go-project-starter " + covered + "**. Remote URIs",
		"in `path:` are supported since v0.24.0.",
		"",
		"```bash",
		"make update-starter STARTER_VERSION=" + pinned,
		"```",
		"",
	}, "\n")
}

// releasesPage renders a reference page named for the release line it
// describes, with the commands and tagged doc links of that line.
func releasesPage(covered string) string {
	return strings.Join([]string{
		"# go-project-starter " + covered,
		"",
		"```bash",
		"go run github.com/Educentr/go-project-starter/cmd/go-project-starter@" + covered,
		"```",
		"",
		"See [regeneration](https://github.com/Educentr/go-project-starter/blob/" +
			covered + "/docs/workflow/regeneration.md).",
		"",
	}, "\n")
}

// writePlugin materializes a plugin tree and returns its root.
func writePlugin(t *testing.T, files map[string]string) string {
	t.Helper()

	root := filepath.Join(t.TempDir(), "go-project-starter")
	for name, body := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", name, err)
		}

		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	return root
}

// runSkillCheck runs the script against the plugin tree at src.
func runSkillCheck(t *testing.T, src, version string) (int, string) {
	t.Helper()

	script, err := filepath.Abs(filepath.Join("..", "scripts", "check-skills-plugin.sh"))
	if err != nil {
		t.Fatalf("resolve script path: %v", err)
	}

	cmd := exec.Command("bash", script, version)

	cmd.Env = append(os.Environ(), "PLUGIN_SRC="+src, "VERSION=")

	out, err := cmd.CombinedOutput()
	if err == nil {
		return 0, string(out)
	}

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("run %s: %v (output: %s)", script, err, out)
	}

	return exitErr.ExitCode(), string(out)
}

// syncedPlugin is a plugin tree that covers v0.31.0 throughout, including a
// reference page named for the v0.30–v0.31 line.
func syncedPlugin() map[string]string {
	return map[string]string{
		"skills/go-project-starter/SKILL.md":                             mainSkill(skillCovered, skillCovered),
		"skills/add-rest-api/SKILL.md":                                   taskSkill(skillCovered, skillCovered),
		"skills/go-project-starter/references/releases-v0.30-v0.31.md":   releasesPage(skillCovered),
		"go-project-starter-workspace/iteration-1/eval/outputs/notes.md": "recorded run against v0.29.0",
	}
}

func TestCheckSkillsPlugin(t *testing.T) {
	tests := []struct {
		name     string
		files    func() map[string]string
		version  string
		wantCode int
		wantOut  string
	}{
		{
			name:     "in sync",
			files:    syncedPlugin,
			version:  skillCovered,
			wantCode: skillCheckOK,
			wantOut:  "OK: the skills plugin covers v0.31.0.",
		},
		{
			name:     "version without the v prefix is normalized",
			files:    syncedPlugin,
			version:  strings.TrimPrefix(skillCovered, "v"),
			wantCode: skillCheckOK,
			wantOut:  "OK: the skills plugin covers v0.31.0.",
		},
		{
			// A recorded eval run is historical by its nature: it must not be
			// read as an instruction that went stale.
			name: "eval workspace is not instructions",
			files: func() map[string]string {
				files := syncedPlugin()
				files["go-project-starter-workspace/iteration-1/eval/outputs/cmd.md"] =
					"go install github.com/Educentr/go-project-starter/cmd/go-project-starter@v0.29.0"
				return files
			},
			version:  skillCovered,
			wantCode: skillCheckOK,
			wantOut:  "OK: the skills plugin covers v0.31.0.",
		},
		{
			name:     "plugin left behind by the release",
			files:    syncedPlugin,
			version:  skillNext,
			wantCode: skillCheckProblem,
			wantOut:  "ERROR: the skills plugin documents v0.31.0, but v0.32.0 is being released.",
		},
		{
			// A minor bump is not what makes a release: the check must not
			// start passing because only the patch digit moved.
			name:     "plugin left behind by a patch release",
			files:    syncedPlugin,
			version:  "v0.31.1",
			wantCode: skillCheckProblem,
			wantOut:  "ERROR: the skills plugin documents v0.31.0, but v0.31.1 is being released.",
		},
		{
			// The main skill is the one that gets remembered; the nine task
			// skills are read on their own and are the easy ones to miss.
			name: "task skill left behind",
			files: func() map[string]string {
				files := syncedPlugin()
				files["skills/go-project-starter/SKILL.md"] = mainSkill(skillNext, skillNext)
				return files
			},
			version:  skillNext,
			wantCode: skillCheckProblem,
			wantOut:  "skills/add-rest-api/SKILL.md: states v0.31.0",
		},
		{
			// The version statements are the easy half to bump; the pins are
			// what users actually run.
			name: "statements bumped but the install command left stale",
			files: func() map[string]string {
				files := syncedPlugin()
				files["skills/go-project-starter/SKILL.md"] = mainSkill(skillNext, skillCovered)
				files["skills/add-rest-api/SKILL.md"] = taskSkill(skillNext, skillNext)
				return files
			},
			version:  skillNext,
			wantCode: skillCheckProblem,
			wantOut:  "ERROR: commands and tagged links still point at v0.31.0 instead of v0.32.0:",
		},
		{
			// A page named for an older release line describes that line, so
			// its commands and tagged links stay where they are.
			name: "reference page of an older release line stays untouched",
			files: func() map[string]string {
				files := syncedPlugin()
				files["skills/go-project-starter/SKILL.md"] = mainSkill(skillNext, skillNext)
				files["skills/add-rest-api/SKILL.md"] = taskSkill(skillNext, skillNext)
				files["skills/go-project-starter/references/releases-v0.31-v0.32.md"] = releasesPage(skillNext)
				return files
			},
			version:  skillNext,
			wantCode: skillCheckOK,
			wantOut:  "releases-v0.30-v0.31.md   (not checked)",
		},
		{
			// The plugin may legitimately be updated before the tag exists.
			name: "plugin ahead of the version being checked",
			files: func() map[string]string {
				files := syncedPlugin()
				files["skills/go-project-starter/SKILL.md"] = mainSkill(skillNext, skillNext)
				files["skills/add-rest-api/SKILL.md"] = taskSkill(skillNext, skillNext)
				files["skills/go-project-starter/references/releases-v0.31-v0.32.md"] = releasesPage(skillNext)
				return files
			},
			version:  skillCovered,
			wantCode: skillCheckOK,
			wantOut:  "NOTE: the plugin already documents v0.32.0, ahead of v0.31.0",
		},
		{
			name: "no version statement in the main skill",
			files: func() map[string]string {
				files := syncedPlugin()
				files["skills/go-project-starter/SKILL.md"] = "# go-project-starter\n\nNo version here.\n"
				return files
			},
			version:  skillCovered,
			wantCode: skillCheckInternal,
			wantOut:  "declares no generator version",
		},
		{
			name: "no main skill at all",
			files: func() map[string]string {
				return map[string]string{
					"skills/add-rest-api/SKILL.md": taskSkill(skillCovered, skillCovered),
				}
			},
			version:  skillCovered,
			wantCode: skillCheckInternal,
			wantOut:  "has no skills/go-project-starter/SKILL.md",
		},
		{
			// `git describe` of an untagged commit, a branch name: not a
			// released version, so there is nothing the plugin could name.
			name:     "argument is not a release version",
			files:    syncedPlugin,
			version:  "v0.31.0-5-gdeadbee",
			wantCode: skillCheckInternal,
			wantOut:  "is not a release version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out := runSkillCheck(t, writePlugin(t, tt.files()), tt.version)
			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d\noutput:\n%s", code, tt.wantCode, out)
			}

			if !strings.Contains(out, tt.wantOut) {
				t.Errorf("output does not contain %q\noutput:\n%s", tt.wantOut, out)
			}
		})
	}
}

// A plugin path that does not exist is a broken invocation, not a verdict on
// the plugin: it must not be reported as "plugin is up to date".
func TestCheckSkillsPluginMissingSource(t *testing.T) {
	code, out := runSkillCheck(t, filepath.Join(t.TempDir(), "absent"), skillCovered)
	if code != skillCheckInternal {
		t.Errorf("exit code = %d, want %d\noutput:\n%s", code, skillCheckInternal, out)
	}

	if !strings.Contains(out, "is not a directory") {
		t.Errorf("output does not contain %q\noutput:\n%s", "is not a directory", out)
	}
}
