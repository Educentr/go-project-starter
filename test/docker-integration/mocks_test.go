package dockerintegration

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// execStep runs a shell command in the builder container and returns its exit
// code together with the combined output.
func execStep(ctx context.Context, t *testing.T, container *GolangBuilderContainer, step, command string) (int, string) {
	t.Helper()
	t.Logf("%s...", step)

	code, outputReader, err := container.Exec(ctx, []string{"sh", "-c", command})
	output := readOutput(outputReader)

	if os.Getenv("GOAT_DISABLE_STDOUT") != "true" {
		t.Logf("%s output:\n%s", step, output)
	}

	require.NoError(t, err, "failed to execute %s", step)

	return code, output
}

// TestIntegrationMocksRegenerate reproduces the incident behind TOOLS-21 /
// issue #54 end to end: the generator writes mock scaffolding whose
// //go:generate directive mirrors an ogen interface the generator also writes,
// so dropping an operation from the contract invalidates the mock. Before the
// mock step joined the `generate` aggregator, nothing regenerated it and the
// test package stopped compiling — and that only surfaced a stage later.
//
// Only ./tests/<app>/mocks/... is built, not ./..., because the generated
// tests/<app> package deliberately calls a user-written initMockServers that a
// freshly generated project does not have yet.
func TestIntegrationMocksRegenerate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := t.Context()
	container := GetContainer()

	const (
		testName    = "rest-client-mocks"
		mocksPkg    = "./tests/api/mocks/..."
		droppedOp   = "ListInvoices"
		remainingOp = "GetInvoice"
	)

	outputDir := fmt.Sprintf("/workspace/output/%s", testName)
	projectConfigDir := fmt.Sprintf("%s/.project-config", outputDir)
	mockFile := fmt.Sprintf("%s/tests/api/mocks/billing/mock_handler.go", outputDir)
	copiedSpec := fmt.Sprintf("%s/api/rest/billing/v1/billing.swagger.yml", outputDir)

	code, output := execStep(ctx, t, container, "Setting up config dir",
		fmt.Sprintf("mkdir -p %s && cp -r /workspace/test-configs/%s/. %s/", projectConfigDir, testName, projectConfigDir))
	require.Equal(t, 0, code, "setup failed: %s", output)

	code, output = execStep(ctx, t, container, "Running go-project-starter",
		fmt.Sprintf("go-project-starter --target=%s --configDir=%s", outputDir, projectConfigDir))
	require.Equal(t, 0, code, "generator failed: %s", output)

	code, output = execStep(ctx, t, container, "Running make generate",
		fmt.Sprintf("cd %s && make generate", outputDir))
	require.Equal(t, 0, code, "make generate failed: %s", output)

	// make generate — not a separate mock target — must have produced the mock.
	code, output = execStep(ctx, t, container, "Reading generated mock", "cat "+mockFile)
	require.Equal(t, 0, code, "make generate did not produce the mock: %s", output)
	require.Contains(t, output, droppedOp, "generated mock must cover every operation of the contract")

	code, output = execStep(ctx, t, container, "Building mocks package",
		fmt.Sprintf("cd %s && go build %s", outputDir, mocksPkg))
	require.Equal(t, 0, code, "go build of the mocks package failed: %s", output)

	// Drop an operation from the contract — the ogen interface shrinks and the
	// mock on disk keeps a method whose params type no longer exists.
	code, output = execStep(ctx, t, container, "Shrinking the billing contract",
		fmt.Sprintf("cp %s/billing-reduced.swagger.yml %s", projectConfigDir, copiedSpec))
	require.Equal(t, 0, code, "failed to replace the spec: %s", output)

	code, output = execStep(ctx, t, container, "Running make generate after the contract change",
		fmt.Sprintf("cd %s && make generate", outputDir))
	require.Equal(t, 0, code, "make generate failed after the contract change: %s", output)

	code, output = execStep(ctx, t, container, "Reading regenerated mock", "cat "+mockFile)
	require.Equal(t, 0, code, "regenerated mock is missing: %s", output)
	require.NotContains(t, output, droppedOp, "regenerated mock must drop the removed operation")
	require.Contains(t, output, remainingOp, "regenerated mock must keep the remaining operation")

	code, output = execStep(ctx, t, container, "Building mocks package after the contract change",
		fmt.Sprintf("cd %s && go build %s", outputDir, mocksPkg))
	require.Equal(t, 0, code, "go build of the mocks package failed after the contract change: %s", output)

	t.Logf("Mock regeneration through `make generate` PASSED for %s", testName)
}
