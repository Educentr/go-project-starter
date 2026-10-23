package dockerintegration

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	gtt "github.com/Educentr/goat"
	"github.com/Educentr/goat/services"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// GolangBuilderContainer is a custom container type for the golang builder
type GolangBuilderContainer struct {
	testcontainers.Container
}

// testConfig holds configuration for a single test case
type testConfig struct {
	name        string
	configDir   string
	appName     string // application name for make run-{appName}
	requiresTG  bool   // whether this test requires telegram mock
	serviceName string // service name for onlineconf env vars (without hyphens)
	projectName string // project name from main.name in config (for docker image)

	// restPorts overrides the default REST transport port env vars passed to
	// the running container as OC_<service>__transport__rest__<key>__port=<value>.
	// When nil, the default {"api_v1": 8080, "sys_v1": 8085} layout is used.
	// Tests with non-default transport names (e.g. multiple ogen services) must
	// set this explicitly — otherwise the binary fails on port lookup.
	restPorts map[string]int
}

const (
	serviceName = "golang-builder"
)

var (
	env         *gtt.Env
	projectRoot string
)

func init() {
	var err error

	projectRoot, err = getProjectRoot()
	if err != nil {
		panic(fmt.Errorf("failed to get project root: %w", err))
	}

	// Check if binary exists (built by Makefile)
	binaryPath := filepath.Join(projectRoot, "bin", "go-project-starter")
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		panic("Binary not found. Run 'make buildfortest' first")
	}

	// Register golang-builder service
	services.MustRegisterServiceFuncTyped(serviceName, RunGolangBuilder)

	// Register telegram-mock service for tests with telegram
	services.MustRegisterServiceFuncTyped(telegramMockServiceName, RunTelegramMock)

	// Initialize test environment
	servicesMap := services.NewServicesMap(serviceName, telegramMockServiceName)
	manager := services.NewManager(servicesMap, services.DefaultManagerConfig())

	env = gtt.NewEnv(gtt.EnvConfig{}, manager)
}

// getProjectRoot returns the absolute path to the project root directory
func getProjectRoot() (string, error) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("failed to get current file path")
	}
	// Go up from test/docker-integration to project root
	return filepath.Abs(filepath.Join(filepath.Dir(filename), "..", ".."))
}

// RunGolangBuilder creates and starts a golang builder container
func RunGolangBuilder(ctx context.Context, opts ...testcontainers.ContainerCustomizer) (*GolangBuilderContainer, error) {
	imageName := os.Getenv("TEST_IMAGE")
	if imageName == "" {
		return nil, fmt.Errorf("TEST_IMAGE environment variable is not set. Run 'make build-test-image' first")
	}

	req := testcontainers.ContainerRequest{
		Image:      imageName,
		WaitingFor: wait.ForLog("ready").WithStartupTimeout(5 * time.Minute),
		Cmd:        []string{"sh", "-c", "echo ready && tail -f /dev/null"},
		// Mount Docker socket for building images inside container (stage 3)
		Binds: []string{"/var/run/docker.sock:/var/run/docker.sock"},
	}

	// Apply customizers
	genericReq := testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	}

	for _, opt := range opts {
		if err := opt.Customize(&genericReq); err != nil {
			return nil, fmt.Errorf("failed to apply customizer: %w", err)
		}
	}

	container, err := testcontainers.GenericContainer(ctx, genericReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}

	// Setup container: copy binary and configs
	builderContainer := &GolangBuilderContainer{Container: container}
	if err := setupContainer(ctx, builderContainer); err != nil {
		_ = container.Terminate(ctx) //nolint:errcheck
		return nil, fmt.Errorf("failed to setup container: %w", err)
	}

	return builderContainer, nil
}

// setupContainer copies binary and configs to the container after it starts
func setupContainer(ctx context.Context, container *GolangBuilderContainer) error {
	binaryPath := filepath.Join(projectRoot, "bin", "go-project-starter")

	// Copy binary to container
	if err := container.CopyFileToContainer(ctx, binaryPath, "/usr/local/bin/go-project-starter", 0755); err != nil {
		return fmt.Errorf("failed to copy binary: %w", err)
	}

	// Create test-configs directory
	if _, _, err := container.Exec(ctx, []string{"mkdir", "-p", "/workspace/test-configs"}); err != nil {
		return fmt.Errorf("failed to create test-configs dir: %w", err)
	}

	// Copy each config directory separately
	configDirs := []string{
		"rest-only", "rest-logrus", "grpc-only", "worker-telegram", "combined", "grafana",
		"rest-remote-http", "rest-remote-git", "rest-only-rewrite-refs",
		"rest-shared-spec-rewrite-refs", "cli-only", "rest-client-basic-auth",
		"rest-client-mocks", "rest-envs-goat",
	}
	for _, dir := range configDirs {
		srcPath := filepath.Join(projectRoot, "test/docker-integration/configs", dir)
		dstPath := fmt.Sprintf("/workspace/test-configs/%s", dir)

		if err := container.CopyDirToContainer(ctx, srcPath, dstPath, 0755); err != nil {
			return fmt.Errorf("failed to copy config %s: %w", dir, err)
		}
	}

	return nil
}

// TestMain is the entry point for all tests in this package
func TestMain(m *testing.M) {
	gtt.CallMain(env, m)
}

