package config

import (
	"strings"
	"testing"
)

// webEntry is a valid web declaration the tests mutate one field at a time.
func webEntry() WebApp {
	return WebApp{
		Name:   "admin",
		Dir:    "web/admin",
		Build:  "npm run build",
		Output: "static",
	}
}

func TestWebApp_IsValid(t *testing.T) {
	withCodegen := func(c *WebCodegen) WebApp {
		w := webEntry()
		w.Codegen = c

		return w
	}

	tests := []struct {
		name    string
		web     WebApp
		wantOK  bool
		wantMsg string
	}{
		{
			name:   "build only",
			web:    webEntry(),
			wantOK: true,
		},
		{
			name: "install, build and codegen",
			web: withCodegen(&WebCodegen{
				Cmd:        "npx openapi-typescript {{ .Spec }} -o src/api/{{ .SpecName }}.ts",
				Rest:       []string{"api"},
				JSONSchema: []string{"models"},
			}),
			wantOK: true,
		},
		{
			name:    "empty name",
			web:     WebApp{Dir: "web", Build: "x", Output: "static"},
			wantMsg: "empty web name",
		},
		{
			name:    "uppercase name",
			web:     WebApp{Name: "Admin", Dir: "web", Build: "x", Output: "static"},
			wantMsg: "must match",
		},
		{
			name:    "missing dir",
			web:     WebApp{Name: "admin", Build: "x", Output: "static"},
			wantMsg: "dir is required",
		},
		{
			name:    "dir escapes the project",
			web:     WebApp{Name: "admin", Dir: "../web", Build: "x", Output: "static"},
			wantMsg: "must not escape the project root",
		},
		{
			name:    "missing output",
			web:     WebApp{Name: "admin", Dir: "web", Build: "x"},
			wantMsg: "output is required",
		},
		{
			name:    "empty build",
			web:     WebApp{Name: "admin", Dir: "web", Output: "static"},
			wantMsg: "empty build",
		},
		{
			name:    "build takes no variables",
			web:     WebApp{Name: "admin", Dir: "web", Build: "npm run build {{ .Spec }}", Output: "static"},
			wantMsg: "build takes no template variables",
		},
		{
			name:    "install takes no variables",
			web:     WebApp{Name: "admin", Dir: "web", Install: "npm ci {{ .Spec }}", Build: "x", Output: "static"},
			wantMsg: "install takes no template variables",
		},
		{
			name:    "node_image with arguments",
			web:     WebApp{Name: "admin", Dir: "web", Build: "x", Output: "static", NodeImage: "node:22 --rm"},
			wantMsg: "single image reference",
		},
		{
			name:    "codegen without cmd",
			web:     withCodegen(&WebCodegen{Rest: []string{"api"}}),
			wantMsg: "codegen: empty cmd",
		},
		{
			name:    "codegen without inputs",
			web:     withCodegen(&WebCodegen{Cmd: "npx openapi-typescript {{ .Spec }}"}),
			wantMsg: "declares no input",
		},
		{
			name:    "unknown variable in codegen cmd",
			web:     withCodegen(&WebCodegen{Cmd: "gen {{ .Target }}", Rest: []string{"api"}}),
			wantMsg: "unknown generator command variable: .Target",
		},
		{
			name:    "unparsable codegen cmd",
			web:     withCodegen(&WebCodegen{Cmd: "gen {{ .Spec", Rest: []string{"api"}}),
			wantMsg: "invalid template",
		},
		{
			name:    "duplicate rest input",
			web:     withCodegen(&WebCodegen{Cmd: "gen {{ .Spec }}", Rest: []string{"api", "api"}}),
			wantMsg: "duplicate rest input: api",
		},
		{
			name:    "empty input name",
			web:     withCodegen(&WebCodegen{Cmd: "gen {{ .Spec }}", JSONSchema: []string{""}}),
			wantMsg: "empty jsonschema input name",
		},
		{
			// Same name in two sections is not a duplicate: they are
			// different inputs that happen to share a name.
			name: "same name in both sections",
			web: withCodegen(&WebCodegen{
				Cmd: "gen {{ .Spec }}", Rest: []string{"models"}, JSONSchema: []string{"models"},
			}),
			wantOK: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, msg := tt.web.IsValid()

			if ok != tt.wantOK {
				t.Fatalf("IsValid() ok = %v, want %v (msg: %s)", ok, tt.wantOK, msg)
			}

			if tt.wantMsg != "" && !strings.Contains(msg, tt.wantMsg) {
				t.Errorf("IsValid() msg = %q, want it to contain %q", msg, tt.wantMsg)
			}
		})
	}
}

