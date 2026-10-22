package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	cfg "github.com/Educentr/go-project-starter/internal/pkg/config"
	"github.com/Educentr/go-project-starter/internal/pkg/ds"
	"github.com/Educentr/go-project-starter/internal/pkg/meta"
	"github.com/pkg/errors"
)

func TestProcessGenerators_RendersCommandsPerContract(t *testing.T) {
	config := cfg.Config{
		GeneratorList: cfg.CustomGeneratorList{
			{
				Name:    "limits_ts",
				Install: "npm ci --prefix web",
				Cmd:     "node web/gen.mjs {{ .Spec }} --out {{ .Target }}",
				Lint:    "npx --prefix web spec-lint {{ .Spec }}",
			},
			{
				Name: "codex_types",
				Cmd:  "go run ./internal/cmd/codextypes {{ .Spec }}",
			},
		},
		ContractList: cfg.ContractList{
			{Path: "./limits.yaml", Copy: "api/contract/limits.yaml", Target: "web/src/limits.ts", GeneratorName: "limits_ts"},
			{Path: "./limits_extra.yaml", Copy: "api/contract/limits_extra.yaml", Target: "web/src/limitsExtra.ts", GeneratorName: "limits_ts"},
			{Path: "./codex.yaml", Copy: "api/contract/codex.yaml", GeneratorName: "codex_types"},
		},
	}

	g := Generator{}
	if err := g.processGenerators(config); err != nil {
		t.Fatalf("processGenerators() error = %v", err)
	}

	if len(g.CustomGenerators) != 2 {
		t.Fatalf("got %d generators, want 2", len(g.CustomGenerators))
	}

	// Declaration order of project.yaml is kept so the Makefile is stable
	// between runs.
	limits := g.CustomGenerators[0]
	if limits.Name != "limits_ts" {
		t.Errorf("first generator = %q, want limits_ts", limits.Name)
	}

	if limits.Install != "npm ci --prefix web" {
		t.Errorf("install = %q", limits.Install)
	}

	wantCmds := []string{
		"node web/gen.mjs api/contract/limits.yaml --out web/src/limits.ts",
		"node web/gen.mjs api/contract/limits_extra.yaml --out web/src/limitsExtra.ts",
	}

	if len(limits.Cmds) != len(wantCmds) {
		t.Fatalf("got %d cmds, want %d: %v", len(limits.Cmds), len(wantCmds), limits.Cmds)
	}

	for i, want := range wantCmds {
		if limits.Cmds[i] != want {
			t.Errorf("cmds[%d] = %q, want %q", i, limits.Cmds[i], want)
		}
	}

	if len(limits.Lints) != 2 {
		t.Fatalf("got %d lints, want 2: %v", len(limits.Lints), limits.Lints)
	}

	if limits.Lints[0] != "npx --prefix web spec-lint api/contract/limits.yaml" {
		t.Errorf("lints[0] = %q", limits.Lints[0])
	}

	// A generator declaring only `cmd` must not acquire an install or lint
	// command — the Makefile keys its install-/lint- targets off these.
	codex := g.CustomGenerators[1]
	if codex.Install != "" || len(codex.Lints) != 0 {
		t.Errorf("cmd-only generator picked up install=%q lints=%v", codex.Install, codex.Lints)
	}

	if len(codex.Cmds) != 1 || codex.Cmds[0] != "go run ./internal/cmd/codextypes api/contract/codex.yaml" {
		t.Errorf("cmd-only generator cmds = %v", codex.Cmds)
	}

	if len(g.Contracts) != 3 {
		t.Fatalf("got %d contracts, want 3", len(g.Contracts))
	}

	// RawPath survives resolution so the manifest can record the source the
	// project declared rather than the staging path it was fetched into.
	if g.Contracts[0].RawPath != "./limits.yaml" || g.Contracts[0].SpecPath != "./limits.yaml" {
		t.Errorf("contract[0] = %+v", g.Contracts[0])
	}
}

