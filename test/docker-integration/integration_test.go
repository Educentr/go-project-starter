package dockerintegration

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
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
		"rest-shared-spec-rewrite-refs",
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

	// Build env var flags for docker run
	// Include essential onlineconf values for transport initialization
	envFlags := fmt.Sprintf(
		"-e ONLINECONFIG_FROM_ENV=true "+
			"-e OC_%s__devstand=1 "+
			"-e OC_%s__log__default__log_level=debug "+
			"-e OC_%s__transport__rest__api_v1__port=8080 "+
			"-e OC_%s__transport__rest__sys_v1__port=8085",
		cfg.serviceName, cfg.serviceName, cfg.serviceName, cfg.serviceName,
	)

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