// GetContainer returns the golang builder container
func GetContainer() *GolangBuilderContainer {
	return services.MustGetTyped[*GolangBuilderContainer](env.Manager(), serviceName)
}

// GetTelegramMock returns the telegram mock container
func GetTelegramMock() *TelegramMockContainer {
	return services.MustGetTyped[*TelegramMockContainer](env.Manager(), telegramMockServiceName)
}

// readOutput reads the output from exec result
func readOutput(reader io.Reader) string {
	if reader == nil {
		return ""
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Sprintf("error reading output: %v", err)
	}

	return string(data)
}

// getTestConfigs returns configuration for all test cases
func getTestConfigs() map[string]testConfig {
	return map[string]testConfig{
		"rest-only": {
			name:        "rest-only",
			configDir:   "rest-only",
			appName:     "api",
			requiresTG:  false,
			serviceName: "resttest",
			projectName: "resttest",
		},
		"grpc-only": {
			name:        "grpc-only",
			configDir:   "grpc-only",
			appName:     "api",
			requiresTG:  false,
			serviceName: "grpcclienttest",
			projectName: "grpcclienttest",
		},
		"worker-telegram": {
			name:        "worker-telegram",
			configDir:   "worker-telegram",
			appName:     "bot",
			requiresTG:  true,
			serviceName: "telegramtest",
			projectName: "telegramtest",
		},
		"combined": {
			name:        "combined",
			configDir:   "combined",
			appName:     "api", // Test api first (no telegram dependency)
			requiresTG:  false,
			serviceName: "combinedtest",
			projectName: "combinedtest",
		},
		"grafana": {
			name:        "grafana",
			configDir:   "grafana",
			appName:     "api",
			requiresTG:  false,
			serviceName: "grafanatest",
			projectName: "grafanatest",
		},
		"rest-logrus": {
			name:        "rest-logrus",
			configDir:   "rest-logrus",
			appName:     "api",
			requiresTG:  false,
			serviceName: "resttest",
			projectName: "resttest",
		},
		"rest-remote-http": {
			name:        "rest-remote-http",
			configDir:   "rest-remote-http",
			appName:     "api",
			requiresTG:  false,
			serviceName: "restremotehttp",
			projectName: "restremotehttp",
		},
		"rest-remote-git": {
			name:        "rest-remote-git",
			configDir:   "rest-remote-git",
			appName:     "api",
			requiresTG:  false,
			serviceName: "restremotegit",
			projectName: "restremotegit",
		},
		"rest-only-rewrite-refs": {
			name:        "rest-only-rewrite-refs",
			configDir:   "rest-only-rewrite-refs",
			appName:     "api",
			requiresTG:  false,
			serviceName: "resttestrewrite",
			projectName: "resttestrewrite",
		},
		"rest-shared-spec-rewrite-refs": {
			name:        "rest-shared-spec-rewrite-refs",
			configDir:   "rest-shared-spec-rewrite-refs",
			appName:     "api",
			requiresTG:  false,
			serviceName: "resttestsharedrewrite",
			projectName: "resttestsharedrewrite",
			// Project defines three REST transports (svc_a, svc_b, sys), so
			// the default {api_v1, sys_v1} port envs don't apply.
			restPorts: map[string]int{
				"svc_a_v1": 8081,
				"svc_b_v1": 8082,
				"sys_v1":   8085,
			},
		},
	}
}

