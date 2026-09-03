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

// TestGolangciLintV2Consistency verifies that the generated Makefile uses
// golangci-lint v2 import path with a v2 version tag (fixes #16).
func TestGolangciLintV2Consistency(t *testing.T) {
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
	}, "Generate project for golangci-lint test ("+tmpDir+")")
	if err != nil {
		t.Fatalf("Error creating project: %s\n%s", err, out)
	}

	makefileContent, err := os.ReadFile(filepath.Join(tmpDir, "Makefile"))
	if err != nil {
		t.Fatalf("Error reading Makefile: %v", err)
	}

	makefile := string(makefileContent)

	// GOLANGCI_TAG must be a v2 version
	if strings.Contains(makefile, "GOLANGCI_TAG:=1.") {
		t.Error("GOLANGCI_TAG should use v2 version, not v1")
	}

	if !strings.Contains(makefile, "GOLANGCI_TAG:=2.") {
		t.Error("GOLANGCI_TAG should start with 2.x")
	}

	// install-lint must use v2 import path
	if !strings.Contains(makefile, "golangci-lint/v2/cmd/golangci-lint") {
		t.Error("install-lint should use v2 import path")
	}

	// Version tag and import path must be consistent (both v2)
	if strings.Contains(makefile, "golangci-lint/v2/") && strings.Contains(makefile, "GOLANGCI_TAG:=1.") {
		t.Error("v2 import path with v1 tag — install-lint will fail (issue #16)")
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

	// Verify key files exist (REST server/mw now come from runtime, only client and restconfig are generated)
	expectedFiles := []string{
		"Makefile",
		"go.mod",
		"cmd/api/psg_main_gen.go",
		"pkg/app/logger/psg_logrus_gen.go",
		"pkg/app/rest/psg_client_gen.go",
		"pkg/app/restconfig/psg_config_oc_gen.go",
	}

	for _, f := range expectedFiles {
		path := filepath.Join(tmpDir, f)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Expected file not found: %s", f)
		}
	}

	// Verify REST server/mw files are NOT generated (moved to runtime)
	removedFiles := []string{
		"pkg/app/rest/psg_server_gen.go",
		"pkg/app/rest/psg_closer_gen.go",
		"pkg/app/rest/mw/psg_mw_gen.go",
		"pkg/app/rest/mw/psg_metrics_gen.go",
	}

	for _, f := range removedFiles {
		path := filepath.Join(tmpDir, f)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("File should NOT exist (moved to runtime): %s", f)
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

	// Verify rlog import in generated restconfig (logrus uses rlog alias)
	restconfigFile := filepath.Join(tmpDir, "pkg", "app", "restconfig", "psg_config_oc_gen.go")

	restconfigContent, err := os.ReadFile(restconfigFile)
	if err != nil {
		t.Fatalf("Error reading restconfig file: %v", err)
	}

	if !strings.Contains(string(restconfigContent), `rlog "github.com/Educentr/go-project-starter-runtime/pkg/logger"`) {
		t.Error("Restconfig file should import runtime logger as rlog for logrus")
	}

	// Verify zerolog is NOT used
	zerologFile := filepath.Join(tmpDir, "pkg", "app", "logger", "psg_zlog_gen.go")
	if _, err := os.Stat(zerologFile); !os.IsNotExist(err) {
		t.Error("Zerolog file should NOT exist in logrus project")
	}
}

