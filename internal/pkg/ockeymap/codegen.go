package ockeymap

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// pathConstPrefix is prepended to an accessor name to build the exported path
// constant, so that a caller can hand a declared path to Subscribe without
// importing onlineconf itself.
const pathConstPrefix = "Path"

// defaultVarPrefix is prepended to an accessor name to build the unexported
// variable holding the declared default.
const defaultVarPrefix = "default"

// errNotStringListFmt wraps a `strings` default that decodes neither as a YAML
// sequence nor as a comma-separated scalar.
const errNotStringListFmt = "%w: default is not a list of strings: %w"

// ErrVarNameCollision reports two declared keys whose accessors would carry the
// same Go identifier (e.g. /svc/queue/max-size and /svc/queue/max_size).
var ErrVarNameCollision = errors.New("onlineconf accessor name collision")

// ErrBadDefault reports a declared default that does not parse as the declared
// type.
var ErrBadDefault = errors.New("invalid default value")

// commonInitialisms maps an upper-cased path word to the spelling Go uses for
// it. A path segment is prose from the tree, so a derived identifier would
// otherwise read `ConfluenceUrl` / `TgApiEndpoint` — which every Go linter
// flags and which no reviewer would have written by hand. The list is the one
// golint carries, so the result matches what the surrounding code already
// looks like.
//
// A derivation that still reads badly is what the `name` field is for.
var commonInitialisms = map[string]string{ //nolint:gochecknoglobals // fixed lookup table, written as a literal and read-only afterwards
	"ACL": "ACL", "API": "API", "ASCII": "ASCII", "CPU": "CPU", "CSS": "CSS",
	"DB": "DB", "DNS": "DNS", "DSN": "DSN", "EOF": "EOF", "GUID": "GUID",
	"HTML": "HTML", "HTTP": "HTTP", "HTTPS": "HTTPS", "ID": "ID", "IP": "IP",
	"JSON": "JSON", "LHS": "LHS", "QPS": "QPS", "RAM": "RAM", "RHS": "RHS",
	"RPC": "RPC", "SLA": "SLA", "SMTP": "SMTP", "SQL": "SQL", "SSH": "SSH",
	"TCP": "TCP", "TLS": "TLS", "TTL": "TTL", "UDP": "UDP", "UI": "UI",
	"UID": "UID", "UUID": "UUID", "URI": "URI", "URL": "URL", "UTF8": "UTF8",
	"VM": "VM", "XML": "XML", "XMPP": "XMPP", "XSRF": "XSRF", "XSS": "XSS",
	"CA": "CA", "CSRF": "CSRF", "GRPC": "GRPC", "OS": "OS", "TS": "TS",
}

// durationUnits are the time.Duration units a declared default is rendered
// with, largest first: the generated source should read like the YAML it came
// from ("5m" → 5 * time.Minute), not like a nanosecond count.
var durationUnits = []struct { //nolint:gochecknoglobals // fixed lookup table, written as a literal and read-only afterwards
	size time.Duration
	name string
}{
	{time.Hour, "time.Hour"},
	{time.Minute, "time.Minute"},
	{time.Second, "time.Second"},
	{time.Millisecond, "time.Millisecond"},
	{time.Microsecond, "time.Microsecond"},
}

// VarName is the exported Go identifier of the generated accessor.
//
// It is the explicit `name` when the declaration carries one; otherwise it is
// derived from the path segments below the service tree root:
// /myservice/supervisor/min_lifetime → SupervisorMinLifetime. The root is
// dropped because the map is one per service, so repeating it in every
// identifier would say nothing — and `occonf.SupervisorMinLifetime(ctx)`
// already names the package.
//
// Separators inside a segment ('_', '-', '.') fold the way '_' does. Two
// declarations that collide after folding are rejected by ValidateVarNames
// rather than silently producing one accessor.
func (k Key) VarName() string {
	if k.Name != "" {
		return k.Name
	}

	segments := k.Segments()
	if len(segments) < minPathSegments {
		return ""
	}

	var b strings.Builder

	for _, segment := range segments[1:] {
		b.WriteString(camelSegment(segment))
	}

	return b.String()
}

// PathConstName returns the identifier of the exported path constant.
func (k Key) PathConstName() string {
	return pathConstPrefix + k.VarName()
}

// DefaultVarName returns the identifier of the unexported default variable.
func (k Key) DefaultVarName() string {
	return defaultVarPrefix + k.VarName()
}

