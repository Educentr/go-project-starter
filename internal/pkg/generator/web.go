package generator

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	cfg "github.com/Educentr/go-project-starter/internal/pkg/config"
	"github.com/Educentr/go-project-starter/internal/pkg/ds"
	"github.com/Educentr/go-project-starter/internal/pkg/specsource"
	"github.com/pkg/errors"
)

// errWebInputUnknown reports a codegen input that names a rest/jsonschema
// entry the project does not declare. Config validation rejects this, so
// reaching it means the two have drifted apart.
var errWebInputUnknown = errors.New("web codegen input not found")

// processWebApps folds the `web` section into the generator state: the
// frontends with their codegen commands rendered once per spec file of every
// input they consume.
//
// The commands are rendered here rather than in the template for the same
// reason as the custom generators' — the template prints recipe lines and has
// no business knowing the variable set. What is specific to the web section is
// where the spec paths come from: the inputs are referenced by name, and the
// path handed to the command is the generator's own copy of that input, so the
// contract is fetched once and no second copy exists to drift from it.
//
// Must run before the resolve pre-pass, while SpecPath still holds the sources
// as written in project.yaml: the destination names are derived from them
// exactly the way resolveAllSources derives SpecTargetFiles.
func (g *Generator) processWebApps(config cfg.Config) error {
	if len(config.WebList) == 0 {
		return nil
	}

	g.WebApps = make(ds.WebApps, 0, len(config.WebList))

	for _, w := range config.WebList {
		web := ds.WebApp{
			Name:      w.Name,
			Dir:       w.Dir,
			Install:   w.Install,
			Build:     w.Build,
			Output:    w.Output,
			NodeImage: w.NodeImage,
		}

		if web.NodeImage == "" {
			web.NodeImage = ds.DefaultNodeImage
		}

		cmds, err := g.renderWebCodegen(w)
		if err != nil {
			return errors.Wrapf(err, "web %q codegen", w.Name)
		}

		web.CodegenCmds = cmds

		g.WebApps = append(g.WebApps, web)
	}

	return nil
}

// renderWebCodegen renders the codegen command of each declared channel once
// per spec file of the inputs that channel consumes — the rest channel first,
// inputs in declaration order. Each channel brings its own command: one TS
// generator reads OpenAPI, another reads JSON Schema, and a command run over
// the format it does not understand fails the whole `make generate`.
func (g *Generator) renderWebCodegen(w cfg.WebApp) ([]string, error) {
	if w.Codegen == nil {
		return nil, nil
	}

	cmds := []string{}

	if ch := w.Codegen.Rest; ch != nil {
		for _, name := range ch.Inputs {
			transport, ex := g.Transports[name]
			if !ex {
				return nil, fmt.Errorf("%w: rest %s", errWebInputUnknown, name)
			}

			rendered, err := renderCodegenForInput(
				ch.Cmd, w.Dir, name, filepath.ToSlash(transport.GetTargetSpecDir("")), transport.SpecPath,
			)
			if err != nil {
				return nil, err
			}

			cmds = append(cmds, rendered...)
		}
	}

	if ch := w.Codegen.JSONSchema; ch != nil {
		for _, name := range ch.Inputs {
			schema, ex := g.JSONSchemas[name]
			if !ex {
				return nil, fmt.Errorf("%w: jsonschema %s", errWebInputUnknown, name)
			}

			rendered, err := renderCodegenForInput(
				ch.Cmd, w.Dir, name, filepath.ToSlash(schema.GetTargetSpecDir("")), schemaSourcePaths(schema),
			)
			if err != nil {
				return nil, err
			}

			cmds = append(cmds, rendered...)
		}
	}

	return cmds, nil
}

// renderCodegenForInput renders a channel's command for every spec of one
// input. dir is where the command runs, specDir is where the generator copies
// that input's specs to, raw holds the sources as written in project.yaml.
func renderCodegenForInput(cmd, dir, input, specDir string, raw []string) ([]string, error) {
	cmds := make([]string, 0, len(raw))

	for _, source := range raw {
		src, err := specsource.ParseSpecSource(source)
		if err != nil {
			// Skipped, not reported: the resolve pass fails the run on this
			// source anyway, and does it with the transport or schema name and
			// the entry index attached. Until then the recipe is simply one
			// line short, and no recipe will ever be executed from this run.
			continue
		}

		file := src.TargetFilename()

		rendered, err := cfg.RenderWebCodegenCommand(
			cmd,
			relFromWebDir(dir, path.Join(specDir, file)),
			strings.TrimSuffix(file, path.Ext(file)),
			input,
		)
		if err != nil {
			return nil, err
		}

		cmds = append(cmds, rendered)
	}

	return cmds, nil
}

// relFromWebDir expresses a project-relative path relative to the frontend
// directory, which is where the codegen command runs. Both paths are
// project-relative and clean (config validation guarantees it for dir), so the
// result is always a plain relative path.
func relFromWebDir(dir, target string) string {
	rel, err := filepath.Rel(filepath.FromSlash(dir), filepath.FromSlash(target))
	if err != nil {
		// Two relative paths under the same root always have a relation;
		// falling back to the project-relative path keeps the command
		// readable if that ever stops holding.
		return target
	}

	return filepath.ToSlash(rel)
}
