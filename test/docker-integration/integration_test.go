package dockerintegration

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	gtt "github.com/Educentr/goat"
	"github.com/Educentr/goat/services"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	serviceName = "golang-builder"
)

var (
	env         *gtt.Env
	projectRoot string
)

// GolangBuilderContainer is a custom container type for the golang builder
type GolangBuilderContainer struct {
	testcontainers.Container
}

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

	// Initialize test environment
	servicesMap := services.NewServicesMap(serviceName)
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
		container.Terminate(ctx)
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
	configDirs := []string{"rest-only", "grpc-only", "worker-telegram", "combined", "grafana"}
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

// runTest runs a single integration test
func runTest(t *testing.T, testName string, configDir string) {
	ctx := context.Background()
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

	t.Logf("Test %s PASSED", testName)
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
