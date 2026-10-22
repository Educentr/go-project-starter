// Package ockeymap parses and validates the OnlineConf key map — the
// declaration of every OnlineConf key a generated service is allowed to read.
//
// The map is a contract artifact: it lives in a contract repository and is
// referenced from project.yaml via `onlineconf.path` in the same URI form as
// `rest[].path` (see internal/pkg/specsource). Its shape is a flat list:
//
//	keys:
//	  - path: /myservice/supervisor/min_lifetime
//	    type: duration
//	    default: 5m
//	    description: >-
//	      How long a spawned agent is kept before it may be reaped.
//
// `path` carries the FULL OnlineConf path — the one that exists in the tree.
// Assembling it at runtime out of private segment constants is exactly the
// failure mode this map removes: a path built from `MakePath(serviceName,
// segConfluence, keyURL)` is a path no external tool (guard, registry, review)
// can see, so it diverges from the declaration silently.
//
// The declarations drive two artifacts at once: a typed accessor per key in the
// generated package, and a forbidigo rule banning every other call into
// onlineconf. Together they make "the service reads an undeclared key"
// impossible by construction rather than merely discouraged.
package ockeymap

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Key is one entry of the map.
type Key struct {
	// Path is the full OnlineConf path of the key, leading slash optional.
	// Required.
	Path string `yaml:"path"`
	// Type is one of the Type* constants. Required.
	Type string `yaml:"type"`
	// Name overrides the Go identifier of the generated accessor. Optional —
	// by default it is derived from the path segments below the service tree
	// root.
	Name string `yaml:"name"`
	// Required marks a key the service cannot run without. The generated
	// accessor then reports ErrKeyNotSet instead of a zero value, and
	// CheckRequired fails at startup when the key is absent from the tree.
	// Only the scalar types can be required — see validate.go.
	Required bool `yaml:"required"`
	// Description is the prose that becomes the accessor's godoc. Optional,
	// but it is the one place where the meaning of a key is visible for
	// review.
	Description string `yaml:"description"`

	// Default holds the declared fallback value as written in YAML. It is
	// decoded against Type at validation time, so a default that does not
	// parse (`default: 5 hours` for a duration) is a generation error rather
	// than a value the service silently never gets.
	Default yaml.Node `yaml:"default"`
}

// Map is a parsed OnlineConf key map.
type Map struct {
	Keys []Key `yaml:"keys"`
}

// Value type constants. Each maps onto one go-onlineconf reader, so a reader of
// the map knows exactly which accessor they get.
const (
	// TypeString reads a string value.
	TypeString = "string"
	// TypeInt reads an integer value (int64 — go-onlineconf's width).
	TypeInt = "int"
	// TypeBool reads a boolean value.
	TypeBool = "bool"
	// TypeDuration reads a string value and parses it as a time.Duration.
	TypeDuration = "duration"
	// TypeStrings reads a comma-separated string or a JSON array as []string.
	TypeStrings = "strings"
	// TypeStruct reads a JSON value into a caller-provided destination. This
	// is how a list is declared: one key holding a structured value and one
	// accessor, because OnlineConf has no subtree traversal.
	TypeStruct = "struct"
)

// Value kinds — the shape of the generated accessor behind a type.
const (
	// KindScalar covers the single-value types read through a *IfExists
	// reader, which reports presence.
	KindScalar = "scalar"
	// KindStrings covers TypeStrings, read through GetStrings, which reports
	// no presence.
	KindStrings = "strings"
	// KindStruct covers TypeStruct, read through GetStruct into a destination
	// owned by the caller.
	KindStruct = "struct"
)

// Accessor shapes — what the generated function returns, decided by the
// declaration rather than by the call site.
const (
	// ShapeRequired is `(T, error)` with ErrKeyNotSet when the key is absent.
	ShapeRequired = "required"
	// ShapeDefault is `(T, error)` returning the declared default when the key
	// is absent.
	ShapeDefault = "default"
	// ShapeOptional is `(T, bool, error)`: the presence flag is the honest
	// answer when the declaration names neither a default nor a requirement.
	ShapeOptional = "optional"
)

// keyFields is the closed set of keys a map entry may carry. Enforced
// explicitly because yaml.Decoder.KnownFields does not reach into a custom
// UnmarshalYAML, and a silently ignored key is a declaration that never takes
// effect — exactly the failure mode the map exists to remove.
//
// The process fields a key registry tends to grow (`consumer`, `seeded_by`,
// `declared_in`, `status`) are deliberately absent: the map is the generator's
// input, and who reads a key where is a question about consumers, not about the
// contract. Keeping the schema closed is what makes a typo in a field name an
// error instead of a declaration that quietly does nothing.
var keyFields = map[string]struct{}{
	"path": {}, "type": {}, "name": {},
	"required": {}, "default": {}, "description": {},
}

