package ds

// WebApp is a frontend of the project — sources, the commands that install and
// build them, and the directory the build writes into, which a static
// transport serves. Commands come from the project, as for CustomGenerator:
// the generator owns wiring the targets and the build stage, not the choice of
// bundler or client generator.
type WebApp struct {
	// Name is the web entry name from project.yaml. It is the suffix of the
	// generated make targets (install-web-<Name>, build-web-<Name>,
	// generate-web-<Name>) and of the Dockerfile stage (web-<Name>).
	Name string
	// Dir is the project-relative directory of the frontend sources. Every
	// command of this entry runs in it.
	Dir string
	// Install installs the frontend dependencies. Empty when the project
	// declares none — then no install-web-<Name> target is generated.
	Install string
	// Build builds the frontend into Output.
	Build string
	// Output is the project-relative directory the build writes into, served
	// by a static transport of the application that embeds this frontend.
	Output string
	// NodeImage is the base image of the build stage in the Dockerfile.
	NodeImage string
	// CodegenCmds are the rendered client-generation commands, one per spec
	// file of the declared rest/jsonschema inputs, in declaration order. Empty
	// when the entry declares no codegen.
	CodegenCmds []string
}

// WebApps keeps the declaration order of project.yaml so that the generated
// Makefile and Dockerfiles are stable between runs.
type WebApps []WebApp

// Static file serving, named once so that the transport, the web section and
// the config validation agree on it.
const (
	// StaticTransportTemplate is the generator_template of the static file
	// server (generator_type: template).
	StaticTransportTemplate = "static"

	// DefaultStaticDir is the project directory a static transport serves when
	// generator_params.dir is not set.
	DefaultStaticDir = "static"

	// DefaultNodeImage is the base image of the generated frontend build
	// stage when a web entry does not name one.
	DefaultNodeImage = "node:22-alpine"
)

// HasCodegen reports whether the entry generates a client from the project's
// own copies of its rest/jsonschema inputs.
func (w WebApp) HasCodegen() bool {
	return len(w.CodegenCmds) > 0
}

// StageName is the name of this frontend's build stage in a Dockerfile.
func (w WebApp) StageName() string {
	return "web-" + w.Name
}

// ForApp returns the frontends whose Output is served by a static transport of
// app — the ones whose build belongs in that application's image. The binding
// is derived rather than declared twice: a frontend is embedded exactly where
// its output is served from.
func (ws WebApps) ForApp(app App) WebApps {
	if len(ws) == 0 {
		return nil
	}

	served := make(map[string]bool)

	for _, dir := range app.StaticDirs() {
		served[dir] = true
	}

	out := make(WebApps, 0, len(ws))

	for _, w := range ws {
		if served[w.Output] {
			out = append(out, w)
		}
	}

	return out
}
