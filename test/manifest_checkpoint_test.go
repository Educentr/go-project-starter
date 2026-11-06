package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// generateInto runs the generator with configDir into targetDir and returns its
// combined output. Unlike generateFromConfig it takes an arbitrary config
// directory (a writable copy) and tolerates failure, which is the point of the
// test below.
func generateInto(t *testing.T, configDir, targetDir, msg string) (string, error) {
	t.Helper()

	curDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting current directory: %v", err)
	}

	return ExecCommand(filepath.Join(curDir, ".."), "go", []string{
		"run", filepath.Join(curDir, "..", "cmd", "go-project-starter", "main.go"),
		"--target", targetDir,
		"--configDir", configDir,
		"--config", "project.yaml",
	}, msg)
}

// writableConfigDir copies one of the test configs into a temporary directory,
// so a test can change the inputs of the next run.
func writableConfigDir(t *testing.T, configName string) string {
	t.Helper()

	curDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting current directory: %v", err)
	}

	src := filepath.Join(curDir, "..", "test", "docker-integration", "configs", configName)
	dst := filepath.Join(t.TempDir(), configName)

	if err := os.CopyFS(dst, os.DirFS(src)); err != nil {
		t.Fatalf("copy config dir: %v", err)
	}

	return dst
}

// editFile replaces the content of path through fn.
func editFile(t *testing.T, path string, fn func(string) string) {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	if err := os.WriteFile(path, []byte(fn(string(content))), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestFailedRunRecordsWhatItCopied covers the loop the manifest checkpoint
// closes (TOOLS-80): a run that copied specs and then failed used to leave the
// copies on disk while meta.yaml still described the run before it, so every
// run after it aborted on those copies as if they had been edited by hand — and
// aborted before it could record them, so no run could leave the loop.
//
// The failure is injected by pointing a contract `copy` at a path the generator
// produces itself: that collision is only detectable during generation, after
// the transport specs have been copied.
func TestFailedRunRecordsWhatItCopied(t *testing.T) {
	configDir := writableConfigDir(t, "custom-generator")
	config := filepath.Join(configDir, "project.yaml")
	tmpDir := t.TempDir()

	if out, err := generateInto(t, configDir, tmpDir, "Generate project with custom generators"); err != nil {
		t.Fatalf("first generation failed: %s\n%s", err, out)
	}

	// The failing run has to copy something different from what the successful
	// one recorded, otherwise an unrecorded copy would be indistinguishable
	// from a recorded one.
	editFile(t, filepath.Join(configDir, "api.swagger.yml"), func(s string) string {
		return s + "\n# the contract moved on\n"
	})

	editFile(t, config, func(s string) string {
		return strings.Replace(s, "copy: api/contract/runtime_limits.yaml", "copy: Makefile", 1)
	})

	out, err := generateInto(t, configDir, tmpDir, "Regenerate with a contract that collides with generated code")
	if err == nil {
		t.Fatalf("regeneration should fail on the contract collision, output:\n%s", out)
	}

	// The copies of the failed run are recorded now, so the run after it fails
	// on the same collision — not on the specs its predecessor copied.
	out, err = generateInto(t, configDir, tmpDir, "Regenerate again after the failed run")
	if err == nil {
		t.Fatalf("regeneration should still fail on the contract collision, output:\n%s", out)
	}

	if strings.Contains(out, "edited inside the project") {
		t.Errorf("the run reported its predecessor's copies as hand edits:\n%s", out)
	}

	if !strings.Contains(out, "which the generator produces itself") {
		t.Errorf("error output should still report the collision, got:\n%s", out)
	}

	// And fixing the cause is enough: no deleting of copied files by hand to
	// get a run through.
	editFile(t, config, func(s string) string {
		return strings.Replace(s, "copy: Makefile", "copy: api/contract/runtime_limits.yaml", 1)
	})

	if out, err := generateInto(t, configDir, tmpDir, "Regenerate with the collision resolved"); err != nil {
		t.Fatalf("generation should succeed once the collision is gone: %s\n%s", err, out)
	}
}

// TestPreManifestRunKeepsLegacyPath covers the other half of the checkpoint
// (TOOLS-80): a project generated before meta.yaml carried a manifest has no
// records at all, and for it a partial manifest is worse than none. Writing one
// makes the next run read the project as tracked, so every file the earlier
// runs wrote but the failed one did not reach — the copied specs here — becomes
// a file the generator never wrote, i.e. a conflict. That is the same
// "no run can get through without cleaning the tree by hand" loop, reached
// through the conflict check instead of the drift check.
//
// The failure is injected by pointing a REST spec at a port nothing listens on:
// source resolution runs after the template output has been written, which is
// exactly the shape of the real case (a network flake on a remote spec during
// the first run of the new version).
func TestPreManifestRunKeepsLegacyPath(t *testing.T) {
	const (
		localSpec  = "- ./api.swagger.yml"
		absentSpec = "- https://127.0.0.1:1/api.swagger.yml"
	)

	configDir := writableConfigDir(t, "custom-generator")
	config := filepath.Join(configDir, "project.yaml")
	tmpDir := t.TempDir()

	if out, err := generateInto(t, configDir, tmpDir, "Generate project with custom generators"); err != nil {
		t.Fatalf("first generation failed: %s\n%s", err, out)
	}

	// What a project generated by a version before the manifest looks like.
	metaPath := filepath.Join(tmpDir, ".project-config", "meta.yaml")
	if err := os.WriteFile(metaPath, []byte("version: 4\n"), 0o600); err != nil {
		t.Fatalf("write pre-manifest meta.yaml: %v", err)
	}

	editFile(t, config, func(s string) string {
		return strings.Replace(s, localSpec, absentSpec, 1)
	})

	out, err := generateInto(t, configDir, tmpDir, "Regenerate with an unreachable spec source")
	if err == nil {
		t.Fatalf("regeneration should fail on the unreachable spec source, output:\n%s", out)
	}

	// Removing the cause is enough, as it is on a project with a manifest: the
	// run that failed must not have left records that make its predecessors'
	// output look foreign.
	editFile(t, config, func(s string) string {
		return strings.Replace(s, absentSpec, localSpec, 1)
	})

	out, err = generateInto(t, configDir, tmpDir, "Regenerate after the failed pre-manifest run")
	if err != nil {
		t.Fatalf("generation should succeed once the spec source is reachable again: %s\n%s", err, out)
	}

	if strings.Contains(out, "was not written by the generator") {
		t.Errorf("the run reported the output of earlier runs as foreign files:\n%s", out)
	}
}
