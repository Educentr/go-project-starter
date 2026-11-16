package generator

import (
	"strings"
	"testing"

	cfg "github.com/Educentr/go-project-starter/internal/pkg/config"
	"github.com/Educentr/go-project-starter/internal/pkg/ds"
	"github.com/pkg/errors"
)

// webGenerator is a generator state with the two inputs a web codegen may
// reference, holding the spec sources exactly as project.yaml writes them —
// processWebApps runs before the resolve pre-pass rewrites them.
func webGenerator() Generator {
	return Generator{
		Transports: ds.Transports{
			"api": ds.Transport{
				Name:       "api",
				ApiVersion: "v1",
				Type:       ds.RestTransportType,
				SpecPath:   []string{"./api.swagger.yml", "git+ssh://git@github.com/org/specs.git#rest/common.yaml"},
			},
		},
		JSONSchemas: ds.JSONSchemas{
			"models": ds.JSONSchema{
				Name:    "models",
				Schemas: []ds.JSONSchemaItem{{ID: "user", Path: "./user.schema.json"}},
			},
		},
	}
}

func TestProcessWebApps_RendersCodegenPerSpec(t *testing.T) {
	config := cfg.Config{
		WebList: cfg.WebAppList{
			{
				Name:    "admin",
				Dir:     "web/admin",
				Install: "npm ci",
				Build:   "npm run build",
				Output:  "static",
				Codegen: &cfg.WebCodegen{
					Rest: &cfg.WebCodegenChannel{
						Cmd:    "npx openapi-typescript {{ .Spec }} -o src/api/{{ .SpecName }}.ts # {{ .Input }}",
						Inputs: []string{"api"},
					},
					JSONSchema: &cfg.WebCodegenChannel{
						Cmd:    "npx json2ts {{ .Spec }} -o src/models/{{ .SpecName }}.d.ts # {{ .Input }}",
						Inputs: []string{"models"},
					},
				},
			},
			{Name: "landing", Dir: "web/landing", Build: "npm run build", Output: "public_web", NodeImage: "node:20-alpine"},
		},
	}

	g := webGenerator()
	if err := g.processWebApps(config); err != nil {
		t.Fatalf("processWebApps() error = %v", err)
	}

	if len(g.WebApps) != 2 {
		t.Fatalf("got %d web apps, want 2", len(g.WebApps))
	}

	admin := g.WebApps[0]

	// The commands are handed the generator's own copies of the inputs —
	// api/rest/<name>/<version>/ and api/schema/<name>/ — relative to the
	// frontend directory, which is where they run. No second copy exists.
	//
	// Each channel is generated with its own command: a JSON Schema fed to an
	// OpenAPI generator fails the recipe, and with it the whole
	// `make generate` that generate-web-admin is part of.
	wantCmds := []string{
		"npx openapi-typescript ../../api/rest/api/v1/api.swagger.yml -o src/api/api.swagger.ts # api",
		"npx openapi-typescript ../../api/rest/api/v1/common.yaml -o src/api/common.ts # api",
		"npx json2ts ../../api/schema/models/user.schema.json -o src/models/user.schema.d.ts # models",
	}

	if len(admin.CodegenCmds) != len(wantCmds) {
		t.Fatalf("codegen commands = %v, want %v", admin.CodegenCmds, wantCmds)
	}

	for i := range wantCmds {
		if admin.CodegenCmds[i] != wantCmds[i] {
			t.Errorf("codegen command[%d] = %q, want %q", i, admin.CodegenCmds[i], wantCmds[i])
		}
	}

	if !admin.HasCodegen() {
		t.Error("HasCodegen() = false for an entry with rendered commands")
	}

	if admin.NodeImage != ds.DefaultNodeImage {
		t.Errorf("NodeImage = %q, want the default %q", admin.NodeImage, ds.DefaultNodeImage)
	}

	if admin.StageName() != "web-admin" {
		t.Errorf("StageName() = %q, want web-admin", admin.StageName())
	}

	landing := g.WebApps[1]

	if landing.HasCodegen() {
		t.Error("HasCodegen() = true for an entry without a codegen block")
	}

	if landing.NodeImage != "node:20-alpine" {
		t.Errorf("NodeImage = %q, want the declared node:20-alpine", landing.NodeImage)
	}
}

// An empty section leaves the state nil, which is what keeps the generated
// Makefile and Dockerfiles byte-identical for a project that declares no
// frontend.
func TestProcessWebApps_EmptySectionLeavesStateNil(t *testing.T) {
	g := webGenerator()
	if err := g.processWebApps(cfg.Config{}); err != nil {
		t.Fatalf("processWebApps() error = %v", err)
	}

	if g.WebApps != nil {
		t.Errorf("WebApps = %v, want nil", g.WebApps)
	}
}

// Config validation rejects a dangling input, so reaching processWebApps with
// one means the two have drifted apart — it must say so, not render an empty
// path into the command.
func TestProcessWebApps_UnknownInput(t *testing.T) {
	config := cfg.Config{
		WebList: cfg.WebAppList{{
			Name: "admin", Dir: "web/admin", Build: "npm run build", Output: "static",
			Codegen: &cfg.WebCodegen{
				Rest: &cfg.WebCodegenChannel{Cmd: "gen {{ .Spec }}", Inputs: []string{"missing"}},
			},
		}},
	}

	g := webGenerator()

	err := g.processWebApps(config)
	if !errors.Is(err, errWebInputUnknown) {
		t.Fatalf("processWebApps() error = %v, want errWebInputUnknown", err)
	}

	if !strings.Contains(err.Error(), "missing") {
		t.Errorf("error = %q, want it to name the input", err)
	}
}

// ForApp is what binds a frontend to an image: the application that serves the
// output directory is the one whose Dockerfile builds it.
func TestWebAppsForApp(t *testing.T) {
	webApps := ds.WebApps{
		{Name: "admin", Output: "static"},
		{Name: "landing", Output: "public_web"},
	}

	serving := func(dir string) ds.App {
		return ds.App{Transports: ds.Transports{dir: ds.Transport{
			GeneratorType: "template", GeneratorTemplate: ds.StaticTransportTemplate,
			GeneratorParams: map[string]string{"dir": dir},
		}}}
	}

	got := webApps.ForApp(serving("public_web"))
	if len(got) != 1 || got[0].Name != "landing" {
		t.Errorf("ForApp() = %v, want only landing", got)
	}

	if got := webApps.ForApp(ds.App{}); len(got) != 0 {
		t.Errorf("ForApp() on an application with no static transport = %v, want none", got)
	}
}
