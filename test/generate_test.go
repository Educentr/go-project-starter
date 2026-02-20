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

	// Verify REST client uses type aliases to runtime (regression: Middleware type mismatch)
	clientFile := filepath.Join(tmpDir, "pkg", "app", "rest", "psg_client_gen.go")
	clientContent, err := os.ReadFile(clientFile)
	if err != nil {
		t.Fatalf("Error reading client file: %v", err)
	}

	clientStr := string(clientContent)

	// Must import runtime rest package with alias
	if !strings.Contains(clientStr, `rtrest "github.com/Educentr/go-project-starter-runtime/pkg/app/rest"`) {
		t.Error("Client file should import runtime rest package with rtrest alias")
	}

	// Must use type aliases (=), not type definitions
	if !strings.Contains(clientStr, "type Middleware = rtrest.Middleware") {
		t.Error("Client file should define Middleware as type alias to runtime")
	}

	if !strings.Contains(clientStr, "type DefaultClient = rtrest.DefaultClient") {
		t.Error("Client file should define DefaultClient as type alias to runtime")
	}

	// Must NOT define local Middleware type (the old pattern)
	if strings.Contains(clientStr, "type Middleware func(http.RoundTripper) http.RoundTripper") {
		t.Error("Client file should NOT define local Middleware type — must use alias to runtime")
	}

	// Verify CORS configuration is read from OnlineConf (not hardcoded AllowAll)
	ocConfigContent, err := os.ReadFile(filepath.Join(tmpDir, "pkg", "app", "restconfig", "psg_config_oc_gen.go"))
	if err != nil {
		t.Fatalf("Error reading restconfig file: %v", err)
	}

	ocConfigStr := string(ocConfigContent)

	if !strings.Contains(ocConfigStr, "GetCORSOptions") {
		t.Error("restconfig file should contain GetCORSOptions method")
	}

	if !strings.Contains(ocConfigStr, `"github.com/rs/cors"`) {
		t.Error("restconfig file should import github.com/rs/cors")
	}

	// Verify router uses config-driven CORS (not hardcoded AllowAll)
	routerContent, err := os.ReadFile(filepath.Join(tmpDir, "internal", "app", "transport", "rest", "example", "v1", "psg_router_gen.go"))
	if err != nil {
		t.Fatalf("Error reading router file: %v", err)
	}

	routerStr := string(routerContent)

	if !strings.Contains(routerStr, "GetCORSOptions") {
		t.Error("Router file should use GetCORSOptions for CORS configuration")
	}

	if strings.Contains(routerStr, "cors.AllowAll()") {
		t.Error("Router file should NOT use hardcoded cors.AllowAll()")
	}
}

