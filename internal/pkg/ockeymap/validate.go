package ockeymap

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// minPathSegments is the shortest usable path: the service tree root plus one
// key segment. A bare root is a subtree, and OnlineConf has no subtree read.
const minPathSegments = 2

const (
	// problemIndent prefixes every line of a problem list, so that a report of
	// several problems reads as one indented block under its sentinel.
	problemIndent = "\n  "

	// errWrapFmt wraps one error in another, both unwrappable.
	errWrapFmt = "%w: %w"

	// errNotExportedNameFmt reports a `name` override that cannot be the
	// identifier of a generated function. Shared by the two halves of the map,
	// which validate the same rule.
	errNotExportedNameFmt = "name %q is not an exported Go identifier; " +
		"it names a function in the generated package"
)

// Sentinel errors. Use errors.Is to distinguish.
var (
	// ErrInvalidMap reports an OnlineConf key map that cannot be used for
	// generation.
	ErrInvalidMap = errors.New("invalid onlineconf key map")
	// ErrDuplicatePath reports the same path declared twice inside one map.
	ErrDuplicatePath = errors.New("duplicate onlineconf key path")
	// ErrMalformedMap reports YAML that cannot be decoded into a key map at
	// all — a wrong node kind or an unknown field.
	ErrMalformedMap = errors.New("malformed onlineconf key map")
	// ErrReservedPath reports a declaration of a key the generated code reads
	// itself — either the same path, or a path inside a subtree generated code
	// reads under a runtime key. See CheckGeneratorKeys.
	ErrReservedPath = errors.New("onlineconf key is read by generated code")
	// ErrMultipleTrees reports entries sitting in different service trees. The
	// map is one per service: a key belongs to exactly one tree, and a shared
	// map would mix the surfaces of several services in one file.
	ErrMultipleTrees = errors.New("onlineconf key map spans several service trees")
)

var (
	// segmentRx is the accepted grammar of one path segment.
	segmentRx = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)
	// identRx is the accepted grammar of an explicit `name` override: an
	// exported Go identifier, because it names a function in the generated
	// package.
	identRx = regexp.MustCompile(`^[A-Z][A-Za-z0-9]*$`)
	// templateSegmentRx catches the list-template form (`.../<i>/...`) a key
	// registry tends to carry, so that it gets its own explanation rather than
	// a bare grammar complaint.
	templateSegmentRx = regexp.MustCompile(`[<>{}]`)
)

// knownTypes is the closed set of accepted `type` values.
var knownTypes = map[string]struct{}{
	TypeString:   {},
	TypeInt:      {},
	TypeBool:     {},
	TypeDuration: {},
	TypeStrings:  {},
	TypeStruct:   {},
}

// Validate checks the whole map: every entry on its own, uniqueness of paths
// and of generated accessor names, and that all entries belong to one service
// tree. All problems are reported at once so that a bad map takes one round
// trip to fix, not one per entry.
func (m *Map) Validate() error {
	if m == nil || len(m.Keys) == 0 {
		return fmt.Errorf("%w: no keys declared", ErrInvalidMap)
	}

	var problems []string

	seen := make(map[string]int, len(m.Keys))

	for i, key := range m.Keys {
		for _, msg := range key.problems() {
			problems = append(problems, fmt.Sprintf("keys[%d]: %s", i, msg))
		}

		if key.Path == "" {
			continue
		}

		if first, dup := seen[key.Path]; dup {
			problems = append(problems, fmt.Sprintf("keys[%d]: %s: %q (first declared at keys[%d])",
				i, ErrDuplicatePath, key.Path, first))

			continue
		}

		seen[key.Path] = i
	}

	problems = append(problems, m.treeProblems()...)

	if len(problems) > 0 {
		return problemList(ErrInvalidMap, problems)
	}

	// Name collisions are checked after the per-entry rules so that the
	// message names two valid declarations rather than echoing a path already
	// reported as malformed.
	return ValidateVarNames(m.Keys)
}

// treeProblems reports entries that do not share the service tree root of the
// first entry.
func (m *Map) treeProblems() []string {
	root := m.TreeRoot()
	if root == "" {
		return nil
	}

	problems := make([]string, 0, len(m.Keys))

	for i, key := range m.Keys {
		keyRoot := key.TreeRoot()
		if keyRoot == "" || keyRoot == root {
			continue
		}

		problems = append(problems, fmt.Sprintf("keys[%d]: %s: %q is in tree %q, keys[0] is in tree %q",
			i, ErrMultipleTrees, key.Path, keyRoot, root))
	}

	return problems
}