// GoType returns the Go type of the value the accessor yields. A struct key has
// no type of its own — the value shape belongs to the caller, who passes the
// destination.
func (k Key) GoType() string {
	return GoTypeOf(k.Type)
}

// ZeroLiteral returns the Go expression for the value an accessor returns
// alongside an error.
func (k Key) ZeroLiteral() string {
	return ZeroLiteralOf(k.Type)
}

// Getter returns the go-onlineconf reader the accessor is built on.
func (k Key) Getter() string {
	return GetterOf(k.Type)
}

// NeedsTimeImport reports whether the entry pulls "time" into the generated
// package.
func (k Key) NeedsTimeImport() bool {
	return k.Type == TypeDuration
}

// GoTypeOf returns the Go type a declared value type reads into. A struct has
// none of its own: the value shape belongs to the caller, who passes the
// destination.
func GoTypeOf(valueType string) string {
	switch valueType {
	case TypeString:
		return "string"
	case TypeInt:
		return "int64"
	case TypeBool:
		return "bool"
	case TypeDuration:
		return "time.Duration"
	case TypeStrings:
		return "[]string"
	default:
		return ""
	}
}

// ZeroLiteralOf returns the Go expression for the value an accessor of this type
// returns alongside an error.
func ZeroLiteralOf(valueType string) string {
	switch valueType {
	case TypeString:
		return `""`
	case TypeInt, TypeDuration:
		return "0"
	case TypeBool:
		return "false"
	case TypeStrings:
		return "nil"
	default:
		return ""
	}
}

// GetterOf returns the go-onlineconf reader an accessor of this type is built
// on.
//
// The scalar types use the *IfExists form on purpose: the defaulting form
// (onlineconf.GetString(ctx, path, d...)) indexes d[0] on a read error, so it
// panics when called without a default — and generated code must not depend on
// a caller remembering to pass one.
func GetterOf(valueType string) string {
	switch valueType {
	case TypeString:
		return "GetStringIfExists"
	case TypeInt:
		return "GetIntIfExists"
	case TypeBool:
		return "GetBoolIfExists"
	case TypeDuration:
		return "GetDurationIfExists"
	case TypeStrings:
		return "GetStrings"
	case TypeStruct:
		return "GetStruct"
	default:
		return ""
	}
}

// KindOf returns the accessor shape family behind a declared value type.
func KindOf(valueType string) string {
	switch valueType {
	case TypeString, TypeInt, TypeBool, TypeDuration:
		return KindScalar
	case TypeStrings:
		return KindStrings
	case TypeStruct:
		return KindStruct
	default:
		return ""
	}
}

// DefaultLiteral renders the declared default as a Go expression of the
// declared type. It returns the empty string when no default is declared.
//
// The value is decoded against the type here rather than copied through, so a
// default that cannot be the type it is declared as fails generation with the
// offending declaration named — instead of becoming a value the service never
// actually gets.
func (k Key) DefaultLiteral() (string, error) {
	if !k.HasDefault() {
		return "", nil
	}

	node := k.Default

	switch k.Type {
	case TypeString:
		var v string
		if err := node.Decode(&v); err != nil {
			return "", fmt.Errorf("%w: default is not a string: %w", ErrBadDefault, err)
		}

		return strconv.Quote(v), nil
	case TypeInt:
		var v int64
		if err := node.Decode(&v); err != nil {
			return "", fmt.Errorf("%w: default is not an integer: %w", ErrBadDefault, err)
		}

		return strconv.FormatInt(v, 10), nil
	case TypeBool:
		var v bool
		if err := node.Decode(&v); err != nil {
			return "", fmt.Errorf("%w: default is not a boolean: %w", ErrBadDefault, err)
		}

		return strconv.FormatBool(v), nil
	case TypeDuration:
		return durationLiteral(&node)
	case TypeStrings:
		return stringsLiteral(&node)
	default:
		// TypeStruct and unknown types are rejected by Validate before this is
		// reached.
		return "", fmt.Errorf("%w: type %q takes no default", ErrBadDefault, k.Type)
	}
}