// TestOgenClientTemplateUsesProjectLocalImport verifies that the ogen_client template
// imports rest from the project-local path (pkg/app/rest), not from the runtime directly.
// This is critical because the project's pkg/app/rest provides type aliases that are
// compatible with both old (local types) and new (runtime aliases) projects.
func TestOgenClientTemplateUsesProjectLocalImport(t *testing.T) {
	curDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting current directory: %v", err)
	}

	templatePath := filepath.Join(curDir, "..", "internal", "pkg", "templater", "embedded",
		"templates", "transport", "rest", "ogen_client", "files", "client.go.tmpl")

	content, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("Error reading ogen_client template: %v", err)
	}

	tmplStr := string(content)

	// Must import from project path (template variable), NOT from runtime
	if !strings.Contains(tmplStr, `"{{ .ProjectPath }}/pkg/app/rest"`) {
		t.Error("ogen_client template must import rest from project path, not runtime")
	}

	if strings.Contains(tmplStr, `"github.com/Educentr/go-project-starter-runtime/pkg/app/rest"`) {
		t.Error("ogen_client template must NOT import rest directly from runtime — use project-local aliases")
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

// TestGenerateDocsS3 tests that documentation with S3 deployment generates correctly.
func TestGenerateDocsS3(t *testing.T) {
	curDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting current directory: %v", err)
	}

	configDir := filepath.Join(curDir, "..", "test", "docker-integration", "configs", "docs-s3")
	tmpDir := t.TempDir()

	out, err := ExecCommand(filepath.Join(curDir, ".."), "go", []string{
		"run", filepath.Join(curDir, "..", "cmd", "go-project-starter", "main.go"),
		"--target", tmpDir,
		"--configDir", configDir,
		"--config", "project.yaml",
	}, "Generate docs-s3 project ("+tmpDir+")")
	if err != nil {
		t.Fatalf("Error creating project: %s\n%s", err, out)
	}

	t.Logf("Docs S3 project created in %s: %s", tmpDir, out)

	// Verify docs files exist
	expectedFiles := []string{
		"mkdocs.yml",
		"docs/index.md",
		"Makefile",
		".gitignore",
	}

	for _, f := range expectedFiles {
		path := filepath.Join(tmpDir, f)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Expected file not found: %s", f)
		}
	}

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

	assertFileNotContains := func(t *testing.T, relPath string, unexpected []string) {
		t.Helper()

		content, err := os.ReadFile(filepath.Join(tmpDir, relPath))
		if err != nil {
			t.Fatalf("Error reading %s: %v", relPath, err)
		}

		s := string(content)
		for _, unexp := range unexpected {
			if strings.Contains(s, unexp) {
				t.Errorf("%s should NOT contain %q", relPath, unexp)
			}
		}
	}

	// mkdocs.yml
	assertFileContains(t, "mkdocs.yml", []string{
		"site_name: docs-test",
		"name: material",
	})

	// docs/index.md
	assertFileContains(t, "docs/index.md", []string{
		"docs-test",
	})

	// Makefile — S3 targets
	assertFileContains(t, "Makefile", []string{
		"docs-build",
		"docs-serve",
		"docs-deploy",
		"DOCS_BUCKET",
		"aws s3 sync",
	})

	assertFileNotContains(t, "Makefile", []string{
		"gh-deploy",
	})

	// .gitignore
	assertFileContains(t, ".gitignore", []string{
		"site/",
	})

	// CI/CD — GitHub Actions
	assertFileContains(t, ".github/workflows/ci_cd.yml", []string{
		"deploy-docs",
		"DOCS_AWS_ACCESS_KEY_ID",
	})

	// CI/CD — GitLab CI
	assertFileContains(t, ".gitlab-ci.yml", []string{
		"deploy-docs",
		"DOCS_BUCKET",
	})
}