// problems returns every rule the entry breaks, as human-readable messages.
func (k Key) problems() []string {
	problems := k.pathProblems()

	if _, ok := knownTypes[k.Type]; !ok {
		problems = append(problems, fmt.Sprintf("type %q is unknown, expected one of %s",
			k.Type, strings.Join(KnownTypes(), ", ")))

		// Without a known type the remaining per-type rules are meaningless.
		return problems
	}

	problems = append(problems, k.namingProblems()...)
	problems = append(problems, k.defaultProblems()...)
	problems = append(problems, k.requiredProblems()...)

	return problems
}

// pathProblems checks the declared path: grammar, depth and the subtrees the
// generator owns.
func (k Key) pathProblems() []string {
	if k.Path == "" {
		return []string{"path is required"}
	}

	segments := k.Segments()

	if len(segments) < minPathSegments {
		return []string{fmt.Sprintf(
			"path %q has no key segment: a path must be /<service tree>/<key>, "+
				"because OnlineConf reads a value, not a subtree", k.Path)}
	}

	var problems []string

	for _, segment := range segments {
		switch {
		case templateSegmentRx.MatchString(segment):
			problems = append(problems, fmt.Sprintf(
				"path %q uses the template segment %q: a path with a list index is not a key. "+
					"OnlineConf has no subtree traversal, so a list is declared as one key with a "+
					"structured value (type: %s)", k.Path, segment, TypeStruct))
		case !segmentRx.MatchString(segment):
			problems = append(problems, fmt.Sprintf("path %q has an invalid segment %q, expected %s",
				k.Path, segment, segmentRx.String()))
		}
	}

	return problems
}

// namingProblems checks that the entry yields a usable accessor identifier.
func (k Key) namingProblems() []string {
	if k.Name != "" && !identRx.MatchString(k.Name) {
		return []string{fmt.Sprintf(errNotExportedNameFmt, k.Name)}
	}

	varName := k.VarName()
	if varName == "" {
		// Only reachable for a path already reported as too short.
		return nil
	}

	if !startsWithLetter(varName) {
		return []string{fmt.Sprintf(
			"path %q generates the accessor %s, which does not start with a letter; "+
				"declare an explicit `name` for it", k.Path, varName)}
	}

	return nil
}

// defaultProblems checks the declared default against the declared type.
func (k Key) defaultProblems() []string {
	if !k.HasDefault() {
		return nil
	}

	if k.Type == TypeStruct {
		return []string{fmt.Sprintf(
			"type %s takes no `default`: the value shape belongs to the caller, who passes a "+
				"destination to the accessor and can pre-fill it with a default", TypeStruct)}
	}

	if _, err := k.DefaultLiteral(); err != nil {
		return []string{err.Error()}
	}

	return nil
}

// requiredProblems checks that `required` can actually be enforced for the
// declared type.
//
// It can be enforced wherever the generated accessor can tell an absent key from
// a present one. For a scalar and for a list that is the presence flag of a
// *IfExists read; a struct is the exception, because verifying it would mean
// unmarshaling into a destination type the map does not carry, so a `required`
// there would be a promise the generated code could not keep.
func (k Key) requiredProblems() []string {
	if !k.Required {
		return nil
	}

	var problems []string

	if k.HasDefault() {
		problems = append(problems, "`required` and `default` contradict each other: "+
			"a key with a default is never missing")
	}

	if k.Kind() == KindStruct {
		problems = append(problems, fmt.Sprintf(
			"type %s cannot be `required`: verifying it would mean unmarshaling the value, and the "+
				"destination type belongs to the caller, not to the map. Read it with the accessor's "+
				"presence flag instead", k.Type))
	}

	return problems
}

// Required returns the entries declared `required`, in declaration order.
func (m *Map) Required() []Key {
	if m == nil {
		return nil
	}

	out := make([]Key, 0, len(m.Keys))

	for _, key := range m.Keys {
		if key.Required {
			out = append(out, key)
		}
	}

	return out
}

// startsWithLetter reports whether a generated identifier may open a
// declaration. Callers pass a non-empty VarName, which holds only ASCII letters
// and digits.
func startsWithLetter(varName string) bool {
	c := varName[0]

	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// problemList wraps sentinel with every collected problem on its own indented
// line. All problems are reported together on purpose: fixing a bad map should
// take one round trip, not one per entry.
func problemList(sentinel error, problems []string) error {
	return fmt.Errorf("%w:%s%s", sentinel, problemIndent, strings.Join(problems, problemIndent))
}