// TestGenerateStaticTransport verifies the static-files template transport:
// router, drop-zone, main registration and Dockerfile copy are generated.
func TestGenerateStaticTransport(t *testing.T) {
	curDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting current directory: %v", err)
	}

	configDir := filepath.Join(curDir, "..", "test", "docker-integration", "configs", "static")
	tmpDir := t.TempDir()

	out, err := ExecCommand(filepath.Join(curDir, ".."), "go", []string{
		"run", filepath.Join(curDir, "..", "cmd", "go-project-starter", "main.go"),
		"--target", tmpDir,
		"--configDir", configDir,
		"--config", "project.yaml",
	}, "Generate static project ("+tmpDir+")")
	if err != nil {
		t.Fatalf("Error creating project: %s\n%s", err, out)
	}

	t.Logf("Static project created in %s: %s", tmpDir, out)

	expectedFiles := []string{
		"static/.keep",
		"internal/app/transport/rest/static/v1/psg_router_gen.go",
		"cmd/web/psg_main_gen.go",
		"Dockerfile-web",
	}

	for _, f := range expectedFiles {
		if _, err := os.Stat(filepath.Join(tmpDir, f)); os.IsNotExist(err) {
			t.Errorf("Expected file not found: %s", f)
		}
	}

	// Router serves files from the filesystem under the configured prefix, no auth.
	routerContent, err := os.ReadFile(filepath.Join(tmpDir, "internal", "app", "transport", "rest", "static", "v1", "psg_router_gen.go"))
	if err != nil {
		t.Fatalf("Error reading router file: %v", err)
	}

	router := string(routerContent)
	for _, want := range []string{"http.FileServer", "http.Dir", "http.StripPrefix", "mw.EmptyMiddlewares", `route := "/static/"`, `dir := "static"`} {
		if !strings.Contains(router, want) {
			t.Errorf("router should contain %q", want)
		}
	}

	// Main registers the static transport server.
	mainContent, err := os.ReadFile(filepath.Join(tmpDir, "cmd", "web", "psg_main_gen.go"))
	if err != nil {
		t.Fatalf("Error reading main file: %v", err)
	}

	if !strings.Contains(string(mainContent), "static_v1.API{}") {
		t.Error("main should register static_v1.API{}")
	}

	// Dockerfile copies the static folder into the image.
	dockerContent, err := os.ReadFile(filepath.Join(tmpDir, "Dockerfile-web"))
	if err != nil {
		t.Fatalf("Error reading Dockerfile: %v", err)
	}

	docker := string(dockerContent)
	for _, want := range []string{"ADD static /static", "COPY --from=builder /static /static"} {
		if !strings.Contains(docker, want) {
			t.Errorf("Dockerfile should contain %q", want)
		}
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

	// pkg/app/restconfig/psg_config_oc_gen.go — timeout config via OnlineConf (server/mw moved to runtime)
	assertFileContains(t, "pkg/app/restconfig/psg_config_oc_gen.go", []string{
		`"server/timeout/read"`,
		`"server/timeout/write"`,
		"SubscribeTimeoutChanges",
		"ResolveHandlerTimeout",
		"CreateContextWithTimeout",
		"GetServerConfig",
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

	// Regression TOOLS-2 / issue #28 — main.go must not print its own
	// "Available commands:" header; PrintHelp is the sole owner.
	mainContent, err := os.ReadFile(filepath.Join(tmpDir, mainFile))
	if err != nil {
		t.Fatalf("Error reading %s: %v", mainFile, err)
	}

	if strings.Contains(string(mainContent), "Available commands:") {
		t.Errorf("%s should NOT print \"Available commands:\" — PrintHelp already owns that header", mainFile)
	}

	// helpSummary must exist and be used to keep the aligned command list
	// readable when descriptions are multi-line or long.
	assertFileContains(t, handlerFile, []string{
		"func helpSummary(desc string) string",
		"helpSummary(cmd.Description)",
		"helpSummary(cmd.Subcommands[sn].Description)",
	})

	// Regeneration must produce a matching unit test file for help formatting.
	if _, err := os.Stat(filepath.Join(tmpDir, "internal/app/transport/cli/admin/psg_handler_test.go")); os.IsNotExist(err) {
		t.Error("Expected generated handler test file not found: internal/app/transport/cli/admin/psg_handler_test.go")
	}
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

// TestGenerateDaemonWorker tests that daemon worker generates correctly and exposes
// loop timers (NewCycleTimeout, ErrorTimeout) as env-var-overridable variables.
func TestGenerateDaemonWorker(t *testing.T) {
	curDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting current directory: %v", err)
	}

	configDir := filepath.Join(curDir, "..", "test", "docker-integration", "configs", "worker-daemon")
	tmpDir := t.TempDir()

	out, err := ExecCommand(filepath.Join(curDir, ".."), "go", []string{
		"run", filepath.Join(curDir, "..", "cmd", "go-project-starter", "main.go"),
		"--target", tmpDir,
		"--configDir", configDir,
		"--config", "project.yaml",
	}, "Generate daemon worker project ("+tmpDir+")")
	if err != nil {
		t.Fatalf("Error creating project: %s\n%s", err, out)
	}

	t.Logf("Daemon worker project created in %s: %s", tmpDir, out)

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

	assertFileContains(t, "internal/app/worker/daemon/psg_daemon_gen.go", []string{
		`WorkerName      = "daemon"`,
		"func durationFromEnv(key string, def time.Duration) time.Duration",
		`durationFromEnv("WORKER_DAEMON_ERROR_TIMEOUT"`,
		`durationFromEnv("WORKER_DAEMON_NEW_CYCLE_TIMEOUT"`,
		"ErrorTimeout    = durationFromEnv(",
		"NewCycleTimeout = durationFromEnv(",
	})
}

// TestGenerateTelegramWorkerPinsGofrsUUID tests that a project with a telegram worker
// pins github.com/gofrs/uuid/v5 below v5.5.0 in go.mod, since v5.5.0+ raises the go
// directive to 1.25 and breaks `go mod tidy`/build on the project's pinned Go 1.24
// toolchain (TOOLS-4).
func TestGenerateTelegramWorkerPinsGofrsUUID(t *testing.T) {
	curDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting current directory: %v", err)
	}

	configDir := filepath.Join(curDir, "..", "test", "docker-integration", "configs", "worker-telegram")
	tmpDir := t.TempDir()

	out, err := ExecCommand(filepath.Join(curDir, ".."), "go", []string{
		"run", filepath.Join(curDir, "..", "cmd", "go-project-starter", "main.go"),
		"--target", tmpDir,
		"--configDir", configDir,
		"--config", "project.yaml",
	}, "Generate telegram worker project ("+tmpDir+")")
	if err != nil {
		t.Fatalf("Error creating project: %s\n%s", err, out)
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, "go.mod"))
	if err != nil {
		t.Fatalf("Error reading go.mod: %v", err)
	}

	s := string(content)
	if !strings.Contains(s, "require github.com/gofrs/uuid/v5 v5.4.0") {
		t.Errorf("go.mod should pin github.com/gofrs/uuid/v5 to v5.4.0, got:\n%s", s)
	}

	if strings.Contains(s, "gofrs/uuid/v5 v5.5.0") || strings.Contains(s, "gofrs/uuid/v5 v5.5.1") {
		t.Errorf("go.mod should not require gofrs/uuid/v5 v5.5.0+ (requires go >= 1.25), got:\n%s", s)
	}
}

