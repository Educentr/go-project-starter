package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// listenIPFixture is the fixture the listen_ip cases are built on: two servers
// (api and sys) and no listen_ip of its own, so the test declares the
// addresses itself and the unpatched fixture serves as the "declares nothing"
// case.
const listenIPFixture = "rest-only"

// restConfigProviderPath is the generated REST configuration provider, the one
// file the declared listen address reaches.
var restConfigProviderPath = []string{"pkg", "app", "restconfig", "psg_config_oc_gen.go"}

// declareListenIPs copies listenIPFixture into a config directory of its own
// and writes listen_ip onto the named REST transports. Patching a copy rather
// than adding a fixture keeps the address out of the fixture sweeps, where it
// would be one more project-wide output change to explain.
func declareListenIPs(t *testing.T, addresses map[string]string) string {
	t.Helper()

	srcDir := filepath.Join(configFixturesDir(t), listenIPFixture)
	dstDir := t.TempDir()

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		t.Fatalf("Error reading config %s: %v", listenIPFixture, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		content, err := os.ReadFile(filepath.Join(srcDir, entry.Name()))
		if err != nil {
			t.Fatalf("Error reading %s/%s: %v", listenIPFixture, entry.Name(), err)
		}

		if err := os.WriteFile(filepath.Join(dstDir, entry.Name()), content, 0o644); err != nil {
			t.Fatalf("Error writing %s: %v", entry.Name(), err)
		}
	}

	path := filepath.Join(dstDir, "project.yaml")

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Error reading %s: %v", path, err)
	}

	doc := map[string]any{}
	if err := yaml.Unmarshal(content, &doc); err != nil {
		t.Fatalf("Error parsing %s: %v", path, err)
	}

	transports, ok := doc["rest"].([]any)
	if !ok {
		t.Fatalf("%s has no rest section", path)
	}

	patched := map[string]bool{}

	for _, raw := range transports {
		transport, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("%s: rest entry is not a mapping: %#v", path, raw)
		}

		name, _ := transport["name"].(string)
		if address, declared := addresses[name]; declared {
			transport["listen_ip"] = address
			patched[name] = true
		}
	}

	for name := range addresses {
		if !patched[name] {
			t.Fatalf("%s declares no rest transport named %q", listenIPFixture, name)
		}
	}

	out, err := yaml.Marshal(doc)
	if err != nil {
		t.Fatalf("Error marshaling %s: %v", path, err)
	}

	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatalf("Error writing %s: %v", path, err)
	}

	return dstDir
}

// generateWithListenIPs generates the fixture with the given addresses
// declared and returns the generated provider.
func generateWithListenIPs(t *testing.T, addresses map[string]string) string {
	t.Helper()

	configDir := declareListenIPs(t, addresses)
	targetDir := t.TempDir()

	out, err := generateFromConfigDir(t, configDir, targetDir, "Generate project with declared listen addresses")
	if err != nil {
		t.Fatalf("Error creating project: %v\n%s", err, out)
	}

	return readGenerated(t, targetDir, restConfigProviderPath...)
}

// TestGenerateRESTListenIPDefault verifies that the address declared in
// project.yaml becomes the default of the generated configuration provider,
// below both forms of the OnlineConf key.
//
// It is the address half of what TOOLS-88 did for the port: a server that
// serves only the machine it runs on has to bind to loopback, and until the
// address could be declared the only way to say so was to seed
// transport/rest/<server>/ip into the environment of every installation —
// a value the generator had in project.yaml all along.
func TestGenerateRESTListenIPDefault(t *testing.T) {
	provider := generateWithListenIPs(t, map[string]string{"api": "127.0.0.1", "sys": "[::1]"})

	// The table carries one entry per declaring server, keyed by the server
	// name the runtime hands back to the provider (<transport>_<version>).
	for _, want := range []string{`"api_v1": "127.0.0.1",`, `"sys_v1": "[::1]",`} {
		if !strings.Contains(provider, want) {
			t.Errorf("the generated provider does not declare %s", want)
		}
	}

	// The declared address is applied before either key is read, which is what
	// leaves a value in the tree winning over it — under the application name
	// first, transport-wide second.
	lookup := strings.Index(provider, "defaultListenIPs[serverName]")
	if lookup < 0 {
		t.Fatal("the generated provider never reads defaultListenIPs, so the declared address defaults nothing")
	}

	appKeyRead := strings.Index(provider, "occonf.RESTServerAppIP(")
	if appKeyRead < 0 {
		t.Fatal("the generated provider does not read the application-level ip key")
	}

	if lookup > appKeyRead {
		t.Error("the declared address is applied after the OnlineConf reads, so it would override the tree")
	}
}

// TestGenerateRESTListenIPNotDeclared verifies the other half: a project that
// declares no address gets no table and no lookup, so its provider is the one
// it had before the key existed.
func TestGenerateRESTListenIPNotDeclared(t *testing.T) {
	provider := readGenerated(t,
		generateFromConfig(t, listenIPFixture, "Generate project without a declared listen address"),
		restConfigProviderPath...)

	if strings.Contains(provider, "defaultListenIPs") {
		t.Error("a project that declares no listen_ip must not get the address table")
	}

	if !strings.Contains(provider, "ip := rest.DefaultListenIP") {
		t.Error("without a declared address the provider still falls back to the runtime's default")
	}
}

// TestGenerateRESTListenIPRejectsClient verifies that the key is rejected on a
// REST client. A client dials an address it reads from the tree, so a listen
// address on one configures nothing — and a silently ignored binding is how a
// server ends up listening somewhere its project.yaml says it does not.
func TestGenerateRESTListenIPRejectsClient(t *testing.T) {
	configDir := declareListenIPs(t, map[string]string{"api": "127.0.0.1"})

	// Turn the server into a client, keeping the address it just got.
	path := filepath.Join(configDir, "project.yaml")

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Error reading %s: %v", path, err)
	}

	doc := map[string]any{}
	if err := yaml.Unmarshal(content, &doc); err != nil {
		t.Fatalf("Error parsing %s: %v", path, err)
	}

	transports, _ := doc["rest"].([]any)
	for _, raw := range transports {
		transport, _ := raw.(map[string]any)
		if name, _ := transport["name"].(string); name == "api" {
			transport["generator_type"] = "ogen_client"
		}
	}

	out, err := yaml.Marshal(doc)
	if err != nil {
		t.Fatalf("Error marshaling %s: %v", path, err)
	}

	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatalf("Error writing %s: %v", path, err)
	}

	generated, err := generateFromConfigDir(t, configDir, t.TempDir(), "Generate project with listen_ip on a client")
	if err == nil {
		t.Fatalf("the generator accepted listen_ip on a REST client:\n%s", generated)
	}

	if !strings.Contains(generated, "listen_ip") {
		t.Errorf("the failure does not name the offending key:\n%s", generated)
	}
}
