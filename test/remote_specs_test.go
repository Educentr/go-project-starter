package test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// minimalRESTConfig is a project.yaml template that uses an empty
// post_generate so the generator only resolves and copies specs (no `make
// generate-ogen` follow-up — we don't want a real ogen run on a synthetic
// spec body).
const minimalRESTConfig = `main:
  name: remotespec-test
  logger: zerolog
  registry_type: github

post_generate: []

git:
  repo: git@github.com:test/remotespec-test.git
  module_path: github.com/test/remotespec-test

tools:
  protobuf_version: 1.7.0
  golang_version: "1.24"
  ogen_version: v1.18.0
  golangci_version: 1.64.8

rest:
  - name: api
    path:
      - %q
    api_prefix: /
    version: "v1"
    port: 8080
    public_service: true
    generator_type: ogen
    generator_params:
      auth_handler: "off"
  - name: sys
    port: 8085
    version: "v1"
    generator_type: template
    generator_template: sys

applications:
  - name: api
    transport:
      - name: api
      - name: sys
`

// fakeOpenAPIBody is enough for the generator to accept (it doesn't parse
// the spec — only copies it), and small enough to keep tests fast.
const fakeOpenAPIBody = `openapi: 3.0.0
info:
  title: test
  version: "1"
paths: {}
`

func writeRemoteSpecConfig(t *testing.T, dir, specPath string) {
	t.Helper()
	body := fmt.Sprintf(minimalRESTConfig, specPath)
	if err := os.WriteFile(filepath.Join(dir, "project.yaml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write project.yaml: %v", err)
	}
}

func runGenerator(t *testing.T, configDir, targetDir string) (string, error) {
	t.Helper()
	curDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return ExecCommand(filepath.Join(curDir, ".."), "go", []string{
		"run", filepath.Join(curDir, "..", "cmd", "go-project-starter", "main.go"),
		"--target", targetDir,
		"--configDir", configDir,
		"--config", "project.yaml",
	}, "generate with remote spec")
}

// TestRemoteSpec_HTTP_DownloadsAndCopiesIntoApi verifies the happy path:
// an https:// URL in `path:` is fetched at generate time and the file lands
// at api/rest/<name>/<version>/<basename> with the basename taken from the
// URL path (not the staging unique-suffix).
func TestRemoteSpec_HTTP_DownloadsAndCopiesIntoApi(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, fakeOpenAPIBody)
	}))
	defer srv.Close()

	configDir := t.TempDir()
	targetDir := t.TempDir()
	writeRemoteSpecConfig(t, configDir, srv.URL+"/openapi.yaml")

	out, err := runGenerator(t, configDir, targetDir)
	if err != nil {
		t.Fatalf("generator failed: %v\n%s", err, out)
	}

	specPath := filepath.Join(targetDir, "api", "rest", "api", "v1", "openapi.yaml")
	got, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("read copied spec: %v", err)
	}
	if string(got) != fakeOpenAPIBody {
		t.Errorf("copied spec body mismatch:\ngot:\n%s\nwant:\n%s", got, fakeOpenAPIBody)
	}
}

// TestRemoteSpec_HTTP_BasenameFromURLPath verifies that when the URL path is
// nested (e.g. /specs/v1/users.yaml), the file is saved under just the
// basename — not the full URL path or a staging-mangled name.
func TestRemoteSpec_HTTP_BasenameFromURLPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "users.yaml") {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, fakeOpenAPIBody)
	}))
	defer srv.Close()

	configDir := t.TempDir()
	targetDir := t.TempDir()
	writeRemoteSpecConfig(t, configDir, srv.URL+"/specs/v1/users.yaml")

	out, err := runGenerator(t, configDir, targetDir)
	if err != nil {
		t.Fatalf("generator failed: %v\n%s", err, out)
	}

	specPath := filepath.Join(targetDir, "api", "rest", "api", "v1", "users.yaml")
	if _, err := os.Stat(specPath); err != nil {
		t.Fatalf("expected spec at %s, got: %v", specPath, err)
	}

	// The full URL path must NOT be mirrored into api/.
	wrongPath := filepath.Join(targetDir, "api", "rest", "api", "v1", "specs", "v1", "users.yaml")
	if _, err := os.Stat(wrongPath); err == nil {
		t.Errorf("nested URL path should not be mirrored: %s exists", wrongPath)
	}
}

