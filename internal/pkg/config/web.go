package config

import (
	"regexp"
	"strings"

	"github.com/Educentr/go-project-starter/internal/pkg/ds"
)

type (
	// WebApp declares a frontend of the project: where its sources live, how
	// to install its dependencies, how to build it, and which directory the
	// build writes into — the one a static transport serves.
	//
	// The commands belong to the project, as in the `generator` section: the
	// starter owns wiring the make targets and the Dockerfile build stage, not
	// the choice of bundler or client generator. All of them run in `dir`,
	// which is where the frontend's own manifest (package.json and friends)
	// lives.
	//
	// YAML example:
	//
	//	web:
	//	  - name: admin
	//	    dir: web/admin
	//	    install: npm ci
	//	    build: npm run build
	//	    output: static
	//	    codegen:
	//	      cmd: npx openapi-typescript {{ .Spec }} -o src/api/{{ .SpecName }}.ts
	//	      rest: [api]
	//	      jsonschema: [models]
	//
	// Omitting the section keeps the previous behavior: no web targets, no
	// build stage, a Makefile and a Dockerfile unchanged to the byte.
	//
	// See docs/configuration/web.md for full documentation.
	WebApp struct {
		// Name identifies the frontend. It becomes the suffix of the generated
		// make targets and of the Dockerfile stage, so it must match
		// ^[a-z0-9_-]+$. Required.
		Name string `mapstructure:"name"`
		// Dir is the project-relative directory of the frontend sources. Every
		// command of this entry runs in it. Required.
		Dir string `mapstructure:"dir"`
		// Install installs the frontend dependencies (`npm ci`, `yarn
		// install --frozen-lockfile`, …). Optional, but it is also what makes
		// the codegen tool available, so a codegen entry usually needs it.
		// No template variables.
		Install string `mapstructure:"install"`
		// Build builds the frontend into Output. Required.
		// No template variables.
		Build string `mapstructure:"build"`
		// Output is the project-relative directory the build writes into. It
		// must be a directory served by a static transport of some
		// application — that application's image is where the build lands.
		// Required.
		Output string `mapstructure:"output"`
		// NodeImage is the base image of the frontend build stage in the
		// generated Dockerfile. Optional, defaults to ds.DefaultNodeImage.
		NodeImage string `mapstructure:"node_image"`
		// Codegen generates the frontend's API client from the project's own
		// copies of the declared inputs. Optional.
		Codegen *WebCodegen `mapstructure:"codegen"`
	}

	// WebCodegen declares how the frontend's API client is generated and from
	// which of the project's inputs. The inputs are referenced by name, never
	// by path: the specs the generator already fetches and copies
	// (api/rest/<name>/<version>/…, api/schema/<name>/…) are the inputs, and a
	// second copy of the same contract is exactly what this avoids.
	WebCodegen struct {
		// Cmd generates the client from one spec file. It is run once per spec
		// file of every declared input. Required.
		// Template variables: .Spec, .SpecName, .Input.
		Cmd string `mapstructure:"cmd"`
		// Rest lists rest[] entry names the frontend consumes.
		Rest []string `mapstructure:"rest"`
		// JSONSchema lists jsonschema[] entry names the frontend consumes.
		JSONSchema []string `mapstructure:"jsonschema"`
	}

	WebAppList []WebApp

	// webField pairs a config field with its value so that a validation
	// failure can name the field it came from.
	webField struct {
		name  string
		value string
	}
)

// Variables a web codegen command may reference. The command is run per spec
// file, so it is told which file to read, a name to derive its output from,
// and which declared input the file came from.
const (
	// WebVarSpec is the path of the generator's own copy of the spec,
	// relative to the entry's `dir` (where the command runs).
	WebVarSpec = "Spec"
	// WebVarSpecName is the basename of that copy without its extension.
	WebVarSpecName = "SpecName"
	// WebVarInput is the name of the rest[]/jsonschema[] entry the spec file
	// belongs to.
	WebVarInput = "Input"
)

// webVars is the variable vocabulary of the web codegen command, in the order
// the error messages list it.
var webVars = []string{WebVarSpec, WebVarSpecName, WebVarInput}

// webNameRe is the accepted shape of a web entry name: it becomes a make
// target suffix (install-web-<name>, build-web-<name>, generate-web-<name>)
// and a Dockerfile stage name (web-<name>).
var webNameRe = regexp.MustCompile(`^[a-z0-9_-]+$`)

// IsValid validates a single web entry in isolation. Cross-checks against the
// rest of the config (the output being served, the inputs existing) are in
// ValidateWeb.
func (w WebApp) IsValid() (bool, string) {
	if w.Name == "" {
		return false, "empty web name"
	}

	if !webNameRe.MatchString(w.Name) {
		return false, "web name must match ^[a-z0-9_-]+$, got: " + w.Name
	}

	prefix := "web '" + w.Name + "': "

	// Fixed order, not map iteration: an error message that names a
	// different field on every run is a message nobody can act on.
	for _, f := range []webField{{name: "dir", value: w.Dir}, {name: "output", value: w.Output}} {
		if ok, msg := validateProjectRelPath(f.name, f.value); !ok {
			return false, prefix + msg
		}
	}

	if strings.TrimSpace(w.Build) == "" {
		return false, prefix + "empty build"
	}

	// install and build run once per entry, not per spec, so a variable there
	// would have no value to resolve to.
	for _, f := range []webField{{name: "install", value: w.Install}, {name: "build", value: w.Build}} {
		if strings.Contains(f.value, "{{") {
			return false, prefix + f.name + " takes no template variables"
		}
	}

	if strings.ContainsAny(w.NodeImage, " \t\n") {
		return false, prefix + "node_image must be a single image reference, got: " + w.NodeImage
	}

	return w.Codegen.isValid(prefix)
}