// runDockerTest builds and runs the Docker image
func runDockerTest(ctx context.Context, t *testing.T, container *GolangBuilderContainer, outputDir string, cfg testConfig) {
	t.Helper()
	t.Logf("Stage 2: Running Docker test for %s (make docker-%s)...", cfg.name, cfg.appName)

	// Initialize git repo (required for Makefile's git describe/show commands)
	gitInitCmd := fmt.Sprintf(
		"cd %s && git init && git config user.email 'test@test.com' && git config user.name 'Test' && git add . && git commit -m 'initial'",
		outputDir,
	)
	code, outputReader, err := container.Exec(ctx, []string{"sh", "-c", gitInitCmd})
	output := readOutput(outputReader)

	require.NoError(t, err, "failed to init git")
	require.Equal(t, 0, code, "Git init failed: %s", output)

	// Build Docker image
	// Note: Makefile uses SERVICE_NAME from the generated config (projectName), we just set REGISTRY_SERVER and DOCKER_TAG
	imageName := fmt.Sprintf("local/%s-%s:latest", cfg.projectName, cfg.appName)
	buildCmd := fmt.Sprintf(
		"cd %s && DOCKER_TAG=latest REGISTRY_SERVER=local make docker-%s",
		outputDir, cfg.appName,
	)

	t.Logf("Building Docker image: %s", imageName)

	code, outputReader, err = container.Exec(ctx, []string{"sh", "-c", buildCmd})
	output = readOutput(outputReader)

	if os.Getenv("GOAT_DISABLE_STDOUT") != "true" {
		t.Logf("Docker build output:\n%s", output)
	}

	require.NoError(t, err, "failed to build Docker image")
	require.Equal(t, 0, code, "Docker build failed: %s", output)

	// Get telegram mock URL if needed
	var telegramMockURL string

	if cfg.requiresTG {
		tgMock := GetTelegramMock()

		var err error
		telegramMockURL, err = tgMock.GetInternalURL(ctx)
		require.NoError(t, err, "failed to get telegram mock URL")
	}

	// Run the Docker container
	containerName := fmt.Sprintf("%s-%s-test-container", cfg.name, cfg.appName)
	fullImageName := fmt.Sprintf("local/%s-%s:latest", cfg.projectName, cfg.appName)

	// Build env var flags for docker run.
	// Include essential onlineconf values for transport initialization.
	// REST port envs default to the {api_v1: 8080, sys_v1: 8085} layout; tests
	// with non-default transport names override via cfg.restPorts.
	// The process logger reads its level at /{svc}/default/log_level — the
	// prefix of a process-wide logger is the service root (see TOOLS-65).
	envFlags := fmt.Sprintf(
		"-e ONLINECONFIG_FROM_ENV=true "+
			"-e OC_%s__devstand=1 "+
			"-e OC_%s__default__log_level=debug",
		cfg.serviceName, cfg.serviceName,
	)

	restPorts := cfg.restPorts
	if restPorts == nil {
		restPorts = map[string]int{"api_v1": 8080, "sys_v1": 8085}
	}
	// Stable ordering for reproducible docker run command in test logs.
	keys := make([]string, 0, len(restPorts))
	for k := range restPorts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		envFlags += fmt.Sprintf(" -e OC_%s__transport__rest__%s__port=%d", cfg.serviceName, k, restPorts[k])
	}

	if cfg.requiresTG && telegramMockURL != "" {
		// The telegram library expects endpoint as format string: http://host:port/bot%s/%s
		tgEndpoint := telegramMockURL + "/bot%s/%s"
		envFlags += fmt.Sprintf(
			" -e OC_%s__security__tg_token=test_token_123"+
				" -e OC_%s__security__tg_api_endpoint=%s",
			cfg.serviceName, cfg.serviceName, tgEndpoint,
		)
	}

	runCmd := fmt.Sprintf(
		"docker run -d --name %s %s -p 18085:8085 %s",
		containerName, envFlags, fullImageName,
	)

	t.Logf("Starting Docker container: %s", containerName)

	code, outputReader, err = container.Exec(ctx, []string{"sh", "-c", runCmd})
	output = readOutput(outputReader)

	require.NoError(t, err, "failed to start Docker container")
	require.Equal(t, 0, code, "Docker run failed: %s", output)

	// Ensure we stop and remove the container on test completion
	defer func() {
		t.Logf("Stopping Docker container: %s", containerName)
		stopCmd := fmt.Sprintf("docker stop %s && docker rm %s", containerName, containerName)
		_, _, _ = container.Exec(ctx, []string{"sh", "-c", stopCmd}) //nolint:errcheck
	}()

	// Wait for container to be ready (check readiness endpoint)
	// Note: The sys template provides /ready endpoint, not /health
	healthCheckCmd := fmt.Sprintf(
		"docker exec %s wget -q -O /dev/null http://localhost:8085/ready && echo 200 || echo 000",
		containerName,
	)
	maxRetries := 30

	var lastCode string

	for i := 0; i < maxRetries; i++ {
		time.Sleep(1 * time.Second)

		code, outputReader, err := container.Exec(ctx, []string{"sh", "-c", healthCheckCmd})
		if err != nil {
			t.Logf("Docker health check attempt %d failed: %v", i+1, err)
			continue
		}

		if code != 0 {
			t.Logf("Docker health check attempt %d: command failed", i+1)
			continue
		}

		lastCode = strings.TrimSpace(readOutput(outputReader))
		t.Logf("Docker health check attempt %d: %s", i+1, lastCode)

		// Check if the output ends with "200" (may have prefix from exec)
		if lastCode == "200" || strings.HasSuffix(lastCode, "200") {
			t.Logf("Docker container is healthy!")
			return
		}
	}

	// If we get here, health check failed
	// Print container logs for debugging
	logsCmd := fmt.Sprintf("docker logs %s 2>&1 || echo 'No logs available'", containerName)
	_, logsReader, _ := container.Exec(ctx, []string{"sh", "-c", logsCmd}) //nolint:errcheck
	logs := readOutput(logsReader)
	t.Logf("Docker container logs:\n%s", logs)

	require.Fail(t, fmt.Sprintf("Docker health check failed after %d attempts, last status: %s", maxRetries, lastCode))
}

