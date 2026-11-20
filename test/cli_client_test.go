package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGenerateCLIWithOgenClient covers a CLI application that consumes an
// ogen_client (issue #24). Config validation used to reject it outright ("CLI
// application cannot have transports") while every rest entry has to be used by
// some application, so the only way to generate a CLI calling an HTTP API was
// to bind the client to a server application that never called it — putting the
// generated client init in the wrong process.
//
// The wiring itself is what this test pins: a CLI never reaches the runtime's
// app.Init, so the client it calls is initialized by the CLI's own main. That
// the result compiles is TestIntegrationCLIClient's job (same fixture).
func TestGenerateCLIWithOgenClient(t *testing.T) {
	tmpDir := generateFromConfig(t, "cli-client", "Generate CLI project that consumes an ogen_client")

	main := readGenerated(t, tmpDir, "cmd", "admin-cli", "psg_main_gen.go")

	for _, want := range []string{
		// The wrapper import, not the ogen package alias: main only constructs
		// the wrapper.
		`"github.com/test/cliclienttest/internal/app/transport/rest/billing/v1"`,
		`"github.com/Educentr/go-project-starter-runtime/pkg/ds"`,
		"emptySrv.InitClients(mainCtx, []ds.ClientInitializer{",
		"billing.NewClient(),",
	} {
		if !strings.Contains(main, want) {
			t.Errorf("generated CLI main should contain %q:\n%s", want, main)
		}
	}

	// A CLI has no listener: nothing may try to start a REST server in it.
	for _, unwanted := range []string{"SetTransport(", "app.New("} {
		if strings.Contains(main, unwanted) {
			t.Errorf("generated CLI main should not contain %q: a CLI serves no transport", unwanted)
		}
	}

	// The service layer has to export the entry point main calls.
	service := readGenerated(t, tmpDir, "internal", "pkg", "service", "psg_service_gen.go")
	if !strings.Contains(service, "func (s *EmptyService) InitClients(") {
		t.Errorf("service should export InitClients for the CLI to call:\n%s", service)
	}

	// The client wrapper and the ogen target dir are generated for the
	// transport regardless of which application hosts it.
	for _, dir := range []string{
		filepath.Join("internal", "app", "transport", "rest", "billing", "v1"),
		filepath.Join("api", "rest", "billing", "v1"),
	} {
		if _, err := os.Stat(filepath.Join(tmpDir, dir)); err != nil {
			t.Errorf("expected %s in the generated project: %v", dir, err)
		}
	}
}

// TestGenerateCLIWithOgenClient_RejectsServerTransport keeps the relaxation
// narrow: a CLI may host the client it calls, not an inbound transport.
func TestGenerateCLIWithOgenClient_RejectsServerTransport(t *testing.T) {
	configDir := t.TempDir()
	targetDir := t.TempDir()

	const projectYAML = `main:
  name: cliservertest
  logger: zerolog
  registry_type: github

post_generate: []

git:
  repo: git@github.com:test/cliservertest.git
  module_path: github.com/test/cliservertest

tools:
  golang_version: "1.24"
  golangci_version: 2.1.6

rest:
  - name: sys
    port: 8085
    version: "v1"
    generator_type: template
    generator_template: sys

cli:
  - name: admin
    generator_type: template
    generator_template: cli

applications:
  - name: admin-cli
    cli: admin
    transport:
      - name: sys
`

	if err := os.WriteFile(filepath.Join(configDir, "project.yaml"), []byte(projectYAML), 0o644); err != nil {
		t.Fatalf("write project.yaml: %v", err)
	}

	out, err := runGenerator(t, configDir, targetDir)
	if err == nil {
		t.Fatalf("generator should have rejected a server transport in a CLI app; output:\n%s", out)
	}

	if !strings.Contains(out, "can only host client transports") {
		t.Errorf("error should explain which transports a CLI may host; got:\n%s", out)
	}
}
