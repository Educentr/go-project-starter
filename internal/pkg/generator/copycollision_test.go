package generator

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Educentr/go-project-starter/internal/pkg/ds"
)

// collisionCheck runs the pre-pass exactly as Generate does: plan every copy,
// then look for two sources landing on one file.
func collisionCheck(t *testing.T, g *Generator) error {
	t.Helper()

	planned, err := g.plannedCopies(filepath.FromSlash("/project"))
	if err != nil {
		t.Fatalf("unexpected planning error: %v", err)
	}

	return checkCopyDestCollisions(planned)
}

// TestCopyDestCollisionREST is the case from the bug report: two entries of one
// transport's `path:` list pull files of the same name out of different source
// directories. Both copies target api/rest/api/v1/common.swagger.yml, so the
// second used to overwrite the first and the project was built against a
// contract nobody configured.
func TestCopyDestCollisionREST(t *testing.T) {
	const (
		runtimeSpec      = "git+ssh://git@github.com/org/contracts.git#runtime/common.swagger.yml"
		orchestratorSpec = "git+ssh://git@github.com/org/contracts.git#orchestrator/common.swagger.yml"
	)

	g := &Generator{
		Applications: ds.Apps{{Name: "api", Transports: ds.Transports{
			"api": {Name: "api", Type: ds.RestTransportType, ApiVersion: "v1", SpecPath: []string{
				"./api.swagger.yml",
				runtimeSpec,
				orchestratorSpec,
			}},
		}}},
	}

	err := collisionCheck(t, g)
	if err == nil {
		t.Fatal("two sources copying to one file must fail the run")
	}

	if !errors.Is(err, errCopyDestCollision) {
		t.Errorf("error must carry the collision sentinel, got %v", err)
	}

	// Both sources and the shared destination have to be in the message: the
	// operator has to know which two `path:` entries to pick apart.
	for _, want := range []string{
		"api/rest/api/v1/common.swagger.yml",
		"github.com/org/contracts.git@HEAD#runtime/common.swagger.yml",
		"github.com/org/contracts.git@HEAD#orchestrator/common.swagger.yml",
		copyDestCollisionHint,
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error must mention %q, got:\n%s", want, err)
		}
	}

	// The spec that does not collide is nobody's business here.
	if strings.Contains(err.Error(), "api.swagger.yml\n") {
		t.Errorf("only colliding destinations may be reported, got:\n%s", err)
	}
}

// TestCopyDestCollisionJSONSchema covers the schema channel in both config
// forms: the legacy `path:` list and the `schemas[]` one.
func TestCopyDestCollisionJSONSchema(t *testing.T) {
	t.Run("path list", func(t *testing.T) {
		g := &Generator{JSONSchemas: ds.JSONSchemas{"models": {Name: "models", Path: []string{
			"./v1/user.schema.json",
			"./v2/user.schema.json",
		}}}}

		err := collisionCheck(t, g)
		if err == nil {
			t.Fatal("two schemas copying to one file must fail the run")
		}

		for _, want := range []string{
			"api/schema/models/user.schema.json",
			"./v1/user.schema.json",
			"./v2/user.schema.json",
		} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error must mention %q, got:\n%s", want, err)
			}
		}
	})

	t.Run("schemas form", func(t *testing.T) {
		g := &Generator{JSONSchemas: ds.JSONSchemas{"events": {Name: "events", Schemas: []ds.JSONSchemaItem{
			{Path: "https://specs.example.com/v1/event.schema.json"},
			{Path: "./local/event.schema.json"},
		}}}}

		err := collisionCheck(t, g)
		if err == nil {
			t.Fatal("two schemas copying to one file must fail the run")
		}

		for _, want := range []string{
			"api/schema/events/event.schema.json",
			"https://specs.example.com/v1/event.schema.json",
			"./local/event.schema.json",
		} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error must mention %q, got:\n%s", want, err)
			}
		}
	})
}

