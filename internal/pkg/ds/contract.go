package ds

// CustomGenerator is a project-declared code generator, with its commands
// already rendered against the contracts that reference it. The Makefile
// template prints the strings as recipe lines — it does not know, and does not
// need to know, what the generator is.
type CustomGenerator struct {
	// Name is the generator name from project.yaml. It is the suffix of the
	// generated make targets: install-<Name>, generate-<Name>, lint-<Name>.
	Name string
	// Install is the install command, verbatim. Empty when the tool fetches
	// itself — then no install-<Name> target is generated.
	Install string
	// Cmds are the rendered generate commands, one per contract referencing
	// this generator, in the order the contracts are declared.
	Cmds []string
	// Lints are the rendered lint commands, same ordering. Empty when the
	// generator declares no linter — then no lint-<Name> target is generated.
	Lints []string
}

// CustomGenerators keeps the declaration order of project.yaml so that the
// generated Makefile is stable between runs.
type CustomGenerators []CustomGenerator

// Contract is a spec the generator fetches on behalf of a custom generator.
type Contract struct {
	// RawPath is the source URI exactly as written in project.yaml. Kept
	// separate from SpecPath, which the resolve pre-pass rewrites to a staged
	// local file: the manifest records where the spec came from, not where it
	// was briefly staged.
	RawPath string
	// SpecPath is the local path to read the spec from. Equal to RawPath
	// joined with the config dir for local sources; rewritten by the resolve
	// pre-pass for remote ones.
	SpecPath string
	// Copy is the project-relative destination of the spec.
	Copy string
	// Target is the project-relative output path of the generator, or empty.
	Target string
	// GeneratorName is the CustomGenerator this contract feeds.
	GeneratorName string

	// Source is the fetched spec's source as project.yaml writes it,
	// token-redacted, filled by the resolve pre-pass. It is what log lines and
	// the drift message name; the provenance written to the manifest is
	// recorded by the resolver, the same way it is for every other channel.
	Source string
}

type Contracts []Contract