// TestObsoleteFileCleanup tests that stale generated files (with disclaimer, no user code)
// are automatically removed during regeneration.
// TestGenerateTelegramWorkerHooks — шаблон telegram-воркера оставляет проекту точки
// расширения, которые можно переназначить ниже маркера пользовательской секции:
// разбор апдейтов (updateHandler + updateHandlerIdle для GracefulStop) и реакция на
// неизвестную команду (unknownCommandHandler). Без них проект правит Run() руками, и
// regenerate эти правки стирает. Заодно: лог JobStarter не пишет апдейт целиком —
// в сообщении бота может лежать токен или пароль.
func TestGenerateTelegramWorkerHooks(t *testing.T) {
	curDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting current directory: %v", err)
	}

	configDir := filepath.Join(curDir, "..", "test", "docker-integration", "configs", "worker-telegram")
	tmpDir := t.TempDir()

	out, err := ExecCommand(filepath.Join(curDir, ".."), "go", []string{
		"run", filepath.Join(curDir, "..", "cmd", "go-project-starter", "main.go"),
		"--target", tmpDir,
		"--configDir", configDir,
		"--config", "project.yaml",
	}, "Generate telegram worker project ("+tmpDir+")")
	if err != nil {
		t.Fatalf("Error creating project: %s\n%s", err, out)
	}

	read := func(t *testing.T, relPath string) string {
		t.Helper()

		content, err := os.ReadFile(filepath.Join(tmpDir, relPath))
		if err != nil {
			t.Fatalf("Error reading %s: %v", relPath, err)
		}

		return string(content)
	}

	worker := read(t, "internal/app/worker/telegrambot/psg_telegram_gen.go")
	for _, exp := range []string{
		"var updateHandler = (*Worker).handleUpdate",
		"var updateHandlerIdle = func() bool { return true }",
		"func (w *Worker) handleUpdate(ctx context.Context, update tgbotapi.Update)",
		"updateHandler(w, ctx, update)",
		"== 0 && updateHandlerIdle()",
	} {
		if !strings.Contains(worker, exp) {
			t.Errorf("psg_telegram_gen.go should contain %q", exp)
		}
	}

	if strings.Contains(worker, `Interface("Update", update)`) {
		t.Error("JobStarter still logs the whole update: a bot message may carry a token or a password")
	}

	router := read(t, "internal/app/worker/telegrambot/psg_router_gen.go")
	if !strings.Contains(router, "var unknownCommandHandler = func(ctx context.Context, w *Worker, rd telegram.RequestData) {}") {
		t.Error("psg_router_gen.go should declare the unknownCommandHandler hook")
	}

	if got := strings.Count(router, "unknownCommandHandler(ctx, w, requestData)"); got != 3 {
		t.Errorf("unknownCommandHandler should be called on every unknown-command branch (callback, text, document): got %d calls", got)
	}

}