// TestGenerateDocsGitHubPages tests that documentation with GitHub Pages deployment generates correctly.
func TestGenerateDocsGitHubPages(t *testing.T) {
	curDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting current directory: %v", err)
	}

	configDir := filepath.Join(curDir, "..", "test", "docker-integration", "configs", "docs-ghpages")
	tmpDir := t.TempDir()

	out, err := ExecCommand(filepath.Join(curDir, ".."), "go", []string{
		"run", filepath.Join(curDir, "..", "cmd", "go-project-starter", "main.go"),
		"--target", tmpDir,
		"--configDir", configDir,
		"--config", "project.yaml",
	}, "Generate docs-ghpages project ("+tmpDir+")")
	if err != nil {
		t.Fatalf("Error creating project: %s\n%s", err, out)
	}

	t.Logf("Docs GitHub Pages project created in %s: %s", tmpDir, out)

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

	assertFileNotContains := func(t *testing.T, relPath string, unexpected []string) {
		t.Helper()

		content, err := os.ReadFile(filepath.Join(tmpDir, relPath))
		if err != nil {
			t.Fatalf("Error reading %s: %v", relPath, err)
		}

		s := string(content)
		for _, unexp := range unexpected {
			if strings.Contains(s, unexp) {
				t.Errorf("%s should NOT contain %q", relPath, unexp)
			}
		}
	}

	// Verify docs files exist
	expectedFiles := []string{
		"mkdocs.yml",
		"docs/index.md",
	}

	for _, f := range expectedFiles {
		path := filepath.Join(tmpDir, f)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Expected file not found: %s", f)
		}
	}

	// mkdocs.yml
	assertFileContains(t, "mkdocs.yml", []string{
		"site_name: ghpages-test",
	})

	// Makefile — GitHub Pages targets
	assertFileContains(t, "Makefile", []string{
		"docs-deploy",
		"gh-deploy --force",
	})

	assertFileNotContains(t, "Makefile", []string{
		"DOCS_BUCKET",
	})

	// .gitignore
	assertFileContains(t, ".gitignore", []string{
		"site/",
	})

	// CI/CD — GitHub Actions
	assertFileContains(t, ".github/workflows/ci_cd.yml", []string{
		"deploy-docs",
		"gh-deploy",
		"permissions",
	})

	// CI/CD — GitLab CI
	assertFileContains(t, ".gitlab-ci.yml", []string{
		"deploy-docs",
		"gh-deploy",
	})
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
		`"server/timeout/read"`,
		`"server/timeout/write"`,
		"writeTimeout", // atomic.Int64 field for dynamic timeout
		"atomic.Int64",
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

	// pkg/app/rest/psg_rest_gen.go — split default constants
	assertFileContains(t, "pkg/app/rest/psg_rest_gen.go", []string{
		"defaultHTTPReadTimeout",
		"defaultHTTPWriteTimeout",
	})

	// pkg/app/rest/mw/psg_metrics_gen.go — timeout logging
	assertFileContains(t, "pkg/app/rest/mw/psg_metrics_gen.go", []string{
		"Request handler timeout exceeded",
		"r.Context().Deadline()",
	})

	// etc/onlineconf/dev/init-config.sql — SQL init for split timeouts (hierarchical: server/timeout/{read,write})
	assertFileContains(t, "etc/onlineconf/dev/init-config.sql", []string{
		"server_timeout_id",
		"'read', @rest_",
		"'write', @rest_",
	})

	// etc/onlineconf/dev/init-config.sql — CORS config in security section
	assertFileContains(t, "etc/onlineconf/dev/init-config.sql", []string{
		"'cors', @security_id",
		"'allow_all', @cors_id",
	})
}

