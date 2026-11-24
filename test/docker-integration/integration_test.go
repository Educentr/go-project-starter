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

// setupTestEnv resolves the project root, checks the prerequisites and builds the
// goat environment shared by every test in the package. Called from TestMain so
// that a missing prerequisite is a readable error on stderr instead of a panic
// stack from package initialization.
func setupTestEnv() error {
	root, err := getProjectRoot()
	if err != nil {
		return fmt.Errorf("failed to get project root: %w", err)
	}

	projectRoot = root

	// Check if binary exists (built by Makefile)
	binaryPath := filepath.Join(projectRoot, "bin", "go-project-starter")
	if _, err := os.Stat(binaryPath); err != nil {
		return fmt.Errorf("generator binary %s is not usable, run 'make buildfortest' first: %w", binaryPath, err)
	}

	// Register golang-builder service
	services.MustRegisterServiceFuncTyped(serviceName, RunGolangBuilder)

	// Register telegram-mock service for tests with telegram
	services.MustRegisterServiceFuncTyped(telegramMockServiceName, RunTelegramMock)

	// Initialize test environment
	servicesMap := services.NewServicesMap(serviceName, telegramMockServiceName)
	manager := services.NewManager(servicesMap, services.DefaultManagerConfig())

	env = gtt.NewEnv(gtt.EnvConfig{}, manager)

	return nil
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
		"rest-client-mocks", "rest-envs-goat", "cli-client", "onlineconf-map",
		"metrics-map",
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
	if err := setupTestEnv(); err != nil {
		fmt.Fprintln(os.Stderr, "docker-integration setup failed:", err)
		os.Exit(1)
	}

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

	// Compile-only — a service application would start listening. Last, after
	// the image build, so every stage above sees the project as generated.
	assertEarlyInitCompiles(t.Context(), t, GetContainer(), "/workspace/output/rest-only", "api")
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
	assertTelegramPollerRotation(t.Context(), t, GetContainer(), "/workspace/output/worker-telegram")
}

