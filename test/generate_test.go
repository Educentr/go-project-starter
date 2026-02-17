package test

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// TestGenerateRESTLogrus tests that REST project with logrus logger generates correctly.
func TestGenerateRESTLogrus(t *testing.T) {
	curDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting current directory: %v", err)
	}

	configDir := filepath.Join(curDir, "..", "test", "docker-integration", "configs", "rest-logrus")
	tmpDir := t.TempDir()

	out, err := ExecCommand(filepath.Join(curDir, ".."), "go", []string{
		"run", filepath.Join(curDir, "..", "cmd", "go-project-starter", "main.go"),
		"--target", tmpDir,
		"--configDir", configDir,
		"--config", "project.yaml",
	}, "Generate logrus project ("+tmpDir+")")
	if err != nil {
		t.Fatalf("Error creating project: %s\n%s", err, out)
	}

	t.Logf("Logrus project created in %s: %s", tmpDir, out)

	// Verify key files exist
	expectedFiles := []string{
		"Makefile",
		"go.mod",
		"cmd/api/psg_main_gen.go",
		"pkg/app/logger/psg_logrus_gen.go",
		"pkg/app/rest/psg_server_gen.go",
		"pkg/app/rest/psg_closer_gen.go",
		"pkg/app/rest/mw/psg_mw_gen.go",
		"pkg/app/rest/mw/psg_metrics_gen.go",
	}

	for _, f := range expectedFiles {
		path := filepath.Join(tmpDir, f)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Expected file not found: %s", f)
		}
	}

	// Verify logrus logger is used (not zerolog)
	loggerFile := filepath.Join(tmpDir, "pkg", "app", "logger", "psg_logrus_gen.go")

	content, err := os.ReadFile(loggerFile)
	if err != nil {
		t.Fatalf("Error reading logger file: %v", err)
	}

	if !strings.Contains(string(content), "InitLogrus") {
		t.Error("Logger file should contain InitLogrus function")
	}

	if !strings.Contains(string(content), "github.com/sirupsen/logrus") {
		t.Error("Logger file should import sirupsen/logrus")
	}

	// Verify rlog import in generated server
	serverFile := filepath.Join(tmpDir, "pkg", "app", "rest", "psg_server_gen.go")

	serverContent, err := os.ReadFile(serverFile)
	if err != nil {
		t.Fatalf("Error reading server file: %v", err)
	}

	if !strings.Contains(string(serverContent), `rlog "github.com/Educentr/go-project-starter-runtime/pkg/logger"`) {
		t.Error("Server file should import runtime logger as rlog")
	}

	if !strings.Contains(string(serverContent), "rlog.LogrusFromContext") {
		t.Error("Server file should use rlog.LogrusFromContext")
	}

	// Verify zerolog is NOT used
	zerologFile := filepath.Join(tmpDir, "pkg", "app", "logger", "psg_zlog_gen.go")
	if _, err := os.Stat(zerologFile); !os.IsNotExist(err) {
		t.Error("Zerolog file should NOT exist in logrus project")
	}
}

// TestGenerateRESTTimeouts verifies that split timeout configuration
// is correctly generated in REST server, middleware, and SQL files.
func TestGenerateRESTTimeouts(t *testing.T) {
	curDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting current directory: %v", err)
	}

	configDir := filepath.Join(curDir, "..", "test", "docker-integration", "configs", "rest-only")
	tmpDir := t.TempDir()

	out, err := ExecCommand(filepath.Join(curDir, ".."), "go", []string{
		"run", filepath.Join(curDir, "..", "cmd", "go-project-starter", "main.go"),
		"--target", tmpDir,
		"--configDir", configDir,
		"--config", "project.yaml",
	}, "Generate REST project for timeout tests ("+tmpDir+")")
	if err != nil {
		t.Fatalf("Error creating project: %s\n%s", err, out)
	}

	t.Logf("REST project created in %s: %s", tmpDir, out)

	// Helper to read file and check multiple strings
	assertFileContains := func(t *testing.T, relPath string, expected []string) {
		t.Helper()

		content, err := os.ReadFile(filepath.Join(tmpDir, relPath))
		if err != nil {
			t.Fatalf("Error reading %s: %v", relPath, err)
		}

		s := string(content)
		for _, exp := range expected {
			if !strings.Contains(s, exp) {
				t.Errorf("%s should contain %q", relPath, exp)
			}
		}
	}

	// pkg/app/rest/psg_server_gen.go — split timeouts, atomic field, subscription
	assertFileContains(t, "pkg/app/rest/psg_server_gen.go", []string{
		`"timeout_read"`,
		`"timeout_write"`,
		"writeTimeout atomic.Int64",
		"RegisterSubscription",
		"GetWriteTimeout",
		"updateTimeouts",
	})

	// pkg/app/rest/mw/psg_mw_gen.go — CreateContextWithTimeout, fallback, clamping
	assertFileContains(t, "pkg/app/rest/mw/psg_mw_gen.go", []string{
		"CreateContextWithTimeout",
		"resolveHandlerTimeout",
		"getWriteTimeout()",
		"handlerTimeout = writeTimeout",
	})

	// pkg/app/rest/psg_rest_gen.go — split default constants, exported accessor
	assertFileContains(t, "pkg/app/rest/psg_rest_gen.go", []string{
		"defaultHTTPReadTimeout",
		"defaultHTTPWriteTimeout",
		"DefaultHandlerTimeout",
	})

	// pkg/app/rest/mw/psg_metrics_gen.go — timeout logging
	assertFileContains(t, "pkg/app/rest/mw/psg_metrics_gen.go", []string{
		"Request handler timeout exceeded",
		"r.Context().Deadline()",
	})

	// etc/onlineconf/dev/init-config.sql — SQL init for split timeouts
	assertFileContains(t, "etc/onlineconf/dev/init-config.sql", []string{
		"timeout_read",
		"timeout_write",
	})
}