// UnmarshalYAML decodes a key entry. It is hand-rolled to reject unknown
// fields, which yaml.Decoder.KnownFields cannot do for a type that defines its
// own unmarshaler.
func (k *Key) UnmarshalYAML(value *yaml.Node) error {
	type plain struct {
		Path        string    `yaml:"path"`
		Type        string    `yaml:"type"`
		Name        string    `yaml:"name"`
		Required    bool      `yaml:"required"`
		Description string    `yaml:"description"`
		Default     yaml.Node `yaml:"default"`
	}

	if err := rejectUnknownFields(value, keyFields); err != nil {
		return err
	}

	var p plain
	if err := value.Decode(&p); err != nil {
		return err
	}

	k.Path = normalizePath(p.Path)
	k.Type = p.Type
	k.Name = p.Name
	k.Required = p.Required
	k.Description = p.Description
	k.Default = p.Default

	return nil
}

// Parse decodes an OnlineConf key map from YAML and validates it. The returned
// map is safe to hand to code generation: every entry has a usable path, type
// and accessor name, every declared default parses as its type, and no two
// entries share a path or a generated identifier.
func Parse(data []byte) (*Map, error) {
	var m Map

	if len(strings.TrimSpace(string(data))) == 0 {
		// Without this the yaml decoder reports a bare "EOF", which says
		// nothing about what to do.
		return nil, fmt.Errorf("%w: the file is empty", ErrInvalidMap)
	}

	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)

	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMalformedMap, err)
	}

	if err := m.Validate(); err != nil {
		return nil, err
	}

	return &m, nil
}

// Load reads and parses a service's OnlineConf key map.
func Load(path string) (*Map, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read onlineconf key map %s: %w", path, err)
	}

	m, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("onlineconf key map %s: %w", path, err)
	}

	return m, nil
}

// HasDefault reports whether the entry declares a fallback value.
func (k Key) HasDefault() bool {
	return k.Default.Kind != 0
}

// Segments returns the path segments, root first.
func (k Key) Segments() []string {
	return pathSegments(k.Path)
}

// TreeRoot returns the first path segment — the service tree the key lives in.
// The map is one per service, so every entry shares it (enforced by Validate).
func (k Key) TreeRoot() string {
	segments := k.Segments()
	if len(segments) == 0 {
		return ""
	}

	return segments[0]
}

// Kind returns the accessor shape family behind the declared type.
func (k Key) Kind() string {
	return KindOf(k.Type)
}

// Shape returns what the generated accessor returns. The declaration decides:
// a required key cannot be mistaken for absent, and an optional key without a
// default cannot be mistaken for present.
func (k Key) Shape() string {
	switch {
	case k.Required:
		return ShapeRequired
	case k.HasDefault():
		return ShapeDefault
	default:
		return ShapeOptional
	}
}

// TreeRoot returns the service tree root shared by every entry, or the empty
// string for an empty map.
func (m *Map) TreeRoot() string {
	if m == nil || len(m.Keys) == 0 {
		return ""
	}

	return m.Keys[0].TreeRoot()
}

// rejectUnknownFields fails on any mapping key outside allowed, naming the
// offending key and the line it sits on.
func rejectUnknownFields(value *yaml.Node, allowed map[string]struct{}) error {
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("%w: expected a mapping, got %s", ErrMalformedMap, nodeKindName(value.Kind))
	}

	for i := 0; i < len(value.Content)-1; i += 2 {
		key := value.Content[i].Value
		if _, ok := allowed[key]; !ok {
			return fmt.Errorf("%w: line %d: unknown field %q", ErrMalformedMap, value.Content[i].Line, key)
		}
	}

	return nil
}

// normalizePath gives the path exactly one leading slash and no trailing one,
// so that a declaration written `myservice/a/b` and one written
// `/myservice/a/b/` are the same key rather than two.
func normalizePath(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	return "/" + strings.Trim(trimmed, "/")
}

// pathSegments splits a normalized path into its segments.
func pathSegments(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}

	return strings.Split(trimmed, "/")
}

func nodeKindName(kind yaml.Kind) string {
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
		return "nothing"
	}
}
