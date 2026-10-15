package generator

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Educentr/go-project-starter/internal/pkg/ds"
	"github.com/Educentr/go-project-starter/internal/pkg/specsource"
)

// The ref override has to reach BOTH resolve paths: the pre-pass that stages
// REST/gRPC/JSON Schema specs, and the inline resolve used by queue workers
// and CLI commands. If it reached only one of them, the generated tree would
// be built from two different states of the contracts — silently.
//
// Both tests below use a git+https source whose token_env is unset, so the
// resolve stops before any network access and the error names the source with
// its effective ref — which is the assertion: the override arrived.
const (
	// tokenEnvNotSet must not exist in the environment.
	tokenEnvNotSet = "GPS_TEST_TOKEN_ENV_THAT_IS_NEVER_SET"
	specURI        = "git+https://github.com/org/specs.git?token_env=" + tokenEnvNotSet + "#openapi/users.yaml"
	scopedRefEntry = "github.com/org/specs:release/scoped"
	scopedRef      = "release/scoped"
)

func mustSourceRefs(t *testing.T, entries ...string) *specsource.SourceRefs {
	t.Helper()

	refs, err := specsource.ParseSourceRefs(entries)
	if err != nil {
		t.Fatalf("ParseSourceRefs(%q): %v", entries, err)
	}

	return refs
}

// assertResolvedWithRef checks that resolving failed at the token lookup (no
// network) and that the source it names carries ref.
func assertResolvedWithRef(t *testing.T, err error, ref string) {
	t.Helper()

	if !errors.Is(err, specsource.ErrTokenEnvMissing) {
		t.Fatalf("got %v, want ErrTokenEnvMissing", err)
	}

	if !strings.Contains(err.Error(), ref) {
		t.Errorf("resolved source must carry ref %q, got %v", ref, err)
	}
}

func TestResolveInlineSpec_PassesSourceRefsToResolver(t *testing.T) {
	refs := mustSourceRefs(t, scopedRefEntry)

	_, cleanup, err := resolveInlineSpec(t.Context(), t.TempDir(), specURI, refs)

	cleanup()

	assertResolvedWithRef(t, err, scopedRef)

	if cerr := refs.CheckUsed(); cerr != nil {
		t.Errorf("the selector matched the inline source, got %v", cerr)
	}
}

func TestResolveAllSources_PassesSourceRefsToResolver(t *testing.T) {
	refs := mustSourceRefs(t, scopedRefEntry)

	g := &Generator{
		BasePath:   t.TempDir(),
		SourceRefs: refs,
		Transports: ds.Transports{
			"users": ds.Transport{
				Name:     "users",
				SpecPath: []string{specURI},
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

	assertResolvedWithRef(t, err, scopedRef)

	if cerr := refs.CheckUsed(); cerr != nil {
		t.Errorf("the selector matched the staged source, got %v", cerr)
	}
}

// A selector naming a repository the project does not use is a typo: it must
// be reported, not silently ignored, or the build would quietly take that
// repository from its default branch.
func TestSourceRefs_UnusedSelectorIsReported(t *testing.T) {
	refs := mustSourceRefs(t, "github.com/org/typo:release/scoped")

	g := &Generator{
		BasePath:   t.TempDir(),
		SourceRefs: refs,
		Transports: ds.Transports{
			"users": ds.Transport{
				Name:     "users",
				SpecPath: []string{specURI},
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

	// The source itself still fails on the missing token — with the ref it had
	// in the config, since no selector matched it.
	assertResolvedWithRef(t, err, "HEAD")

	if cerr := refs.CheckUsed(); !errors.Is(cerr, specsource.ErrUnusedSourceRef) {
		t.Fatalf("got %v, want ErrUnusedSourceRef", cerr)
	}
}

// A local-only project must not notice the knob at all: no staging, no git.
func TestResolveInlineSpec_LocalSourceIgnoresSourceRefs(t *testing.T) {
	base := t.TempDir()

	path, cleanup, err := resolveInlineSpec(t.Context(), base, "./commands.yaml", mustSourceRefs(t, "release/a"))

	cleanup()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if want := filepath.Join(base, "commands.yaml"); path != want {
		t.Errorf("path: got %q, want %q", path, want)
	}
}
