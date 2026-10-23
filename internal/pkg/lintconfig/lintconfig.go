// Package lintconfig assembles configs/golangci-lint.yml out of two inputs:
// the generator's own template output and the override layer the project
// declares in tools.lint_config.overrides.
//
// The merge happens during generation, not on top of an already written file:
// the generator renders its template, folds the project's overrides into the
// rendered document and writes the result. The written file therefore stays
// generator-owned end to end (hand edits above the disclaimer are still
// overwritten); the override file is project-owned and the generator never
// writes to it.
//
// # Merge rules
//
// The whole merge is two YAML documents folded key by key:
//
//  1. Mappings merge recursively — the project's keys win, the generator's
//     untouched keys survive.
//  2. Scalars and sequences replace. Declaring linters.settings.misspell
//     replaces the generator's misspell settings wholesale.
//  3. An explicit null deletes the key, so a project can drop a settings
//     block instead of restating it.
//  4. One exception to rule 2, for the sequences that name linters rather
//     than carry values — linters.enable/disable and
//     formatters.enable/formatters.disable are set operations on the
//     generator's enable list: enable adds, disable removes. Replacing the
//     list would force every project to restate all ~50 generator linters
//     just to drop one, and the restated copy would then never pick up a
//     linter the generator adds later.
//
// Rule 4 resolves disable at generation time: a linter the generator enables
// explicitly is dropped from enable rather than listed under disable, because
// golangci-lint rejects a config that both enables and disables the same
// linter. A disabled name the generator does not enable explicitly is a
// golangci-lint default (errcheck, govet, ineffassign, staticcheck, unused
// come from `linters.default: standard`); such a name is passed through as a
// real disable entry, which is the only way to switch a default off.
package lintconfig

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

// ConfigPath is the project-relative path of the generated golangci-lint
// config. It is the path the Makefile lint targets pass to --config, so a
// project that owns the file (tools.lint_config.own) owns it at exactly this
// path.
const ConfigPath = "configs/golangci-lint.yml"

// nullTag is the resolved tag of an explicit YAML null — `key:`, `key: ~` and
// `key: null` all carry it. Rule 3 keys off the tag rather than off the
// spelling.
const nullTag = "!!null"

// yamlIndent matches the indentation of the generator's own template, so the
// merged document does not reindent the parts no override touched.
const yamlIndent = 2

// The two keys rule 4 treats as sets of linter names rather than as values.
const (
	keyEnable  = "enable"
	keyDisable = "disable"
)

// Shape errors of an override or generated document. Static so that callers
// can tell a malformed document from a read failure.
var (
	// ErrMultiDocument is returned for a stream of several YAML documents.
	ErrMultiDocument = errors.New("expected a single YAML document")
	// ErrNotMapping is returned when a document's root is not a mapping.
	ErrNotMapping = errors.New("expected a mapping at the top level")
	// ErrNotNameList is returned when an enable/disable key does not hold a
	// flat list of linter names.
	ErrNotNameList = errors.New("must be a list of names")
)

// linterSections are the two mappings whose enable/disable sequences name
// linters instead of carrying values (rule 4).
var linterSections = []string{"linters", "formatters"}

// Validate reports whether an override document is usable as the right-hand
// side of Merge. It is meant for the config-load stage, where failing early
// beats failing halfway through generation: the file is parsed and its shape
// checked, but nothing is merged.
//
// An empty file is valid and means "no overrides".
func Validate(overrides []byte) error {
	_, err := parseOverrides(overrides)

	return err
}

// Merge folds overrides into base and returns the merged YAML document. base
// is the generator's rendered template body; overrides is the project's
// override document. Empty overrides return base unchanged, byte for byte —
// the no-override path must not reformat anything.
//
// See the package comment for the merge rules.
func Merge(base, overrides []byte) ([]byte, error) {
	overRoot, err := parseOverrides(overrides)
	if err != nil {
		return nil, err
	}

	// Nothing to fold in: hand back the generator's own output untouched, so
	// the no-override path cannot reformat anything.
	if len(overRoot.Content) == 0 {
		return base, nil
	}

	var baseDoc yaml.Node
	if err := yaml.Unmarshal(base, &baseDoc); err != nil {
		return nil, fmt.Errorf("parse generated lint config: %w", err)
	}

	baseRoot, err := mappingRoot(&baseDoc)
	if err != nil {
		return nil, fmt.Errorf("generated lint config: %w", err)
	}

	// Set operations run first: they consume the enable/disable keys of the
	// override so the generic merge below never sees them (it would replace
	// the generator's list instead of folding into it).
	for _, section := range linterSections {
		if err := applyLinterSets(baseRoot, overRoot, section); err != nil {
			return nil, err
		}
	}

	mergeMapping(baseRoot, overRoot)

	var buf bytes.Buffer

	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(yamlIndent)

	if err := enc.Encode(&baseDoc); err != nil {
		_ = enc.Close()

		return nil, fmt.Errorf("marshal merged lint config: %w", err)
	}

	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("close yaml encoder: %w", err)
	}

	return buf.Bytes(), nil
}