// runTest runs a single integration test
func runTest(t *testing.T, testName string, configDir string) {
	t.Helper()
	ctx := t.Context()
	container := GetContainer()

	outputDir := fmt.Sprintf("/workspace/output/%s", testName)
	projectConfigDir := fmt.Sprintf("%s/.project-config", outputDir)

	// Create output directory and copy config
	setupCmd := fmt.Sprintf("mkdir -p %s && cp -r /workspace/test-configs/%s/. %s/",
		projectConfigDir, configDir, projectConfigDir)

	code, outputReader, err := container.Exec(ctx, []string{"sh", "-c", setupCmd})
	require.NoError(t, err, "failed to setup directories")
	require.Equal(t, 0, code, "setup failed: %s", readOutput(outputReader))

	// Run generator
	t.Logf("Running go-project-starter for %s...", testName)

	genCmd := fmt.Sprintf("go-project-starter --target=%s --configDir=%s", outputDir, projectConfigDir)
	code, outputReader, err = container.Exec(ctx, []string{"sh", "-c", genCmd})
	output := readOutput(outputReader)

	if os.Getenv("GOAT_DISABLE_STDOUT") != "true" {
		t.Logf("Generator output:\n%s", output)
	}

	require.NoError(t, err, "failed to execute generator")
	require.Equal(t, 0, code, "generator failed: %s", output)

	// Check go.mod exists
	code, _, err = container.Exec(ctx, []string{"test", "-f", outputDir + "/go.mod"})
	require.NoError(t, err)
	require.Equal(t, 0, code, "go.mod not found in generated project")

	// Run make generate to generate ogen/buf code
	t.Logf("Running make generate for %s...", testName)

	generateCmd := fmt.Sprintf("cd %s && make generate", outputDir)
	code, outputReader, err = container.Exec(ctx, []string{"sh", "-c", generateCmd})
	output = readOutput(outputReader)

	if os.Getenv("GOAT_DISABLE_STDOUT") != "true" {
		t.Logf("make generate output:\n%s", output)
	}

	require.NoError(t, err, "failed to execute make generate")
	require.Equal(t, 0, code, "make generate failed: %s", output)

	// Run go mod tidy
	t.Logf("Running go mod tidy for %s...", testName)

	tidyCmd := fmt.Sprintf("cd %s && go mod tidy", outputDir)
	code, outputReader, err = container.Exec(ctx, []string{"sh", "-c", tidyCmd})
	output = readOutput(outputReader)

	if os.Getenv("GOAT_DISABLE_STDOUT") != "true" {
		t.Logf("go mod tidy output:\n%s", output)
	}

	require.NoError(t, err, "failed to execute go mod tidy")
	require.Equal(t, 0, code, "go mod tidy failed: %s", output)

	// Run go build
	t.Logf("Running go build for %s...", testName)

	buildCmd := fmt.Sprintf("cd %s && go build ./...", outputDir)
	code, outputReader, err = container.Exec(ctx, []string{"sh", "-c", buildCmd})
	output = readOutput(outputReader)

	if os.Getenv("GOAT_DISABLE_STDOUT") != "true" {
		t.Logf("go build output:\n%s", output)
	}

	require.NoError(t, err, "failed to execute go build")
	require.Equal(t, 0, code, "go build failed: %s", output)

	t.Logf("Stage 1 (Generation and Build) PASSED for %s", testName)

	// Get test configuration for stage 2 (Docker)
	configs := getTestConfigs()

	cfg, exists := configs[testName]
	if !exists {
		t.Logf("No extended test config for %s, skipping Docker stage", testName)
		return
	}

	// Stage 2: Docker test
	runDockerTest(ctx, t, container, outputDir, cfg)
	t.Logf("Stage 2 (Docker) PASSED for %s", testName)

	t.Logf("All stages PASSED for %s", testName)
}

// TestIntegrationRESTOnly tests REST-only project generation
func TestIntegrationRESTOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	runTest(t, "rest-only", "rest-only")
}

// TestIntegrationGRPCOnly tests gRPC-only project generation
func TestIntegrationGRPCOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	runTest(t, "grpc-only", "grpc-only")
}

// TestIntegrationWorkerTelegram tests Telegram worker project generation
func TestIntegrationWorkerTelegram(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	runTest(t, "worker-telegram", "worker-telegram")
}

// TestIntegrationCombined tests combined project generation (REST + gRPC + Workers)
func TestIntegrationCombined(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	runTest(t, "combined", "combined")
}

func TestIntegrationGrafana(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	runTest(t, "grafana", "grafana")
}

// TestIntegrationRESTLogrus tests REST project generation with logrus logger
func TestIntegrationRESTLogrus(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	runTest(t, "rest-logrus", "rest-logrus")
}

// TestIntegrationCLIOnly tests CLI-only project generation and then exercises the
// built binary: the CLI scaffold's help output is only verifiable by running it.
func TestIntegrationCLIOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	runTest(t, "cli-only", "cli-only")
	assertCLIHelpBehavior(t.Context(), t, GetContainer(), "/workspace/output/cli-only")
}

// runCLI builds the generated CLI binary (once per call is cheap — the build
// cache is warm after runTest) and runs it with args, returning combined output.
func runCLI(ctx context.Context, t *testing.T, container *GolangBuilderContainer, outputDir, args string) string {
	t.Helper()

	cmd := fmt.Sprintf("cd %s && go build -o ./bin/admin-cli ./cmd/admin-cli && ./bin/admin-cli %s", outputDir, args)
	_, outputReader, err := container.Exec(ctx, []string{"sh", "-c", cmd})
	require.NoError(t, err, "failed to run CLI with args %q", args)

	out := readOutput(outputReader)
	t.Logf("$ admin-cli %s\n%s", args, out)

	return out
}