// TestRemoteSpec_HTTP_BearerTokenSent verifies that token_env=NAME causes the
// resolver to send Authorization: Bearer <env value>. The token is set via
// t.Setenv so the `go run` subprocess inherits it; the value MUST NOT appear
// in the generator's output.
func TestRemoteSpec_HTTP_BearerTokenSent(t *testing.T) {
	const tokenValue = "test-secret-token-do-not-leak"

	var seenAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		_, _ = io.WriteString(w, fakeOpenAPIBody)
	}))
	defer srv.Close()

	t.Setenv("REMOTE_SPEC_TEST_TOKEN", tokenValue)

	configDir := t.TempDir()
	targetDir := t.TempDir()
	writeRemoteSpecConfig(t, configDir, srv.URL+"/openapi.yaml?token_env=REMOTE_SPEC_TEST_TOKEN")

	out, err := runGenerator(t, configDir, targetDir)
	if err != nil {
		t.Fatalf("generator failed: %v\n%s", err, out)
	}

	if seenAuth != "Bearer "+tokenValue {
		t.Errorf("Authorization header: got %q, want %q", seenAuth, "Bearer "+tokenValue)
	}
	if strings.Contains(out, tokenValue) {
		t.Errorf("token leaked into generator output:\n%s", out)
	}
}

// TestRemoteSpec_HTTP_TokenEnvMissingFailsCleanly verifies that an unset env
// variable referenced via ?token_env= produces a clear error, not a confusing
// network-level failure.
func TestRemoteSpec_HTTP_TokenEnvMissingFailsCleanly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("server must not be hit when token env is unset")
	}))
	defer srv.Close()

	// Make sure the env var is empty even if the runner has it set.
	t.Setenv("REMOTE_SPEC_TEST_MISSING", "")

	configDir := t.TempDir()
	targetDir := t.TempDir()
	writeRemoteSpecConfig(t, configDir, srv.URL+"/openapi.yaml?token_env=REMOTE_SPEC_TEST_MISSING")

	out, err := runGenerator(t, configDir, targetDir)
	if err == nil {
		t.Fatalf("generator should have failed when token_env is empty; output:\n%s", out)
	}
	if !strings.Contains(out, "token_env") {
		t.Errorf("error should mention token_env; got:\n%s", out)
	}
}

// TestRemoteSpec_HTTP_Non2xxFailsWithStatus verifies that an HTTP 404 (or any
// non-2xx) is reported with a clear status code, so users can distinguish
// auth/network failures from a wrong URL.
func TestRemoteSpec_HTTP_Non2xxFailsWithStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	configDir := t.TempDir()
	targetDir := t.TempDir()
	writeRemoteSpecConfig(t, configDir, srv.URL+"/missing.yaml")

	out, err := runGenerator(t, configDir, targetDir)
	if err == nil {
		t.Fatalf("generator should have failed on 404; output:\n%s", out)
	}
	if !strings.Contains(out, "404") {
		t.Errorf("error should mention 404 status; got:\n%s", out)
	}
}

