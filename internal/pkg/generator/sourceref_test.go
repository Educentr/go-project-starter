package generator

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/Educentr/go-project-starter/internal/pkg/ds"
	"github.com/Educentr/go-project-starter/internal/pkg/specsource"
)

// The ref override has to reach BOTH resolve paths: the pre-pass that stages
// REST/gRPC/JSON Schema specs, and the inline resolve used by queue workers
// and CLI commands. If it reached only one of them, the generated tree would
// be built from two different states of the contracts — silently. Both tests
// below use a ref that NewResolver rejects outright, so the assertion is a
// hermetic "the option arrived", with no clone attempted.
const unusableRef = "-not-a-ref"

func TestResolveInlineSpec_PassesSourceRefToResolver(t *testing.T) {
	_, cleanup, err := resolveInlineSpec(
		t.Context(),
		t.TempDir(),
		"git+https://github.com/org/repo.git#commands.yaml",
		unusableRef,
	)

	cleanup()

	if !errors.Is(err, specsource.ErrInvalidRef) {
		t.Fatalf("got %v, want ErrInvalidRef (sourceRef not forwarded to the resolver?)", err)
	}
}

func TestResolveAllSources_PassesSourceRefToResolver(t *testing.T) {
	g := &Generator{
		BasePath:  t.TempDir(),
		SourceRef: unusableRef,
		Transports: ds.Transports{
			"users": ds.Transport{
				Name:     "users",
				SpecPath: []string{"git+https://github.com/org/repo.git#openapi/users.yaml"},
			},
		},
		JSONSchemas: ds.JSONSchemas{},
	}

	cleanup, err := g.resolveAllSources()
	if cleanup != nil {
		if cerr := cleanup(); cerr != nil {
			t.Logf("cleanup: %v", cerr)
		}
	}

	if !errors.Is(err, specsource.ErrInvalidRef) {
		t.Fatalf("got %v, want ErrInvalidRef (sourceRef not forwarded to the resolver?)", err)
	}
}

// A local-only project must not notice the knob at all: no staging, no git.
func TestResolveInlineSpec_LocalSourceIgnoresSourceRef(t *testing.T) {
	base := t.TempDir()

	path, cleanup, err := resolveInlineSpec(t.Context(), base, "./commands.yaml", "release/a")

	cleanup()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if want := filepath.Join(base, "commands.yaml"); path != want {
		t.Errorf("path: got %q, want %q", path, want)
	}
}
