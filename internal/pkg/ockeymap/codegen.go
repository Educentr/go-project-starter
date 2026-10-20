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

// ErrVarNameCollision reports two declared keys whose accessors would carry the
// same Go identifier (e.g. /svc/queue/max-size and /svc/queue/max_size).
var ErrVarNameCollision = errors.New("onlineconf accessor name collision")

// ErrBadDefault reports a declared default that does not parse as the declared
// type.
var ErrBadDefault = errors.New("invalid default value")

// pathConstPrefix is prepended to an accessor name to build the exported path
// constant, so that a caller can hand a declared path to Subscribe without
// importing onlineconf itself.
const pathConstPrefix = "Path"

// defaultVarPrefix is prepended to an accessor name to build the unexported
// variable holding the declared default.
const defaultVarPrefix = "default"

// commonInitialisms maps an upper-cased path word to the spelling Go uses for
// it. A path segment is prose from the tree, so a derived identifier would
// otherwise read `ConfluenceUrl` / `TgApiEndpoint` — which every Go linter
// flags and which no reviewer would have written by hand. The list is the one
// golint carries, so the result matches what the surrounding code already
// looks like.
//
// A derivation that still reads badly is what the `name` field is for.
var commonInitialisms = map[string]string{
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
var durationUnits = []struct {
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
	switch k.Type {
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

// ZeroLiteral returns the Go expression for the value an accessor returns
// alongside an error.
func (k Key) ZeroLiteral() string {
	switch k.Type {
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

// Getter returns the go-onlineconf reader the accessor is built on. The scalar
// types use the *IfExists form on purpose: the defaulting form
// (onlineconf.GetString(ctx, path, d...)) indexes d[0] on a read error, so it
// panics when called without a default — and generated code must not depend on
// a caller remembering to pass one.
func (k Key) Getter() string {
	switch k.Type {
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

// NeedsTimeImport reports whether the entry pulls "time" into the generated
// package.
func (k Key) NeedsTimeImport() bool {
	return k.Type == TypeDuration
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
// DefaultLiteral; this is the spelling a reader of the map would recognise.
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
			return "", fmt.Errorf("%w: default is not a list of strings: %w", ErrBadDefault, err)
		}
	case yaml.ScalarNode:
		var raw string
		if err := node.Decode(&raw); err != nil {
			return "", fmt.Errorf("%w: default is not a list of strings: %w", ErrBadDefault, err)
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