// DefaultText returns the declared default as it was written in YAML, for the
// accessor's doc comment. The literal the accessor is built from comes from
// DefaultLiteral; this is the spelling a reader of the map would recognize.
func (k Key) DefaultText() string {
	if !k.HasDefault() {
		return ""
	}

	if k.Default.Kind == yaml.SequenceNode {
		items := make([]string, 0, len(k.Default.Content))
		for _, item := range k.Default.Content {
			items = append(items, item.Value)
		}

		return strings.Join(items, ", ")
	}

	return k.Default.Value
}

// ValidateVarNames checks that the declared keys produce distinct Go
// identifiers.
func ValidateVarNames(keys []Key) error {
	owner := make(map[string]string, len(keys))

	var problems []string

	for _, key := range keys {
		varName := key.VarName()
		if varName == "" {
			continue
		}

		if prev, taken := owner[varName]; taken {
			problems = append(problems, fmt.Sprintf("%q and %q both generate the accessor %s",
				prev, key.Path, varName))

			continue
		}

		owner[varName] = key.Path
	}

	if len(problems) > 0 {
		return problemList(ErrVarNameCollision, problems)
	}

	return nil
}

// camelSegment turns one path segment into a PascalCase identifier fragment:
// "min_lifetime" → "MinLifetime", "tls-ca.cert" → "TLSCACert".
func camelSegment(segment string) string {
	var b strings.Builder

	for _, word := range strings.FieldsFunc(segment, isWordSeparator) {
		if upper, ok := commonInitialisms[strings.ToUpper(word)]; ok {
			b.WriteString(upper)

			continue
		}

		b.WriteString(strings.ToUpper(word[:1]))
		b.WriteString(word[1:])
	}

	return b.String()
}

// isWordSeparator reports whether r separates words inside a path segment.
func isWordSeparator(r rune) bool {
	return r == '_' || r == '-' || r == '.'
}

// durationLiteral renders a duration default the way it was written: "5m"
// becomes `5 * time.Minute`, an awkward value falls back to the nanosecond
// count with the original spelling in a comment.
func durationLiteral(node *yaml.Node) (string, error) {
	var raw string
	if err := node.Decode(&raw); err != nil {
		return "", fmt.Errorf("%w: default is not a duration string: %w", ErrBadDefault, err)
	}

	d, err := time.ParseDuration(raw)
	if err != nil {
		return "", fmt.Errorf("%w: default %q is not a duration: %w", ErrBadDefault, raw, err)
	}

	if d == 0 {
		return "0", nil
	}

	for _, unit := range durationUnits {
		if d%unit.size == 0 {
			return fmt.Sprintf("%d * %s", int64(d/unit.size), unit.name), nil
		}
	}

	return fmt.Sprintf("time.Duration(%d) // %s", int64(d), raw), nil
}

// stringsLiteral renders a []string default. Both YAML forms are accepted — a
// sequence and a comma-separated scalar — because OnlineConf itself reads a
// comma-separated string into a list.
func stringsLiteral(node *yaml.Node) (string, error) {
	var items []string

	switch node.Kind {
	case yaml.SequenceNode:
		if err := node.Decode(&items); err != nil {
			return "", fmt.Errorf(errNotStringListFmt, ErrBadDefault, err)
		}
	case yaml.ScalarNode:
		var raw string
		if err := node.Decode(&raw); err != nil {
			return "", fmt.Errorf(errNotStringListFmt, ErrBadDefault, err)
		}

		items = splitList(raw)
	default:
		return "", fmt.Errorf("%w: default: expected a list or a comma-separated string, got %s",
			ErrBadDefault, nodeKindName(node.Kind))
	}

	if len(items) == 0 {
		return "[]string{}", nil
	}

	quoted := make([]string, 0, len(items))
	for _, item := range items {
		quoted = append(quoted, strconv.Quote(strings.TrimSpace(item)))
	}

	return "[]string{" + strings.Join(quoted, ", ") + "}", nil
}

// splitList turns "a, b" into []string{"a", "b"}, dropping empty entries so
// that a trailing comma does not become an empty element.
func splitList(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))

	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}

	return out
}

// KnownTypes returns the accepted `type` values, sorted, for error messages.
func KnownTypes() []string {
	out := make([]string, 0, len(knownTypes))

	for t := range knownTypes {
		out = append(out, t)
	}

	sort.Strings(out)

	return out
}

// --- code generation of the generator's own accessors ---

// GoType returns the Go type the accessor yields.
func (a GenAccessor) GoType() string {
	return GoTypeOf(a.Type)
}

