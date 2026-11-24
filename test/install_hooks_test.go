package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// installHooksTarget is the Makefile target that puts the project's git hooks
// where git looks for them.
const installHooksTarget = "install-hooks"

// hookName is the one hook the generated project ships.
const hookName = "pre-commit"

// TestGenerateInstallHooksInWorktree is the regression test for `make
// regenerate` in a git worktree.
//
// The recipe used to copy the hook into ./.git/hooks. In a worktree `.git` is
// a file pointing at the real directory, so there was no such path and `cp`
// failed — which took `make regenerate` down with it (post_generate:
// tools_install -> install-tools -> install-hooks). The run stopped before
// call_generate, so ogen and argen never ran and the generated code silently
// stayed as it was. Asking git for the directory answers it in a worktree, in
// an ordinary clone and under core.hooksPath alike.
//
// The target is run for real rather than grepped: the recipe is shell, and the
// defect was that the shell command failed, not that the text looked wrong.
func TestGenerateInstallHooksInWorktree(t *testing.T) {
	// A generated project is a git repository already (the generator inits
	// one), so it is also the main checkout a worktree can be added to.
	project := generateFromConfig(t, "rest-only", "Generate project for the git hooks test")

	t.Run("clone", func(t *testing.T) {
		hooksDir := gitPath(t, project, "hooks")

		removeHook(t, hooksDir)
		runInstallHooks(t, project)
		assertHookInstalled(t, hooksDir)
	})

	t.Run("worktree", func(t *testing.T) {
		worktree := filepath.Join(t.TempDir(), "wt")

		if out, err := ExecCommand(project, "git",
			[]string{"worktree", "add", "-b", "hooks-test", worktree}, "add a worktree"); err != nil {
			t.Fatalf("Error adding worktree: %v\n%s", err, out)
		}

		// The hooks of a worktree are the shared ones of the main checkout:
		// that is where git looks for them from inside the worktree, so that
		// is where the hook has to land.
		hooksDir := gitPath(t, worktree, "hooks")

		if _, err := os.Stat(filepath.Join(worktree, ".git")); err != nil {
			t.Fatalf("Error reading the .git of the worktree: %v", err)
		}

		removeHook(t, hooksDir)
		runInstallHooks(t, worktree)
		assertHookInstalled(t, hooksDir)
	})

	t.Run("custom hooksPath", func(t *testing.T) {
		// core.hooksPath moves the directory git looks in, and asking git for
		// the path is what makes the recipe follow it.
		if out, err := ExecCommand(project, "git",
			[]string{"config", "core.hooksPath", ".myhooks"}, "set core.hooksPath"); err != nil {
			t.Fatalf("Error setting core.hooksPath: %v\n%s", err, out)
		}

		t.Cleanup(func() {
			if out, err := ExecCommand(project, "git",
				[]string{"config", "--unset", "core.hooksPath"}, "unset core.hooksPath"); err != nil {
				t.Fatalf("Error unsetting core.hooksPath: %v\n%s", err, out)
			}
		})

		hooksDir := gitPath(t, project, "hooks")

		if filepath.Base(hooksDir) != ".myhooks" {
			t.Fatalf("git reports the hooks directory as %q, want the configured core.hooksPath", hooksDir)
		}

		runInstallHooks(t, project)
		assertHookInstalled(t, hooksDir)
	})

	t.Run("outside a git repository", func(t *testing.T) {
		// A tree without a repository has nowhere to put hooks, and the target
		// is a prerequisite of install-tools, which `make regenerate` runs.
		// So it reports and succeeds — and it must not create a .git of its
		// own, which would make git refuse to work in the tree at all.
		outside := t.TempDir()

		copyTree(t, project, outside)

		if err := os.RemoveAll(filepath.Join(outside, ".git")); err != nil {
			t.Fatalf("Error removing .git: %v", err)
		}

		out, err := ExecCommand(outside, "make", []string{installHooksTarget}, "make install-hooks outside a repo")
		if err != nil {
			t.Fatalf("make %s failed outside a git repository, which fails `make regenerate` with it: %v\n%s",
				installHooksTarget, err, out)
		}

		if !strings.Contains(out, "not a git repository") {
			t.Errorf("make %s said nothing about the missing repository:\n%s", installHooksTarget, out)
		}

		if _, err := os.Stat(filepath.Join(outside, ".git")); err == nil {
			t.Error("make " + installHooksTarget + " created a .git directory outside a repository: " +
				"git refuses to work in a tree with a .git that is not a repository")
		}
	})
}

// gitPath asks git, from dir, for the path it resolves gitPath to — the same
// question the recipe asks.
func gitPath(t *testing.T, dir, path string) string {
	t.Helper()

	out, err := ExecCommand(dir, "git", []string{"rev-parse", "--git-path", path}, "git rev-parse --git-path "+path)
	if err != nil {
		t.Fatalf("Error resolving the git path %q in %s: %v\n%s", path, dir, err, out)
	}

	resolved := strings.TrimSpace(out)
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(dir, resolved)
	}

	return resolved
}

// runInstallHooks runs the target in dir and fails the test with its output.
func runInstallHooks(t *testing.T, dir string) {
	t.Helper()

	if out, err := ExecCommand(dir, "make", []string{installHooksTarget}, "make install-hooks in "+dir); err != nil {
		t.Fatalf("make %s failed in %s: %v\n%s", installHooksTarget, dir, err, out)
	}
}

// removeHook drops an already installed hook, so that finding one afterwards
// means this run installed it.
func removeHook(t *testing.T, hooksDir string) {
	t.Helper()

	if err := os.Remove(filepath.Join(hooksDir, hookName)); err != nil && !os.IsNotExist(err) {
		t.Fatalf("Error removing the installed hook: %v", err)
	}
}

// assertHookInstalled checks that the hook landed in hooksDir and that git can
// execute it: a hook without the bit is one git skips without a word.
func assertHookInstalled(t *testing.T, hooksDir string) {
	t.Helper()

	info, err := os.Stat(filepath.Join(hooksDir, hookName))
	if err != nil {
		t.Fatalf("the %s hook is not in %s, the directory git looks in: %v", hookName, hooksDir, err)
	}

	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("the installed %s hook is not executable (mode %v): git skips such a hook silently",
			hookName, info.Mode().Perm())
	}
}

// copyTree copies the regular files of src into dst, directories included. It
// is enough for the case it serves: a project tree without its repository.
func copyTree(t *testing.T, src, dst string) {
	t.Helper()

	if out, err := ExecCommand(filepath.Dir(src), "cp", []string{"-a", src + "/.", dst}, "copy the project tree"); err != nil {
		t.Fatalf("Error copying %s to %s: %v\n%s", src, dst, err, out)
	}
}
