package generator

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/Educentr/go-project-starter/internal/pkg/meta"
	"github.com/Educentr/go-project-starter/internal/pkg/templater"
	"github.com/Educentr/go-project-starter/internal/pkg/tools"
)

// copyTarget is one file the generator places into the project tree verbatim
// (an API spec or a JSON schema): source outside the project, destination
// inside it.
type copyTarget struct {
	source string
	dest   string
	kind   string
}

// specCopyTargets lists every transport spec copy the run is going to perform.
// Planning is separate from copying so the conflict check in Generate can see
// all destinations — specs and schemas alike — before the first file is
// written.
func (g *Generator) specCopyTargets() ([]copyTarget, error) {
	targets := []copyTarget{}

	for _, app := range g.Applications {
		for _, transport := range app.Transports {
			for specNum, spec := range transport.SpecPath {
				if _, err := os.Stat(spec); err != nil {
					return nil, fmt.Errorf("%w: %s", errSpecNotFound, spec)
				}

				targets = append(targets, copyTarget{
					source: spec,
					dest: filepath.Join(
						transport.GetTargetSpecDir(g.TargetDir),
						transport.GetTargetSpecFile(specNum),
					),
					kind: "spec",
				})
			}
		}
	}

	return targets, nil
}

// schemaCopyTargets lists every JSON schema copy the run is going to perform.
func (g *Generator) schemaCopyTargets() ([]copyTarget, error) {
	targets := []copyTarget{}

	for _, schema := range g.JSONSchemas {
		targetDir := schema.GetTargetSpecDir(g.TargetDir)

		// Collect paths from both legacy Path[] and new Schemas[]
		var paths []string

		if len(schema.Schemas) > 0 {
			for _, item := range schema.Schemas {
				paths = append(paths, item.Path)
			}
		} else {
			paths = schema.Path
		}

		for i, schemaPath := range paths {
			if _, err := os.Stat(schemaPath); err != nil {
				return nil, errors.Wrapf(err, "schema file not found: %s", schemaPath)
			}

			fileName := filepath.Base(schemaPath)
			if i < len(schema.SchemaTargetFiles) && schema.SchemaTargetFiles[i] != "" {
				fileName = schema.SchemaTargetFiles[i]
			}

			targets = append(targets, copyTarget{
				source: schemaPath,
				dest:   filepath.Join(targetDir, fileName),
				kind:   "schema",
			})
		}
	}

	return targets, nil
}

// ensureSchemaDirs creates the target directory of every configured JSON
// schema, including those that contribute no file — a schema block with no
// paths still gets its directory, as it did before copying was split out.
func (g *Generator) ensureSchemaDirs() error {
	for _, schema := range g.JSONSchemas {
		targetDir := schema.GetTargetSpecDir(g.TargetDir)

		if err := os.MkdirAll(targetDir, tools.DefaultDirPerm); err != nil {
			return fmt.Errorf("failed to create schema directory %s: %w", targetDir, err)
		}
	}

	return nil
}

// copyTargets performs the planned copies and returns the paths it wrote, so
// they can be recorded in the manifest.
func copyTargets(targets []copyTarget) ([]string, error) {
	written := make([]string, 0, len(targets))

	for _, target := range targets {
		log.Printf("copy %s: `%s` to `%s`\n", target.kind, target.source, target.dest)

		if err := tools.CopyFile(target.source, target.dest); err != nil {
			return nil, err
		}

		written = append(written, target.dest)
	}

	return written, nil
}

// checkCopyTargetConflicts fails the run when a copy destination is occupied by
// a file the generator did not write. Same rule as for generated files
// (templater.GetUserCodeFromFiles): the previous manifest is the only authority
// on "ours", so a hand-written spec sitting on a path the config has only now
// started to claim is reported instead of silently overwritten — copied files
// carry no disclaimer marker, so there would be nothing left of it.
//
// Without a manifest (project predates meta.yaml v5) there is nothing to
// compare against and the check is skipped, exactly as the generated-file diff
// falls back to its old behavior.
func checkCopyTargetConflicts(targetPath string, prev meta.Manifest, targets []copyTarget) error {
	if len(prev) == 0 {
		return nil
	}

	prevIdx := prev.Index()
	conflicts := []string{}

	for _, target := range targets {
		rel, err := meta.RelPath(targetPath, target.dest)
		if err != nil {
			return fmt.Errorf(errRelPathFmt, target.dest, err)
		}

		if _, ours := prevIdx[rel]; ours {
			continue
		}

		if st, err := os.Lstat(target.dest); err == nil && st.Mode().IsRegular() {
			conflicts = append(conflicts, target.dest)
		}
	}

	return templater.NewTargetConflictError(conflicts)
}
