package ockeymap_test

import (
	"strings"
	"testing"

	"github.com/Educentr/go-project-starter/internal/pkg/ockeymap"
)

// parseOne parses a one-entry map and returns the entry. It goes through Parse
// rather than building a Key literal so that the codegen helpers are only ever
// exercised on input validation has already accepted.
func parseOne(t *testing.T, entry string) ockeymap.Key {
	t.Helper()

	m, err := ockeymap.Parse([]byte("keys:\n" + entry))
	if err != nil {
		t.Fatalf("Parse(%q): %v", entry, err)
	}

	return m.Keys[0]
}

// The accessor name comes from the path segments below the tree root: the map
// is one per service, so repeating the root in every identifier would say
// nothing.
func TestVarName(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/svc/supervisor/min_lifetime", "SupervisorMinLifetime"},
		{"/svc/queue", "Queue"},
		{"/svc/a/b/c/d", "ABCD"},
		// Separators inside a segment fold the way '_' does.
		{"/svc/tls-ca.cert", "TLSCACert"},
		// Initialisms get Go's spelling: a derived ConfluenceUrl is an
		// identifier no reviewer would have written and every linter flags.
		{"/svc/confluence/url", "ConfluenceURL"},
		{"/svc/db/dsn", "DBDSN"},
		{"/svc/api/http_timeout", "APIHTTPTimeout"},
		{"/svc/user/id", "UserID"},
	}

	for _, tt := range tests {
		key := parseOne(t, "  - path: "+tt.path+"\n    type: string\n")

		if got := key.VarName(); got != tt.want {
			t.Errorf("VarName(%q) = %q, want %q", tt.path, got, tt.want)
		}

		if got := key.PathConstName(); got != "Path"+tt.want {
			t.Errorf("PathConstName(%q) = %q, want %q", tt.path, got, "Path"+tt.want)
		}
	}
}

func TestVarNameExplicitOverride(t *testing.T) {
	key := parseOne(t, "  - path: /svc/db/dsn\n    type: string\n    name: DatabaseDSN\n")

	if got := key.VarName(); got != "DatabaseDSN" {
		t.Errorf("VarName() = %q, want the declared override %q", got, "DatabaseDSN")
	}
}

func TestGetterAndGoType(t *testing.T) {
	tests := []struct {
		declType string
		getter   string
		goType   string
		zero     string
	}{
		// The *IfExists readers, deliberately: go-onlineconf's defaulting form
		// indexes its variadic default on a read error, so it panics when
		// called without one.
		{ockeymap.TypeString, "GetStringIfExists", "string", `""`},
		{ockeymap.TypeInt, "GetIntIfExists", "int64", "0"},
		{ockeymap.TypeBool, "GetBoolIfExists", "bool", "false"},
		{ockeymap.TypeDuration, "GetDurationIfExists", "time.Duration", "0"},
		{ockeymap.TypeStrings, "GetStrings", "[]string", "nil"},
		{ockeymap.TypeStruct, "GetStruct", "", ""},
	}

	for _, tt := range tests {
		key := parseOne(t, "  - path: /svc/a\n    type: "+tt.declType+"\n")

		if got := key.Getter(); got != tt.getter {
			t.Errorf("%s: Getter() = %q, want %q", tt.declType, got, tt.getter)
		}

		if got := key.GoType(); got != tt.goType {
			t.Errorf("%s: GoType() = %q, want %q", tt.declType, got, tt.goType)
		}

		if got := key.ZeroLiteral(); got != tt.zero {
			t.Errorf("%s: ZeroLiteral() = %q, want %q", tt.declType, got, tt.zero)
		}
	}
}

func TestNeedsTimeImport(t *testing.T) {
	if !parseOne(t, "  - path: /svc/a\n    type: duration\n").NeedsTimeImport() {
		t.Error("a duration key needs the time import")
	}

	if parseOne(t, "  - path: /svc/a\n    type: int\n").NeedsTimeImport() {
		t.Error("an int key does not need the time import")
	}
}

