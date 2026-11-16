package ockeymap_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Educentr/go-project-starter/internal/pkg/ockeymap"
)

// validMap is the shape a contract repository is expected to carry: every
// supported type, every accessor shape.
const validMap = `
keys:
  - path: /svc/supervisor/min_lifetime
    type: duration
    default: 5m
    description: How long an agent is kept before it may be reaped.

  - path: /svc/supervisor/max_agents
    type: int
    required: true

  - path: /svc/feature/enabled
    type: bool
    default: false

  - path: /svc/notify/recipients
    type: strings
    default: a@example.com, b@example.com

  - path: /svc/queue/name
    type: string

  - path: /svc/agent_env
    type: struct
`

func TestParseValidMap(t *testing.T) {
	m, err := ockeymap.Parse([]byte(validMap))
	if err != nil {
		t.Fatalf("Parse() on a valid map: %v", err)
	}

	if len(m.Keys) != 6 {
		t.Fatalf("expected 6 declared keys, got %d", len(m.Keys))
	}

	if root := m.TreeRoot(); root != "svc" {
		t.Errorf("TreeRoot() = %q, want %q", root, "svc")
	}

	if required := m.Required(); len(required) != 1 || required[0].Path != "/svc/supervisor/max_agents" {
		t.Errorf("Required() = %v, want only /svc/supervisor/max_agents", required)
	}
}

// The three accessor shapes are decided by the declaration, not by the call
// site: that is what keeps a required key from being mistaken for absent and an
// optional key from being mistaken for present.
func TestKeyShapeAndKind(t *testing.T) {
	m, err := ockeymap.Parse([]byte(validMap))
	if err != nil {
		t.Fatalf("Parse(): %v", err)
	}

	want := map[string]struct{ shape, kind string }{
		"/svc/supervisor/min_lifetime": {ockeymap.ShapeDefault, ockeymap.KindScalar},
		"/svc/supervisor/max_agents":   {ockeymap.ShapeRequired, ockeymap.KindScalar},
		"/svc/feature/enabled":         {ockeymap.ShapeDefault, ockeymap.KindScalar},
		"/svc/notify/recipients":       {ockeymap.ShapeDefault, ockeymap.KindStrings},
		"/svc/queue/name":              {ockeymap.ShapeOptional, ockeymap.KindScalar},
		"/svc/agent_env":               {ockeymap.ShapeOptional, ockeymap.KindStruct},
	}

	for _, key := range m.Keys {
		expected, ok := want[key.Path]
		if !ok {
			t.Errorf("unexpected key %q", key.Path)

			continue
		}

		if key.Shape() != expected.shape {
			t.Errorf("%s: Shape() = %q, want %q", key.Path, key.Shape(), expected.shape)
		}

		if key.Kind() != expected.kind {
			t.Errorf("%s: Kind() = %q, want %q", key.Path, key.Kind(), expected.kind)
		}
	}
}

