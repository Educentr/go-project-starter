package generator

import (
	"context"
	"fmt"
	"os"

	"github.com/Educentr/go-project-starter/internal/pkg/meta"
	"github.com/Educentr/go-project-starter/internal/pkg/specsource"
)

// Channels of project.yaml that feed the source resolver without leaving a file
// in the project. The label is the config key the input serves, so a record in
// the `inputs` section of meta.yaml says which declaration brought it in; the
// two channels that are declared per entity (workers, CLI) get their name
// appended by the call site.
const (
	channelMetricsPath    = "metrics.path"
	channelOnlineConfPath = "onlineconf.path"
	channelLintOverrides  = "tools.lint_config.overrides"
	channelWorkerSpecFmt  = "workers.%s.path"
	channelCLISpecFmt     = "applications.cli.%s.path"
)

// recordSourceProvenance remembers where a resolved file came from, keyed by
// the local path the resolver materialized it into. The copy step knows only
// that path, and the manifest record it builds has to carry the provenance of
// the source — not of the staging copy.
//
// Recording happens in the two functions every channel resolves through
// (resolveOne for the pre-pass, resolveInlineSpec for the inputs parsed during
// config load), which is what makes a channel added later recorded by the same
// code as the existing ones. TestEveryResolveRecordsProvenance holds that
// property.
func (g *Generator) recordSourceProvenance(localPath string, p specsource.Provenance) {
	if localPath == "" || p.IsZero() {
		return
	}

	if g.sourceProvenance == nil {
		g.sourceProvenance = map[string]meta.Provenance{}
	}

	g.sourceProvenance[localPath] = metaProvenance(p)
}

// provenanceOf returns the provenance recorded for the file the copy step read
// from localPath, or the zero value when the file did not come through the
// resolver.
func (g *Generator) provenanceOf(localPath string) meta.Provenance {
	return g.sourceProvenance[localPath]
}

// recordInput adds an input the generator read without copying it into the
// project (the metrics map, the OnlineConf key map, the lint overrides, the CLI
// and queue-worker specs). Such an input has no file record to carry its
// provenance, and the question it answers — which contracts the service was
// generated from — is about it just as much as about a copied spec.
func (g *Generator) recordInput(channel string, p specsource.Provenance) {
	if p.IsZero() {
		return
	}

	g.inputs = append(g.inputs, meta.InputRecord{Channel: channel, Provenance: metaProvenance(p)})
}

// metaProvenance converts the resolver's provenance into its manifest form. The
// manifest spells the source kind out as a string so that reading meta.yaml
// does not depend on the resolver's enum.
func metaProvenance(p specsource.Provenance) meta.Provenance {
	return meta.Provenance{
		SourceKind:   p.Kind.String(),
		SourceRepo:   p.Repo,
		SourcePath:   p.Path,
		Ref:          p.Ref,
		Commit:       p.Commit,
		SourceSHA256: p.SHA256,
	}
}

// resolveOne materializes one source of the resolve pre-pass and records its
// provenance. Every channel whose files are copied into the project goes
// through it, so none of them has to know that provenance is recorded at all.
func (g *Generator) resolveOne(
	ctx context.Context, r *specsource.Resolver, raw string,
) (specsource.ResolvedSpec, error) {
	src, err := specsource.ParseSpecSource(raw)
	if err != nil {
		return specsource.ResolvedSpec{}, err
	}

	res, err := r.Resolve(ctx, src)
	if err != nil {
		return specsource.ResolvedSpec{}, err
	}

	g.recordSourceProvenance(res.LocalPath, res.Provenance)

	return res, nil
}

// resolveInlineSpec materializes a spec source into a local file for one-shot
// parsing during config load (CLI commands.yaml, queue worker contracts, the
// metrics and OnlineConf key maps, the lint overrides) and records it under
// channel in the `inputs` section of the manifest. Unlike the resolve pre-pass
// — which keeps staged files alive for the spec/schema copy step — inline
// specs are read and parsed immediately, so the caller invokes the returned
// cleanup right after the parse and the staging dir is gone before Generate()
// runs. The provenance outlives it: that is the only trace such an input leaves.
//
// LocalSource short-circuits the staging directory: the file is read where it
// lies, so existing local-only flows incur no copy.
//
// The ref override (g.SourceRefs) applies here as it does to the pre-pass —
// the very same set, so that selector usage accumulates across both paths —
// otherwise the inline channels would be built from a different state of the
// contracts than the REST/gRPC/JSON Schema ones.
func (g *Generator) resolveInlineSpec(
	ctx context.Context, basePath, channel, raw string,
) (specPath string, cleanup func(), err error) {
	src, perr := specsource.ParseSpecSource(raw)
	if perr != nil {
		return "", func() {}, perr
	}

	stagingDir := ""

	if src.Kind() != specsource.KindLocal {
		dir, mkErr := os.MkdirTemp("", "gps-inline-spec-")
		if mkErr != nil {
			return "", func() {}, fmt.Errorf("create inline-spec staging dir: %w", mkErr)
		}

		stagingDir = dir
	}

	cleanupStaging := func() {
		if stagingDir != "" {
			_ = os.RemoveAll(stagingDir)
		}
	}

	resolver, rerr := specsource.NewResolver(basePath, stagingDir, specsource.ResolverOptions{
		SkipGitCheck: src.Kind() != specsource.KindGit,
		SourceRefs:   g.SourceRefs,
	})
	if rerr != nil {
		cleanupStaging()

		return "", func() {}, rerr
	}

	res, resErr := resolver.Resolve(ctx, src)
	if resErr != nil {
		_ = resolver.Close()

		cleanupStaging()

		return "", func() {}, resErr
	}

	g.recordInput(channel, res.Provenance)
	g.recordSourceProvenance(res.LocalPath, res.Provenance)

	return res.LocalPath, func() {
		_ = resolver.Close()

		cleanupStaging()
	}, nil
}

// inlineInputs returns the recorded inputs of this run, for the manifest.
func (g *Generator) inlineInputs() meta.Inputs {
	return meta.Inputs(g.inputs).Normalize()
}
