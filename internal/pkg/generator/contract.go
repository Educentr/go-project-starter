package generator

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	cfg "github.com/Educentr/go-project-starter/internal/pkg/config"
	"github.com/Educentr/go-project-starter/internal/pkg/ds"
	"github.com/Educentr/go-project-starter/internal/pkg/meta"
	"github.com/Educentr/go-project-starter/internal/pkg/specsource"
	"github.com/Educentr/go-project-starter/internal/pkg/tools"
	"github.com/pkg/errors"
)

// errContractCtxFmt prefixes a contract resolve failure with the entry it came
// from: with several contracts in a project, the source URI alone does not say
// which line of project.yaml to fix.
const errContractCtxFmt = "contract[%d] path=%s"

// errContractDrift reports a copied contract spec that was edited inside the
// project. Specs are owned by the contract repository: a local edit would be
// silently overwritten on the next run and, worse, would mean the generated
// code no longer matches any published contract.
var errContractDrift = errors.New("contract spec was changed inside the project")

// processGenerators folds the `generator` and `contract` sections into the
// generator state: the generators with their commands rendered once per
// contract, and the contracts themselves for the fetch/copy step.
//
// Commands are rendered here rather than in the template because the template
// has no business knowing the variable set — it prints recipe lines. The
// config is already validated at this point (cfg.ValidateGenerators), so a
// render failure here is a bug in that validation, not bad input.
func (g *Generator) processGenerators(config cfg.Config) error {
	if len(config.GeneratorList) == 0 && len(config.ContractList) == 0 {
		return nil
	}

	g.Contracts = make(ds.Contracts, 0, len(config.ContractList))

	for _, c := range config.ContractList {
		g.Contracts = append(g.Contracts, ds.Contract{
			RawPath:       c.Path,
			SpecPath:      c.Path,
			Copy:          c.Copy,
			Target:        c.Target,
			GeneratorName: c.GeneratorName,
		})
	}

	g.CustomGenerators = make(ds.CustomGenerators, 0, len(config.GeneratorList))

	for _, declared := range config.GeneratorList {
		gen := ds.CustomGenerator{Name: declared.Name, Install: declared.Install}

		for _, c := range config.ContractList {
			if c.GeneratorName != declared.Name {
				continue
			}

			cmd, err := cfg.RenderGeneratorCommand(declared.Cmd, c.Copy, c.Target)
			if err != nil {
				return errors.Wrapf(err, "generator %q cmd", declared.Name)
			}

			gen.Cmds = append(gen.Cmds, cmd)

			if declared.Lint == "" {
				continue
			}

			lint, err := cfg.RenderGeneratorCommand(declared.Lint, c.Copy, c.Target)
			if err != nil {
				return errors.Wrapf(err, "generator %q lint", declared.Name)
			}

			gen.Lints = append(gen.Lints, lint)
		}

		g.CustomGenerators = append(g.CustomGenerators, gen)
	}

	return nil
}

// resolveContracts materializes every contract spec and records where it came
// from. Runs inside the resolve pre-pass, so contract specs honor --sourceRef
// like every other channel — a release built from one state of the contracts
// must not mix in another.
func (g *Generator) resolveContracts(ctx context.Context, resolver *specsource.Resolver) error {
	for i := range g.Contracts {
		raw := g.Contracts[i].RawPath

		src, err := specsource.ParseSpecSource(raw)
		if err != nil {
			return errors.Wrapf(err, errContractCtxFmt, i, raw)
		}

		res, err := resolver.Resolve(ctx, src)
		if err != nil {
			return errors.Wrapf(err, errContractCtxFmt, i, raw)
		}

		g.Contracts[i].SpecPath = res.LocalPath
		g.Contracts[i].Source = res.Source.Describe()
		g.Contracts[i].Commit = res.Commit

		// Only a git source has a ref. For https and local, Source plus the
		// content hash is the whole of the provenance there is.
		if git, ok := res.Source.(specsource.GitSource); ok {
			g.Contracts[i].Ref = git.Ref
		}
	}

	return nil
}