// A path written without the leading slash, or with a trailing one, is the same
// key — otherwise a map could declare one key twice and the duplicate check
// would not see it.
func TestPathNormalization(t *testing.T) {
	m, err := ockeymap.Parse([]byte("keys:\n  - path: svc/queue/name/\n    type: string\n"))
	if err != nil {
		t.Fatalf("Parse(): %v", err)
	}

	if got := m.Keys[0].Path; got != "/svc/queue/name" {
		t.Errorf("normalized path = %q, want %q", got, "/svc/queue/name")
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		sentinel error
		wantMsg  string
	}{
		{
			name:     "empty file",
			yaml:     "   \n",
			sentinel: ockeymap.ErrInvalidMap,
			wantMsg:  "the file is empty",
		},
		{
			name:     "no keys",
			yaml:     "keys: []\n",
			sentinel: ockeymap.ErrInvalidMap,
			wantMsg:  "no keys declared",
		},
		{
			// A silently ignored field is a declaration that never takes
			// effect — the very failure the map exists to remove. The process
			// fields a key registry grows (consumer, seeded_by, declared_in)
			// land here too: the schema is closed on purpose.
			name:     "unknown field",
			yaml:     "keys:\n  - path: /svc/a\n    type: string\n    consumer: runtime\n",
			sentinel: ockeymap.ErrMalformedMap,
			wantMsg:  `unknown field "consumer"`,
		},
		{
			// `service_tree` is one of the process fields a key registry
			// carries. The map is one per service, so the tree comes from the
			// paths — and the schema is closed at the top level too.
			name:     "unknown top-level field",
			yaml:     "keys:\n  - path: /svc/a\n    type: string\nservice_tree: svc\n",
			sentinel: ockeymap.ErrMalformedMap,
			wantMsg:  "field service_tree not found",
		},
		{
			name:     "typo in field name",
			yaml:     "keys:\n  - path: /svc/a\n    type: string\n    requires: true\n",
			sentinel: ockeymap.ErrMalformedMap,
			wantMsg:  `unknown field "requires"`,
		},
		{
			name:     "missing path",
			yaml:     "keys:\n  - type: string\n",
			sentinel: ockeymap.ErrInvalidMap,
			wantMsg:  "path is required",
		},
		{
			name:     "unknown type",
			yaml:     "keys:\n  - path: /svc/a\n    type: float\n",
			sentinel: ockeymap.ErrInvalidMap,
			wantMsg:  `type "float" is unknown`,
		},
		{
			// OnlineConf reads a value, not a subtree: a bare tree root names
			// no key.
			name:     "no key segment",
			yaml:     "keys:\n  - path: /svc\n    type: string\n",
			sentinel: ockeymap.ErrInvalidMap,
			wantMsg:  "has no key segment",
		},
		{
			// A dimension placeholder belongs to the generator's half of the
			// map, which is expanded over the project. A service path is the
			// path of one service already, so there is nothing to expand — and
			// the message has to say what to write instead of complaining about
			// grammar.
			name:     "dimension placeholder",
			yaml:     "keys:\n  - path: /svc/transport/rest/{transport}/timeout\n    type: duration\n",
			sentinel: ockeymap.ErrInvalidMap,
			wantMsg:  "dimensions are the generator's",
		},
		{
			// The name of a runtime segment becomes a Go parameter, so it has
			// to be able to be one.
			name:     "malformed runtime segment",
			yaml:     "keys:\n  - path: /svc/account/<Account id>/quota\n    type: int\n",
			sentinel: ockeymap.ErrInvalidMap,
			wantMsg:  "invalid runtime segment",
		},
		{
			// The tree root is the one segment that cannot be computed: the
			// whole map lives in one tree, and which tree it is is known before
			// the service starts.
			name:     "runtime segment as the tree root",
			yaml:     "keys:\n  - path: /<service>/quota\n    type: int\n",
			sentinel: ockeymap.ErrInvalidMap,
			wantMsg:  "starts with the runtime segment",
		},
		{
			name:     "two runtime segments with one name",
			yaml:     "keys:\n  - path: /svc/a/<id>/b/<id>/quota\n    type: int\n",
			sentinel: ockeymap.ErrInvalidMap,
			wantMsg:  `parameter name "id" twice`,
		},
		{
			// Every segment below the root is a parameter, so the derivation has
			// nothing left to build a name from.
			name:     "nothing to derive a name from",
			yaml:     "keys:\n  - path: /svc/<account>\n    type: int\n",
			sentinel: ockeymap.ErrInvalidMap,
			wantMsg:  "derives no accessor name",
		},
		{
			// CheckRequired reads every required key at process start, and this
			// path does not exist until the service composes it.
			name:     "required with a runtime segment",
			yaml:     "keys:\n  - path: /svc/account/<account>/quota\n    type: int\n    required: true\n",
			sentinel: ockeymap.ErrInvalidMap,
			wantMsg:  "`required` but has a runtime segment",
		},
		{
			// Spelled differently, the same key: the runtime segment stands for
			// `default` too, so the map would generate two accessors for one
			// path — which is what the duplicate check exists to prevent, in a
			// form it cannot see.
			name: "a shape that reaches another declaration",
			yaml: "keys:\n  - path: /svc/account/<account>/quota\n    type: int\n" +
				"  - path: /svc/account/default/quota\n    type: int\n",
			sentinel: ockeymap.ErrInvalidMap,
			wantMsg:  "one key, two accessors",
		},
		{
			name:     "invalid segment",
			yaml:     "keys:\n  - path: /svc/a b\n    type: string\n",
			sentinel: ockeymap.ErrInvalidMap,
			wantMsg:  `invalid segment "a b"`,
		},
		{
			name:     "duplicate path",
			yaml:     "keys:\n  - path: /svc/a\n    type: string\n  - path: /svc/a\n    type: int\n",
			sentinel: ockeymap.ErrInvalidMap,
			wantMsg:  "duplicate onlineconf key path",
		},
		{
			// The map is one per service. A key belongs to exactly one tree, so
			// a file spanning two trees mixes the surfaces of two services.
			name:     "two service trees",
			yaml:     "keys:\n  - path: /svc/a\n    type: string\n  - path: /other/b\n    type: string\n",
			sentinel: ockeymap.ErrInvalidMap,
			wantMsg:  "spans several service trees",
		},
		{
			name:     "required with default",
			yaml:     "keys:\n  - path: /svc/a\n    type: int\n    required: true\n    default: 1\n",
			sentinel: ockeymap.ErrInvalidMap,
			wantMsg:  "contradict each other",
		},
		{
			// A struct is the one type whose presence the accessor cannot verify
			// on its own: it would have to unmarshal the value, and the
			// destination type belongs to the caller, not to the map.
			name:     "required struct",
			yaml:     "keys:\n  - path: /svc/a\n    type: struct\n    required: true\n",
			sentinel: ockeymap.ErrInvalidMap,
			wantMsg:  "cannot be `required`",
		},
		{
			name:     "default on struct",
			yaml:     "keys:\n  - path: /svc/a\n    type: struct\n    default: '{}'\n",
			sentinel: ockeymap.ErrInvalidMap,
			wantMsg:  "takes no `default`",
		},
		{
			// A default that does not parse as its type is a value the service
			// would never actually get.
			name:     "bad duration default",
			yaml:     "keys:\n  - path: /svc/a\n    type: duration\n    default: 5 hours\n",
			sentinel: ockeymap.ErrInvalidMap,
			wantMsg:  "is not a duration",
		},
		{
			name:     "bad int default",
			yaml:     "keys:\n  - path: /svc/a\n    type: int\n    default: many\n",
			sentinel: ockeymap.ErrInvalidMap,
			wantMsg:  "default is not an integer",
		},
		{
			name:     "name is not an exported identifier",
			yaml:     "keys:\n  - path: /svc/a\n    type: string\n    name: my_key\n",
			sentinel: ockeymap.ErrInvalidMap,
			wantMsg:  "not an exported Go identifier",
		},
		{
			// Prometheus-style leading digits: the path is legal in the tree
			// but the derived identifier would not compile.
			name:     "derived name starts with a digit",
			yaml:     "keys:\n  - path: /svc/2fa\n    type: bool\n",
			sentinel: ockeymap.ErrInvalidMap,
			wantMsg:  "does not start with a letter",
		},
		{
			name:     "accessor name collision",
			yaml:     "keys:\n  - path: /svc/max-size\n    type: int\n  - path: /svc/max_size\n    type: int\n",
			sentinel: ockeymap.ErrVarNameCollision,
			wantMsg:  "both generate the accessor MaxSize",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ockeymap.Parse([]byte(tt.yaml))
			if err == nil {
				t.Fatal("Parse() succeeded, want an error")
			}

			if !errors.Is(err, tt.sentinel) {
				t.Errorf("Parse() error = %v, want errors.Is(..., %v)", err, tt.sentinel)
			}

			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("Parse() error = %q, want it to mention %q", err, tt.wantMsg)
			}
		})
	}
}