// TestGenerateCLIOnly tests that CLI-only project with spec generates correctly.
func TestGenerateCLIOnly(t *testing.T) {
	curDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting current directory: %v", err)
	}

	configDir := filepath.Join(curDir, "..", "test", "docker-integration", "configs", "cli-only")
	tmpDir := t.TempDir()

	out, err := ExecCommand(filepath.Join(curDir, ".."), "go", []string{
		"run", filepath.Join(curDir, "..", "cmd", "go-project-starter", "main.go"),
		"--target", tmpDir,
		"--configDir", configDir,
		"--config", "project.yaml",
	}, "Generate CLI project ("+tmpDir+")")
	if err != nil {
		t.Fatalf("Error creating project: %s\n%s", err, out)
	}

	t.Logf("CLI project created in %s: %s", tmpDir, out)

	// Verify key files exist
	expectedFiles := []string{
		"Makefile",
		"go.mod",
		"cmd/admin-cli/psg_main_gen.go",
		"internal/app/transport/cli/admin/psg_handler_gen.go",
	}

	for _, f := range expectedFiles {
		path := filepath.Join(tmpDir, f)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Expected file not found: %s", f)
		}
	}

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

	handlerFile := "internal/app/transport/cli/admin/psg_handler_gen.go"

	// Verify Params structs
	assertFileContains(t, handlerFile, []string{
		"type UserCreateParams struct",
		"Email string",
		"type UserListParams struct",
		"Limit int",
		"type MigrateParams struct",
		"Dir string",
		"Steps int",
	})

	// Verify UnimplementedCLI
	assertFileContains(t, handlerFile, []string{
		"type UnimplementedCLI struct{}",
		"func (UnimplementedCLI) RunUserCreate(ctx context.Context, params UserCreateParams) error",
		"func (UnimplementedCLI) RunUserList(ctx context.Context, params UserListParams) error",
		"func (UnimplementedCLI) RunPing(ctx context.Context) error",
		"func (UnimplementedCLI) RunMigrate(ctx context.Context, params MigrateParams) error",
	})

	// Verify Handler struct embeds UnimplementedCLI
	assertFileContains(t, handlerFile, []string{
		"UnimplementedCLI",
		"srv      ds.IService",
	})

	// Verify registerCommands with flag parsing
	assertFileContains(t, handlerFile, []string{
		"func (h *Handler) registerCommands()",
		`fs.String("email", "", "User email")`,
		`flag --email is required`,
		`fs.Int("limit", 100, "Max results")`,
		`fs.String("dir", "up", "Direction: up or down")`,
		"h.RunUserCreate(ctx, UserCreateParams{",
		"h.RunPing(ctx)",
	})

	// Verify Command/Subcommand structs
	assertFileContains(t, handlerFile, []string{
		"type Command struct",
		"Subcommands map[string]*Subcommand",
		"type Subcommand struct",
	})

	// Verify Execute handles subcommands
	assertFileContains(t, handlerFile, []string{
		"if cmd.Subcommands != nil",
		"requires a subcommand",
	})

	// Regression: issue #11 — handler must import ds from runtime, not from project path
	assertFileContains(t, handlerFile, []string{
		`"github.com/Educentr/go-project-starter-runtime/pkg/ds"`,
	})
	// Ensure it does NOT use the project-local ds path
	handlerContent, _ := os.ReadFile(filepath.Join(tmpDir, handlerFile))
	if strings.Contains(string(handlerContent), "/internal/pkg/ds") {
		t.Errorf("handler should import ds from runtime, not from project's internal/pkg/ds")
	}

	// Regression: issue #10 — main must have import alias for CLI handler package
	mainFile := "cmd/admin-cli/psg_main_gen.go"
	assertFileContains(t, mainFile, []string{
		`cliAdmin "github.com/test/clitest/internal/app/transport/cli/admin"`,
	})
}

// TestGenerateQueueWorker tests that queue worker generates correctly from contract.
func TestGenerateQueueWorker(t *testing.T) {
	curDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting current directory: %v", err)
	}

	configDir := filepath.Join(curDir, "..", "test", "docker-integration", "configs", "worker-queue")
	tmpDir := t.TempDir()

	out, err := ExecCommand(filepath.Join(curDir, ".."), "go", []string{
		"run", filepath.Join(curDir, "..", "cmd", "go-project-starter", "main.go"),
		"--target", tmpDir,
		"--configDir", configDir,
		"--config", "project.yaml",
	}, "Generate queue worker project ("+tmpDir+")")
	if err != nil {
		t.Fatalf("Error creating project: %s\n%s", err, out)
	}

	t.Logf("Queue worker project created in %s: %s", tmpDir, out)

	// Verify key files exist
	expectedFiles := []string{
		"Makefile",
		"go.mod",
		"internal/app/worker/task_processor/psg_worker_gen.go",
		"internal/app/worker/task_processor/task_processor/psg_types_gen.go",
		"internal/app/worker/task_processor/task_processor/psg_serializer_gen.go",
		"internal/app/worker/task_processor/task_processor/psg_handler_gen.go",
		"internal/app/worker/task_processor/task_processor/psg_dispatcher_gen.go",
	}

	for _, f := range expectedFiles {
		path := filepath.Join(tmpDir, f)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Expected file not found: %s", f)
		}
	}

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

	tpDir := "internal/app/worker/task_processor/task_processor/"

	// Verify types
	assertFileContains(t, tpDir+"psg_types_gen.go", []string{
		"type EmailsTask struct",
		"TaskID        int64",
		"Attempts      int",
		"PrevStartTime time.Time",
		"To string",
		"Subject string",
		"Body []byte",
		"UserId int64",
		"type NotificationsTask struct",
		"Message string",
		"TargetIds []int64",
		"IsUrgent bool",
	})

	// Verify handler interfaces
	assertFileContains(t, tpDir+"psg_handler_gen.go", []string{
		"type EmailsHandler interface",
		"HandleEmails(ctx context.Context, storage queue.Storage, tasks []*EmailsTask)",
		"type NotificationsHandler interface",
		"HandleNotifications(ctx context.Context, storage queue.Storage, tasks []*NotificationsTask)",
	})

	// Verify dispatcher
	assertFileContains(t, tpDir+"psg_dispatcher_gen.go", []string{
		"type QueueHandlers struct",
		"Emails EmailsHandler",
		"Notifications NotificationsHandler",
		"func NewDispatcher(h QueueHandlers) queue.HandlerFunc",
		"case 1:",
		"case 2:",
		"h.Emails.HandleEmails",
		"h.Notifications.HandleNotifications",
	})

	// Verify serializer
	assertFileContains(t, tpDir+"psg_serializer_gen.go", []string{
		"func SerializeEmailsTask(task *EmailsTask) ([]byte, error)",
		"func DeserializeEmailsTask(data []byte) (*EmailsTask, error)",
		"func SerializeNotificationsTask(task *NotificationsTask) ([]byte, error)",
		"func DeserializeNotificationsTask(data []byte) (*NotificationsTask, error)",
	})

	// Verify worker
	assertFileContains(t, "internal/app/worker/task_processor/psg_worker_gen.go", []string{
		"type Worker struct",
		"daemon.EmptyWorker",
		"queueWorker *queue.QueueWorker",
		`WorkerName      = "task_processor"`,
		"queue.NewMemoryStorage",
		"queue.NewQueueWorker",
		"tp.NewDispatcher",
		"[]int{1, 2}",
		"queue.WithMetrics",
	})
}