// checkContractCollisions rejects a contract whose `copy` lands on a file the
// generator produces itself. Contracts are copied after the templates are
// written, so such an entry would overwrite generated code and then claim its
// manifest path — a silent clobber that only shows up as a build failure.
func (g *Generator) checkContractCollisions(generated meta.Manifest) error {
	if len(g.Contracts) == 0 {
		return nil
	}

	genPaths := generated.Index()

	for _, c := range g.Contracts {
		if _, clash := genPaths[c.Copy]; clash {
			return fmt.Errorf("%w: contract %s wants to copy to %s, which the generator produces itself",
				cfg.ErrInvalidConfig, c.RawPath, c.Copy)
		}
	}

	return nil
}

// checkContractDrift fails the run when a copied contract spec on disk no
// longer matches what the manifest recorded. It must be called before
// copyContracts, which would overwrite the evidence.
//
// This is an error, not the warning used for hand edits above a disclaimer
// marker: there the generated file is the artifact and regenerating it is the
// fix, whereas here the edit means the project and the contract repository
// disagree about what the contract is. The fix is a change in the contract
// repository, which the generator cannot make.
func (g *Generator) checkContractDrift(targetPath string) error {
	if len(g.Contracts) == 0 {
		return nil
	}

	prev := g.Meta.Manifest.Index()

	for _, c := range g.Contracts {
		rec, known := prev[c.Copy]
		if !known || rec.Hash == "" {
			continue
		}

		path := filepath.Join(targetPath, filepath.FromSlash(c.Copy))

		content, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				// Deleted by hand or never copied: the run below restores it.
				continue
			}

			return fmt.Errorf("error read contract spec %s: %w", path, err)
		}

		if hash := meta.HashContent(content); hash != rec.Hash {
			return fmt.Errorf("%w: %s (source: %s): edit the contract in its own repository, "+
				"not the copy inside the project", errContractDrift, c.Copy, contractSourceDesc(c, rec))
		}
	}

	return nil
}

// contractSourceDesc picks the most informative description of where a
// contract spec came from: the provenance of this run, falling back to what
// the previous run recorded.
func contractSourceDesc(c ds.Contract, rec meta.FileRecord) string {
	if c.Source != "" {
		return c.Source
	}

	if rec.Source != "" {
		return rec.Source
	}

	return c.RawPath
}

// copyContracts copies every resolved contract spec to its `copy` destination
// and returns the manifest records for them, provenance included.
func (g *Generator) copyContracts(targetPath string) (meta.Manifest, error) {
	if len(g.Contracts) == 0 {
		return nil, nil
	}

	records := make(meta.Manifest, 0, len(g.Contracts))

	for _, c := range g.Contracts {
		if _, err := os.Stat(c.SpecPath); err != nil {
			return nil, fmt.Errorf("%w: %s (contract %s)", errSpecNotFound, c.SpecPath, c.RawPath)
		}

		dest := filepath.Join(targetPath, filepath.FromSlash(c.Copy))

		if err := os.MkdirAll(filepath.Dir(dest), tools.DefaultDirPerm); err != nil {
			return nil, fmt.Errorf("error create contract dir %s: %w", filepath.Dir(dest), err)
		}

		log.Printf("copy contract: `%s` to `%s`\n", c.Source, dest)

		if err := tools.CopyFile(c.SpecPath, dest); err != nil {
			return nil, errors.Wrapf(err, "copy contract %s", c.RawPath)
		}

		content, err := os.ReadFile(dest)
		if err != nil {
			return nil, fmt.Errorf("error read copied contract %s: %w", dest, err)
		}

		records = append(records, meta.FileRecord{
			Path:   c.Copy,
			Kind:   meta.KindCopied,
			Hash:   meta.HashContent(content),
			Source: c.Source,
			Ref:    c.Ref,
			Commit: c.Commit,
		})
	}

	return records, nil
}