// parseOverrides decodes an override document and checks that its root is a
// mapping. A document that is empty or holds nothing but comments yields an
// empty mapping, which every caller reads as "no overrides" — the same as an
// explicit `{}`.
func parseOverrides(overrides []byte) (*yaml.Node, error) {
	empty := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}

	if len(bytes.TrimSpace(overrides)) == 0 {
		return empty, nil
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(overrides, &doc); err != nil {
		return nil, fmt.Errorf("parse lint overrides: %w", err)
	}

	if doc.Kind == 0 || len(doc.Content) == 0 {
		return empty, nil
	}

	root, err := mappingRoot(&doc)
	if err != nil {
		return nil, fmt.Errorf("lint overrides: %w", err)
	}

	return root, nil
}

// mappingRoot unwraps a document node down to its root mapping.
func mappingRoot(doc *yaml.Node) (*yaml.Node, error) {
	node := doc
	if node.Kind == yaml.DocumentNode {
		if len(node.Content) != 1 {
			return nil, fmt.Errorf("%w, got %d", ErrMultiDocument, len(node.Content))
		}

		node = node.Content[0]
	}

	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%w, got %s", ErrNotMapping, kindName(node.Kind))
	}

	return node, nil
}

// mergeMapping folds over into base per rules 1-3.
func mergeMapping(base, over *yaml.Node) {
	for i := 0; i+1 < len(over.Content); i += 2 {
		keyNode := over.Content[i]
		valNode := over.Content[i+1]

		if valNode.Tag == nullTag {
			removeKey(base, keyNode.Value)

			continue
		}

		baseVal := findValue(base, keyNode.Value)
		if baseVal == nil {
			base.Content = append(base.Content, keyNode, valNode)

			continue
		}

		if baseVal.Kind == yaml.MappingNode && valNode.Kind == yaml.MappingNode {
			mergeMapping(baseVal, valNode)

			continue
		}

		setKey(base, keyNode.Value, valNode)
	}
}

// applyLinterSets implements rule 4 for one section (linters or formatters):
// the override's enable list is added to the section's enable list and its
// disable list is subtracted from it. Both keys are then dropped from the
// override so mergeMapping leaves the section's enable list alone.
//
// Names the generator does not enable explicitly cannot be switched off by
// subtraction — they come from golangci-lint's own default set — so they are
// emitted as a real disable list instead.
func applyLinterSets(base, over *yaml.Node, section string) error {
	overSection := findValue(over, section)
	if overSection == nil || overSection.Kind != yaml.MappingNode {
		return nil
	}

	addNode := findValue(overSection, keyEnable)
	delNode := findValue(overSection, keyDisable)

	if addNode == nil && delNode == nil {
		return nil
	}

	add, err := names(addNode, section+"."+keyEnable)
	if err != nil {
		return err
	}

	del, err := names(delNode, section+"."+keyDisable)
	if err != nil {
		return err
	}

	removeKey(overSection, keyEnable)
	removeKey(overSection, keyDisable)

	baseSection := findValue(base, section)
	if baseSection == nil || baseSection.Kind != yaml.MappingNode {
		baseSection = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		setKey(base, section, baseSection)
	}

	kept, passthrough := resolveEnableList(
		findValue(baseSection, keyEnable), findValue(baseSection, keyDisable), add, del,
	)

	setKey(baseSection, keyEnable, sequence(kept))

	if len(passthrough) > 0 {
		setKeyAfter(baseSection, keyDisable, sequence(passthrough), keyEnable)
	} else {
		removeKey(baseSection, keyDisable)
	}

	return nil
}