// TestObsoleteFileCleanup tests that stale generated files (with disclaimer, no user code)
// are automatically removed during regeneration.
func TestObsoleteFileCleanup(t *testing.T) {
	curDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting current directory: %v", err)
	}

	exampleDir := filepath.Join(curDir, "..", "example")
	tmpDir := t.TempDir()

	// First generation
	out, err := ExecCommand(filepath.Join(curDir, ".."), "go", []string{
		"run", filepath.Join(curDir, "..", "cmd", "go-project-starter", "main.go"),
		"--target", tmpDir,
		"--configDir", exampleDir,
		"--config", "project.yaml",
	}, "Generate project for obsolete file test ("+tmpDir+")")
	if err != nil {
		t.Fatalf("First generation failed: %s\n%s", err, out)
	}

	// Plant a fake obsolete generated file with disclaimer but no user code
	fakeDir := filepath.Join(tmpDir, "pkg", "app", "rest", "mw")
	fakeFile := filepath.Join(fakeDir, "psg_fake_gen.go")

	fakeContent := `package mw

// Code generated by go-project-starter. DO NOT EDIT.

// If you need you can add your code after this message
`
	if err := os.MkdirAll(fakeDir, 0755); err != nil {
		t.Fatalf("Error creating directory: %v", err)
	}

	if err := os.WriteFile(fakeFile, []byte(fakeContent), 0644); err != nil {
		t.Fatalf("Error writing fake file: %v", err)
	}

	// Verify the fake file exists
	if _, err := os.Stat(fakeFile); os.IsNotExist(err) {
		t.Fatalf("Fake file was not created: %s", fakeFile)
	}

	// Regenerate into the same directory
	out, err = ExecCommand(filepath.Join(curDir, ".."), "go", []string{
		"run", filepath.Join(curDir, "..", "cmd", "go-project-starter", "main.go"),
		"--target", tmpDir,
		"--configDir", exampleDir,
		"--config", "project.yaml",
	}, "Regenerate project to test obsolete cleanup ("+tmpDir+")")
	if err != nil {
		t.Fatalf("Regeneration failed: %s\n%s", err, out)
	}

	// Verify the fake file was removed
	if _, err := os.Stat(fakeFile); !os.IsNotExist(err) {
		t.Errorf("Obsolete file should have been removed: %s", fakeFile)
	}
}