// TestRemoteSpec_RejectsSCPStyleGitURL verifies that scp-style git URLs
// (`git@host:org/repo.git`) are rejected at parse time with a clear hint to
// rewrite as `git+ssh://`.
func TestRemoteSpec_RejectsSCPStyleGitURL(t *testing.T) {
	configDir := t.TempDir()
	targetDir := t.TempDir()
	writeRemoteSpecConfig(t, configDir, "git@github.com:org/repo.git")

	out, err := runGenerator(t, configDir, targetDir)
	if err == nil {
		t.Fatalf("generator should have failed on SCP-style URL; output:\n%s", out)
	}
	if !strings.Contains(out, "git+ssh") {
		t.Errorf("error should suggest git+ssh:// rewrite; got:\n%s", out)
	}
}

// TestRemoteSpec_RejectsGitSubpathDirectory verifies that a git URI with a
// trailing-slash subpath (directory) is rejected — v1 only supports files.
func TestRemoteSpec_RejectsGitSubpathDirectory(t *testing.T) {
	configDir := t.TempDir()
	targetDir := t.TempDir()
	writeRemoteSpecConfig(t, configDir, "git+ssh://git@github.com/org/repo.git@v1#openapi/")

	out, err := runGenerator(t, configDir, targetDir)
	if err == nil {
		t.Fatalf("generator should have failed on directory subpath; output:\n%s", out)
	}
	if !strings.Contains(out, "directory") && !strings.Contains(out, "subpath") {
		t.Errorf("error should mention directory/subpath limitation; got:\n%s", out)
	}
}

// queueContract is a minimal valid queues.yaml — enough for ParseQueueSpec
// to succeed. The exact schema doesn't matter for this test; we only verify
// that the resolver materializes the file before ParseQueueSpec runs.
const queueContract = `queues:
  - id: 1
    name: emails
    fields:
      - name: to
        type: string
      - name: subject
        type: string
      - name: body
        type: "[]byte"
      - name: user_id
        type: int64
`