// resolveEnableList applies the add/del sets to the generator's enable and
// disable sequences. It returns the resulting enable entries and the names
// whose disable has to reach golangci-lint verbatim: those the generator never
// enabled explicitly, so they are its own defaults and subtraction would be a
// no-op.
//
// disableNode is the generator's own disable list. It is empty in today's
// template, but a template that grows one must not have it silently dropped —
// so its entries are carried over unless the project enables them back.
//
// Both results are sorted: the output has to be deterministic across runs and
// to keep the generator's alphabetical order readable in a diff.
func resolveEnableList(enableNode, disableNode *yaml.Node, add, del []string) (kept, passthrough []*yaml.Node) {
	delSet := toSet(del)
	addSet := toSet(add)

	enabled := make(map[string]struct{})
	kept = []*yaml.Node{}

	for _, node := range sequenceItems(enableNode) {
		enabled[node.Value] = struct{}{}

		if _, dropped := delSet[node.Value]; dropped {
			continue
		}

		kept = append(kept, node)
	}

	keptSet := make(map[string]struct{}, len(kept))
	for _, node := range kept {
		keptSet[node.Value] = struct{}{}
	}

	for _, name := range add {
		if _, ex := keptSet[name]; ex {
			continue
		}

		keptSet[name] = struct{}{}

		kept = append(kept, scalar(name))
	}

	passthrough = []*yaml.Node{}
	seen := make(map[string]struct{})

	// The generator's own disable entries, minus whatever the project enables
	// back — an explicit enable from the project outranks them (rule 4).
	for _, node := range sequenceItems(disableNode) {
		if _, enabledBack := addSet[node.Value]; enabledBack {
			continue
		}

		seen[node.Value] = struct{}{}

		passthrough = append(passthrough, node)
	}

	for _, name := range del {
		if _, explicit := enabled[name]; explicit {
			continue
		}

		if _, dup := seen[name]; dup {
			continue
		}

		seen[name] = struct{}{}

		passthrough = append(passthrough, scalar(name))
	}

	sortNodes(kept)
	sortNodes(passthrough)

	return kept, passthrough
}

// sequenceItems returns the items of a sequence node, or nothing for a node
// that is absent or not a sequence.
func sequenceItems(node *yaml.Node) []*yaml.Node {
	if node == nil || node.Kind != yaml.SequenceNode {
		return nil
	}

	return node.Content
}

func toSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, v := range values {
		set[v] = struct{}{}
	}

	return set
}

// names reads a sequence of scalars into a string slice. A nil node is an
// absent key and yields nothing; anything that is not a sequence of scalars is
// a config error rather than something to guess at.
func names(node *yaml.Node, path string) ([]string, error) {
	if node == nil {
		return nil, nil
	}

	if node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("lint overrides: %s %w, got %s", path, ErrNotNameList, kindName(node.Kind))
	}

	out := make([]string, 0, len(node.Content))

	for _, item := range node.Content {
		if item.Kind != yaml.ScalarNode {
			return nil, fmt.Errorf("lint overrides: %s %w, got a nested %s", path, ErrNotNameList, kindName(item.Kind))
		}

		out = append(out, item.Value)
	}

	return out, nil
}

// findValue returns the value node of key in a mapping, or nil when absent.
func findValue(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}

	return nil
}

// setKey replaces the value of key in place, keeping its position, or appends
// the pair when the key is absent.
func setKey(mapping *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content[i+1] = value

			return
		}
	}

	mapping.Content = append(mapping.Content, scalar(key), value)
}

// setKeyAfter behaves like setKey but, when the key has to be added, puts it
// right behind afterKey instead of at the end of the mapping — a generated
// disable list reads as part of the enable list it complements, not as a
// footnote after the exclusion rules.
func setKeyAfter(mapping *yaml.Node, key string, value *yaml.Node, afterKey string) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content[i+1] = value

			return
		}
	}

	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value != afterKey {
			continue
		}

		tail := append([]*yaml.Node{scalar(key), value}, mapping.Content[i+2:]...)
		mapping.Content = append(mapping.Content[:i+2], tail...)

		return
	}

	mapping.Content = append(mapping.Content, scalar(key), value)
}

// removeKey drops a key/value pair from a mapping; absent keys are a no-op.
func removeKey(mapping *yaml.Node, key string) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)

			return
		}
	}
}

func scalar(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

func sequence(items []*yaml.Node) *yaml.Node {
	return &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Content: items}
}

func sortNodes(nodes []*yaml.Node) {
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Value < nodes[j].Value })
}

func kindName(kind yaml.Kind) string {
	switch kind {
	case yaml.DocumentNode:
		return "document"
	case yaml.SequenceNode:
		return "list"
	case yaml.MappingNode:
		return "mapping"
	case yaml.ScalarNode:
		return "scalar"
	case yaml.AliasNode:
		return "alias"
	default:
		return "empty document"
	}
}
