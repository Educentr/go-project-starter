package test

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Educentr/go-project-starter/internal/pkg/tools"
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

func TestGenerateNew(t *testing.T) {
	curDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting current directory: %v", err)
	}

	tmpDir, err := os.MkdirTemp(os.TempDir(), "go-project-starter")
	if err != nil {
		t.Fatalf("Error creating temporary directory: %v", err)
	}
	//defer os.RemoveAll(tmpDir)

	if err = os.MkdirAll(filepath.Join(tmpDir, ".project-config"), 0755); err != nil {
		t.Fatalf("Error creating directory: %v", err)
	}

	for _, copyFile := range [][2]string{
		{"config1.yml", "project.yaml"},
		{"example.swagger.yml", "example.swagger.yml"},
		{"example.proto", "example.proto"},
		{"admin.proto", "admin.proto"},
	} {
		if err := tools.CopyFile(filepath.Join(curDir, "configs", copyFile[0]), filepath.Join(tmpDir, ".project-config", copyFile[1])); err != nil {
			t.Fatalf("Error copying file: %v", err)
		}
	}

	out, err := ExecCommand(filepath.Join(curDir, ".."), "go", []string{"run", filepath.Join(curDir, "..", "cmd", "go-project-starter", "main.go"), "--target", tmpDir, "--configDir", filepath.Join(tmpDir, ".project-config")}, "Create project by file ("+tmpDir+")")
	if err != nil {
		t.Fatalf("Error creating project: %s\n%s", err, out)
	}

	t.Logf("Project created in %s: %s", tmpDir, out)
}
