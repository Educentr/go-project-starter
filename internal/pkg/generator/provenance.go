package generator

import (
	"context"

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
// channel in the `inputs` section of the manifest. The file itself is not
// copied into the project — the provenance is the only trace such an input
// leaves there.
//
// It resolves through the same Resolver as the pre-pass (g.sourceResolver), so
// an inline channel and a REST/gRPC/contract entry naming the same (repo, ref)
// share one clone and therefore one commit: the two used to clone separately,
// and a branch that moved in between built the tree from two states of the
// contracts without saying so. The staged file lives until Generator.Close
// rather than being dropped right after the parse, which is what keeps that
// clone — and the ref-override usage accounting (g.SourceRefs) — shared.
//
// LocalSource is read where it lies, so a local-only run still stages nothing.
func (g *Generator) resolveInlineSpec(ctx context.Context, channel, raw string) (string, error) {
	src, err := specsource.ParseSpecSource(raw)
	if err != nil {
		return "", err
	}

	res, err := g.sourceResolver().Resolve(ctx, src)
	if err != nil {
		return "", err
	}

	g.recordInput(channel, res.Provenance)
	g.recordSourceProvenance(res.LocalPath, res.Provenance)

	return res.LocalPath, nil
}

// inlineInputs returns the recorded inputs of this run, for the manifest.
func (g *Generator) inlineInputs() meta.Inputs {
	return meta.Inputs(g.inputs).Normalize()
}