func TestProcessGenerators_EmptySectionsLeaveStateNil(t *testing.T) {
	g := Generator{}

	if err := g.processGenerators(cfg.Config{}); err != nil {
		t.Fatalf("processGenerators() error = %v", err)
	}

	if g.CustomGenerators != nil || g.Contracts != nil {
		t.Errorf("empty sections produced CustomGenerators=%v Contracts=%v, want both nil",
			g.CustomGenerators, g.Contracts)
	}
}

func TestProcessGenerators_UnusedGeneratorGetsNoCommands(t *testing.T) {
	g := Generator{}

	config := cfg.Config{
		GeneratorList: cfg.CustomGeneratorList{{Name: "unused", Cmd: "gen {{ .Spec }}"}},
	}

	if err := g.processGenerators(config); err != nil {
		t.Fatalf("processGenerators() error = %v", err)
	}

	if len(g.CustomGenerators) != 1 || len(g.CustomGenerators[0].Cmds) != 0 {
		t.Errorf("unused generator = %+v, want one entry with no cmds", g.CustomGenerators)
	}
}

// writeContractCopy writes content at rel inside dir and returns its hash.
func writeContractCopy(t *testing.T, dir, rel, content string) string {
	t.Helper()

	path := filepath.Join(dir, filepath.FromSlash(rel))

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	return meta.HashContent([]byte(content))
}

func TestCheckContractDrift(t *testing.T) {
	const (
		rel      = "api/contract/limits.yaml"
		original = "limits:\n  cpu_millis: 500\n"
	)

	t.Run("untouched copy passes", func(t *testing.T) {
		dir := t.TempDir()
		hash := writeContractCopy(t, dir, rel, original)

		g := Generator{
			Contracts: ds.Contracts{{RawPath: "./limits.yaml", Copy: rel, Source: "./limits.yaml"}},
			Meta:      meta.Meta{Manifest: meta.Manifest{{Path: rel, Kind: meta.KindCopied, Hash: hash}}},
		}

		if err := g.checkContractDrift(dir); err != nil {
			t.Fatalf("checkContractDrift() error = %v", err)
		}
	})

	t.Run("edited copy fails", func(t *testing.T) {
		dir := t.TempDir()
		hash := writeContractCopy(t, dir, rel, original)
		writeContractCopy(t, dir, rel, original+"  disk_mb: 1\n")

		g := Generator{
			Contracts: ds.Contracts{{RawPath: "./limits.yaml", Copy: rel, Source: "git+ssh://git@host/org/contracts.git@v1#limits.yaml"}},
			Meta:      meta.Meta{Manifest: meta.Manifest{{Path: rel, Kind: meta.KindCopied, Hash: hash}}},
		}

		err := g.checkContractDrift(dir)
		if !errors.Is(err, errContractDrift) {
			t.Fatalf("checkContractDrift() error = %v, want errContractDrift", err)
		}

		// The message has to name both the file and where it came from:
		// the fix is a change in the contract repository.
		for _, want := range []string{rel, "git+ssh://git@host/org/contracts.git@v1#limits.yaml"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q should mention %q", err, want)
			}
		}
	})

	t.Run("deleted copy is restored, not reported", func(t *testing.T) {
		dir := t.TempDir()

		g := Generator{
			Contracts: ds.Contracts{{RawPath: "./limits.yaml", Copy: rel}},
			Meta:      meta.Meta{Manifest: meta.Manifest{{Path: rel, Kind: meta.KindCopied, Hash: "deadbeef"}}},
		}

		if err := g.checkContractDrift(dir); err != nil {
			t.Fatalf("checkContractDrift() error = %v", err)
		}
	})

	t.Run("first run has no manifest record", func(t *testing.T) {
		dir := t.TempDir()
		writeContractCopy(t, dir, rel, "whatever the user left here\n")

		g := Generator{
			Contracts: ds.Contracts{{RawPath: "./limits.yaml", Copy: rel}},
		}

		if err := g.checkContractDrift(dir); err != nil {
			t.Fatalf("checkContractDrift() error = %v", err)
		}
	})
}