// A bad map should take one round trip to fix, not one per entry.
func TestValidateReportsEveryProblemAtOnce(t *testing.T) {
	bad := "keys:\n" +
		"  - path: /svc/a\n    type: float\n" +
		"  - path: /svc/b\n    type: int\n    required: true\n    default: 2\n" +
		"  - path: /svc/c/{app}/name\n    type: string\n"

	_, err := ockeymap.Parse([]byte(bad))
	if err == nil {
		t.Fatal("Parse() succeeded, want an error")
	}

	for _, want := range []string{`type "float" is unknown`, "contradict each other", "{app}"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q, got:\n%s", want, err)
		}
	}
}

// A key whose path the service completes while running — the account whose
// quota is read, the action being authorized — is declared with a
// <runtimeSegment>, and the segment becomes a parameter of the accessor. Without
// it the only way to read such a key was to compose the path at the call site,
// which is the one thing the map removes (TOOLS-89).
func TestRuntimeSegmentKey(t *testing.T) {
	m, err := ockeymap.Parse([]byte("keys:\n" +
		"  - path: /svc/account/<account>/quota\n    type: int\n    default: 100\n" +
		"  - path: /svc/limits/<account>/<action>/rate\n    type: int\n" +
		"  - path: /svc/queue/name\n    type: string\n"))
	if err != nil {
		t.Fatalf("Parse() on a map with runtime segments: %v", err)
	}

	tests := []struct {
		path        string
		params      []string
		varName     string
		pathConst   string
		pathBuilder string
		paramDecl   string
		pathArgs    string
	}{
		{
			path: "/svc/account/<account>/quota", params: []string{"account"},
			// The runtime segment is a parameter, not part of the key, so it
			// contributes nothing to the derived name.
			varName: "AccountQuota", pathBuilder: "AccountQuotaPath",
			paramDecl: "account string",
			pathArgs:  `"svc", "account", account, "quota"`,
		},
		{
			path: "/svc/limits/<account>/<action>/rate", params: []string{"account", "action"},
			varName: "LimitsRate", pathBuilder: "LimitsRatePath",
			paramDecl: "account, action string",
			pathArgs:  `"svc", "limits", account, action, "rate"`,
		},
		{
			// A key without one is untouched: its path is a constant.
			path: "/svc/queue/name", varName: "QueueName", pathConst: "PathQueueName",
		},
	}

	byPath := make(map[string]ockeymap.Key, len(m.Keys))
	for _, key := range m.Keys {
		byPath[key.Path] = key
	}

	for _, tt := range tests {
		key, ok := byPath[tt.path]
		if !ok {
			t.Errorf("map should declare %s", tt.path)

			continue
		}

		if got := key.Params(); !slices.Equal(got, tt.params) {
			t.Errorf("%s: Params() = %v, want %v", tt.path, got, tt.params)
		}

		if got := key.HasParams(); got != (len(tt.params) > 0) {
			t.Errorf("%s: HasParams() = %v", tt.path, got)
		}

		if got := key.VarName(); got != tt.varName {
			t.Errorf("%s: VarName() = %q, want %q", tt.path, got, tt.varName)
		}

		// One of the two, never both: a path composed from an argument cannot
		// be a constant, and a constant needs no builder.
		if got := key.PathConstName(); got != tt.pathConst {
			t.Errorf("%s: PathConstName() = %q, want %q", tt.path, got, tt.pathConst)
		}

		if got := key.PathBuilderName(); got != tt.pathBuilder {
			t.Errorf("%s: PathBuilderName() = %q, want %q", tt.path, got, tt.pathBuilder)
		}

		if got := key.ParamDecl(); got != tt.paramDecl {
			t.Errorf("%s: ParamDecl() = %q, want %q", tt.path, got, tt.paramDecl)
		}

		if tt.pathArgs != "" {
			if got := key.PathArgs(); got != tt.pathArgs {
				t.Errorf("%s: PathArgs() = %q, want %q", tt.path, got, tt.pathArgs)
			}
		}
	}

	// The accessor shape is still the declaration's: a default is a default
	// whether or not the path takes an argument.
	if shape := byPath["/svc/account/<account>/quota"].Shape(); shape != ockeymap.ShapeDefault {
		t.Errorf("Shape() = %q, want %q", shape, ockeymap.ShapeDefault)
	}
}

// Two declarations told apart only by a runtime segment derive one identifier,
// and that is reported rather than silently generating one accessor for both.
func TestRuntimeSegmentNameCollision(t *testing.T) {
	_, err := ockeymap.Parse([]byte("keys:\n" +
		"  - path: /svc/account/<account>/quota\n    type: int\n" +
		"  - path: /svc/account/quota\n    type: int\n"))
	if err == nil {
		t.Fatal("Parse() succeeded, want a collision")
	}

	if !errors.Is(err, ockeymap.ErrVarNameCollision) {
		t.Errorf("error = %v, want errors.Is(..., ErrVarNameCollision)", err)
	}

	if !strings.Contains(err.Error(), "AccountQuota") {
		t.Errorf("error = %q, want it to name the accessor AccountQuota", err)
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := ockeymap.Load("/nonexistent/onlineconf-key-map.yaml"); err == nil {
		t.Fatal("Load() on a missing file succeeded, want an error")
	}
}
