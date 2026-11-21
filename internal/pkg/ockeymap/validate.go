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

	// keyProblemFmt prefixes a problem with the entry of the map it belongs to,
	// by index: the map is a list, and the path alone would not say which entry
	// to fix when two of them are spelled alike.
	keyProblemFmt = "keys[%d]: %s"

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
	// ErrOverlappingPath reports two declarations that reach one path, because
	// one of them carries a runtime segment standing for the other's literal.
	// Not the same error as ErrDuplicatePath — the paths are spelled
	// differently — but the same defect: one key, two accessors.
	ErrOverlappingPath = errors.New("onlineconf key paths reach the same key")
)

var (
	// segmentRx is the accepted grammar of one path segment.
	segmentRx = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)
	// identRx is the accepted grammar of an explicit `name` override: an
	// exported Go identifier, because it names a function in the generated
	// package.
	identRx = regexp.MustCompile(`^[A-Z][A-Za-z0-9]*$`)
)

// knownTypes is the closed set of accepted `type` values.
var knownTypes = map[string]struct{}{ //nolint:gochecknoglobals // fixed lookup table, written as a literal and read-only afterwards
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
			problems = append(problems, fmt.Sprintf(keyProblemFmt, i, msg))
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
	problems = append(problems, m.overlapProblems()...)

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

// overlapProblems reports two declarations that reach one and the same key.
//
// A runtime segment stands for any segment, so two paths spelled differently
// can be the same key: `/svc/account/<account>/limit` reaches
// `/svc/account/default/limit`, and declaring both gives one key two accessors
// with two shapes — the thing the map exists to prevent, written in a way the
// duplicate-path check cannot see.
func (m *Map) overlapProblems() []string {
	var problems []string

	for i, key := range m.Keys {
		if key.Path == "" || !key.HasParams() {
			// Two paths without a runtime segment overlap only by being equal,
			// which the duplicate check already reports.
			continue
		}

		for j, other := range m.Keys {
			if i == j || other.Path == "" || other.Path == key.Path {
				continue
			}

			// Reported once per pair, by the shape: it is the declaration that
			// reaches the other, and the one whose path is likely to change.
			if j < i && other.HasParams() {
				continue
			}

			if !pathsOverlap(key.Path, other.Path) {
				continue
			}

			problems = append(problems, fmt.Sprintf("keys[%d]: %s: %q also reaches %q (keys[%d]), "+
				"where a runtime segment stands for any segment — one key, two accessors. Drop the "+
				"second declaration and read that path through the first accessor, passing the "+
				"segment it names", i, ErrOverlappingPath, key.Path, other.Path, j))
		}
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

// pathProblems checks the declared path: grammar, depth, and the segments the
// service's own code computes while running.
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

	problems := k.segmentProblems(segments)

	return append(problems, k.paramProblems()...)
}

// segmentProblems checks the grammar of every path segment.
//
// Three forms are accepted and the difference is what each of them is: a
// literal segment is a segment of the tree; a <runtimeSegment> is a segment the
// service's own code computes while running, and becomes a parameter of the
// accessor; a {dimension} is neither — dimensions belong to the generator's half
// of the map, which is expanded over the project, and a service path is already
// the path of one service.
func (k Key) segmentProblems(segments []string) []string {
	var problems []string

	for i, segment := range segments {
		switch {
		case isPlaceholder(segment):
			problems = append(problems, fmt.Sprintf(
				"path %q uses the dimension placeholder %q: dimensions are the generator's, which "+
					"expands its declarations over the project. A service path is the path itself, "+
					"with <runtimeSegment> for a segment the service computes while running",
				k.Path, segment))
		case strings.HasPrefix(segment, "<"), strings.HasSuffix(segment, ">"):
			problems = append(problems, k.serviceRuntimeSegmentProblems(i, segment)...)
		case !segmentRx.MatchString(segment):
			problems = append(problems, fmt.Sprintf(
				"path %q has an invalid segment %q, expected %s or a <runtimeSegment>",
				k.Path, segment, segmentRx.String()))
		}
	}

	return problems
}

// runtimeSegmentProblems checks one segment the service computes while running:
// the grammar of the segment, and the name it gives the parameter of the
// generated accessor. Both halves of the map check it with this, because the
// accessor they generate is the same shape.
//
// The two are reported one at a time on purpose: the grammar decides whether
// there is a name to judge at all.
func runtimeSegmentProblems(path, segment string) []string {
	if !runtimeParamRx.MatchString(segment) {
		return []string{fmt.Sprintf(
			"path %q has an invalid runtime segment %q, expected %s: the name becomes a "+
				"parameter of the generated accessor", path, segment, runtimeParamRx.String())}
	}

	if problem := runtimeParamProblem(path, segment); problem != "" {
		return []string{problem}
	}

	return nil
}

// serviceRuntimeSegmentProblems adds the one position a service key's runtime
// segment cannot stand in: the tree root, which is the one segment of the map
// that is known before the service starts.
func (k Key) serviceRuntimeSegmentProblems(i int, segment string) []string {
	if i == 0 && runtimeParamRx.MatchString(segment) {
		return []string{fmt.Sprintf(
			"path %q starts with the runtime segment %q: the first segment is the service "+
				"tree the whole map lives in, and it is known before the service runs",
			k.Path, segment)}
	}

	return runtimeSegmentProblems(k.Path, segment)
}

// paramProblems checks the parameters the runtime segments of the path turn
// into: two of them cannot share a name, because the accessor could not be
// written.
func (k Key) paramProblems() []string {
	params := k.Params()
	if len(params) == 0 {
		return nil
	}

	var problems []string

	seen := make(map[string]struct{}, len(params))

	for _, param := range params {
		if _, taken := seen[param]; taken {
			problems = append(problems, fmt.Sprintf(
				"path %q uses the parameter name %q twice, so the accessor could not be written",
				k.Path, param))
		}

		seen[param] = struct{}{}
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
		if len(k.Segments()) < minPathSegments {
			// Already reported as a path with no key segment.
			return nil
		}

		// Every segment below the tree root is computed at runtime, so there is
		// nothing to derive a name from — the path has no fixed part but the
		// root, which the derivation drops.
		return []string{fmt.Sprintf(
			"path %q derives no accessor name: every segment below the service tree is a runtime "+
				"segment, and those become parameters rather than parts of the name; declare an "+
				"explicit `name` for it", k.Path)}
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

	if k.HasParams() {
		problems = append(problems, fmt.Sprintf(
			"path %q is `required` but has a runtime segment: CheckRequired reads every required key "+
				"at process start, and a path the service only composes once it runs is not a path "+
				"the check can enumerate. Read it with the accessor's presence flag instead", k.Path))
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