// TestGenerateDevPorts — applications[].dev_ports пробрасываются на сервис приложения в
// docker-compose-dev.yaml: порты REST-транспортов шаблон знает сам, а то, что приложение
// слушает помимо них (отдельный прокси), раньше приходилось дописывать руками.
func TestGenerateDevPorts(t *testing.T) {
	curDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting current directory: %v", err)
	}

	configDir := filepath.Join(curDir, "..", "test", "docker-integration", "configs", "rest-envs-goat")
	tmpDir := t.TempDir()

	out, err := ExecCommand(filepath.Join(curDir, ".."), "go", []string{
		"run", filepath.Join(curDir, "..", "cmd", "go-project-starter", "main.go"),
		"--target", tmpDir,
		"--configDir", configDir,
		"--config", "project.yaml",
	}, "Generate dev-stand project ("+tmpDir+")")
	if err != nil {
		t.Fatalf("Error creating project: %s\n%s", err, out)
	}

	compose, err := os.ReadFile(filepath.Join(tmpDir, "docker-compose-dev.yaml"))
	if err != nil {
		t.Fatalf("Error reading docker-compose-dev.yaml: %v", err)
	}

	if !strings.Contains(string(compose), `- "8102:8102"`) {
		t.Error("docker-compose-dev.yaml should expose applications[].dev_ports on the application service")
	}
}

// TestGenerateLintExcludePaths — tools.lint_exclude_paths попадают в оба блока
// exclusions.paths конфига golangci-lint (linters и formatters): вендорная копия или
// сабмодуль иначе линтуются как свой код, а вписанное руками исключение regenerate стирал.
func TestGenerateLintExcludePaths(t *testing.T) {
	curDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting current directory: %v", err)
	}

	configDir := filepath.Join(curDir, "..", "test", "docker-integration", "configs", "rest-envs-goat")
	tmpDir := t.TempDir()

	out, err := ExecCommand(filepath.Join(curDir, ".."), "go", []string{
		"run", filepath.Join(curDir, "..", "cmd", "go-project-starter", "main.go"),
		"--target", tmpDir,
		"--configDir", configDir,
		"--config", "project.yaml",
	}, "Generate project with lint_exclude_paths ("+tmpDir+")")
	if err != nil {
		t.Fatalf("Error creating project: %s\n%s", err, out)
	}

	cfg, err := os.ReadFile(filepath.Join(tmpDir, "configs", "golangci-lint.yml"))
	if err != nil {
		t.Fatalf("Error reading configs/golangci-lint.yml: %v", err)
	}

	if got := strings.Count(string(cfg), "- etc/vendored"); got != 2 {
		t.Errorf("tools.lint_exclude_paths should appear in both exclusions.paths blocks: got %d occurrences", got)
	}
}

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

