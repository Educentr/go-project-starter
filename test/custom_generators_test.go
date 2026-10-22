package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makeTarget returns the recipe lines of target from a generated Makefile,
// plus its prerequisite line. A target is "<name>:" at the start of a line;
// its recipe is the tab-indented lines that follow.
func makeTarget(t *testing.T, makefile, target string) (prereqs string, recipe []string, found bool) {
	t.Helper()

	lines := strings.Split(makefile, "\n")

	for i, line := range lines {
		if !strings.HasPrefix(line, target+":") {
			continue
		}

		prereqs = strings.TrimSpace(strings.TrimPrefix(line, target+":"))

		for _, next := range lines[i+1:] {
			if !strings.HasPrefix(next, "\t") {
				break
			}

			recipe = append(recipe, strings.TrimPrefix(next, "\t"))
		}

		return prereqs, recipe, true
	}

	return "", nil, false
}

// TestGenerateCustomGenerators covers the whole custom-generator feature on a
// project that declares two of them: the make targets, the fetched contract
// specs and the provenance recorded for them.
func TestGenerateCustomGenerators(t *testing.T) {
	tmpDir := generateFromConfig(t, "custom-generator", "Generate project with custom generators")

	makefile := readGenerated(t, tmpDir, "Makefile")

	// install → install-<gen>, cmd → generate-<gen>, lint → lint-<gen>:
	// one target per generator, the command run once per contract.
	t.Run("generator with all three commands", func(t *testing.T) {
		_, recipe, found := makeTarget(t, makefile, "install-runtime_limits_ts")
		if !found {
			t.Fatal("install-runtime_limits_ts target is missing")
		}

		// Two contracts feed this generator, but installing it is a
		// per-generator step and must not be repeated.
		if len(recipe) != 1 || recipe[0] != "npm ci --prefix web" {
			t.Errorf("install recipe = %v, want one 'npm ci --prefix web' line", recipe)
		}

		_, recipe, found = makeTarget(t, makefile, "generate-runtime_limits_ts")
		if !found {
			t.Fatal("generate-runtime_limits_ts target is missing")
		}

		want := []string{
			"node web/scripts/generate-runtime-limits.mjs api/contract/runtime_limits.yaml --out web/src/generated/runtimeLimits.ts",
			"node web/scripts/generate-runtime-limits.mjs api/contract/runtime_limits_extra.yaml --out web/src/generated/runtimeLimitsExtra.ts",
		}

		if len(recipe) != len(want) {
			t.Fatalf("generate recipe = %v, want %v", recipe, want)
		}

		for i := range want {
			if recipe[i] != want[i] {
				t.Errorf("generate recipe[%d] = %q, want %q", i, recipe[i], want[i])
			}
		}

		_, recipe, found = makeTarget(t, makefile, "lint-runtime_limits_ts")
		if !found {
			t.Fatal("lint-runtime_limits_ts target is missing")
		}

		if len(recipe) != 2 || recipe[0] != "npx --prefix web spec-lint api/contract/runtime_limits.yaml" {
			t.Errorf("lint recipe = %v", recipe)
		}
	})

	// A generator declaring only `cmd` must not produce an empty install- or
	// lint- target: an empty recipe would make `make install-tools` a no-op
	// that looks like it did something.
	t.Run("generator with cmd only", func(t *testing.T) {
		_, recipe, found := makeTarget(t, makefile, "generate-codex_types")
		if !found {
			t.Fatal("generate-codex_types target is missing")
		}

		if len(recipe) != 1 || recipe[0] != "go run ./internal/cmd/codextypes api/contract/codex_broker.swagger.yml" {
			t.Errorf("generate recipe = %v", recipe)
		}

		for _, absent := range []string{"install-codex_types", "lint-codex_types"} {
			if _, _, found := makeTarget(t, makefile, absent); found {
				t.Errorf("%s should not be generated for a cmd-only generator", absent)
			}
		}
	})

	t.Run("wired into the built-in targets", func(t *testing.T) {
		prereqs, _, found := makeTarget(t, makefile, "install-tools")
		if !found {
			t.Fatal("install-tools target is missing")
		}

		if !strings.Contains(prereqs, "install-runtime_limits_ts") {
			t.Errorf("install-tools prerequisites = %q, want install-runtime_limits_ts", prereqs)
		}

		if strings.Contains(prereqs, "install-codex_types") {
			t.Errorf("install-tools must not depend on a generator with no install step: %q", prereqs)
		}

		prereqs, _, found = makeTarget(t, makefile, "generate")
		if !found {
			t.Fatal("generate target is missing")
		}

		for _, want := range []string{"generate-runtime_limits_ts", "generate-codex_types"} {
			if !strings.Contains(prereqs, want) {
				t.Errorf("generate prerequisites = %q, want %s", prereqs, want)
			}
		}

		// Custom generators run after the built-in ones: a contract-derived
		// artifact may well reference code ogen produced.
		if idx := strings.Index(prereqs, "generate-runtime_limits_ts"); idx >= 0 {
			if ogen := strings.Index(prereqs, "generate-ogen"); ogen < 0 || ogen > idx {
				t.Errorf("generate should run built-in generators first: %q", prereqs)
			}
		}

		prereqs, _, found = makeTarget(t, makefile, "lint")
		if !found {
			t.Fatal("lint target is missing")
		}

		if !strings.Contains(prereqs, "lint-runtime_limits_ts") {
			t.Errorf("lint prerequisites = %q, want lint-runtime_limits_ts", prereqs)
		}

		if strings.Contains(prereqs, "lint-codex_types") {
			t.Errorf("lint must not depend on a generator with no linter: %q", prereqs)
		}
	})

	t.Run("contract specs copied to their declared paths", func(t *testing.T) {
		limits := readGenerated(t, tmpDir, "api", "contract", "runtime_limits.yaml")
		if !strings.Contains(limits, "cpu_millis: 500") {
			t.Errorf("copied contract content = %q", limits)
		}

		for _, rel := range []string{"runtime_limits_extra.yaml", "codex_broker.swagger.yml"} {
			if _, err := os.Stat(filepath.Join(tmpDir, "api", "contract", rel)); err != nil {
				t.Errorf("contract %s not copied: %v", rel, err)
			}
		}
	})

	t.Run("manifest records the contract specs with their source", func(t *testing.T) {
		manifest := readGenerated(t, tmpDir, ".project-config", "meta.yaml")

		for _, want := range []string{
			"{path: api/contract/runtime_limits.yaml, kind: copied,",
			"source: ./runtime_limits.yaml}",
		} {
			if !strings.Contains(manifest, want) {
				t.Errorf("meta.yaml should contain %q, got:\n%s", want, manifest)
			}
		}
	})

	// The generator does not produce the contract's output — that is the
	// project's own generator's job — so nothing is written under `target`.
	t.Run("target path is not created by the starter", func(t *testing.T) {
		if _, err := os.Stat(filepath.Join(tmpDir, "web")); err == nil {
			t.Error("the starter must not create the generator's output tree")
		}
	})
}

