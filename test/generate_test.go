package test

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func ExecCommand(targetPath, command string, args []string, msg string) (string, error) {
	cmd := exec.Command(command, args...)
	cmd.Dir = targetPath

	log.Printf("run: %s\n", msg)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("error while execute command: %w", err)
	}

	return string(out), nil
}

// TestGenerateFromExample tests generation using the example/ directory.
// The example/ directory serves as both documentation and the source of truth for tests.
func TestGenerateFromExample(t *testing.T) {
	curDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting current directory: %v", err)
	}

	// example/ is at the root of the repository
	exampleDir := filepath.Join(curDir, "..", "example")

	tmpDir, err := os.MkdirTemp(os.TempDir(), "go-project-starter")
	if err != nil {
		t.Fatalf("Error creating temporary directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Use example/ directory directly as configDir
	out, err := ExecCommand(filepath.Join(curDir, ".."), "go", []string{
		"run", filepath.Join(curDir, "..", "cmd", "go-project-starter", "main.go"),
		"--target", tmpDir,
		"--configDir", exampleDir,
		"--config", "project.yaml",
	}, "Generate project from example/ ("+tmpDir+")")
	if err != nil {
		t.Fatalf("Error creating project: %s\n%s", err, out)
	}

	t.Logf("Project created in %s: %s", tmpDir, out)

	// Verify key files exist
	expectedFiles := []string{
		"Makefile",
		"go.mod",
		"cmd/publicApi/psg_main_gen.go",
		"api/rest/example/v1/example.swagger.yml",
		"api/schema/models/user.schema.json",
		"api/schema/models/event.schema.json",
	}

	for _, f := range expectedFiles {
		path := filepath.Join(tmpDir, f)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Expected file not found: %s", f)
		}
	}
}
