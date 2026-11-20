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
	//	      rest:
	//	        cmd: npx openapi-typescript {{ .Spec }} -o src/api/{{ .SpecName }}.ts
	//	        inputs: [api]
	//	      jsonschema:
	//	        cmd: npx json2ts {{ .Spec }} -o src/models/{{ .SpecName }}.d.ts
	//	        inputs: [models]
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
	//
	// The command is declared per channel rather than once for both, because a
	// TS generator reads one of the two formats and not the other:
	// openapi-typescript takes OpenAPI, json-schema-to-typescript takes JSON
	// Schema. A single command would fail on the first spec of the other
	// channel and, since generate-web-<name> is a prerequisite of `generate`,
	// take `make generate` and `make regenerate` down with it.
	WebCodegen struct {
		// Rest generates from the rest[] entries the frontend consumes.
		// Optional, but at least one channel must be declared.
		Rest *WebCodegenChannel `mapstructure:"rest"`
		// JSONSchema generates from the jsonschema[] entries it consumes.
		// Optional, but at least one channel must be declared.
		JSONSchema *WebCodegenChannel `mapstructure:"jsonschema"`
	}

	// WebCodegenChannel is the codegen of one input channel: the command and
	// the entries of that section it is run over.
	WebCodegenChannel struct {
		// Cmd generates the client from one spec file. It is run once per spec
		// file of every input listed here. Required.
		// Template variables: .Spec, .SpecName, .Input.
		Cmd string `mapstructure:"cmd"`
		// Inputs names the entries of this channel's section the frontend
		// consumes. At least one. Required.
		Inputs []string `mapstructure:"inputs"`
	}

	WebAppList []WebApp

	// webField pairs a config field with its value so that a validation
	// failure can name the field it came from.
	webField struct {
		name  string
		value string
	}

	// webChannel pairs a declared codegen channel with the config section it
	// draws its inputs from, so that validation and rendering walk the
	// channels in one fixed order.
	webChannel struct {
		section string
		codegen *WebCodegenChannel
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

	// WebChannelRest and WebChannelJSONSchema name the codegen channels the
	// way project.yaml spells them, and the way the error messages do.
	WebChannelRest       = "rest"
	WebChannelJSONSchema = "jsonschema"
)

// webVars is the variable vocabulary of the web codegen command, in the order
// the error messages list it.
var webVars = []string{WebVarSpec, WebVarSpecName, WebVarInput} //nolint:gochecknoglobals // fixed lookup table, written as a literal and read-only afterwards

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

	if ok, msg := validateWebLayout(w); !ok {
		return false, prefix + msg
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

		if ok, msg := validateSingleLineCommand(f.name, f.value); !ok {
			return false, prefix + msg
		}
	}

	if strings.ContainsAny(w.NodeImage, " \t\n") {
		return false, prefix + "node_image must be a single image reference, got: " + w.NodeImage
	}

	return w.Codegen.isValid(prefix)
}

// validateWebLayout rejects a frontend whose sources live inside its own build
// output. The output directory belongs to the build: the generated .gitignore
// ignores it whole (`<output>/*`), so the sources would silently stop being
// tracked, and the image copies the build over it.
func validateWebLayout(w WebApp) (bool, string) {
	if w.Dir == w.Output || strings.HasPrefix(w.Dir, w.Output+"/") {
		return false, "dir '" + w.Dir + "' is inside output '" + w.Output +
			"': the output directory belongs to the build (it is git-ignored whole and overwritten in the image), " +
			"so the sources cannot live in it"
	}

	return true, ""
}

// channels returns the declared codegen channels in a fixed order, paired with
// the config section each draws its inputs from.
func (c *WebCodegen) channels() []webChannel {
	if c == nil {
		return nil
	}

	var out []webChannel

	for _, ch := range []webChannel{
		{section: WebChannelRest, codegen: c.Rest},
		{section: WebChannelJSONSchema, codegen: c.JSONSchema},
	} {
		if ch.codegen != nil {
			out = append(out, ch)
		}
	}

	return out
}

// isValid validates the codegen block of a web entry. A nil block is valid:
// a frontend that hand-writes its API client is still a frontend.
func (c *WebCodegen) isValid(prefix string) (bool, string) {
	if c == nil {
		return true, ""
	}

	channels := c.channels()

	// A codegen with no channel generates nothing, which is a declaration that
	// looks like it works and silently does not.
	if len(channels) == 0 {
		return false, prefix + "codegen: declares no channel (list the inputs it consumes under " +
			WebChannelRest + ": and/or " + WebChannelJSONSchema + ":)"
	}

	for _, ch := range channels {
		if ok, msg := ch.codegen.isValid(prefix + "codegen: " + ch.section + ": "); !ok {
			return false, msg
		}
	}

	return true, ""
}

// isValid validates one codegen channel. prefix already names the entry and
// the channel, so the messages only have to name the field.
func (c *WebCodegenChannel) isValid(prefix string) (bool, string) {
	if strings.TrimSpace(c.Cmd) == "" {
		return false, prefix + "empty cmd"
	}

	if ok, msg := validateSingleLineCommand("cmd", c.Cmd); !ok {
		return false, prefix + msg
	}

	if _, err := commandVars(c.Cmd, webVars); err != nil {
		return false, prefix + "cmd: " + err.Error()
	}

	if len(c.Inputs) == 0 {
		return false, prefix + "declares no input (list the entries it consumes)"
	}

	seen := make(map[string]struct{}, len(c.Inputs))

	for _, name := range c.Inputs {
		if name == "" {
			return false, prefix + "empty input name"
		}

		if _, dup := seen[name]; dup {
			return false, prefix + "duplicate input: " + name
		}

		seen[name] = struct{}{}
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
	byOutput := make(map[string]string, len(config.WebList))

	for _, w := range config.WebList {
		if ok, msg := w.IsValid(); !ok {
			return false, msg
		}

		if _, dup := byName[w.Name]; dup {
			return false, "duplicate web name: " + w.Name
		}

		byName[w.Name] = struct{}{}

		// Two frontends writing the same directory overwrite each other: in
		// the image the stage copied last wins, locally whichever build ran
		// last does.
		if other, dup := byOutput[w.Output]; dup {
			return false, "web '" + w.Name + "': output '" + w.Output + "' is already built by web '" + other +
				"' (two builds writing the same directory overwrite each other)"
		}

		byOutput[w.Output] = w.Name

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
	for _, ch := range w.Codegen.channels() {
		prefix := "web '" + w.Name + "': codegen: " + ch.section + ": "

		for _, name := range ch.codegen.Inputs {
			if ok, msg := validateWebInput(config, ch.section, name); !ok {
				return false, prefix + msg
			}
		}
	}

	return true, ""
}

// validateWebInput checks one input name against the section it names.
func validateWebInput(config *Config, section, name string) (bool, string) {
	switch section {
	case WebChannelRest:
		rest, ex := config.RestMap[name]
		if !ex {
			return false, "unknown rest input: " + name
		}

		if len(rest.Path) == 0 {
			return false, "rest input '" + name + "' has no path, so there is no spec to generate from"
		}
	case WebChannelJSONSchema:
		schema, ex := config.JSONSchemaMap[name]
		if !ex {
			return false, "unknown jsonschema input: " + name
		}

		if len(schema.Schemas) == 0 && len(schema.Path) == 0 {
			return false, "jsonschema input '" + name + "' declares no schema file"
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