// assertCLIHelpBehavior checks the generated CLI's usage output by running the
// binary (TOOLS-2 / issue #28). These are behavioral assertions on purpose:
// the duplicated "Available commands:" header lived in main.go while PrintHelp
// always printed it exactly once, so a unit test of PrintHelp — or a grep over
// the templates — passes both before and after the fix.
func assertCLIHelpBehavior(ctx context.Context, t *testing.T, container *GolangBuilderContainer, outputDir string) {
	t.Helper()

	// Text that only ever appears in the second paragraph of the 'seed'
	// description in the cli-only fixture.
	const fullDescTail = "must not leak into the command list"

	t.Log("Checking no-args usage output...")

	noArgs := runCLI(ctx, t, container, outputDir, "")

	require.Equal(t, 1, strings.Count(noArgs, "Available commands:"),
		"no-args usage must print the command-list header exactly once:\n%s", noArgs)
	require.Contains(t, noArgs, "Seed the database with \"fixtures\".",
		"command list must show the first line of a multi-line description:\n%s", noArgs)
	require.NotContains(t, noArgs, fullDescTail,
		"command list must not contain the full multi-line description:\n%s", noArgs)

	// Every listed command must occupy exactly one line.
	for _, line := range strings.Split(noArgs, "\n") {
		if strings.HasPrefix(line, "  ") && strings.Contains(line, "  ") {
			require.LessOrEqual(t, len(line), 100, "command list line is too long to stay readable: %q", line)
		}
	}

	t.Log("Checking '<command> --help' (advertised by the usage footer)...")

	require.Contains(t, noArgs, "Run '<command> --help' for more information about a command.",
		"usage footer missing:\n%s", noArgs)

	seedHelp := runCLI(ctx, t, container, outputDir, "seed --help")
	require.Contains(t, seedHelp, "Command: seed", "seed --help must print detailed help:\n%s", seedHelp)
	require.Contains(t, seedHelp, fullDescTail,
		"detailed help must print the FULL description:\n%s", seedHelp)
	require.Contains(t, seedHelp, "--file", "detailed help must list flags:\n%s", seedHelp)

	// A --help that reaches flag.Parse must print help, not fail the command.
	seedFlagHelp := runCLI(ctx, t, container, outputDir, "seed --file x --help")
	require.Contains(t, seedFlagHelp, "Command: seed",
		"--help after a flag must print help, not report a command failure:\n%s", seedFlagHelp)
	require.NotContains(t, seedFlagHelp, "CLI command failed",
		"--help after a flag must not be reported as an error:\n%s", seedFlagHelp)

	// A command with no flags must not *execute* on --help.
	pingHelp := runCLI(ctx, t, container, outputDir, "ping --help")
	require.Contains(t, pingHelp, "Command: ping", "ping --help must print help:\n%s", pingHelp)
	require.NotContains(t, pingHelp, "is not implemented",
		"ping --help must not execute the command:\n%s", pingHelp)

	t.Log("Checking subcommand help reachability...")

	for _, args := range []string{"user create --help", "help user create"} {
		out := runCLI(ctx, t, container, outputDir, args)
		require.Contains(t, out, "Command: user create", "%q must print subcommand help:\n%s", args, out)
		require.Contains(t, out, "Requires --email",
			"%q must print the FULL subcommand description:\n%s", args, out)
	}
}

// assertSpecDownloaded verifies that the named OpenAPI spec was materialised
// at api/rest/api/v1/<basename> inside the generated project AND that ogen
// produced server code from it. This is the assertion that distinguishes
// remote-spec tests from the plain rest-only flow: we want to see that
// resolveAllSources actually fetched the bytes, not silently skipped.
func assertSpecDownloaded(ctx context.Context, t *testing.T, container *GolangBuilderContainer, outputDir, basename string) {
	t.Helper()

	specPath := fmt.Sprintf("%s/api/rest/api/v1/%s", outputDir, basename)
	code, _, err := container.Exec(ctx, []string{"test", "-s", specPath})
	require.NoError(t, err, "stat spec file")
	require.Equal(t, 0, code, "expected non-empty spec at %s", specPath)

	// Spot-check that the spec is a real OpenAPI document, not an HTML error
	// page or empty staging file.
	checkCmd := fmt.Sprintf("head -c 200 %s | grep -q '^openapi:'", specPath)
	code, _, err = container.Exec(ctx, []string{"sh", "-c", checkCmd})
	require.NoError(t, err)
	require.Equal(t, 0, code, "spec at %s should start with 'openapi:'", specPath)

	// ogen must have run against the downloaded spec — the generated server
	// package always contains an oas.Server type declaration.
	oasPath := fmt.Sprintf("%s/pkg/rest/api/v1/oas_server_gen.go", outputDir)
	code, _, err = container.Exec(ctx, []string{"sh", "-c", "test -s " + oasPath})
	require.NoError(t, err)
	require.Equal(t, 0, code, "ogen should have produced %s from the downloaded spec", oasPath)
}

// TestIntegrationRESTOnlyRewriteRefs verifies that rewrite_refs: true on a
// REST transport rewrites cross-directory $ref values (../common/foo.yml#/X)
// into local sibling references (./foo.yml#/X) after the generator copies
// specs into api/rest/<svc>/<ver>/. ogen would otherwise fail to resolve the
// ../common/ path because both files end up in the same flat directory.
func TestIntegrationRESTOnlyRewriteRefs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	runTestWithExtraAssertions(t, "rest-only-rewrite-refs", "rest-only-rewrite-refs",
		func(ctx context.Context, t *testing.T, container *GolangBuilderContainer, outputDir string) {
			t.Helper()

			// After rewrite, api.swagger.yml must reference ./common.swagger.yml.
			apiSpec := fmt.Sprintf("%s/api/rest/api/v1/api.swagger.yml", outputDir)
			grepLocal := fmt.Sprintf("grep -F './common.swagger.yml' %s", apiSpec)
			code, _, err := container.Exec(ctx, []string{"sh", "-c", grepLocal})
			require.NoError(t, err)
			require.Equal(t, 0, code, "expected rewritten $ref './common.swagger.yml' in %s", apiSpec)

			// And the original ../common/ path must be gone.
			grepParent := fmt.Sprintf("grep -F '../common/common.swagger.yml' %s", apiSpec)
			code, _, err = container.Exec(ctx, []string{"sh", "-c", grepParent})
			require.NoError(t, err)
			require.NotEqual(t, 0, code, "expected ../common/ $ref to be rewritten out of %s", apiSpec)
		})
}

