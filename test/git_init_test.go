package test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestGenerateGitInitOutsideRepository guards the generated Makefile against
// expanding git-derived variables while it is being parsed.
//
// The git_install post-generate step runs `make git-init` in the tree the
// generator has just written — that is, before `git init` has created a
// repository there. A variable such as LAST_COMMIT_HASH or COMMIT_BRANCH, or an
// `ifeq` branching on one, is expanded at parse time, which runs git in that
// tree and makes it print `fatal: not a git repository` once per expansion.
// That noise is what the generator reports back as the result of git_install,
// and it masked the real cause of a flaky test run (TOOLS-101).
func TestGenerateGitInitOutsideRepository(t *testing.T) {
	// t.TempDir() can resolve through a symlink (/tmp on macOS), and
	// GIT_CEILING_DIRECTORIES matches on the literal path.
	target, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("Error resolving temp dir: %v", err)
	}

	// Stop git from walking above the temp tree, so the assertion below does
	// not depend on where TMPDIR happens to live: inside a repository git would
	// find that one and never print the fatal this test looks for.
	ceiling := "GIT_CEILING_DIRECTORIES=" + filepath.Dir(target)

	probe := exec.Command("git", "rev-parse", "--git-dir")
	probe.Dir = target

	probe.Env = append(os.Environ(), ceiling)

	if out, probeErr := probe.CombinedOutput(); probeErr == nil {
		t.Fatalf("%s is inside a git repository (%s), the assertions below would be vacuous",
			target, strings.TrimSpace(string(out)))
	}

	out, err := generateProjectWithEnv(t, "git-init-no-repo", target, []string{ceiling})
	if err != nil {
		t.Fatalf("Error creating project: %s\n%s", err, out)
	}

	if strings.Contains(out, "not a git repository") {
		t.Errorf("the generated Makefile expands git-derived variables while it is parsed:\n%s", out)
	}

	// Without these two the check above would pass for the wrong reason: a
	// git_install step that never ran prints nothing at all.
	if !strings.Contains(out, "run: initialize git") {
		t.Errorf("post_generate did not run git_install:\n%s", out)
	}

	if _, err := os.Stat(filepath.Join(target, ".git")); err != nil {
		t.Errorf("git_install did not create a repository in %s: %v", target, err)
	}
}

// generateProjectWithEnv runs the generator for a config dir from
// test/docker-integration/configs into target, with env appended to the
// generator's environment, and returns the combined output. Unlike
// generateProject it neither picks the target directory nor fails the test on a
// generator error: a test that controls the environment usually wants to assert
// on the output either way.
func generateProjectWithEnv(t *testing.T, configName, target string, env []string) (string, error) {
	t.Helper()

	curDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting current directory: %v", err)
	}

	rootDir := filepath.Join(curDir, "..")
	configDir := filepath.Join(rootDir, "test", "docker-integration", "configs", configName)

	cmd := exec.Command("go", "run", filepath.Join(rootDir, "cmd", "go-project-starter", "main.go"),
		"--target", target,
		"--configDir", configDir,
		"--config", "project.yaml",
	)
	cmd.Dir = rootDir

	cmd.Env = append(os.Environ(), env...)

	out, err := cmd.CombinedOutput()

	return string(out), err
}
