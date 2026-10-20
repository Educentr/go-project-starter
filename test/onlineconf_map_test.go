package test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGenerateOnlineConfMap covers the whole OnlineConf-key-map feature on a
// project that declares one: the accessor package, the startup check, and the
// validator.
func TestGenerateOnlineConfMap(t *testing.T) {
	tmpDir := generateFromConfig(t, "onlineconf-map", "Generate project with an OnlineConf key map")

	occonfFile := readGenerated(t, tmpDir, "internal", "pkg", "occonf", "psg_occonf_gen.go")

	// One exported path constant per declared key, carrying the FULL path. The
	// full path is the point: a path assembled at the call site out of private
	// segment constants exists nowhere as a whole, so no guard, registry or
	// review can check it against the declaration.
	wantPaths := map[string]string{
		"PathSupervisorMinLifetime":     `"/octest/supervisor/min_lifetime"`,
		"PathSupervisorMaxAgents":       `"/octest/supervisor/max_agents"`,
		"PathFeatureNewPipelineEnabled": `"/octest/feature/new_pipeline_enabled"`,
		"PathConfluenceURL":             `"/octest/confluence/url"`,
		"PathNotifyRecipients":          `"/octest/notify/recipients"`,
		"PathQueueName":                 `"/octest/queue/name"`,
		"PathAgentEnv":                  `"/octest/agent_env"`,
		"PathDatabaseDSN":               `"/octest/db/dsn"`,
	}

	for constName, path := range wantPaths {
		if !strings.Contains(occonfFile, constName+" = "+path) {
			t.Errorf("expected generated path constant %s = %s", constName, path)
		}
	}

	// The accessor signature is decided by the declaration: a required key
	// cannot be mistaken for absent, and an optional key without a default
	// cannot be mistaken for present.
	wantSignatures := []string{
		// default → never reports absence
		"func SupervisorMinLifetime(ctx context.Context) (time.Duration, error)",
		"func FeatureNewPipelineEnabled(ctx context.Context) (bool, error)",
		// required → ErrKeyNotSet instead of a zero value
		"func SupervisorMaxAgents(ctx context.Context) (int64, error)",
		"func ConfluenceURL(ctx context.Context) (string, error)",
		// neither → presence comes back alongside the value
		"func QueueName(ctx context.Context) (string, bool, error)",
		// strings: GetStrings reports no presence, hence no bool
		"func NotifyRecipients(ctx context.Context) ([]string, error)",
		// struct: the value shape belongs to the caller
		"func AgentEnv(ctx context.Context, dst any) (bool, error)",
		// explicit `name` override wins over the derived DBDSN
		"func DatabaseDSN(ctx context.Context) (string, error)",
	}

	for _, want := range wantSignatures {
		if !strings.Contains(occonfFile, want) {
			t.Errorf("generated accessor package should declare %q", want)
		}
	}

	// The *IfExists readers, deliberately: go-onlineconf's defaulting form
	// (GetString(ctx, path, d...)) indexes d[0] on a read error, so it panics
	// when called without a default.
	for _, want := range []string{
		"onlineconf.GetDurationIfExists(ctx, PathSupervisorMinLifetime)",
		"onlineconf.GetIntIfExists(ctx, PathSupervisorMaxAgents)",
		"onlineconf.GetStringIfExists(ctx, PathQueueName)",
		"onlineconf.GetStruct(ctx, PathAgentEnv, dst)",
	} {
		if !strings.Contains(occonfFile, want) {
			t.Errorf("generated accessor package should read through %q", want)
		}
	}

	// A declared default is rendered as the declared type, readably — a
	// nanosecond count in the source would be unreviewable.
	for _, want := range []string{
		"return 5 * time.Minute, nil",
		`fallback := []string{"ops@example.com", "oncall@example.com"}`,
	} {
		if !strings.Contains(occonfFile, want) {
			t.Errorf("generated accessor package should contain %q", want)
		}
	}

	// Required keys report ErrKeyNotSet rather than a zero value.
	if !strings.Contains(occonfFile, "ErrKeyNotSet, PathSupervisorMaxAgents") {
		t.Error("a required key should report ErrKeyNotSet when absent")
	}

	// The strongest check of the above: the generated package must parse.
	if _, err := parser.ParseFile(token.NewFileSet(), "psg_occonf_gen.go", occonfFile, parser.SkipObjectResolution); err != nil {
		t.Errorf("generated accessor package is not valid Go: %v", err)
	}

	// Subscribe exists because the validator forbids calling onlineconf from
	// service code: without it a service reacting to configuration changes
	// would have no way to do so except by breaking the rule.
	for _, want := range []string{
		"func Subscribe(ctx context.Context, paths []string, cb func() error) error",
		"func Paths() []string",
		"func CheckRequired(ctx context.Context) error",
	} {
		if !strings.Contains(occonfFile, want) {
			t.Errorf("generated accessor package should declare %q", want)
		}
	}

	// CheckRequired covers exactly the three required declarations, through the
	// accessors — so the startup check and the running service agree on what
	// "usable" means.
	for _, want := range []string{
		"_, err := SupervisorMaxAgents(ctx)",
		"_, err := ConfluenceURL(ctx)",
		"_, err := DatabaseDSN(ctx)",
	} {
		if !strings.Contains(occonfFile, want) {
			t.Errorf("requiredChecks should read %q", want)
		}
	}

	if strings.Contains(occonfFile, "_, err := QueueName(ctx)") {
		t.Error("an optional key must not appear in the startup check")
	}

	// The check is wired into the generated main at the configuration
	// boundary: a missing required key is a misconfiguration of the
	// installation, and the place to find out is process start.
	mainFile := readGenerated(t, tmpDir, "cmd", "api", "psg_main_gen.go")

	if !strings.Contains(mainFile, "occonf.CheckRequired(mainCtx)") {
		t.Error("generated main should verify the required keys at startup")
	}

	if !strings.Contains(mainFile, `"github.com/test/octest/internal/pkg/occonf"`) {
		t.Error("generated main should import the generated accessor package")
	}

	// Validator: standalone config, the rule in the main lint config, and the
	// Makefile target.
	ocLint := readGenerated(t, tmpDir, "configs", "golangci-onlineconf.yml")

	for _, want := range []string{
		"- forbidigo",
		`^onlineconf\.`,
		"path: (internal/pkg/occonf/|psg_[^/]*_gen\\.go$)",
	} {
		if !strings.Contains(ocLint, want) {
			t.Errorf("configs/golangci-onlineconf.yml should contain %q", want)
		}
	}

	// Without analyze-types forbidigo matches the literal source text, so
	// `oc "…/onlineconf"` + `oc.GetString(…)` — or a bare `GetString(…)`
	// through a dot-import — sails past a rule written for
	// `onlineconf.GetString`. The pattern names the package, so it only means
	// what it says with type resolution on.
	if !strings.Contains(ocLint, "analyze-types: true") {
		t.Error("the OnlineConf validator must resolve types: an import alias would otherwise defeat the rule")
	}

	mainLint := readGenerated(t, tmpDir, "configs", "golangci-lint.yml")

	if !strings.Contains(mainLint, "- forbidigo") {
		t.Error("configs/golangci-lint.yml should enable forbidigo when a key map is configured")
	}

	if !strings.Contains(mainLint, `^onlineconf\.`) {
		t.Error("configs/golangci-lint.yml should carry the OnlineConf validator pattern")
	}

	makefile := readGenerated(t, tmpDir, "Makefile")

	if !strings.Contains(makefile, "onlineconf-validate:") {
		t.Error("Makefile should expose an onlineconf-validate target")
	}

	if !strings.Contains(makefile, "--config=./configs/golangci-onlineconf.yml") {
		t.Error("onlineconf-validate should use the standalone validator config, not a flag")
	}

	// The map is read at generation time and is not copied into the project:
	// nothing downstream reads it, and a second copy would diverge from the
	// contract silently.
	if _, err := os.Stat(filepath.Join(tmpDir, "onlineconf-key-map.yaml")); !os.IsNotExist(err) {
		t.Error("the key map must not be copied into the generated project")
	}
}