// assertTelegramPollerRotation runs the generated telegram tests against the
// generated tree, with the race detector on.
//
// The project ships them — psg_telegram_poller_test.go in the driver (a fake Bot
// API, a token swap under a live long poll, the offset carried over to a new
// token of the same bot and dropped on another one, shutdown without a leaked
// goroutine) and psg_telegram_loop_test.go in the worker (a closed update
// channel must leave the loop, not feed the handler zero updates). The build
// stage of this file compiles the generated project but never runs its tests,
// so without this step the generator could ship a driver whose own tests fail.
//
// Race detector, not just `go test`: the bug behind these tests was a bare write
// to the driver's bot field from the OnlineConf watcher goroutine while request
// goroutines read it, and that is what `-race` answers for.
func assertTelegramPollerRotation(ctx context.Context, t *testing.T, container *GolangBuilderContainer, outputDir string) {
	t.Helper()

	cmd := fmt.Sprintf("cd %s && go test -race -count=1 -v ./pkg/drivers/telegram/... ./internal/app/worker/...", outputDir)
	code, outputReader, err := container.Exec(ctx, []string{"sh", "-c", cmd})
	output := readOutput(outputReader)

	if os.Getenv("GOAT_DISABLE_STDOUT") != "true" {
		t.Logf("telegram driver tests output:\n%s", output)
	}

	require.NoError(t, err, "failed to run the telegram tests of the generated project")
	require.Equal(t, 0, code, "the telegram tests of the generated project failed: %s", output)

	// The tests have to have run, not just the packages have to have compiled: a
	// test template dropped from the output would otherwise read as a pass.
	for _, name := range []string{
		"TestTelegramPollerFollowsRotatedToken",
		"TestTelegramPollerResetsOffsetOnAnotherBot",
		"TestWorkerLoopLeavesClosedChannel",
	} {
		require.Contains(t, output, name,
			"the generated project must carry %s: %s", name, output)
	}
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

// TestIntegrationOnlineConfMap builds a project whose OnlineConf key map
// declares keys of every shape, runtime segments included.
//
// The accessor package is generated Go that nothing in this repository compiles:
// the unit tests parse it and read signatures out of it, which is why a key map
// could name a runtime segment after an identifier the package uses itself and
// produce a package that parses, generates and does not build (TOOLS-89). Here
// the generated project is actually built.
func TestIntegrationOnlineConfMap(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	runTest(t, "onlineconf-map", "onlineconf-map")

	// Last: it leaves a test file of its own in the project.
	assertDerivedContextBehavior(t.Context(), t, GetContainer(), "/workspace/output/onlineconf-map")
}

// assertDerivedContextBehavior runs the contract of the generated
// derived-context helper and the generated OnlineConf test fixture against the
// generated project (TOOLS-132, issue #158 items 2 and 3).
//
// It has to run, not be read. The claim is that a context derived through the
// helper still answers a configuration read after its parent was canceled —
// three packages have to agree on that: the helper, the accessors and the
// library's context plumbing. And the fixture's claim is that the read answers
// from a real tree rather than from the declared default, which is only visible
// if a value is actually in the tree and differs from the default.
func assertDerivedContextBehavior(ctx context.Context, t *testing.T, container *GolangBuilderContainer, outputDir string) {
	t.Helper()

	writeDerivedContextProbe(ctx, t, container, outputDir)

	cmd := fmt.Sprintf("cd %s && go test -race -count=1 ./internal/pkg/appctx/", outputDir)

	code, outputReader, err := container.Exec(ctx, []string{"sh", "-c", cmd})
	require.NoError(t, err, "failed to run the derived-context probe")

	out := readOutput(outputReader)
	t.Logf("derived-context probe:\n%s", out)

	require.Equal(t, 0, code, "the generated derived-context helper does not hold its contract:\n%s", out)
}

// writeDerivedContextProbe puts a test of the generated helper and fixture into
// the generated project.
//
// Written here rather than generated into every project, for the same reason as
// the state-family probe: the generator does not ship tests of its own output,
// and this one asserts the contract of machinery that is identical in every
// project. It lives in the helper's own package so that it reads the way a
// project's test would — Detach unqualified, the accessors and the fixture
// imported.
func writeDerivedContextProbe(ctx context.Context, t *testing.T, container *GolangBuilderContainer, outputDir string) {
	t.Helper()

	const probe = `package appctx

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/test/octest/internal/pkg/occonf"
	"github.com/test/octest/internal/pkg/occonf/occonftest"
)

// treeContext is the fixture's whole job: a context that carries a real tree,
// with values that differ from the declared defaults so that an answer coming
// from the default is visible as such.
func treeContext(t *testing.T) context.Context {
	t.Helper()

	return occonftest.Context(t, map[string]string{
		occonf.PathSupervisorMinLifetime: "11m",
		occonf.PathSupervisorMaxAgents:   "7",
		occonf.PathQueueName:             "probe-queue",
	})
}

// The fixture answers from the tree, not from the key map's defaults.
func TestFixtureReadsFromTheTree(t *testing.T) {
	ctx := treeContext(t)

	lifetime, err := occonf.SupervisorMinLifetime(ctx)
	if err != nil {
		t.Fatalf("read a duration from the fixture's tree: %v", err)
	}

	if lifetime != 11*time.Minute {
		t.Errorf("the fixture read %v, want 11m — the declared default is 5m, so this answered from the default", lifetime)
	}

	agents, err := occonf.SupervisorMaxAgents(ctx)
	if err != nil {
		t.Fatalf("read a required key from the fixture's tree: %v", err)
	}

	if agents != 7 {
		t.Errorf("the fixture read %d agents, want 7", agents)
	}

	name, ok, err := occonf.QueueName(ctx)
	if err != nil || !ok || name != "probe-queue" {
		t.Errorf("the fixture read queue %q (present=%v, err=%v), want \"probe-queue\"", name, ok, err)
	}
}

// A key the fixture was not given is absent, and the accessors say so the way
// their declarations promise: a default for a key with one, presence reported
// for an optional key, ErrKeyNotSet for a required one.
func TestFixtureLeavesUndeclaredKeysAbsent(t *testing.T) {
	ctx := occonftest.Context(t, nil)

	lifetime, err := occonf.SupervisorMinLifetime(ctx)
	if err != nil {
		t.Fatalf("read a key with a default from an empty tree: %v", err)
	}

	if lifetime != 5*time.Minute {
		t.Errorf("an absent key with a declared default read %v, want the declared 5m", lifetime)
	}

	if _, ok, err := occonf.QueueName(ctx); err != nil || ok {
		t.Errorf("an absent optional key reported present=%v (err=%v)", ok, err)
	}

	if _, err := occonf.SupervisorMaxAgents(ctx); !errors.Is(err, occonf.ErrKeyNotSet) {
		t.Errorf("an absent required key reported %v, want ErrKeyNotSet", err)
	}
}

// The reason the helper exists: work that must outlive its parent still reads
// configuration. This is the defect the guard addresses, stated as a test — on
// a context.Background() the same read would not answer from a default, it
// would dereference the instance that is not in the context.
func TestDetachedContextStillReadsOnlineConf(t *testing.T) {
	parent, cancel := context.WithCancel(treeContext(t))

	detached, stop := Detach(parent, 0)
	defer stop()

	cancel()

	if parent.Err() == nil {
		t.Fatal("the parent should be canceled at this point")
	}

	if err := detached.Err(); err != nil {
		t.Fatalf("the detached context must outlive its parent, it is %v", err)
	}

	if _, ok := detached.Deadline(); ok {
		t.Error("a zero timeout must leave the detached context without a deadline")
	}

	lifetime, err := occonf.SupervisorMinLifetime(detached)
	if err != nil {
		t.Fatalf("read on the detached context: %v", err)
	}

	if lifetime != 11*time.Minute {
		t.Errorf("the detached context read %v, want the tree's 11m: it lost the configuration", lifetime)
	}

	// The cancel the helper returns is the only thing that ends a context
	// detached with no timeout.
	stop()

	if !errors.Is(detached.Err(), context.Canceled) {
		t.Errorf("the returned cancel should end the detached context, it is %v", detached.Err())
	}
}

// The optional timeout: a deadline of its own, and the values still there up to
// and past it — a shutdown step reads configuration while it winds down.
func TestDetachedContextHonorsTheTimeout(t *testing.T) {
	parent, cancel := context.WithCancel(treeContext(t))
	defer cancel()

	detached, stop := Detach(parent, 50*time.Millisecond)
	defer stop()

	deadline, ok := detached.Deadline()
	if !ok {
		t.Fatal("a positive timeout should give the detached context a deadline")
	}

	if until := time.Until(deadline); until <= 0 || until > time.Second {
		t.Errorf("the deadline is %v away, want about 50ms", until)
	}

	cancel()

	select {
	case <-detached.Done():
		t.Fatal("the detached context must not end with its parent")
	case <-time.After(10 * time.Millisecond):
	}

	if _, err := occonf.SupervisorMinLifetime(detached); err != nil {
		t.Fatalf("read on the detached context before its deadline: %v", err)
	}

	select {
	case <-detached.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("the detached context should have hit its own deadline")
	}

	if !errors.Is(detached.Err(), context.DeadlineExceeded) {
		t.Errorf("the detached context ended with %v, want DeadlineExceeded", detached.Err())
	}

	// Expiry ends the context; it does not empty it. A read after the deadline
	// is a read the caller should not be making, but it must fail for its own
	// reason rather than panic on a missing tree.
	if _, err := occonf.SupervisorMinLifetime(detached); err != nil {
		t.Errorf("the expired context lost its values: %v", err)
	}
}
`

	writeCmd := fmt.Sprintf("cat > %s/internal/pkg/appctx/derived_context_probe_test.go <<'PROBE_EOF'\n%s\nPROBE_EOF",
		outputDir, probe)

	code, outputReader, err := container.Exec(ctx, []string{"sh", "-c", writeCmd})
	require.NoError(t, err, "failed to write the derived-context probe")
	require.Equal(t, 0, code, "writing the derived-context probe failed: %s", readOutput(outputReader))
}

// TestIntegrationCLIOnly tests CLI-only project generation and then exercises the
// built binary: the CLI scaffold's help output is only verifiable by running it.
func TestIntegrationCLIOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	runTest(t, "cli-only", "cli-only")
	assertCLIHelpBehavior(t.Context(), t, GetContainer(), "/workspace/output/cli-only")
	assertCLIRepeatedFlagBehavior(t.Context(), t, GetContainer(), "/workspace/output/cli-only")

	// Last: it leaves a file of its own in the project and rebuilds the binary,
	// so the assertions above see the project as the generator produced it.
	assertEarlyInitHook(t.Context(), t, GetContainer(), "/workspace/output/cli-only", "admin-cli")
}

