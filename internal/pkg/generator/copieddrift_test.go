package generator

import (
	"path/filepath"
	"testing"

	"github.com/Educentr/go-project-starter/internal/pkg/ds"
	"github.com/Educentr/go-project-starter/internal/pkg/meta"
)

// TestCopiedSpecTargets pins the destinations the drift pre-pass derives.
// They must equal what the copy pass goes on to write — a mismatch would
// make the check compare the wrong file (or nothing at all) and let an edited
// spec through.
func TestCopiedSpecTargets(t *testing.T) {
	const gitURI = "git+https://github.com/org/specs.git@v1.0.0?token_env=GH_TOKEN#proto/admin.proto"

	restTransport := ds.Transport{
		Name:       "example",
		Type:       ds.RestTransportType,
		ApiVersion: "v1",
		SpecPath:   []string{"./example.swagger.yml", "https://specs.example.com/v1/common.yaml?token_env=TOK"},
	}

	grpcTransport := ds.Transport{
		Name:     "AdminService",
		Type:     ds.GrpcTransportType,
		SpecPath: []string{gitURI},
	}

	g := &Generator{
		// The same transport is served by two applications: the copy pass walks
		// both, so the pre-pass must not report its destination twice.
		Applications: ds.Apps{
			{Name: "publicApi", Transports: ds.Transports{
				"example":      restTransport,
				"AdminService": grpcTransport,
			}},
			{Name: "internalApi", Transports: ds.Transports{"example": restTransport}},
		},
		JSONSchemas: ds.JSONSchemas{
			"models": {Name: "models", Path: []string{"./user.schema.json"}},
			"events": {Name: "events", Schemas: []ds.JSONSchemaItem{{Path: "./event.schema.json"}}},
		},
	}

	targets, err := g.copiedSpecTargets(filepath.FromSlash("/project"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Source strings are what the operator wrote in project.yaml, with the
	// token name kept but no secret value — never a staging path.
	want := map[string]string{
		"api/rest/example/v1/example.swagger.yml": "./example.swagger.yml",
		"api/rest/example/v1/common.yaml":         "https://specs.example.com/v1/common.yaml (token_env=TOK)",
		"api/grpc/AdminService/admin.proto": "https://github.com/org/specs.git@v1.0.0#proto/admin.proto" +
			" (token_env=GH_TOKEN)",
		"api/schema/models/user.schema.json":  "./user.schema.json",
		"api/schema/events/event.schema.json": "./event.schema.json",
	}

	if len(targets) != len(want) {
		t.Fatalf("got %d targets, want %d: %+v", len(targets), len(want), targets)
	}

	for _, got := range targets {
		wantSource, ok := want[got.Path]
		if !ok {
			t.Errorf("unexpected target %q", got.Path)

			continue
		}

		if got.Source != wantSource {
			t.Errorf("target %q: got source %q, want %q", got.Path, got.Source, wantSource)
		}

		delete(want, got.Path)
	}

	for path := range want {
		t.Errorf("missing target %q", path)
	}
}

// A malformed source is the resolve pass's error to report (with the transport
// name and entry index attached), so the pre-pass skips it instead of failing
// with a message that has less context.
func TestCopiedSpecTargetsSkipsUnparsableSource(t *testing.T) {
	g := &Generator{
		Applications: ds.Apps{{Name: "api", Transports: ds.Transports{
			"api": {Name: "api", Type: ds.RestTransportType, ApiVersion: "v1", SpecPath: []string{
				"git@github.com:org/specs.git",
				"./ok.yaml",
			}},
		}}},
	}

	targets, err := g.copiedSpecTargets(filepath.FromSlash("/project"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(targets) != 1 || targets[0].Path != "api/rest/api/v1/ok.yaml" {
		t.Errorf("only the parsable source must be checked, got %+v", targets)
	}
}

// Belt-and-braces on the contract between the two halves: the pre-pass only
// compares targets, so a manifest record for a file no longer in the output
// (an orphan) is out of its scope.
func TestCopiedSpecTargetsDriftIgnoresOrphanRecords(t *testing.T) {
	manifest := meta.Manifest{{
		Path: "api/rest/dropped/v1/dropped.yaml",
		Kind: meta.KindCopied,
		Hash: "0000",
	}}

	if err := manifest.CheckCopiedDrift(t.TempDir(), nil); err != nil {
		t.Errorf("orphan records are not this check's business, got %v", err)
	}
}