func TestCopyContracts_RecordsProvenance(t *testing.T) {
	src := t.TempDir()
	spec := filepath.Join(src, "limits.yaml")

	if err := os.WriteFile(spec, []byte("limits: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	target := t.TempDir()

	g := Generator{
		Contracts: ds.Contracts{{
			RawPath:  "git+ssh://git@github.com/org/contracts.git#limits/limits.yaml",
			SpecPath: spec,
			Copy:     "api/contract/limits.yaml",
			Source:   "ssh://git@github.com/org/contracts.git@HEAD#limits/limits.yaml",
			Ref:      "main",
			Commit:   "0123456789abcdef0123456789abcdef01234567",
		}},
	}

	records, err := g.copyContracts(target)
	if err != nil {
		t.Fatalf("copyContracts() error = %v", err)
	}

	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}

	rec := records[0]

	if rec.Path != "api/contract/limits.yaml" || rec.Kind != meta.KindCopied {
		t.Errorf("record = %+v", rec)
	}

	if rec.Hash != meta.HashContent([]byte("limits: {}\n")) {
		t.Errorf("hash = %q, want the hash of the copied content", rec.Hash)
	}

	if rec.Ref != "main" || rec.Commit != "0123456789abcdef0123456789abcdef01234567" {
		t.Errorf("provenance = ref %q commit %q", rec.Ref, rec.Commit)
	}

	copied, err := os.ReadFile(filepath.Join(target, "api", "contract", "limits.yaml"))
	if err != nil {
		t.Fatalf("copied spec not written: %v", err)
	}

	if string(copied) != "limits: {}\n" {
		t.Errorf("copied content = %q", copied)
	}
}

func TestCopyContracts_MissingResolvedSpec(t *testing.T) {
	g := Generator{
		Contracts: ds.Contracts{{
			RawPath:  "./limits.yaml",
			SpecPath: filepath.Join(t.TempDir(), "gone.yaml"),
			Copy:     "api/contract/limits.yaml",
		}},
	}

	if _, err := g.copyContracts(t.TempDir()); !errors.Is(err, errSpecNotFound) {
		t.Fatalf("copyContracts() error = %v, want errSpecNotFound", err)
	}
}

func TestHasGitSource_Contracts(t *testing.T) {
	g := Generator{
		Contracts: ds.Contracts{{RawPath: "./limits.yaml"}},
	}

	if g.hasGitSource() {
		t.Error("local-only contract should not require git")
	}

	g.Contracts = append(g.Contracts, ds.Contract{
		RawPath: "git+https://github.com/org/contracts.git#limits/limits.yaml",
	})

	if !g.hasGitSource() {
		t.Error("git contract should require git")
	}
}

func TestCheckContractCollisions(t *testing.T) {
	generated := meta.Manifest{
		{Path: "Makefile", Kind: meta.KindGenerated, Hash: "h"},
		{Path: "cmd/api/psg_main_gen.go", Kind: meta.KindGenerated, Hash: "h"},
	}

	t.Run("no collision", func(t *testing.T) {
		g := Generator{Contracts: ds.Contracts{{RawPath: "./a.yaml", Copy: "api/contract/a.yaml"}}}

		if err := g.checkContractCollisions(generated); err != nil {
			t.Fatalf("checkContractCollisions() error = %v", err)
		}
	})

	t.Run("contract copies over a generated file", func(t *testing.T) {
		g := Generator{Contracts: ds.Contracts{{RawPath: "./a.yaml", Copy: "Makefile"}}}

		err := g.checkContractCollisions(generated)
		if !errors.Is(err, cfg.ErrInvalidConfig) {
			t.Fatalf("checkContractCollisions() error = %v, want ErrInvalidConfig", err)
		}

		if !strings.Contains(err.Error(), "Makefile") {
			t.Errorf("error %q should name the colliding path", err)
		}
	})
}