// ZeroLiteral returns the Go expression for the value the accessor returns
// alongside an error.
func (a GenAccessor) ZeroLiteral() string {
	return ZeroLiteralOf(a.Type)
}

// Getter returns the go-onlineconf reader the accessor is built on.
func (a GenAccessor) Getter() string {
	return GetterOf(a.Type)
}

// Kind returns the accessor shape family behind the declared type.
func (a GenAccessor) Kind() string {
	return KindOf(a.Type)
}

// Shape returns what the accessor returns: always the value alongside its
// presence.
//
// One shape, unlike a service key's three, and `required` does not change it.
// A generator-owned key has no machine-readable default — the fallback is the
// generated reader's own, applied where that reader implements its fallback
// chain — and a reader of a key declared `required` still has to be able to see
// an absent one: a transport marked `optional: true` in project.yaml is skipped
// rather than reported. So `required` moves the question to process start, where
// the startup check knows which components this application actually runs, and
// leaves the read itself alone.
func (GenAccessor) Shape() string {
	return ShapeOptional
}

// NeedsTimeImport reports whether the accessor pulls "time" into the generated
// package.
func (a GenAccessor) NeedsTimeImport() bool {
	return a.Type == TypeDuration
}

// PathBuilderName returns the identifier of the function returning the
// accessor's path. Callers need it for a subscription: the path of a key whose
// dimensions are runtime arguments cannot be a constant.
func (a GenAccessor) PathBuilderName() string {
	if a.IsPathBuilder() {
		return a.Name
	}

	return a.Name + pathBuilderSuffix
}

// ParamDecl renders the accessor's parameters as a Go parameter list —
// "transport, app string" — or the empty string when it takes none.
func (a GenAccessor) ParamDecl() string {
	if len(a.Params) == 0 {
		return ""
	}

	return a.ParamNames() + " string"
}

// ParamNames renders the accessor's parameters as an argument list, for the call
// from the accessor to its path builder.
func (a GenAccessor) ParamNames() string {
	names := make([]string, 0, len(a.Params))
	for _, param := range a.Params {
		names = append(names, param.Name)
	}

	return strings.Join(names, ", ")
}

// PathArgs renders the arguments of the onlineconf.MakePath call that builds the
// accessor's path: the service tree, then one argument per segment — a quoted
// literal for a fixed segment, the parameter itself for a variable one.
//
// tree is the service tree root the generated code reads under. It is passed in
// rather than taken from the declaration because the declaration carries the
// {service} placeholder, and what that expands to is the project's name.
func (a GenAccessor) PathArgs(tree string) string {
	segments := pathSegments(a.Template)
	args := make([]string, 0, len(segments))

	for i, segment := range segments {
		switch {
		case i == 0 && segment == PlaceholderService:
			args = append(args, tree)
		case isPlaceholder(segment):
			args = append(args, strings.Trim(segment, "{}"))
		case isRuntimeParam(segment):
			args = append(args, strings.Trim(segment, "<>"))
		default:
			args = append(args, strconv.Quote(segment))
		}
	}

	return strings.Join(args, ", ")
}

// DocPath renders the declared path for the accessor's doc comment, with the
// service tree substituted and the variable segments left visible.
func (a GenAccessor) DocPath(tree string) string {
	segments := pathSegments(a.Template)
	out := make([]string, 0, len(segments))

	for i, segment := range segments {
		if i == 0 && segment == PlaceholderService {
			segment = tree
		}

		out = append(out, segment)
	}

	return "/" + strings.Join(out, "/")
}

// QuotedArgs renders a startup check's dimension values as Go string literals.
func (c GenRequiredCheck) QuotedArgs() string {
	quoted := make([]string, 0, len(c.Args))
	for _, arg := range c.Args {
		quoted = append(quoted, strconv.Quote(arg))
	}

	return strings.Join(quoted, ", ")
}

// ConstPath returns the accessor's whole path as a Go string literal, or the
// empty string when the path has a variable segment.
//
// An accessor without parameters reads one fixed path, and the generator knows
// it — so the path builder can return a constant instead of composing the same
// string on every read. The dev-stand flag is read once per request through the
// logger; rebuilding `/svc/devstand` there is work nobody asked for.
func (a GenAccessor) ConstPath(tree string) string {
	if len(a.Params) > 0 {
		return ""
	}

	return strconv.Quote(a.DocPath(tree))
}