// The generated default should read like the YAML it came from: a nanosecond
// count in the source would be unreviewable.
func TestDefaultLiteral(t *testing.T) {
	tests := []struct {
		name  string
		entry string
		want  string
	}{
		{"string", "  - path: /svc/a\n    type: string\n    default: hello\n", `"hello"`},
		{"string with quotes", "  - path: /svc/a\n    type: string\n    default: 'say \"hi\"'\n", `"say \"hi\""`},
		{"int", "  - path: /svc/a\n    type: int\n    default: 42\n", "42"},
		{"negative int", "  - path: /svc/a\n    type: int\n    default: -1\n", "-1"},
		{"bool", "  - path: /svc/a\n    type: bool\n    default: true\n", "true"},
		{"duration hours", "  - path: /svc/a\n    type: duration\n    default: 2h\n", "2 * time.Hour"},
		{"duration minutes", "  - path: /svc/a\n    type: duration\n    default: 5m\n", "5 * time.Minute"},
		{"duration seconds", "  - path: /svc/a\n    type: duration\n    default: 90s\n", "90 * time.Second"},
		{"duration millis", "  - path: /svc/a\n    type: duration\n    default: 1500ms\n", "1500 * time.Millisecond"},
		{"duration zero", "  - path: /svc/a\n    type: duration\n    default: 0s\n", "0"},
		{
			"strings comma separated",
			"  - path: /svc/a\n    type: strings\n    default: x, y\n",
			`[]string{"x", "y"}`,
		},
		{
			"strings yaml list",
			"  - path: /svc/a\n    type: strings\n    default:\n      - x\n      - y\n",
			`[]string{"x", "y"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := parseOne(t, tt.entry)

			got, err := key.DefaultLiteral()
			if err != nil {
				t.Fatalf("DefaultLiteral(): %v", err)
			}

			if got != tt.want {
				t.Errorf("DefaultLiteral() = %q, want %q", got, tt.want)
			}
		})
	}
}

// A duration that divides no round unit still has to come out as valid Go.
func TestDefaultLiteralAwkwardDuration(t *testing.T) {
	key := parseOne(t, "  - path: /svc/a\n    type: duration\n    default: 1500ns\n")

	got, err := key.DefaultLiteral()
	if err != nil {
		t.Fatalf("DefaultLiteral(): %v", err)
	}

	if !strings.HasPrefix(got, "time.Duration(1500)") {
		t.Errorf("DefaultLiteral() = %q, want a time.Duration(...) fallback", got)
	}
}

func TestDefaultLiteralAbsent(t *testing.T) {
	key := parseOne(t, "  - path: /svc/a\n    type: string\n")

	got, err := key.DefaultLiteral()
	if err != nil {
		t.Fatalf("DefaultLiteral(): %v", err)
	}

	if got != "" {
		t.Errorf("DefaultLiteral() = %q, want empty for a key without a default", got)
	}
}

// DefaultText is what the doc comment shows, so it should be the spelling a
// reader of the map would recognise.
func TestDefaultText(t *testing.T) {
	if got := parseOne(t, "  - path: /svc/a\n    type: duration\n    default: 5m\n").DefaultText(); got != "5m" {
		t.Errorf("DefaultText() = %q, want %q", got, "5m")
	}

	entry := "  - path: /svc/a\n    type: strings\n    default:\n      - x\n      - y\n"
	if got := parseOne(t, entry).DefaultText(); got != "x, y" {
		t.Errorf("DefaultText() = %q, want %q", got, "x, y")
	}

	if got := parseOne(t, "  - path: /svc/a\n    type: string\n").DefaultText(); got != "" {
		t.Errorf("DefaultText() = %q, want empty for a key without a default", got)
	}
}

func TestKnownTypesIsSorted(t *testing.T) {
	types := ockeymap.KnownTypes()
	if len(types) == 0 {
		t.Fatal("KnownTypes() is empty")
	}

	for i := 1; i < len(types); i++ {
		if types[i-1] > types[i] {
			t.Fatalf("KnownTypes() is not sorted: %v", types)
		}
	}
}
