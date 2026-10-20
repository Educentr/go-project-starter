package ockeymap_test

import (
	"errors"
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
			name:     "typo in field name",
			yaml:     "keys:\n  - path: /svc/a\n    type: string\n    requred: true\n",
			sentinel: ockeymap.ErrMalformedMap,
			wantMsg:  `unknown field "requred"`,
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
			// The list-template form a key registry carries. There is no
			// subtree read to generate for it, so it gets its own explanation
			// rather than a bare grammar complaint.
			name:     "list template segment",
			yaml:     "keys:\n  - path: /svc/agent_env/<i>/name\n    type: string\n",
			sentinel: ockeymap.ErrInvalidMap,
			wantMsg:  "type: struct",
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
			// The subtrees the generated readers own. A declaration here would
			// either shadow a generated read or drift from it.
			name:     "reserved subtree",
			yaml:     "keys:\n  - path: /svc/transport/rest/api/port\n    type: string\n",
			sentinel: ockeymap.ErrInvalidMap,
			wantMsg:  "which the generator reads itself",
		},
		{
			name:     "required with default",
			yaml:     "keys:\n  - path: /svc/a\n    type: int\n    required: true\n    default: 1\n",
			sentinel: ockeymap.ErrInvalidMap,
			wantMsg:  "contradict each other",
		},
		{
			// go-onlineconf's GetStrings reports no presence, so a `required`
			// here would be a promise the generated accessor could not keep.
			name:     "required strings",
			yaml:     "keys:\n  - path: /svc/a\n    type: strings\n    required: true\n",
			sentinel: ockeymap.ErrInvalidMap,
			wantMsg:  "cannot be `required`",
		},
		{
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
		"  - path: /svc/transport/rest/api/port\n    type: string\n"

	_, err := ockeymap.Parse([]byte(bad))
	if err == nil {
		t.Fatal("Parse() succeeded, want an error")
	}

	for _, want := range []string{`type "float" is unknown`, "contradict each other", "reads itself"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q, got:\n%s", want, err)
		}
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := ockeymap.Load("/nonexistent/onlineconf-key-map.yaml"); err == nil {
		t.Fatal("Load() on a missing file succeeded, want an error")
	}
}

func TestReservedSegmentsAreSorted(t *testing.T) {
	segments := ockeymap.ReservedSegments()
	if len(segments) == 0 {
		t.Fatal("ReservedSegments() is empty: the generator does read OnlineConf itself")
	}

	for i := 1; i < len(segments); i++ {
		if segments[i-1] > segments[i] {
			t.Fatalf("ReservedSegments() is not sorted: %v", segments)
		}
	}
}