// TestGenerateRejectsListTemplatePath pins the shape a key registry tends to
// carry: /svc/agent_env/<i>/name is not a key. OnlineConf has no subtree
// traversal, so there is nothing to generate for it — and the message has to
// say what to declare instead, not just that the path is malformed.
func TestGenerateRejectsListTemplatePath(t *testing.T) {
	out, targetDir := generateWithOnlineConfMap(t,
		"keys:\n"+
			"  - path: /octest/agent_env/<i>/name\n"+
			"    type: string\n")

	for _, want := range []string{"<i>", "type: struct"} {
		if !strings.Contains(out, want) {
			t.Errorf("generator error should mention %q, got:\n%s", want, out)
		}
	}

	if _, err := os.Stat(filepath.Join(targetDir, "internal", "pkg", "occonf")); err == nil {
		t.Error("a rejected map must not leave an accessor package behind")
	}
}

// TestGenerateRejectsReservedSubtree pins the acceptance criterion that keys
// the generator reads itself cannot be declared by the service. Those paths are
// built by generated readers from the transport, application and client names,
// so a hand-written declaration would either shadow a generated read or drift
// from it.
func TestGenerateRejectsReservedSubtree(t *testing.T) {
	out, _ := generateWithOnlineConfMap(t,
		"keys:\n"+
			"  - path: /octest/transport/rest/api/port\n"+
			"    type: string\n")

	for _, want := range []string{"/octest/transport", "reads itself"} {
		if !strings.Contains(out, want) {
			t.Errorf("generator error should mention %q, got:\n%s", want, out)
		}
	}
}