// TestIntegrationRESTSharedSpecRewriteRefs covers the regression where two
// REST services in the same project pull the same shared spec
// (common/common.swagger.yml). The resolver's per-staging-dir uniqueness
// guard prefixes the second occurrence with `1-`; before the
// SpecTargetFiles propagation fix that prefix leaked into the per-service
// target dir, refrewrite's whitelist (built from canonical basenames) did
// not recognise the prefixed sibling, and ogen failed on the unresolved
// `../common/common.swagger.yml` $ref.
//
// After the fix BOTH services must land at the canonical basename in their
// own target dir AND refrewrite must rewrite the cross-dir ref to a local
// sibling in both transport-specific copies of svc_a.swagger.yml and
// svc_b.swagger.yml.
func TestIntegrationRESTSharedSpecRewriteRefs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	runTestWithExtraAssertions(t, "rest-shared-spec-rewrite-refs", "rest-shared-spec-rewrite-refs",
		func(ctx context.Context, t *testing.T, container *GolangBuilderContainer, outputDir string) {
			t.Helper()

			for _, svc := range []string{"svc_a", "svc_b"} {
				// Each service must receive the shared file at its canonical
				// basename — not `1-common.swagger.yml`.
				canonical := fmt.Sprintf("%s/api/rest/%s/v1/common.swagger.yml", outputDir, svc)
				code, _, err := container.Exec(ctx, []string{"test", "-s", canonical})
				require.NoError(t, err, "stat canonical common spec for %s", svc)
				require.Equal(t, 0, code, "expected canonical common spec at %s (staging prefix leaked into target dir)", canonical)

				prefixed := fmt.Sprintf("%s/api/rest/%s/v1/1-common.swagger.yml", outputDir, svc)
				code, _, err = container.Exec(ctx, []string{"test", "-e", prefixed})
				require.NoError(t, err)
				require.NotEqual(t, 0, code, "staging-collision `1-` prefix leaked into target dir: %s exists", prefixed)

				// refrewrite must have rewritten the cross-dir $ref into a
				// local sibling form in this service's per-transport copy.
				svcSpec := fmt.Sprintf("%s/api/rest/%s/v1/%s.swagger.yml", outputDir, svc, svc)
				grepLocal := fmt.Sprintf("grep -F './common.swagger.yml' %s", svcSpec)
				code, _, err = container.Exec(ctx, []string{"sh", "-c", grepLocal})
				require.NoError(t, err)
				require.Equal(t, 0, code, "expected rewritten $ref './common.swagger.yml' in %s", svcSpec)

				grepParent := fmt.Sprintf("grep -F '../common/common.swagger.yml' %s", svcSpec)
				code, _, err = container.Exec(ctx, []string{"sh", "-c", grepParent})
				require.NoError(t, err)
				require.NotEqual(t, 0, code, "expected ../common/ $ref to be rewritten out of %s", svcSpec)

				// ogen must have run against the per-service spec — the
				// generated server package always contains an oas.Server type.
				oasPath := fmt.Sprintf("%s/pkg/rest/%s/v1/oas_server_gen.go", outputDir, svc)
				code, _, err = container.Exec(ctx, []string{"sh", "-c", "test -s " + oasPath})
				require.NoError(t, err)
				require.Equal(t, 0, code, "ogen should have produced %s for %s", oasPath, svc)
			}
		})
}

// TestIntegrationRESTRemoteSpecHTTP tests that rest.path can be a public
// https:// URL: the generator fetches the spec, copies it into api/, ogen
// runs against the local copy, and the resulting service builds and starts
// in Docker.
//
// The spec URL points at example/example.swagger.yml in this same
// repository on the main branch — keeps the fixture self-contained.
func TestIntegrationRESTRemoteSpecHTTP(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	runTestWithExtraAssertions(t, "rest-remote-http", "rest-remote-http", func(ctx context.Context, t *testing.T, container *GolangBuilderContainer, outputDir string) {
		t.Helper()
		assertSpecDownloaded(ctx, t, container, outputDir, "example.swagger.yml")
	})
}

// TestIntegrationRESTRemoteSpecGit tests that rest.path can be a
// git+https:// URI: the generator clones the public repo, extracts the
// referenced subpath, and the result builds and runs identically to a
// project with a local spec.
//
// Like the HTTP variant, the spec lives in this same repository
// (Educentr/go-project-starter, public, ref main) — no external dependency.
func TestIntegrationRESTRemoteSpecGit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	runTestWithExtraAssertions(t, "rest-remote-git", "rest-remote-git", func(ctx context.Context, t *testing.T, container *GolangBuilderContainer, outputDir string) {
		t.Helper()
		assertSpecDownloaded(ctx, t, container, outputDir, "example.swagger.yml")
	})
}