// webConfig is a config with one static transport serving "static" and the
// inputs a codegen may reference.
func webConfig(web ...WebApp) *Config {
	config := &Config{
		WebList: web,
		RestMap: map[string]Rest{
			"api": {Name: "api", Path: []string{"./api.swagger.yml"}, GeneratorType: "ogen"},
			"static": {
				Name:              "static",
				GeneratorType:     "template",
				GeneratorTemplate: "static",
			},
			"tpl": {Name: "tpl", GeneratorType: "template", GeneratorTemplate: "sys"},
		},
		JSONSchemaMap: map[string]JSONSchema{
			"models": {Name: "models", Schemas: []JSONSchemaItem{{ID: "user", Path: "./user.json"}}},
			"empty":  {Name: "empty"},
		},
		Applications: []Application{
			{Name: "web", TransportList: []AppTransport{{Name: "static"}, {Name: "api"}}},
		},
	}

	return config
}

func TestValidateWeb(t *testing.T) {
	codegen := func(c WebCodegen) WebApp {
		w := webEntry()
		w.Codegen = &c

		return w
	}

	tests := []struct {
		name    string
		config  *Config
		wantOK  bool
		wantMsg string
	}{
		{
			name:   "no web section",
			config: webConfig(),
			wantOK: true,
		},
		{
			name:   "output served by a static transport",
			config: webConfig(webEntry()),
			wantOK: true,
		},
		{
			name: "codegen over both input channels",
			config: webConfig(codegen(WebCodegen{
				Cmd: "gen {{ .Spec }}", Rest: []string{"api"}, JSONSchema: []string{"models"},
			})),
			wantOK: true,
		},
		{
			name: "duplicate web name",
			config: func() *Config {
				c := webConfig(webEntry(), webEntry())

				return c
			}(),
			wantMsg: "duplicate web name: admin",
		},
		{
			name: "output nobody serves",
			config: func() *Config {
				w := webEntry()
				w.Output = "frontend"

				return webConfig(w)
			}(),
			wantMsg: "output 'frontend' is not served by any application",
		},
		{
			name: "output served by a transport the application does not use",
			config: func() *Config {
				c := webConfig(webEntry())
				c.Applications[0].TransportList = []AppTransport{{Name: "api"}}

				return c
			}(),
			wantMsg: "is not served by any application",
		},
		{
			name:    "unknown rest input",
			config:  webConfig(codegen(WebCodegen{Cmd: "gen {{ .Spec }}", Rest: []string{"nope"}})),
			wantMsg: "unknown rest input: nope",
		},
		{
			name:    "rest input without a spec",
			config:  webConfig(codegen(WebCodegen{Cmd: "gen {{ .Spec }}", Rest: []string{"tpl"}})),
			wantMsg: "has no path",
		},
		{
			name:    "unknown jsonschema input",
			config:  webConfig(codegen(WebCodegen{Cmd: "gen {{ .Spec }}", JSONSchema: []string{"nope"}})),
			wantMsg: "unknown jsonschema input: nope",
		},
		{
			name:    "jsonschema input without a schema",
			config:  webConfig(codegen(WebCodegen{Cmd: "gen {{ .Spec }}", JSONSchema: []string{"empty"}})),
			wantMsg: "declares no schema file",
		},
		{
			name: "make targets collide with an application",
			config: func() *Config {
				c := webConfig(webEntry())
				c.Applications = append(c.Applications, Application{Name: "web-admin"})

				return c
			}(),
			wantMsg: "collide with application 'web-admin'",
		},
		{
			name: "make targets collide with a custom generator",
			config: func() *Config {
				c := webConfig(webEntry())
				c.GeneratorList = CustomGeneratorList{{Name: "web-admin", Cmd: "x"}}

				return c
			}(),
			wantMsg: "collide with generator 'web-admin'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, msg := ValidateWeb(tt.config)

			if ok != tt.wantOK {
				t.Fatalf("ValidateWeb() ok = %v, want %v (msg: %s)", ok, tt.wantOK, msg)
			}

			if tt.wantMsg != "" && !strings.Contains(msg, tt.wantMsg) {
				t.Errorf("ValidateWeb() msg = %q, want it to contain %q", msg, tt.wantMsg)
			}
		})
	}
}

// The static dir default is the transport's, not the web entry's: an output of
// "static" is served by a transport that does not spell its dir out.
func TestAppStaticDirs_DefaultDir(t *testing.T) {
	config := webConfig()

	dirs := appStaticDirs(config.Applications[0], config.RestMap)

	if len(dirs) != 1 || dirs[0] != "static" {
		t.Errorf("appStaticDirs() = %v, want [static]", dirs)
	}
}

func TestRenderWebCodegenCommand(t *testing.T) {
	got, err := RenderWebCodegenCommand(
		"npx openapi-typescript {{ .Spec }} -o src/api/{{ .SpecName }}.ts # {{ .Input }}",
		"../../api/rest/api/v1/api.yml", "api", "api",
	)
	if err != nil {
		t.Fatalf("RenderWebCodegenCommand() error = %v", err)
	}

	want := "npx openapi-typescript ../../api/rest/api/v1/api.yml -o src/api/api.ts # api"
	if got != want {
		t.Errorf("RenderWebCodegenCommand() = %q, want %q", got, want)
	}

	if _, err := RenderWebCodegenCommand("gen {{ .Spec }}\nsecond line", "a", "a", "api"); err == nil {
		t.Error("a command rendering to two lines must be rejected: it is one make recipe line")
	}
}
