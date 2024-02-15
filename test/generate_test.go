package test

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"gitlab.educentr.info/golang/service-starter/pkg/tools"
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
		t.Errorf("Error getting current directory: %v", err)
	}

	tmpDir, err := os.MkdirTemp(os.TempDir(), "go-project-starter")
	if err != nil {
		t.Errorf("Error creating temporary directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := tools.CopyFile(filepath.Join(curDir, "configs", "config1.yml"), filepath.Join(tmpDir, "project-config.yml")); err != nil {
		t.Errorf("Error copying file: %v", err)
	}

	if out, err := ExecCommand(filepath.Join(curDir, ".."), "go", []string{"run", filepath.Join(curDir, "..", "main.go"), "--target", tmpDir, "--config", filepath.Join(tmpDir, "project-config.yml")}, "Create project by file"); err != nil {
		t.Errorf("Error creating project: %s\n%s", err, out)
	}
}
