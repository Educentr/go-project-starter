package meta

import (
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

// Provenance records where an input of the run came from. It is written for
// every file the generator fetched through the source resolver — whatever
// channel of project.yaml asked for it (`rest[]`, `grpc`, `jsonschema[]`,
// `contract:`, `metrics.path`, `onlineconf.path`, `tools.lint_config.overrides`,
// the CLI and queue-worker specs) — so that a consumer can state which tree of
// contracts a service was generated from before it is released.
//
// The identity of the source and the path inside it are separate fields: a
// single "url@ref#path" string has to be parsed before it can be compared with
// anything. No field ever carries a token; an https source keeps the name of
// its token_env variable in project.yaml, not here.
//
// SourceSHA256 is the digest of the bytes received from the source, before
// anything rewrote them. For a file copied as-is it equals FileRecord.Hash; for
// a spec whose $refs were rewritten on the way in (`rewrite_refs`) it does not,
// and that is the whole reason it is recorded separately — Hash cannot be
// compared with the blob in the source repository.
type Provenance struct {
	// SourceKind is local, http or git — which of the fields below the record
	// is expected to carry. Recorded explicitly so a reader does not have to
	// infer the kind from which fields are empty.
	SourceKind string `yaml:"source_kind,omitempty"`
	// SourceRepo identifies the source: the repository URL for git (credentials
	// stripped), scheme and host for http, empty for a local path.
	SourceRepo string `yaml:"source_repo,omitempty"`
	// SourcePath is the path of the file inside SourceRepo: the git subpath,
	// the URL path for http, the path as written in project.yaml for local.
	SourcePath string `yaml:"source_path,omitempty"`
	// Ref is the effective git ref the file was fetched at, so a ref that came
	// from --sourceRef is recorded rather than the config default. Git only.
	Ref string `yaml:"ref,omitempty"`
	// Commit is the SHA the git source resolved to at fetch time. Git only.
	Commit string `yaml:"commit,omitempty"`
	// SourceSHA256 is the digest of the bytes as received from the source.
	SourceSHA256 string `yaml:"source_sha256,omitempty"`
}

// InputRecord is the provenance of an input the generator read but did not
// copy into the project: the metrics map, the OnlineConf key map, the lint
// overrides, the CLI and queue-worker specs. Their artifact is generated code,
// so they have no file record to hang provenance on — and the question a
// consumer asks ("which contracts was this built from?") is about them just as
// much as about the copied specs.
//
// Channel is the project.yaml key the input serves (`metrics.path`,
// `workers.<name>.path`, …), so the record says which declaration brought it in.
type InputRecord struct {
	Channel    string `yaml:"channel"`
	Provenance `yaml:",inline"`
}

// Inputs is the list of resolver inputs of the last run that left no file in
// the project.
type Inputs []InputRecord

// Kinds of source recorded in Provenance.SourceKind. They are the string form
// of specsource.Kind; spelled out here so that reading a manifest does not
// depend on the resolver.
const (
	SourceKindLocal = "local"
	SourceKindHTTP  = "http"
	SourceKindGit   = "git"
)

// provenanceFieldCount is how many fields a provenance record can carry; the
// capacity hint of kvs.
const provenanceFieldCount = 6

// IsZero reports whether no provenance was recorded — a file the generator
// produced from a template rather than fetched.
func (p Provenance) IsZero() bool {
	return p == Provenance{}
}

// Describe renders the source the way project.yaml writes it, for messages:
// "<repo>@<ref>#<path>" for git, the URL for http, the path for local. Empty
// when there is no provenance.
func (p Provenance) Describe() string {
	switch p.SourceKind {
	case SourceKindGit:
		return fmt.Sprintf("%s@%s#%s", p.SourceRepo, p.Ref, p.SourcePath)
	case SourceKindHTTP:
		return p.SourceRepo + p.SourcePath
	default:
		return p.SourcePath
	}
}

// kvs returns the provenance fields in manifest order, skipping the empty ones.
// Shared by the flow-style marshalers of FileRecord and InputRecord so the two
// cannot drift into different key orders.
func (p Provenance) kvs() [][2]string {
	out := make([][2]string, 0, provenanceFieldCount)

	for _, kv := range [][2]string{
		{"source_kind", p.SourceKind},
		{"source_repo", p.SourceRepo},
		{"source_path", p.SourcePath},
		{"ref", p.Ref},
		{"commit", p.Commit},
		{"source_sha256", p.SourceSHA256},
	} {
		if kv[1] != "" {
			out = append(out, kv)
		}
	}

	return out
}

// MarshalYAML renders a record as a single-line flow mapping, for the same
// reason FileRecord does: meta.yaml is merged across branches, so its diffs
// have to stay line-local.
func (r InputRecord) MarshalYAML() (any, error) {
	return flowMapping(append([][2]string{{"channel", r.Channel}}, r.Provenance.kvs()...)), nil
}

// Normalize sorts inputs by channel and then by source, and drops exact
// duplicates: one channel may resolve the same source twice in a run (two
// workers pointing at one contract), and the manifest states facts, not counts.
func (i Inputs) Normalize() Inputs {
	if len(i) == 0 {
		return nil
	}

	seen := make(map[InputRecord]struct{}, len(i))
	res := make(Inputs, 0, len(i))

	for _, rec := range i {
		if _, dup := seen[rec]; dup {
			continue
		}

		seen[rec] = struct{}{}

		res = append(res, rec)
	}

	sort.Slice(res, func(a, b int) bool {
		if res[a].Channel != res[b].Channel {
			return res[a].Channel < res[b].Channel
		}

		return res[a].Describe() < res[b].Describe()
	})

	return res
}

// flowMapping builds a single-line YAML mapping node out of ordered key-value
// pairs.
func flowMapping(kvs [][2]string) *yaml.Node {
	node := &yaml.Node{Kind: yaml.MappingNode, Style: yaml.FlowStyle}

	for _, kv := range kvs {
		node.Content = append(node.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: kv[0]},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: kv[1]},
		)
	}

	return node
}