// TestIntegrationCLIClient covers a CLI application that consumes an
// ogen_client (issue #24): a CLI tool calling an HTTP API — ops script, bot,
// wrapper agent — used to be unrepresentable, because a rest entry had to be
// hosted by some application and a CLI was not allowed to host one.
//
// The CLI wires the client into the service in its own main: it has no HTTP
// listener and no metrics registry, so it never reaches the runtime's app.Init,
// where a server application's clients are initialized. Stage 1 (make generate
// + go mod tidy + go build) is the assertion that matters — that wiring has to
// compile against the generated service and the ogen client alike. No Docker
// stage: there is no long-running process to boot.
func TestIntegrationCLIClient(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	runTest(t, "cli-client", "cli-client")

	// After stage 1, not through runTestWithExtraAssertions: that hook fires
	// right after the generator, before `make generate` — there is no binary to
	// run yet. Running one is the only way to see that the client is
	// initialized at startup rather than handed to the handler as a nil
	// pointer: the binary has no OnlineConf tree here, so a non-optional client
	// cannot be built and the command must not run at all.
	//
	// Asserted on the two messages that are the wiring, not on the reason the
	// client was unusable: whether the read reports "URL not configured" (empty
	// value) or a missing TREE module depends on what onlineconf state the host
	// happens to have.
	out := runCLI(t.Context(), t, GetContainer(), "/workspace/output/cli-client", "status")
	require.Contains(t, out, "can't initialize clients",
		"the CLI's main must initialize its clients at startup:\n%s", out)
	require.Contains(t, out, "error init billing client",
		"the client init has to go through the service's setClients:\n%s", out)
	require.NotContains(t, out, "is not implemented",
		"a command must not be dispatched after client init failed:\n%s", out)
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

// assertCLIRepeatedFlagBehavior checks that the generated CLI refuses a flag
// given twice (TOOLS-96 / issue #124 item 3).
//
// Behavioral on purpose: the defect was flag.FlagSet's own documented
// behavior — it keeps the last value and drops the rest — so nothing in the
// generated source looked wrong, and the only way to see the answer silently
// change is to ask the binary for it.
func assertCLIRepeatedFlagBehavior(ctx context.Context, t *testing.T, container *GolangBuilderContainer, outputDir string) {
	t.Helper()

	t.Log("Checking a repeated flag is refused rather than resolved to its last value...")

	for _, args := range []string{
		"migrate --dir up --dir down",     // separate values
		"migrate --dir=up --dir=down",     // inline values
		"migrate -dir up --dir down",      // the flag package accepts one dash or two
		"user create --email a --email b", // on a subcommand too
	} {
		out := runCLI(ctx, t, container, outputDir, args)
		require.Contains(t, out, "is specified more than once",
			"%q must be refused, not resolved to the last value:\n%s", args, out)
	}

	// The scan must not mistake a value for a flag of its own: a negative
	// number is the ordinary case, and counting it as a repeat would reject a
	// command line that is perfectly well formed.
	t.Log("Checking a value that looks like a flag is not counted as one...")

	negative := runCLI(ctx, t, container, outputDir, "migrate --dir -1 --steps -1")
	require.NotContains(t, negative, "is specified more than once",
		"a value starting with '-' must not be counted as a repeated flag:\n%s", negative)

	// Distinct flags of the same command stay acceptable; the only error left
	// is the unimplemented handler the scaffold ships.
	t.Log("Checking distinct flags still parse...")

	distinct := runCLI(ctx, t, container, outputDir, "migrate --dir up --steps 3")
	require.NotContains(t, distinct, "is specified more than once",
		"distinct flags must parse:\n%s", distinct)
	require.Contains(t, distinct, "is not implemented",
		"distinct flags must reach the handler:\n%s", distinct)
}

// assertSpecDownloaded verifies that the named OpenAPI spec was materialized
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
// not recognize the prefixed sibling, and ogen failed on the unresolved
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
				// client's auth_params subtree — read through the generated
				// accessor package, which is the only place the library is
				// called from (see configs/golangci-onlineconf.yml).
				clientPkg := fmt.Sprintf("%s_v1", client)

				for _, leaf := range []string{"Username", "Password"} {
					grepLeaf := fmt.Sprintf(`grep -F 'occonf.RESTClient%s(ctx, "%s")' %s`, leaf, clientPkg, authFile)
					code, _, err = container.Exec(ctx, []string{"sh", "-c", grepLeaf})
					require.NoError(t, err)
					require.Equal(t, 0, code, "expected %s to read %s through the accessor package", authFile, leaf)
				}

				// And the path those accessors build is still the auth_params
				// subtree of this client: the move behind the accessors must
				// not move the keys.
				occonfFile := fmt.Sprintf("%s/internal/pkg/occonf/psg_occonf_gen.go", outputDir)

				for _, leaf := range []string{"username", "password"} {
					grepPath := fmt.Sprintf(`grep -F '"transport", "rest", transport, "auth_params", "%s"' %s`,
						leaf, occonfFile)
					code, _, err = container.Exec(ctx, []string{"sh", "-c", grepPath})
					require.NoError(t, err)
					require.Equal(t, 0, code, "expected the accessor of auth_params/%s to keep its path", leaf)
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

// writeEarlyInitOverride drops the override into cmd/<app>/ of the generated
// project and rebuilds it.
func writeEarlyInitOverride(ctx context.Context, t *testing.T, container *GolangBuilderContainer, outputDir, app string) {
	t.Helper()

	// earlyInitOverride is the project's implementation of the early extension
	// point (TOOLS-123 / issue #142): a method on *UserEarlyInit in a file of
	// package main. No init(), no //nolint:gochecknoinits — which is the whole
	// point of the hook.
	//
	// The two env switches are what make the hook observable from outside the
	// process: one makes it fail, the other makes it change the environment
	// occonf.Initialize builds the configuration tree from.
	const earlyInitOverride = `package main

import (
	"errors"
	"fmt"
	"os"
)

func (u *UserEarlyInit) earlyInit() error {
	fmt.Fprintln(os.Stderr, "project-early-init-ran")

	if os.Getenv("EARLY_INIT_FAIL") != "" {
		return errors.New("early init refused to start the process")
	}

	if os.Getenv("EARLY_INIT_BREAK_CONFIG") != "" {
		return os.Setenv("ONLINECONFIG_FROM_ENV", "1")
	}

	return nil
}
`

	target := fmt.Sprintf("%s/cmd/%s/early_init.go", outputDir, app)
	write := fmt.Sprintf("cat > %s <<'PSGEOF'\n%sPSGEOF", target, earlyInitOverride)

	code, outputReader, err := container.Exec(ctx, []string{"sh", "-c", write})
	require.NoError(t, err, "failed to write the early init override")
	require.Equal(t, 0, code, "writing %s failed: %s", target, readOutput(outputReader))

	buildCmd := fmt.Sprintf("cd %s && go build ./...", outputDir)
	code, outputReader, err = container.Exec(ctx, []string{"sh", "-c", buildCmd})
	output := readOutput(outputReader)

	require.NoError(t, err, "failed to build the project with the override")
	require.Equal(t, 0, code,
		"a project that overrides earlyInit in its own file of package main has to compile: %s", output)
}

// assertEarlyInitCompiles is the service-application half (cmd): the generated
// main declares the hook, and a project that overrides it compiles. There is
// nothing to run here — a service application would start listening.
func assertEarlyInitCompiles(ctx context.Context, t *testing.T, container *GolangBuilderContainer, outputDir, app string) {
	t.Helper()

	writeEarlyInitOverride(ctx, t, container, outputDir, app)
}

// assertEarlyInitHook is the CLI half (cmd_cli), where the hook can be observed
// from outside: the binary runs to completion in one process.
//
// The run that matters is the third one. The hook sets ONLINECONFIG_FROM_ENV,
// and the process then dies on a configuration tree it cannot build from the
// environment — which can only happen if the hook ran before
// occonf.Initialize. That is the ordering the feature is about, and no amount
// of grepping the generated source proves it.
func assertEarlyInitHook(ctx context.Context, t *testing.T, container *GolangBuilderContainer, outputDir, app string) {
	t.Helper()

	writeEarlyInitOverride(ctx, t, container, outputDir, app)

	binary := fmt.Sprintf("./bin/%s", app)

	buildCmd := fmt.Sprintf("cd %s && go build -o %s ./cmd/%s", outputDir, binary, app)
	code, outputReader, err := container.Exec(ctx, []string{"sh", "-c", buildCmd})
	require.NoError(t, err, "failed to build the CLI binary")
	require.Equal(t, 0, code, "building the CLI with the override failed: %s", readOutput(outputReader))

	runCLIEnv := func(env string) (int, string) {
		t.Helper()

		cmd := fmt.Sprintf("cd %s && %s%s", outputDir, env, binary)
		code, outputReader, err := container.Exec(ctx, []string{"sh", "-c", cmd})
		require.NoError(t, err, "failed to run the CLI with env %q", env)

		out := readOutput(outputReader)
		t.Logf("$ %s%s (exit %d)\n%s", env, binary, code, out)

		return code, out
	}

	// 1. The hook runs, and the command is dispatched as before: overriding it
	//    does not change anything else about the process.
	_, out := runCLIEnv("")
	require.Contains(t, out, "project-early-init-ran", "the overridden hook has to be called:\n%s", out)
	require.Contains(t, out, "Available commands:", "the command still has to be dispatched:\n%s", out)

	// 2. A failing hook stops the process with its own exit code, before the
	//    command: ExitCodeErrorEarlyInit is the last of the CLI's iota block.
	code, out = runCLIEnv("EARLY_INIT_FAIL=1 ")
	require.Equal(t, 5, code, "a failing hook exits with ExitCodeErrorEarlyInit:\n%s", out)
	require.Contains(t, out, "early initialization failed", "the failure is reported on stderr:\n%s", out)
	require.NotContains(t, out, "Available commands:", "no command may run after the hook failed:\n%s", out)

	// 3. The hook's change to the environment reaches occonf.Initialize —
	//    the process cannot build a tree from the environment here, so it dies
	//    with a configuration error instead of running the command.
	code, out = runCLIEnv("EARLY_INIT_BREAK_CONFIG=1 ")
	require.NotEqual(t, 0, code,
		"the hook changed the environment Initialize reads, so startup must fail:\n%s", out)
	require.Contains(t, out, "project-early-init-ran", "the hook has to run first:\n%s", out)
	require.NotContains(t, out, "Available commands:",
		"the hook runs before occonf.Initialize: a tree that cannot be built stops the process "+
			"before the command:\n%s", out)
}

// TestIntegrationMetricsMap builds a project whose metrics map declares every
// shape of entry, state families (type GaugeSet) included, and then exercises
// the generated collectors in the generated project.
//
// Both halves need the container. The collector package is generated Go that
// nothing in this repository compiles — the unit tests read identifiers out of
// its text — and a state family is the one entry whose output is more than a
// constructor call: a generic collector, a sample struct per family and an
// atomic slot. And its contract is behavioral: a scrape must never see a
// half-filled set, a series dropped from the set must stop being exported, a
// family with no source yet must read empty rather than fail, and a set the
// registry would refuse must cost its own family rather than the whole
// endpoint. None of that is visible in the text of the file.
func TestIntegrationMetricsMap(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	runTest(t, "metrics-map", "metrics-map")

	// Last: it leaves a test file of its own in the project.
	assertStateFamilyBehavior(t.Context(), t, GetContainer(), "/workspace/output/metrics-map")
}

// assertStateFamilyBehavior runs the state-family contract against the
// collectors the generator produced, inside the generated project.
func assertStateFamilyBehavior(ctx context.Context, t *testing.T, container *GolangBuilderContainer, outputDir string) {
	t.Helper()

	writeStateFamilyProbe(ctx, t, container, outputDir)

	// -race: the whole reason a state family is not a GaugeVec is what
	// concurrent scrapes see while the set is being replaced.
	cmd := fmt.Sprintf("cd %s && go test -race -count=1 ./internal/pkg/metrics/", outputDir)

	code, outputReader, err := container.Exec(ctx, []string{"sh", "-c", cmd})
	require.NoError(t, err, "failed to run the state-family probe")

	out := readOutput(outputReader)
	t.Logf("state-family probe:\n%s", out)

	require.Equal(t, 0, code, "the generated state families do not hold their contract:\n%s", out)
}

// writeStateFamilyProbe puts a test of the generated collectors into the
// generated project.
//
// It is written here rather than generated into every project on purpose: the
// generator does not ship tests of its own output, and this one asserts the
// contract of the machinery, which is the same in every project that declares
// a state family.
func writeStateFamilyProbe(ctx context.Context, t *testing.T, container *GolangBuilderContainer, outputDir string) {
	t.Helper()

	const probe = `package metrics

import (
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// fleetSeries scrapes the registry the way Prometheus does and returns the
// fleet_state family as "<node>/<zone>" -> value.
func fleetSeries(t *testing.T, reg *prometheus.Registry) map[string]float64 {
	t.Helper()

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}

	out := map[string]float64{}

	for _, family := range families {
		if family.GetName() != "fleet_state" {
			continue
		}

		for _, metric := range family.GetMetric() {
			key := ""

			for _, label := range metric.GetLabel() {
				if key != "" {
					key += "/"
				}

				key += label.GetValue()
			}

			out[key] = metric.GetGauge().GetValue()
		}
	}

	return out
}

func registerFleetState(t *testing.T) *prometheus.Registry {
	t.Helper()

	reg := prometheus.NewRegistry()
	if err := reg.Register(FleetState); err != nil {
		t.Fatalf("register: %v", err)
	}

	t.Cleanup(func() {
		FleetState.SetFunc(nil)
		reg.Unregister(FleetState)
	})

	return reg
}

// A family with no source yet reads empty, like a freshly registered GaugeVec:
// the first tick does not have to race the first scrape.
func TestStateFamilyIsEmptyBeforeTheFirstSet(t *testing.T) {
	reg := registerFleetState(t)

	if series := fleetSeries(t, reg); len(series) != 0 {
		t.Fatalf("a family with no source must export nothing, got %v", series)
	}
}

// The set is published as a unit, and a series the last set leaves out stops
// being exported instead of freezing at its last value.
func TestStateFamilyReplacesTheWholeSet(t *testing.T) {
	reg := registerFleetState(t)

	FleetState.Set([]FleetStateSample{
		{Node: "a", Zone: "z1", Value: 1},
		{Node: "b", Zone: "z1", Value: 2},
	})

	series := fleetSeries(t, reg)
	if len(series) != 2 || series["a/z1"] != 1 || series["b/z1"] != 2 {
		t.Fatalf("unexpected set: %v", series)
	}

	FleetState.Set([]FleetStateSample{{Node: "a", Zone: "z1", Value: 7}})

	series = fleetSeries(t, reg)
	if len(series) != 1 || series["a/z1"] != 7 {
		t.Fatalf("a series missing from the last set must not be exported, got %v", series)
	}

	FleetState.SetFunc(nil)

	if series := fleetSeries(t, reg); len(series) != 0 {
		t.Fatalf("a detached source must export nothing, got %v", series)
	}
}

// The source is called on every scrape — that is what lets a family that
// cannot be rebuilt cheaply hand back the last finished result of its tick.
func TestStateFamilySourceIsCalledPerScrape(t *testing.T) {
	reg := registerFleetState(t)

	calls := 0

	var source FleetStateFunc = func() []FleetStateSample {
		calls++

		return []FleetStateSample{{Node: "a", Zone: "z", Value: float64(calls)}}
	}

	FleetState.SetFunc(source)

	for want := 1; want <= 3; want++ {
		if got := fleetSeries(t, reg)["a/z"]; got != float64(want) {
			t.Fatalf("scrape %d read %v, want %v", want, got, want)
		}
	}
}

// The point of the type: a scrape sees either the whole previous set or the
// whole new one. Reset() + refill on a GaugeVec fails here — it publishes an
// empty family for the duration of the refill.
func TestStateFamilyScrapeNeverSeesAPartialSet(t *testing.T) {
	reg := registerFleetState(t)

	const (
		generations  = 2000
		seriesPerGen = 50
	)

	done := make(chan struct{})

	var wg sync.WaitGroup

	wg.Add(1)

	go func() {
		defer wg.Done()
		defer close(done)

		for generation := 1; generation <= generations; generation++ {
			samples := make([]FleetStateSample, 0, seriesPerGen)

			for i := 0; i < seriesPerGen; i++ {
				samples = append(samples, FleetStateSample{
					Node:  "n" + strconv.Itoa(i),
					Zone:  "z",
					Value: float64(generation),
				})
			}

			FleetState.Set(samples)
		}
	}()

	wg.Add(1)

	go func() {
		defer wg.Done()

		for {
			select {
			case <-done:
				return
			default:
			}

			series := fleetSeries(t, reg)
			if len(series) == 0 {
				continue
			}

			if len(series) != seriesPerGen {
				t.Errorf("a scrape saw a partial set: %d series, want %d", len(series), seriesPerGen)

				return
			}

			generation := float64(0)

			for _, value := range series {
				if generation == 0 {
					generation = value
				}

				if value != generation {
					t.Errorf("a scrape mixed two generations: %v and %v", generation, value)

					return
				}
			}
		}
	}()

	wg.Wait()
}

// A bad set costs its own family and nothing else. /metrics is served through
// promhttp.HandlerFor with the default error handling, which answers HTTP 500
// for the whole endpoint on any Gather error — so a set the registry refuses
// would take every metric of the application off the scrape (up=0, every
// series stale), which is a wider version of the hole this type exists to
// close.
//
// Of the two sets the registry refuses, a duplicated label-value combination
// is emitted once with the value of the last such sample (what a GaugeVec
// filled series by series does too), and a sample with a label value
// Prometheus rejects is dropped.
func TestStateFamilyBadSampleDoesNotFailTheScrape(t *testing.T) {
	reg := registerFleetState(t)

	unrelated := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "unrelated_total",
		Help: "A metric of another collector sharing the registry.",
	})
	unrelated.Inc()

	if err := reg.Register(unrelated); err != nil {
		t.Fatalf("register the unrelated collector: %v", err)
	}

	FleetState.Set([]FleetStateSample{
		{Node: "a", Zone: "z", Value: 1},
		{Node: "a", Zone: "z", Value: 2},
		{Node: "b\xff", Zone: "z", Value: 3},
		{Node: "c", Zone: "z", Value: 4},
	})

	srv := httptest.NewServer(promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("scrape: %v", err)
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read the scrape: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("a bad sample must not fail the scrape, got %d:\n%s", resp.StatusCode, body)
	}

	if !strings.Contains(string(body), "unrelated_total 1") {
		t.Fatalf("the rest of the application's metrics must survive a bad sample:\n%s", body)
	}

	want := map[string]float64{"a/z": 2, "c/z": 4}
	if got := fleetSeries(t, reg); !maps.Equal(got, want) {
		t.Fatalf("a set with a duplicate and a rejected sample read %v, want %v", got, want)
	}
}
`

	writeCmd := fmt.Sprintf("cat > %s/internal/pkg/metrics/state_family_probe_test.go <<'PROBE_EOF'\n%s\nPROBE_EOF",
		outputDir, probe)

	code, outputReader, err := container.Exec(ctx, []string{"sh", "-c", writeCmd})
	require.NoError(t, err, "failed to write the state-family probe")
	require.Equal(t, 0, code, "writing the state-family probe failed: %s", readOutput(outputReader))
}