// TestIntegrationRESTClientBasicAuth covers auth_params {transport: header,
// type: basic} on ogen_client: a spec that declares HTTP Basic the standard
// way (a scheme named BasicAuth) must yield a wrapper implementing ogen's
// SecuritySource.BasicAuth, in both instantiation modes, that compiles with
// no post-generation patching.
//
// The assertion hook inspects the wrapper the generator writes; the generic
// stage-1 pipeline (make generate + go mod tidy + go build) is what proves
// the result actually compiles against the ogen-generated SecuritySource
// interface. No Docker stage: starting the binary would only re-check boot,
// not the credentials path.
func TestIntegrationRESTClientBasicAuth(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	runTestWithExtraAssertions(t, "rest-client-basic-auth", "rest-client-basic-auth",
		func(ctx context.Context, t *testing.T, container *GolangBuilderContainer, outputDir string) {
			t.Helper()

			for _, client := range []string{"billing", "audit"} {
				authFile := fmt.Sprintf("%s/internal/app/transport/rest/%s/v1/psg_auth_gen.go", outputDir, client)

				grepMethod := fmt.Sprintf("grep -F 'func (s *SecuritySource) BasicAuth(' %s", authFile)
				code, _, err := container.Exec(ctx, []string{"sh", "-c", grepMethod})
				require.NoError(t, err)
				require.Equal(t, 0, code, "expected a BasicAuth SecuritySource method in %s", authFile)

				// Credentials come from OnlineConf, one node each, under the
				// client's auth_params subtree.
				for _, leaf := range []string{"username", "password"} {
					grepLeaf := fmt.Sprintf(`grep -F '"auth_params", "%s"' %s`, leaf, authFile)
					code, _, err = container.Exec(ctx, []string{"sh", "-c", grepLeaf})
					require.NoError(t, err)
					require.Equal(t, 0, code, "expected %s to be read from auth_params/%s", authFile, leaf)
				}

				// The wrapper lives in this very package — importing it here
				// is a self-import and breaks the build with an import cycle.
				grepSelfImport := fmt.Sprintf("grep -F '/internal/app/transport/rest/%s/v1\"' %s", client, authFile)
				code, _, err = container.Exec(ctx, []string{"sh", "-c", grepSelfImport})
				require.NoError(t, err)
				require.NotEqual(t, 0, code, "%s must not import its own package", authFile)

				// Both static and dynamic clients must pass the wrapper to ogen.
				clientFile := fmt.Sprintf("%s/internal/app/transport/rest/%s/v1/psg_client_gen.go", outputDir, client)
				grepSource := fmt.Sprintf("grep -F '&SecuritySource{},' %s", clientFile)
				code, _, err = container.Exec(ctx, []string{"sh", "-c", grepSource})
				require.NoError(t, err)
				require.Equal(t, 0, code, "expected &SecuritySource{} to be passed to NewClient in %s", clientFile)
			}

			// dev_stand seeds the OnlineConf nodes the wrapper reads: a basic
			// auth client needs username/password, not the apikey node.
			devSQL := fmt.Sprintf("%s/etc/onlineconf/dev/init-config.sql", outputDir)
			for _, leaf := range []string{"username", "password"} {
				grepSeed := fmt.Sprintf("grep -F \"VALUES ('%s', @rest_client_billing_v1_auth_id\" %s", leaf, devSQL)
				code, _, err := container.Exec(ctx, []string{"sh", "-c", grepSeed})
				require.NoError(t, err)
				require.Equal(t, 0, code, "expected dev-stand config to seed auth_params/%s for billing", leaf)
			}
		})
}

// TestIntegrationRESTEnvsGoat covers generation of a goat_tests project that
// pins Go 1.24, with go_mod_tidy in post_generate — the combination that used
// to abort generation outright (TOOLS-51).
//
// goat-services imports testcontainers-go, whose docker/client is tested by
// docker/testutil, which imports otlptrace. `go mod tidy` must resolve those
// test-only imports of dependencies as well, and since no module in the graph
// requires them it reaches for @latest, which needs Go >= 1.26. go.mod.tmpl
// pins the otlptrace line for goat projects so that tidy stays resolvable on
// Go 1.24; this test is what fails if the pin is dropped or goes stale.
//
// Only reproduces inside the golang image because it ships GOTOOLCHAIN=local
// — with the default `auto`, tidy would silently download Go 1.26 and pass.
// assertGoatTidyPreconditions asserts that, so the test cannot quietly become
// vacuous. No Docker stage: the regression is in generation, not at runtime.
func TestIntegrationRESTEnvsGoat(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	runTestWithExtraAssertions(t, "rest-envs-goat", "rest-envs-goat",
		func(ctx context.Context, t *testing.T, container *GolangBuilderContainer, outputDir string) {
			t.Helper()
			assertGoatTidyPreconditions(ctx, t, container, outputDir)
		})
}