// isValid validates the codegen block of a web entry. A nil block is valid:
// a frontend that hand-writes its API client is still a frontend.
func (c *WebCodegen) isValid(prefix string) (bool, string) {
	if c == nil {
		return true, ""
	}

	if strings.TrimSpace(c.Cmd) == "" {
		return false, prefix + "codegen: empty cmd"
	}

	if _, err := commandVars(c.Cmd, webVars); err != nil {
		return false, prefix + "codegen: cmd: " + err.Error()
	}

	// A codegen with no input generates nothing, which is a declaration that
	// looks like it works and silently does not.
	if len(c.Rest) == 0 && len(c.JSONSchema) == 0 {
		return false, prefix + "codegen: declares no input (list the rest/jsonschema entries it consumes)"
	}

	seen := make(map[string]struct{}, len(c.Rest)+len(c.JSONSchema))

	for _, in := range []struct {
		section string
		names   []string
	}{{section: "rest", names: c.Rest}, {section: "jsonschema", names: c.JSONSchema}} {
		for _, name := range in.names {
			if name == "" {
				return false, prefix + "codegen: empty " + in.section + " input name"
			}

			key := in.section + ":" + name

			if _, dup := seen[key]; dup {
				return false, prefix + "codegen: duplicate " + in.section + " input: " + name
			}

			seen[key] = struct{}{}
		}
	}

	return true, ""
}

// ValidateWeb validates the web section against the rest of the config: the
// entries themselves, the inputs their codegen consumes, and the static
// transport that serves each output.
//
// It runs after the applications have been normalized, because that is what
// says which directories are actually served.
func ValidateWeb(config *Config) (bool, string) {
	byName := make(map[string]struct{}, len(config.WebList))

	for _, w := range config.WebList {
		if ok, msg := w.IsValid(); !ok {
			return false, msg
		}

		if _, dup := byName[w.Name]; dup {
			return false, "duplicate web name: " + w.Name
		}

		byName[w.Name] = struct{}{}

		if ok, msg := validateWebTargets(config, w); !ok {
			return false, msg
		}

		if ok, msg := validateWebOutput(config, w); !ok {
			return false, msg
		}

		if ok, msg := validateWebInputs(config, w); !ok {
			return false, msg
		}
	}

	return true, ""
}

// validateWebTargets rejects a name whose derived make targets would collide
// with the per-application ones (build-<app>): with an application called
// web-admin, the frontend `admin` would silently redefine build-web-admin.
func validateWebTargets(config *Config, w WebApp) (bool, string) {
	derived := "web-" + w.Name

	for _, app := range config.Applications {
		if app.Name == derived {
			return false, "web '" + w.Name + "': its make targets collide with application '" + derived + "'"
		}
	}

	for _, gen := range config.GeneratorList {
		if gen.Name == derived {
			return false, "web '" + w.Name + "': its make targets collide with generator '" + derived + "'"
		}
	}

	return true, ""
}

// validateWebOutput checks that some application actually serves the output
// directory. An output nobody serves is a build whose result never reaches a
// response — and nothing would copy it into an image either.
func validateWebOutput(config *Config, w WebApp) (bool, string) {
	for _, app := range config.Applications {
		for _, dir := range appStaticDirs(app, config.RestMap) {
			if dir == w.Output {
				return true, ""
			}
		}
	}

	return false, "web '" + w.Name + "': output '" + w.Output +
		"' is not served by any application (declare a rest transport with " +
		"generator_template: " + ds.StaticTransportTemplate + " and generator_params.dir: " + w.Output + ")"
}

// validateWebInputs checks that every referenced input exists and brings a
// spec with it: an entry with no spec file (a template transport, an empty
// schema block) would render the codegen command zero times.
func validateWebInputs(config *Config, w WebApp) (bool, string) {
	if w.Codegen == nil {
		return true, ""
	}

	prefix := "web '" + w.Name + "': codegen: "

	for _, name := range w.Codegen.Rest {
		rest, ex := config.RestMap[name]
		if !ex {
			return false, prefix + "unknown rest input: " + name
		}

		if len(rest.Path) == 0 {
			return false, prefix + "rest input '" + name + "' has no path, so there is no spec to generate from"
		}
	}

	for _, name := range w.Codegen.JSONSchema {
		schema, ex := config.JSONSchemaMap[name]
		if !ex {
			return false, prefix + "unknown jsonschema input: " + name
		}

		if len(schema.Schemas) == 0 && len(schema.Path) == 0 {
			return false, prefix + "jsonschema input '" + name + "' declares no schema file"
		}
	}

	return true, ""
}

// appStaticDirs returns the project directories served by the static template
// transports of app, applying the same default as ds.App.StaticDirs — this
// runs at config load, before the ds model exists.
func appStaticDirs(app Application, restMap map[string]Rest) []string {
	dirs := []string{}

	for _, appTr := range app.TransportList {
		rest, ex := restMap[appTr.Name]
		if !ex || rest.GeneratorType != "template" || rest.GeneratorTemplate != ds.StaticTransportTemplate {
			continue
		}

		dir := rest.GeneratorParams["dir"]
		if dir == "" {
			dir = ds.DefaultStaticDir
		}

		dirs = append(dirs, dir)
	}

	return dirs
}

// RenderWebCodegenCommand renders the codegen command for one spec file.
// Callers validate first (ValidateWeb), so a failure here means the template
// was never checked.
func RenderWebCodegenCommand(cmd, spec, specName, input string) (string, error) {
	return renderCommand(cmd, struct {
		Spec     string
		SpecName string
		Input    string
	}{Spec: spec, SpecName: specName, Input: input})
}