// TestGenerateWithoutOnlineConfMap pins that no map means nothing is switched
// on. A half-on transitional mode would leave traces in generated code that
// nobody cleans up later.
func TestGenerateWithoutOnlineConfMap(t *testing.T) {
	tmpDir := generateFromConfig(t, "grafana", "Generate project without an OnlineConf key map")

	if _, err := os.Stat(filepath.Join(tmpDir, "internal", "pkg", "occonf")); !os.IsNotExist(err) {
		t.Error("no key map should mean no generated accessor package")
	}

	if _, err := os.Stat(filepath.Join(tmpDir, "configs", "golangci-onlineconf.yml")); !os.IsNotExist(err) {
		t.Error("no key map should mean no validator config")
	}

	mainLint := readGenerated(t, tmpDir, "configs", "golangci-lint.yml")

	if strings.Contains(mainLint, "forbidigo") {
		t.Error("no key map (and no metrics map) should mean no forbidigo rule in the lint config")
	}

	makefile := readGenerated(t, tmpDir, "Makefile")

	if strings.Contains(makefile, "onlineconf-validate") {
		t.Error("no key map should mean no onlineconf-validate target")
	}

	mainFile := readGenerated(t, tmpDir, "cmd", "api", "psg_main_gen.go")

	if strings.Contains(mainFile, "CheckRequired") {
		t.Error("no key map should mean no startup check in the generated main")
	}
}

// generateWithOnlineConfMap reuses the working fixture, swapping in the given
// key map, so that the only difference from the passing case is the map itself.
// It returns the generator output and the target directory, and fails the test
// when generation succeeds.
func generateWithOnlineConfMap(t *testing.T, keyMap string) (output, targetDir string) {
	t.Helper()

	curDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting current directory: %v", err)
	}

	fixture := filepath.Join(curDir, "..", "test", "docker-integration", "configs", "onlineconf-map")
	configDir := t.TempDir()

	for _, name := range []string{"project.yaml", "api.swagger.yml"} {
		data, rerr := os.ReadFile(filepath.Join(fixture, name))
		if rerr != nil {
			t.Fatalf("Error reading fixture %s: %v", name, rerr)
		}

		if werr := os.WriteFile(filepath.Join(configDir, name), data, 0o600); werr != nil {
			t.Fatalf("Error writing %s: %v", name, werr)
		}
	}

	if werr := os.WriteFile(filepath.Join(configDir, "onlineconf-key-map.yaml"), []byte(keyMap), 0o600); werr != nil {
		t.Fatalf("Error writing key map: %v", werr)
	}

	targetDir = t.TempDir()

	out, err := ExecCommand(filepath.Join(curDir, ".."), "go", []string{
		"run", filepath.Join(curDir, "..", "cmd", "go-project-starter", "main.go"),
		"--target", targetDir,
		"--configDir", configDir,
		"--config", "project.yaml",
	}, "Generate project with a rejected OnlineConf key map")

	if err == nil {
		t.Fatalf("generation succeeded on a map that should have been rejected\n%s", out)
	}

	return out, targetDir
}
