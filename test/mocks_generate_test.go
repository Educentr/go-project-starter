package test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// makefileTargetPrereqs returns the prerequisite list of a target declared in
// the generated Makefile, e.g. "proto generate-ogen generate-mocks".
func makefileTargetPrereqs(t *testing.T, makefile, target string) string {
	t.Helper()

	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(target) + `:(.*)$`)

	match := re.FindStringSubmatch(makefile)
	if match == nil {
		t.Fatalf("generated Makefile has no %q target", target)
	}

	return strings.TrimSpace(match[1])
}

// makefileRecipe returns the recipe lines of a target declared in the
// generated Makefile.
func makefileRecipe(t *testing.T, makefile, target string) string {
	t.Helper()

	re := regexp.MustCompile(`(?ms)^` + regexp.QuoteMeta(target) + `:.*?\n(\t.*?)\n[^\t\n]`)

	match := re.FindStringSubmatch(makefile)
	if match == nil {
		t.Fatalf("generated Makefile has no recipe for %q", target)
	}

	return match[1]
}

// TestGenerateIncludesMocks covers the invariant from TOOLS-21 / issue #54: the
// generator writes the mock scaffolding (tests/{app}/mocks/{transport}/) whose
// //go:generate directive mirrors an ogen interface the generator also writes,
// so the generated `generate` aggregator has to regenerate those mocks itself.
// Without it a contract change leaves the mock referring to types ogen no
// longer emits, and the test package stops compiling.
func TestGenerateIncludesMocks(t *testing.T) {
	tmpDir := generateProject(t, "rest-client-mocks")

	// The fixture must actually produce mock scaffolding, otherwise the
	// assertions below would pass vacuously.
	scaffold := filepath.Join(tmpDir, "tests", "api", "mocks", "billing", "psg_doc_gen.go")
	if _, err := os.Stat(scaffold); err != nil {
		t.Fatalf("expected mock scaffolding at tests/api/mocks/billing/psg_doc_gen.go: %v", err)
	}

	makefile := readGenerated(t, tmpDir, "Makefile")

	prereqs := makefileTargetPrereqs(t, makefile, "generate")
	if !strings.Contains(prereqs, "generate-mocks") {
		t.Errorf("generate aggregator must include generate-mocks, got prerequisites %q", prereqs)
	}

	// The mocks mirror the ogen interfaces, so they have to be regenerated
	// after them — make walks the prerequisite list left to right.
	if ogenIdx, mockIdx := strings.Index(prereqs, "generate-ogen"), strings.Index(prereqs, "generate-mocks"); ogenIdx > mockIdx {
		t.Errorf("generate-mocks must come after generate-ogen, got prerequisites %q", prereqs)
	}

	recipe := makefileRecipe(t, makefile, "generate-mocks")

	// Targeted on purpose: `go generate ./...` would also run user-written
	// directives, which must stay out of the code-generation gate.
	if !strings.Contains(recipe, "go generate ./tests/api/mocks/...") {
		t.Errorf("generate-mocks must run go generate for ./tests/api/mocks/..., got recipe:\n%s", recipe)
	}

	if strings.Contains(recipe, "go generate ./...") {
		t.Errorf("generate-mocks must not run every //go:generate directive of the project, got recipe:\n%s", recipe)
	}

	// mockgen loads the ogen package through the module graph, so the imports
	// the preceding generators introduced have to be resolvable first.
	if !strings.Contains(recipe, "go mod tidy") {
		t.Errorf("generate-mocks must resolve the module before running mockgen, got recipe:\n%s", recipe)
	}
}

// TestGenerateSkipsMocksWithoutScaffolding guards the other half: a project
// whose applications get no mock scaffolding must not get a mock step in
// `generate`, and the standalone target must not point `go generate` at a
// directory that was never generated — that fails the whole run.
func TestGenerateSkipsMocksWithoutScaffolding(t *testing.T) {
	// rest-envs-goat has goat_tests but no ogen_client transport.
	tmpDir := generateProject(t, "rest-envs-goat")

	if _, err := os.Stat(filepath.Join(tmpDir, "tests", "api", "mocks")); err == nil {
		t.Fatal("fixture rest-envs-goat unexpectedly produced mock scaffolding")
	}

	makefile := readGenerated(t, tmpDir, "Makefile")

	if prereqs := makefileTargetPrereqs(t, makefile, "generate"); strings.Contains(prereqs, "generate-mocks") {
		t.Errorf("generate aggregator must not include generate-mocks without scaffolding, got prerequisites %q", prereqs)
	}

	if recipe := makefileRecipe(t, makefile, "generate-mocks"); strings.Contains(recipe, "go generate") {
		t.Errorf("generate-mocks must not run go generate without scaffolding, got recipe:\n%s", recipe)
	}
}