// TestRemoteSpec_QueueWorker_HTTP verifies that queue-worker spec paths
// accept remote URIs. The resolver materializes the contract into a scratch
// directory, ParseQueueSpec reads it, and the staging is removed
// immediately after the parse. Spec was historically a v1 limitation
// (remote queue specs not supported); this test guards the fix.
func TestRemoteSpec_QueueWorker_HTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, queueContract)
	}))
	defer srv.Close()

	cfgTmpl := `main:
  name: queueremote
  logger: zerolog
  registry_type: github

post_generate: []

git:
  repo: git@github.com:test/queueremote.git
  module_path: github.com/test/queueremote

tools:
  protobuf_version: 1.7.0
  golang_version: "1.24"
  ogen_version: v1.18.0
  golangci_version: 1.64.8

rest:
  - name: sys
    port: 8085
    version: "v1"
    generator_type: template
    generator_template: sys

worker:
  - name: task_processor
    generator_type: template
    generator_template: queue
    path:
      - %q

applications:
  - name: server
    transport:
      - name: sys
    worker:
      - task_processor
`

	configDir := t.TempDir()
	targetDir := t.TempDir()
	body := fmt.Sprintf(cfgTmpl, srv.URL+"/queues.yaml")
	if err := os.WriteFile(filepath.Join(configDir, "project.yaml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	out, err := runGenerator(t, configDir, targetDir)
	if err != nil {
		t.Fatalf("generator failed with remote queue spec: %v\n%s", err, out)
	}

	// The dispatcher generated for the worker should reference the typed
	// EmailsTask coming from the remote contract.
	dispatcherPath := filepath.Join(targetDir, "internal", "app", "worker",
		"task_processor", "task_processor", "psg_types_gen.go")
	got, err := os.ReadFile(dispatcherPath)
	if err != nil {
		t.Fatalf("read generated worker types: %v", err)
	}
	if !strings.Contains(string(got), "type EmailsTask struct") {
		t.Errorf("generated types should contain EmailsTask from remote contract; got:\n%s", got)
	}
}

// cliCommands is a minimal valid commands.yaml for the CLI spec parser.
const cliCommands = `commands:
  - name: ping
    description: ping the service
`

// TestRemoteSpec_CLI_HTTP verifies that CLI spec paths accept remote URIs.
// Same inline-resolve flow as queue workers: the contract is materialized
// once, parsed, and the staging is wiped immediately.
func TestRemoteSpec_CLI_HTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, cliCommands)
	}))
	defer srv.Close()

	cfgTmpl := `main:
  name: cliremote
  logger: zerolog
  registry_type: github

post_generate: []

git:
  repo: git@github.com:test/cliremote.git
  module_path: github.com/test/cliremote

tools:
  protobuf_version: 1.7.0
  golang_version: "1.24"
  ogen_version: v1.18.0
  golangci_version: 1.64.8

cli:
  - name: admin
    path:
      - %q
    generator_type: template
    generator_template: cli

applications:
  - name: admin-cli
    cli: admin
`

	configDir := t.TempDir()
	targetDir := t.TempDir()
	body := fmt.Sprintf(cfgTmpl, srv.URL+"/commands.yaml")
	if err := os.WriteFile(filepath.Join(configDir, "project.yaml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	out, err := runGenerator(t, configDir, targetDir)
	if err != nil {
		t.Fatalf("generator failed with remote CLI spec: %v\n%s", err, out)
	}

	// The CLI handler generated from the remote contract must contain the
	// RunPing method derived from the `ping` command.
	handlerPath := filepath.Join(targetDir, "internal", "app", "transport", "cli", "admin", "psg_handler_gen.go")
	got, err := os.ReadFile(handlerPath)
	if err != nil {
		t.Fatalf("read generated CLI handler: %v", err)
	}
	if !strings.Contains(string(got), "RunPing") {
		t.Errorf("generated handler should contain RunPing from remote spec; got first 500 bytes:\n%.500s", got)
	}
}

// TestRemoteSpec_LocalPath_Regression verifies that the addition of the
// remote-spec resolve pre-pass did not regress local-path behaviour. A
// relative `./api.yaml` next to project.yaml must resolve and be copied
// exactly as before.
func TestRemoteSpec_LocalPath_Regression(t *testing.T) {
	configDir := t.TempDir()
	targetDir := t.TempDir()

	// Write a sibling local file that the config will reference.
	if err := os.WriteFile(filepath.Join(configDir, "local.yaml"), []byte(fakeOpenAPIBody), 0o644); err != nil {
		t.Fatalf("write local spec: %v", err)
	}
	writeRemoteSpecConfig(t, configDir, "./local.yaml")

	out, err := runGenerator(t, configDir, targetDir)
	if err != nil {
		t.Fatalf("generator failed: %v\n%s", err, out)
	}

	specPath := filepath.Join(targetDir, "api", "rest", "api", "v1", "local.yaml")
	got, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("read copied spec: %v", err)
	}
	if string(got) != fakeOpenAPIBody {
		t.Errorf("local-path regression: copied body mismatch")
	}
}

// TestRemoteSpec_HTTP_StagingCleaned verifies the resolver does not leak its
// staging directory after a successful generate. We probe by looking at the
// system temp dir before/after for `gps-specs-*` entries created by this
// test run.
func TestRemoteSpec_HTTP_StagingCleaned(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, fakeOpenAPIBody)
	}))
	defer srv.Close()

	configDir := t.TempDir()
	targetDir := t.TempDir()
	writeRemoteSpecConfig(t, configDir, srv.URL+"/openapi.yaml")

	before := countStagingDirs(t)
	if _, err := runGenerator(t, configDir, targetDir); err != nil {
		t.Fatalf("generator failed: %v", err)
	}
	after := countStagingDirs(t)

	if after > before {
		t.Errorf("staging dirs leaked: before=%d after=%d (expected staging dirs to be removed in defer)", before, after)
	}
}

func countStagingDirs(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir(os.TempDir())
	if err != nil {
		t.Fatalf("read temp dir: %v", err)
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "gps-specs-") {
			n++
		}
	}
	return n
}
