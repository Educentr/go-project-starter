package ds

import (
	"errors"
	"testing"

	"github.com/Educentr/go-project-starter/internal/pkg/ockeymap"
)

// A service key whose path the service completes at runtime reads through a path
// builder: there is no constant to name a path composed from an argument. The
// accessor model is the same one the generator's own half uses, which is why the
// template needs no branch of its own for it (TOOLS-89).
func TestServiceAccessorWithRuntimeSegment(t *testing.T) {
	config, err := NewOnlineConfConfig("svc", []ockeymap.Key{
		{Path: "/svc/account/<account>/quota", Type: ockeymap.TypeInt},
		{Path: "/svc/queue/name", Type: ockeymap.TypeString},
	}, nil, nil)
	if err != nil {
		t.Fatalf("NewOnlineConfConfig() error: %v", err)
	}

	byName := make(map[string]OnlineConfAccessor, len(config.Accessors))
	for _, accessor := range config.Accessors {
		byName[accessor.Name] = accessor
	}

	shape, ok := byName["AccountQuota"]
	if !ok {
		t.Fatalf("expected an AccountQuota accessor, got %v", byName)
	}

	if shape.PathConst != "" {
		t.Errorf("PathConst = %q, want none: the path is composed from an argument", shape.PathConst)
	}

	if shape.PathBuilder != "AccountQuotaPath" || shape.PathExpr != "AccountQuotaPath(account)" {
		t.Errorf("PathBuilder = %q, PathExpr = %q, want the builder and a call to it",
			shape.PathBuilder, shape.PathExpr)
	}

	if shape.ParamDecl != "account string" || shape.PathArgs != `"svc", "account", account, "quota"` {
		t.Errorf("ParamDecl = %q, PathArgs = %q", shape.ParamDecl, shape.PathArgs)
	}

	// A key without a runtime segment is untouched: its path is a constant, and
	// the constant is what a caller hands to Subscribe.
	plain := byName["QueueName"]

	if plain.PathConst != "PathQueueName" || plain.PathExpr != "PathQueueName" || plain.PathBuilder != "" {
		t.Errorf("plain key accessor = %+v, want it to read through its path constant", plain)
	}

	// Only the constant-path keys are listed, so Paths() and the constant block
	// carry the same set.
	consts := config.ServicePathConsts()
	if len(consts) != 1 || consts[0].Name != "QueueName" {
		t.Errorf("ServicePathConsts() = %v, want only the key whose path is a constant", consts)
	}

	if !config.HasServicePathConsts() {
		t.Error("HasServicePathConsts() = false, want true")
	}
}

// The two halves of the map land in ONE Go package, and a path builder is one of
// the names a declaration occupies. A collision has to be reported here rather
// than as a package that does not compile in the user's project.
func TestServiceAccessorPathBuilderCollision(t *testing.T) {
	_, err := NewOnlineConfConfig("svc", []ockeymap.Key{
		{Path: "/svc/account/<account>/quota", Type: ockeymap.TypeInt},
		{Path: "/svc/other", Type: ockeymap.TypeString, Name: "AccountQuotaPath"},
	}, nil, nil)
	if err == nil {
		t.Fatal("NewOnlineConfConfig() succeeded, want a name collision")
	}

	if !errors.Is(err, ErrOnlineConfNameTaken) {
		t.Errorf("error = %v, want errors.Is(..., ErrOnlineConfNameTaken)", err)
	}
}

// The exported names the accessor package carries itself are the other half of
// the same rule, and ActiveRecordConfig is one of them: the hand-off is
// generated for a project that uses ActiveRecord, so a key named after it would
// generate one function twice there and pass everywhere else.
func TestServiceAccessorReservedNameCollision(t *testing.T) {
	for _, name := range []string{"ActiveRecordConfig", "Subscribe", "Paths", "CheckRequired"} {
		_, err := NewOnlineConfConfig("svc", []ockeymap.Key{
			{Path: "/svc/other", Type: ockeymap.TypeString, Name: name},
		}, nil, nil)
		if err == nil {
			t.Fatalf("NewOnlineConfConfig() accepted a key named %s, want a name collision", name)
		}

		if !errors.Is(err, ErrOnlineConfNameTaken) {
			t.Errorf("%s: error = %v, want errors.Is(..., ErrOnlineConfNameTaken)", name, err)
		}
	}
}

// A map made only of keys with runtime segments has no constant path at all, so
// neither the constant block nor Paths() is generated: there would be nothing to
// put in either.
func TestServiceMapWithoutPathConstants(t *testing.T) {
	config, err := NewOnlineConfConfig("svc", []ockeymap.Key{
		{Path: "/svc/account/<account>/quota", Type: ockeymap.TypeInt},
	}, nil, nil)
	if err != nil {
		t.Fatalf("NewOnlineConfConfig() error: %v", err)
	}

	if !config.HasServiceKeys() {
		t.Error("HasServiceKeys() = false, want true: the service declared a key")
	}

	if config.HasServicePathConsts() {
		t.Error("HasServicePathConsts() = true, want false: no declared path is a constant")
	}
}
