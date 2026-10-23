package generator

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/Educentr/go-project-starter/internal/pkg/ds"
	"github.com/Educentr/go-project-starter/internal/pkg/meta"
	"github.com/Educentr/go-project-starter/internal/pkg/specsource"
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

		paths := schemaSourcePaths(schema)

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

// schemaSourcePaths returns the schema source paths of one jsonschema entry,
// preferring the Schemas[] form over the legacy Path[] one.
func schemaSourcePaths(schema ds.JSONSchema) []string {
	if len(schema.Schemas) == 0 {
		return schema.Path
	}

	paths := make([]string, 0, len(schema.Schemas))
	for _, item := range schema.Schemas {
		paths = append(paths, item.Path)
	}

	return paths
}

// copiedSpecTargets lists every file the copy pass (specCopyTargets /
// schemaCopyTargets) will write into the project, paired with the source it
// comes from. targetPath must be absolute.
//
// It MUST be called before resolveAllSources: afterwards SpecPath points into
// the per-run staging directory, and a staging path in a drift message tells
// the reader nothing about where to actually fix the contract. Destination
// names are derived exactly the way the resolve pre-pass derives
// SpecTargetFiles (SpecSource.TargetFilename), so they match the files the copy
// pass goes on to write.
func (g *Generator) copiedSpecTargets(targetPath string) ([]meta.CopiedFile, error) {
	targets := []meta.CopiedFile{}
	seen := map[string]struct{}{}

	add := func(dir, raw string) error {
		src, err := specsource.ParseSpecSource(raw)
		if err != nil {
			// Reporting a malformed source is not this pre-pass's job: the
			// resolve pass fails on it anyway, with the transport or schema
			// name and the entry index attached.
			return nil //nolint:nilerr // malformed sources are the resolve pass's error to report
		}

		dest := filepath.Join(dir, src.TargetFilename())

		rel, err := meta.RelPath(targetPath, dest)
		if err != nil {
			return fmt.Errorf(errRelPathFmt, dest, err)
		}

		// A transport is snapshotted into every application that serves it, so
		// the same destination is reached more than once.
		if _, dup := seen[rel]; dup {
			return nil
		}

		seen[rel] = struct{}{}

		targets = append(targets, meta.CopiedFile{Path: rel, Source: src.Describe()})

		return nil
	}

	for _, app := range g.Applications {
		for _, transport := range app.Transports {
			dir := transport.GetTargetSpecDir(targetPath)

			for _, raw := range transport.SpecPath {
				if err := add(dir, raw); err != nil {
					return nil, err
				}
			}
		}
	}

	for _, schema := range g.JSONSchemas {
		dir := schema.GetTargetSpecDir(targetPath)

		for _, raw := range schemaSourcePaths(schema) {
			if err := add(dir, raw); err != nil {
				return nil, err
			}
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
