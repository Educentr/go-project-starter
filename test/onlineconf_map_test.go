package test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
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

	assertOnlineConfRegistry(t, tmpDir)
}

// registryEntry is one row of the generated OnlineConf registry, decoded with a
// closed schema so that a field added to the template without a thought shows
// up here.
type registryEntry struct {
	Path        string   `yaml:"path"`
	Origin      string   `yaml:"origin"`
	Type        string   `yaml:"type"`
	Dynamic     bool     `yaml:"dynamic"`
	Required    bool     `yaml:"required"`
	Accessor    string   `yaml:"accessor"`
	Default     string   `yaml:"default"`
	ReadBy      []string `yaml:"read_by"`
	Description string   `yaml:"description"`
}

// assertOnlineConfRegistry checks the generated registry of the whole OnlineConf
// surface.
//
// This is the half of the tree that used to exist only inside the generator's
// templates: the owner of the contract declares the service's own keys and had
// no way to see the rest at all. One generated list of both is what makes the
// surface reviewable.
func assertOnlineConfRegistry(t *testing.T, tmpDir string) {
	t.Helper()

	raw := readGenerated(t, tmpDir, "configs", "onlineconf-tree.yaml")

	var registry struct {
		Tree string          `yaml:"tree"`
		Keys []registryEntry `yaml:"keys"`
	}

	dec := yaml.NewDecoder(strings.NewReader(raw))
	dec.KnownFields(true)

	if err := dec.Decode(&registry); err != nil {
		t.Fatalf("configs/onlineconf-tree.yaml does not parse as yaml: %v", err)
	}

	if registry.Tree != "/octest" {
		t.Errorf("registry tree = %q, want /octest", registry.Tree)
	}

	byPath := make(map[string]registryEntry, len(registry.Keys))
	for _, entry := range registry.Keys {
		byPath[entry.Path] = entry
	}

	// A service key carries its accessor, so a reader of the registry knows how
	// the service reads it.
	service, ok := byPath["/octest/db/dsn"]
	if !ok {
		t.Fatalf("registry should list every declared key, got %d entries", len(registry.Keys))
	}

	if service.Origin != "service" || service.Accessor != "DatabaseDSN" || !service.Required {
		t.Errorf("service key row = %+v, want origin service, accessor DatabaseDSN, required", service)
	}

	// A generated key carries the template the read comes from, which is what
	// makes the row actionable: that is the file to change.
	generated, ok := byPath["/octest/security/csrf/enabled"]
	if !ok {
		t.Fatal("registry should list the security toggles the generated restconfig reads")
	}

	if generated.Origin != "generated" || generated.Type != "bool" {
		t.Errorf("generated key row = %+v, want origin generated, type bool", generated)
	}

	if len(generated.ReadBy) == 0 ||
		!strings.Contains(strings.Join(generated.ReadBy, " "), "restconfig/config_oc.go.tmpl") {
		t.Errorf("generated key row should name the template it is read by, got %v", generated.ReadBy)
	}

	// A subtree read under a runtime key cannot be listed path by path, so it is
	// listed as a subtree — and that is also what the service map is kept out
	// of.
	dynamic, ok := byPath["/octest/settings/enabled"]
	if !ok {
		t.Fatal("registry should list the per-action allow lists as a subtree")
	}

	if !dynamic.Dynamic || dynamic.Type != "" {
		t.Errorf("dynamic row = %+v, want dynamic: true and no type", dynamic)
	}

	// The transport keys are expanded against this project: the names in them
	// are the transport and application names of project.yaml, not patterns.
	for _, want := range []string{
		"/octest/transport/rest/api_v1/api/port",
		"/octest/transport/rest/sys_v1/api/server/timeout/read",
		"/octest/devstand",
	} {
		if _, found := byPath[want]; !found {
			t.Errorf("registry should list %s", want)
		}
	}

	for _, unwanted := range []string{"{service}", "{transport}", "{app}", "{worker}", "{client}"} {
		if strings.Contains(raw, unwanted) {
			t.Errorf("registry carries the unexpanded placeholder %s", unwanted)
		}
	}

	// Nothing from a dimension this project does not have: the project has no
	// worker, no Kafka client and no telegram bot.
	for _, unwanted := range []string{"/octest/worker/", "/octest/kafka/", "/octest/bot/"} {
		for path := range byPath {
			if strings.HasPrefix(path, unwanted) {
				t.Errorf("registry lists %s, but this project has no such component", path)
			}
		}
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

// TestGenerateRejectsKeysTheGeneratorReads pins the acceptance criterion that
// keys the generated code reads cannot be declared by the service: a second
// declaration would mean a second reader with a second spelling, which is the
// thing the map exists to prevent. The check runs against the generator's own
// declarations expanded over this project, so the error names the template the
// read comes from.
func TestGenerateRejectsKeysTheGeneratorReads(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		keyType  string
		wantMsgs []string
	}{
		{
			name:    "transport key, exact match",
			path:    "/octest/transport/rest/api_v1/port",
			keyType: "string",
			wantMsgs: []string{
				"/octest/transport/rest/api_v1/port", "reads itself",
				"transport/rest/files/pkg/app/restconfig/config_oc.go.tmpl",
			},
		},
		{
			name:     "security toggle, exact match",
			path:     "/octest/security/csrf/enabled",
			keyType:  "bool",
			wantMsgs: []string{"/octest/security/csrf/enabled", "reads itself"},
		},
		{
			name:     "dev-stand flag, read by the logger",
			path:     "/octest/devstand",
			keyType:  "bool",
			wantMsgs: []string{"/octest/devstand", "logger/zlog/zlog.go.tmpl"},
		},
		{
			name:    "inside the per-action allow lists the service layer reads",
			path:    "/octest/settings/enabled/login",
			keyType: "strings",
			wantMsgs: []string{
				"/octest/settings/enabled", "runtime key",
				"main/internal/pkg/service/service.go.tmpl",
			},
		},
		{
			name:     "inside the per-handler subtree",
			path:     "/octest/transport/rest/api_v1/api/handler/some_path/timeout",
			keyType:  "duration",
			wantMsgs: []string{"/octest/transport/rest/api_v1/api/handler", "runtime key"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, _ := generateWithOnlineConfMap(t,
				"keys:\n  - path: "+tt.path+"\n    type: "+tt.keyType+"\n")

			for _, want := range tt.wantMsgs {
				if !strings.Contains(out, want) {
					t.Errorf("generator error should mention %q, got:\n%s", want, out)
				}
			}
		})
	}
}

// TestGenerateRejectsWorkerAndBotKeys is the regression test for the gap the
// reserved-prefix list left behind: the generated code reads
// /<svc>/worker/<name>/…, /<svc>/settings/… and /<svc>/bot/… too, and none of
// those subtrees was protected. A service could declare them and get a second
// reader of the same key — the exact thing this feature is supposed to make
// impossible.
func TestGenerateRejectsWorkerAndBotKeys(t *testing.T) {
	tests := []struct {
		name     string
		keyMap   string
		wantMsgs []string
	}{
		{
			name:   "worker timeout the telegram worker reads",
			keyMap: "keys:\n  - path: /telegramtest/worker/telegrambot/job/timeout\n    type: duration\n",
			wantMsgs: []string{
				"/telegramtest/worker/telegrambot/job/timeout", "reads itself",
				"worker/template/telegram/files/telegram.go.tmpl",
			},
		},
		{
			name:   "payment toggle the telegram router reads",
			keyMap: "keys:\n  - path: /telegramtest/settings/auto_refund_disable\n    type: bool\n",
			wantMsgs: []string{
				"/telegramtest/settings/auto_refund_disable", "reads itself",
				"worker/template/telegram/files/router.go.tmpl",
			},
		},
		{
			name:   "admin ACL the telegram authorization reads",
			keyMap: "keys:\n  - path: /telegramtest/bot/admins/42\n    type: string\n",
			wantMsgs: []string{
				"/telegramtest/bot/admins", "runtime key",
				"worker/template/telegram/shared/pkg/drivers/telegram/service.go.tmpl",
			},
		},
		{
			name:   "bot token the telegram driver reads",
			keyMap: "keys:\n  - path: /telegramtest/security/tg_token\n    type: string\n",
			wantMsgs: []string{
				"/telegramtest/security/tg_token", "reads itself",
				"worker/template/telegram/shared/pkg/drivers/telegram/auth.go.tmpl",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, _ := rejectOnlineConfMap(t, "worker-telegram", tt.keyMap)

			for _, want := range tt.wantMsgs {
				if !strings.Contains(out, want) {
					t.Errorf("generator error should mention %q, got:\n%s", want, out)
				}
			}
		})
	}
}

// A key of a dimension the project does not have is the service's to declare:
// nothing in this project reads it, so there is no second reader to protect
// against. Over-reserving would be the mirror image of the old prefix list,
// wrong in the other direction.
func TestGenerateAllowsKeysOfAbsentDimensions(t *testing.T) {
	_, targetDir, err := generateOnlineConfMap(t, "onlineconf-map",
		"keys:\n"+
			"  - path: /octest/worker/nonexistent/batch_size\n    type: int\n"+
			"  - path: /octest/kafka/nonexistent/brokers\n    type: string\n"+
			"  - path: /octest/bot/admins\n    type: string\n")
	if err != nil {
		t.Fatalf("generation should accept keys no generated code reads: %v", err)
	}

	occonf := readGenerated(t, targetDir, "internal", "pkg", "occonf", "psg_occonf_gen.go")

	if !strings.Contains(occonf, "func WorkerNonexistentBatchSize(") {
		t.Error("a key of an absent project dimension should get an accessor")
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

	if _, err := os.Stat(filepath.Join(tmpDir, "configs", "onlineconf-tree.yaml")); !os.IsNotExist(err) {
		t.Error("no key map should mean no surface registry")
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

	return rejectOnlineConfMap(t, "onlineconf-map", keyMap)
}

// rejectOnlineConfMap runs the generator over a fixture with the given key map
// and fails the test unless generation is rejected.
func rejectOnlineConfMap(t *testing.T, fixture, keyMap string) (output, targetDir string) {
	t.Helper()

	out, targetDir, err := generateOnlineConfMap(t, fixture, keyMap)
	if err == nil {
		t.Fatalf("generation succeeded on a map that should have been rejected\n%s", out)
	}

	return out, targetDir
}

// generateOnlineConfMap copies a fixture, points its `onlineconf.path` at the
// given key map and runs the generator. Every fixture file is copied, so the
// only difference from the fixture's own run is the key map.
func generateOnlineConfMap(t *testing.T, fixture, keyMap string) (output, targetDir string, err error) {
	t.Helper()

	curDir, cerr := os.Getwd()
	if cerr != nil {
		t.Fatalf("Error getting current directory: %v", cerr)
	}

	fixtureDir := filepath.Join(curDir, "..", "test", "docker-integration", "configs", fixture)
	configDir := t.TempDir()

	entries, rerr := os.ReadDir(fixtureDir)
	if rerr != nil {
		t.Fatalf("Error reading fixture %s: %v", fixture, rerr)
	}

	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == onlineConfMapFile {
			continue
		}

		data, derr := os.ReadFile(filepath.Join(fixtureDir, entry.Name()))
		if derr != nil {
			t.Fatalf("Error reading fixture %s/%s: %v", fixture, entry.Name(), derr)
		}

		if entry.Name() == "project.yaml" && !strings.Contains(string(data), "\nonlineconf:") {
			data = append(data, []byte("\nonlineconf:\n  path: ./"+onlineConfMapFile+"\n")...)
		}

		if werr := os.WriteFile(filepath.Join(configDir, entry.Name()), data, 0o600); werr != nil {
			t.Fatalf("Error writing %s: %v", entry.Name(), werr)
		}
	}

	if werr := os.WriteFile(filepath.Join(configDir, onlineConfMapFile), []byte(keyMap), 0o600); werr != nil {
		t.Fatalf("Error writing key map: %v", werr)
	}

	targetDir = t.TempDir()

	out, err := ExecCommand(filepath.Join(curDir, ".."), "go", []string{
		"run", filepath.Join(curDir, "..", "cmd", "go-project-starter", "main.go"),
		"--target", targetDir,
		"--configDir", configDir,
		"--config", "project.yaml",
	}, "Generate "+fixture+" with an OnlineConf key map")

	return out, targetDir, err
}

// onlineConfMapFile is the key map file name the fixtures and the generated
// configs agree on.
const onlineConfMapFile = "onlineconf-key-map.yaml"