// TestGenerateCustomGenerators_ContractDriftFails covers the invariant that a
// contract spec is owned by its own repository: editing the copy inside the
// project fails the next run instead of being silently overwritten.
func TestGenerateCustomGenerators_ContractDriftFails(t *testing.T) {
	tmpDir := generateFromConfig(t, "custom-generator", "Generate project with custom generators")

	spec := filepath.Join(tmpDir, "api", "contract", "runtime_limits.yaml")

	content, err := os.ReadFile(spec)
	if err != nil {
		t.Fatalf("read copied contract: %v", err)
	}

	if err := os.WriteFile(spec, append(content, []byte("  disk_mb: 1\n")...), 0o600); err != nil {
		t.Fatalf("edit copied contract: %v", err)
	}

	curDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting current directory: %v", err)
	}

	out, err := ExecCommand(filepath.Join(curDir, ".."), "go", []string{
		"run", filepath.Join(curDir, "..", "cmd", "go-project-starter", "main.go"),
		"--target", tmpDir,
		"--configDir", filepath.Join(curDir, "..", "test", "docker-integration", "configs", "custom-generator"),
		"--config", "project.yaml",
	}, "Regenerate over an edited contract spec")
	if err == nil {
		t.Fatalf("regeneration should fail on an edited contract spec, output:\n%s", out)
	}

	for _, want := range []string{
		"contract spec was changed inside the project",
		"api/contract/runtime_limits.yaml",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("error output should mention %q, got:\n%s", want, out)
		}
	}
}

// TestGenerateCustomGenerators_RegenerationIsStable checks the other half of
// the drift rule: an untouched project regenerates cleanly, so the error above
// is about hand edits and not about the copy step itself.
func TestGenerateCustomGenerators_RegenerationIsStable(t *testing.T) {
	tmpDir := generateFromConfig(t, "custom-generator", "Generate project with custom generators")

	before := readGenerated(t, tmpDir, "Makefile")

	curDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting current directory: %v", err)
	}

	out, err := ExecCommand(filepath.Join(curDir, ".."), "go", []string{
		"run", filepath.Join(curDir, "..", "cmd", "go-project-starter", "main.go"),
		"--target", tmpDir,
		"--configDir", filepath.Join(curDir, "..", "test", "docker-integration", "configs", "custom-generator"),
		"--config", "project.yaml",
	}, "Regenerate an untouched project")
	if err != nil {
		t.Fatalf("regeneration failed: %s\n%s", err, out)
	}

	if after := readGenerated(t, tmpDir, "Makefile"); after != before {
		t.Error("Makefile changed on a no-op regeneration")
	}
}

// TestGenerateWithoutCustomGenerators_MakefileUnchanged pins the regression
// the feature must not cause: a project that declares no generator gets a
// Makefile with no trace of the feature.
func TestGenerateWithoutCustomGenerators_MakefileUnchanged(t *testing.T) {
	tmpDir := generateFromConfig(t, "metrics-map", "Generate project without custom generators")

	makefile := readGenerated(t, tmpDir, "Makefile")

	if strings.Contains(makefile, "# Generator '") {
		t.Error("Makefile carries a custom-generator block for a project that declares none")
	}

	prereqs, _, found := makeTarget(t, makefile, "lint")
	if !found {
		t.Fatal("lint target is missing")
	}

	if prereqs != "install-lint" {
		t.Errorf("lint prerequisites = %q, want exactly \"install-lint\"", prereqs)
	}

	prereqs, _, found = makeTarget(t, makefile, "install-tools")
	if !found {
		t.Fatal("install-tools target is missing")
	}

	if !strings.HasSuffix(prereqs, "install-goimports") {
		t.Errorf("install-tools prerequisites = %q, want them to end with install-goimports", prereqs)
	}
}