// assertGoatTidyPreconditions verifies that the goat fixture still exercises
// the TOOLS-51 regression, and that tidy kept the otlptrace pin.
//
// The first two checks guard the test itself: the defect only reproduces on a
// project pinned below Go 1.26 running under a toolchain that refuses to
// upgrade. If either precondition silently changes — the fixture gets bumped,
// or the image stops setting GOTOOLCHAIN=local — generation would pass for
// reasons unrelated to the fix, and the regression would be unguarded again.
func assertGoatTidyPreconditions(ctx context.Context, t *testing.T, container *GolangBuilderContainer, outputDir string) {
	t.Helper()

	// Precondition 1: the toolchain may not upgrade itself past the pin.
	code, outputReader, err := container.Exec(ctx, []string{"go", "env", "GOTOOLCHAIN"})
	require.NoError(t, err, "go env GOTOOLCHAIN")
	require.Equal(t, 0, code, "go env GOTOOLCHAIN failed")
	require.Contains(t, readOutput(outputReader), "local",
		"test image must keep GOTOOLCHAIN=local, otherwise tidy auto-upgrades and this test proves nothing")

	// Precondition 2: the generated module still targets Go 1.24.
	goMod := outputDir + "/go.mod"
	code, _, err = container.Exec(ctx, []string{"grep", "-qE", `^go 1\.24`, goMod})
	require.NoError(t, err, "grep go directive")
	require.Equal(t, 0, code,
		"fixture must stay pinned to Go 1.24 in %s, otherwise it no longer reproduces TOOLS-51", goMod)

	// The pin itself must still be there. The version is deliberately not
	// asserted: reaching this point already means tidy resolved the graph
	// under the two preconditions above, so whatever version survived is
	// Go 1.24-compatible by construction — pinning the exact string here
	// would only mean editing two places on every otel bump.
	code, _, err = container.Exec(ctx, []string{"grep", "-qF",
		"go.opentelemetry.io/otel/exporters/otlp/otlptrace", goMod})
	require.NoError(t, err, "grep otlptrace requirement")
	require.Equal(t, 0, code,
		"expected an otlptrace requirement in %s — without it tidy resolves @latest and breaks on Go 1.24", goMod)
}

// runTestWithExtraAssertions is runTest with an extra assertion hook fired
// after generation completes but before make generate. This is where we
// verify remote-spec-specific invariants without duplicating the full
// generate / build / docker pipeline.
func runTestWithExtraAssertions(t *testing.T, testName, configDir string, after func(ctx context.Context, t *testing.T, container *GolangBuilderContainer, outputDir string)) {
	t.Helper()
	ctx := t.Context()
	container := GetContainer()

	outputDir := fmt.Sprintf("/workspace/output/%s", testName)
	projectConfigDir := fmt.Sprintf("%s/.project-config", outputDir)

	setupCmd := fmt.Sprintf("mkdir -p %s && cp -r /workspace/test-configs/%s/. %s/",
		projectConfigDir, configDir, projectConfigDir)
	code, outputReader, err := container.Exec(ctx, []string{"sh", "-c", setupCmd})
	require.NoError(t, err, "failed to setup directories")
	require.Equal(t, 0, code, "setup failed: %s", readOutput(outputReader))

	t.Logf("Running go-project-starter for %s...", testName)
	genCmd := fmt.Sprintf("go-project-starter --target=%s --configDir=%s", outputDir, projectConfigDir)
	code, outputReader, err = container.Exec(ctx, []string{"sh", "-c", genCmd})
	output := readOutput(outputReader)

	if os.Getenv("GOAT_DISABLE_STDOUT") != "true" {
		t.Logf("Generator output:\n%s", output)
	}

	require.NoError(t, err, "failed to execute generator")
	require.Equal(t, 0, code, "generator failed: %s", output)

	// Hook: verify that the remote spec actually landed in api/. This must
	// run after generate (which performs the resolve+copy pre-pass) but
	// before make generate (which would fail anyway if the spec is missing,
	// but with a much less informative error than this targeted check).
	if after != nil {
		after(ctx, t, container, outputDir)
	}

	t.Logf("Running make generate for %s...", testName)
	generateCmd := fmt.Sprintf("cd %s && make generate", outputDir)
	code, outputReader, err = container.Exec(ctx, []string{"sh", "-c", generateCmd})
	output = readOutput(outputReader)

	if os.Getenv("GOAT_DISABLE_STDOUT") != "true" {
		t.Logf("make generate output:\n%s", output)
	}

	require.NoError(t, err, "failed to execute make generate")
	require.Equal(t, 0, code, "make generate failed: %s", output)

	t.Logf("Running go mod tidy for %s...", testName)
	tidyCmd := fmt.Sprintf("cd %s && go mod tidy", outputDir)
	code, outputReader, err = container.Exec(ctx, []string{"sh", "-c", tidyCmd})
	output = readOutput(outputReader)

	if os.Getenv("GOAT_DISABLE_STDOUT") != "true" {
		t.Logf("go mod tidy output:\n%s", output)
	}

	require.NoError(t, err, "failed to execute go mod tidy")
	require.Equal(t, 0, code, "go mod tidy failed: %s", output)

	t.Logf("Running go build for %s...", testName)
	buildCmd := fmt.Sprintf("cd %s && go build ./...", outputDir)
	code, outputReader, err = container.Exec(ctx, []string{"sh", "-c", buildCmd})
	output = readOutput(outputReader)

	if os.Getenv("GOAT_DISABLE_STDOUT") != "true" {
		t.Logf("go build output:\n%s", output)
	}

	require.NoError(t, err, "failed to execute go build")
	require.Equal(t, 0, code, "go build failed: %s", output)

	t.Logf("Stage 1 (Generation and Build) PASSED for %s", testName)

	configs := getTestConfigs()

	cfg, exists := configs[testName]
	if !exists {
		t.Logf("No extended test config for %s, skipping Docker stage", testName)
		return
	}

	runDockerTest(ctx, t, container, outputDir, cfg)
	t.Logf("Stage 2 (Docker) PASSED for %s", testName)
	t.Logf("All stages PASSED for %s", testName)
}