// TestGenerateRESTEnvsGoat verifies that when use_envs=true is combined with
// goat_tests=true, the generated Makefile does NOT invoke onlineconf-update-tests
// or generate-test-env-<app> — env-based projects don't rely on OnlineConf to
// bootstrap test env vars, so the whole onlineconf plumbing must be skipped.
func TestGenerateRESTEnvsGoat(t *testing.T) {
	curDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting current directory: %v", err)
	}

	configDir := filepath.Join(curDir, "..", "test", "docker-integration", "configs", "rest-envs-goat")
	tmpDir := t.TempDir()

	out, err := ExecCommand(filepath.Join(curDir, ".."), "go", []string{
		"run", filepath.Join(curDir, "..", "cmd", "go-project-starter", "main.go"),
		"--target", tmpDir,
		"--configDir", configDir,
		"--config", "project.yaml",
	}, "Generate rest-envs-goat project ("+tmpDir+")")
	if err != nil {
		t.Fatalf("Error creating project: %s\n%s", err, out)
	}

	makefileBytes, err := os.ReadFile(filepath.Join(tmpDir, "Makefile"))
	if err != nil {
		t.Fatalf("Error reading Makefile: %v", err)
	}
	mk := string(makefileBytes)

	// Negative assertions: no onlineconf bootstrap for use_envs+goat_tests.
	// Note: the generic install-onlineconf-updater target and its listing in
	// install-tools are unconditionally generated, so we only forbid the GOAT-
	// block call-site, not the target itself.
	forbidden := []string{
		"onlineconf-update-tests",
		"generate-test-env-api",
		"$(MAKE) install-onlineconf-updater || true",
		"onlineconf-updater -config tests/etc/onlineconf-updater.conf",
		"goat/cmd/testutil",
	}
	for _, s := range forbidden {
		if strings.Contains(mk, s) {
			t.Errorf("Makefile must NOT contain %q for use_envs+goat_tests app", s)
		}
	}

	// Positive assertions: goat-tests-api still exists but depends only on
	// build_for_test-api. Walk lines to find the target definitions.
	var foundGoatTests, foundGoatTestsVerbose bool
	for _, line := range strings.Split(mk, "\n") {
		if strings.HasPrefix(line, "goat-tests-api:") {
			foundGoatTests = true
			if strings.Contains(line, "generate-test-env-api") {
				t.Errorf("goat-tests-api must not depend on generate-test-env-api: %q", line)
			}
			if !strings.Contains(line, "build_for_test-api") {
				t.Errorf("goat-tests-api should still depend on build_for_test-api: %q", line)
			}
		}
		if strings.HasPrefix(line, "goat-tests-api-verbose:") {
			foundGoatTestsVerbose = true
			if strings.Contains(line, "generate-test-env-api") {
				t.Errorf("goat-tests-api-verbose must not depend on generate-test-env-api: %q", line)
			}
		}
	}
	if !foundGoatTests {
		t.Error("Expected goat-tests-api target in generated Makefile")
	}
	if !foundGoatTestsVerbose {
		t.Error("Expected goat-tests-api-verbose target in generated Makefile")
	}

	// Regression guard: run-api should also skip onlineconf-update for use_envs.
	for _, line := range strings.Split(mk, "\n") {
		if strings.HasPrefix(line, "run-api:") && strings.Contains(line, "onlineconf-update") {
			t.Errorf("run-api must not depend on onlineconf-update when use_envs=true: %q", line)
		}
	}
}