// TestCopyDestCollisionGRPC pins the guard for the gRPC channel. A `grpc:`
// block takes a single `path:`, and transport names are unique, so today's
// config cannot reach this state — the transport is built here directly. The
// check walks all three copying channels the same way, and this test is what
// keeps it that way if gRPC ever grows a list of protos.
func TestCopyDestCollisionGRPC(t *testing.T) {
	g := &Generator{
		Applications: ds.Apps{{Name: "api", Transports: ds.Transports{
			"ItemService": {Name: "ItemService", Type: ds.GrpcTransportType, SpecPath: []string{
				"./runtime/item.proto",
				"./admin/item.proto",
			}},
		}}},
	}

	err := collisionCheck(t, g)
	if err == nil {
		t.Fatal("two protos copying to one file must fail the run")
	}

	for _, want := range []string{
		"api/grpc/ItemService/item.proto",
		"./runtime/item.proto",
		"./admin/item.proto",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error must mention %q, got:\n%s", want, err)
		}
	}
}

// TestCopyDestCollisionReportsEveryCollision: a config that got one basename
// wrong usually got more, and a check that stops at the first one turns into a
// run-fix-run loop.
func TestCopyDestCollisionReportsEveryCollision(t *testing.T) {
	g := &Generator{
		Applications: ds.Apps{{Name: "api", Transports: ds.Transports{
			"api": {Name: "api", Type: ds.RestTransportType, ApiVersion: "v1", SpecPath: []string{
				"./a/common.swagger.yml",
				"./b/common.swagger.yml",
			}},
		}}},
		JSONSchemas: ds.JSONSchemas{"models": {Name: "models", Path: []string{
			"./a/user.schema.json",
			"./b/user.schema.json",
		}}},
	}

	err := collisionCheck(t, g)
	if err == nil {
		t.Fatal("collisions in two channels must fail the run")
	}

	for _, want := range []string{
		"api/rest/api/v1/common.swagger.yml",
		"api/schema/models/user.schema.json",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error must mention %q, got:\n%s", want, err)
		}
	}
}

// TestCopyDestCollisionAllowsSameSource: only *different* sources collide.
// Reaching one destination several times is normal — a transport is
// snapshotted into every application that serves it, and the same file may
// legitimately be listed twice.
func TestCopyDestCollisionAllowsSameSource(t *testing.T) {
	shared := ds.Transport{
		Name:       "api",
		Type:       ds.RestTransportType,
		ApiVersion: "v1",
		SpecPath:   []string{"./common.swagger.yml", "./common.swagger.yml"},
	}

	g := &Generator{Applications: ds.Apps{
		{Name: "publicApi", Transports: ds.Transports{"api": shared}},
		{Name: "internalApi", Transports: ds.Transports{"api": shared}},
	}}

	if err := collisionCheck(t, g); err != nil {
		t.Errorf("one source reaching one destination is not a collision, got %v", err)
	}
}

// TestCopyDestCollisionAllowsSeparateDirs: the same basename in two transports
// (or two schema blocks) is fine — each block copies into its own directory,
// which is also the way out of a real collision.
func TestCopyDestCollisionAllowsSeparateDirs(t *testing.T) {
	g := &Generator{
		Applications: ds.Apps{{Name: "api", Transports: ds.Transports{
			"svc_a": {Name: "svc_a", Type: ds.RestTransportType, ApiVersion: "v1", SpecPath: []string{
				"./a/common.swagger.yml",
			}},
			"svc_b": {Name: "svc_b", Type: ds.RestTransportType, ApiVersion: "v1", SpecPath: []string{
				"./b/common.swagger.yml",
			}},
			// Same name, different api version — also a directory of its own.
			"svc_c": {Name: "svc_a", Type: ds.RestTransportType, ApiVersion: "v2", SpecPath: []string{
				"./c/common.swagger.yml",
			}},
		}}},
		JSONSchemas: ds.JSONSchemas{
			"models": {Name: "models", Path: []string{"./a/user.schema.json"}},
			"events": {Name: "events", Path: []string{"./b/user.schema.json"}},
		},
	}

	if err := collisionCheck(t, g); err != nil {
		t.Errorf("separate target directories do not collide, got %v", err)
	}
}
