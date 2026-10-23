package test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// generateFromExample runs the generator over the example/ directory, which is
// the only fixture declaring a jsonschema section.
func generateFromExample(t *testing.T, msg string) string {
	t.Helper()

	curDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting current directory: %v", err)
	}

	tmpDir := t.TempDir()

	out, err := ExecCommand(filepath.Join(curDir, ".."), "go", []string{
		"run", filepath.Join(curDir, "..", "cmd", "go-project-starter", "main.go"),
		"--target", tmpDir,
		"--configDir", filepath.Join(curDir, "..", "example"),
		"--config", "project.yaml",
	}, msg+" ("+tmpDir+")")
	if err != nil {
		t.Fatalf("Error creating project: %s\n%s", err, out)
	}

	return tmpDir
}

// TestMakefileToolGuards covers the install guards for the tools the generated
// Makefile calls as bare binaries (TOOLS-47 / issue #77). Without them
// `make regenerate` dies with "No such file or directory" (Error 127) halfway
// through the generate chain, leaving a partially regenerated tree.
func TestMakefileToolGuards(t *testing.T) {
	t.Run("GOPATH/bin is appended to PATH", func(t *testing.T) {
		makefile := readGenerated(t, generateFromConfig(t, "worker-telegram", "Generate project for PATH test"), "Makefile")

		// The install-* targets write into $(GOPATH)/bin, which is not on PATH
		// in a typical CI container — without this the guards below would
		// install a tool that still cannot be called by name.
		if !strings.Contains(makefile, "export PATH := $(PATH):$(GOBIN_DIR)") {
			t.Error("generated Makefile must export PATH with $(GOBIN_DIR) appended")
		}

		// Appended, not prepended: a binary baked into a toolchain image must
		// keep priority over a copy in GOBIN_DIR.
		if strings.Contains(makefile, "export PATH := $(GOBIN_DIR):$(PATH)") {
			t.Error("GOBIN_DIR must be appended to PATH, not prepended")
		}
	})

	t.Run("goimports", func(t *testing.T) {
		makefile := readGenerated(t, generateFromConfig(t, "worker-telegram", "Generate project for goimports guard test"), "Makefile")

		// clean-import is a post_generate step, so an Error 127 here also
		// lands after the generated files have been written.
		assertGuardedTarget(t, makefile, "clean-import: ensure-goimports", "ensure-goimports", "goimports", "install-goimports")
	})

	t.Run("go-jsonschema", func(t *testing.T) {
		makefile := readGenerated(t, generateFromExample(t, "Generate project with jsonschema"), "Makefile")

		assertGuardedTarget(t, makefile, "generate-jsonschema: ensure-go-jsonschema", "ensure-go-jsonschema", "go-jsonschema", "install-go-jsonschema")
	})

	t.Run("buf and local protoc plugins", func(t *testing.T) {
		makefile := readGenerated(t, generateFromConfig(t, "grpc-only", "Generate project with buf_local_plugins"), "Makefile")

		// With buf_local_plugins the proto target calls buf directly, and buf
		// in turn shells out to protoc-gen-go/protoc-gen-go-grpc by name.
		assertGuardedTarget(t, makefile, "proto: ensure-buf ensure-protoc-plugins", "ensure-buf", "buf", "install-buf")

		if !strings.Contains(makefile, "ensure-protoc-plugins:\n\t@{ command -v protoc-gen-go && command -v protoc-gen-go-grpc; } >/dev/null 2>&1 || $(MAKE) --no-print-directory install-protoc-gen-go-grpc") {
			t.Error("ensure-protoc-plugins must probe both plugins before installing them")
		}
	})

	// A project that uses neither feature must not gain targets referring to
	// install-* targets that are not rendered for it.
	t.Run("guards are not rendered for projects without the tool", func(t *testing.T) {
		makefile := readGenerated(t, generateFromConfig(t, "worker-telegram", "Generate project without jsonschema or grpc"), "Makefile")

		for _, unexpected := range []string{"ensure-go-jsonschema", "ensure-buf", "ensure-protoc-plugins"} {
			if strings.Contains(makefile, unexpected) {
				t.Errorf("generated Makefile should NOT contain %q for a project without jsonschema or gRPC", unexpected)
			}
		}

		// The docker-based proto path does not need buf on PATH.
		if !strings.Contains(makefile, "\nproto:\n") {
			t.Error("proto target must have no prerequisites when buf_local_plugins is not used")
		}
	})
}

// assertGuardedTarget checks that target declares the guard as a prerequisite
// and that the guard probes the tool instead of installing it unconditionally:
// toolchain images bake these binaries on the same pins, so an unconditional
// install would hit the network on every run and shadow the baked binary with
// a copy in GOBIN_DIR.
func assertGuardedTarget(t *testing.T, makefile, targetLine, guard, tool, installTarget string) {
	t.Helper()

	if !strings.Contains(makefile, targetLine+"\n") {
		t.Errorf("generated Makefile must declare %q", targetLine)
	}

	wantGuard := guard + ":\n\t@command -v " + tool + " >/dev/null 2>&1 || $(MAKE) --no-print-directory " + installTarget

	if !strings.Contains(makefile, wantGuard) {
		t.Errorf("generated Makefile must define %s as a probe-then-install guard, want recipe:\n%s", guard, wantGuard)
	}

	if !strings.Contains(makefile, ".PHONY: "+guard+"\n") {
		t.Errorf("%s must be declared .PHONY", guard)
	}
}

// TestMakefileToolGuardsBehavior runs the rendered Makefile to check the two
// cases the guards exist for: a tool reachable only through $(GOPATH)/bin must
// be found, and a tool already on PATH must not be reinstalled.
func TestMakefileToolGuardsBehavior(t *testing.T) {
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("make not available")
	}

	// A real goimports anywhere on PATH would satisfy the guard instead of the
	// stub below, making the result meaningless. Drop the directories holding
	// it, as long as the Makefile still has the tools it probes at parse time.
	sandboxPath, ok := pathWithoutTool(t, "goimports")
	if !ok {
		t.Skip("cannot hide the installed goimports without losing go/make/git from PATH")
	}

	goModCache := hostGoModCache(t)

	tmpDir := generateFromConfig(t, "worker-telegram", "Generate project for guard behavior test")

	// Called with "goimports -w ." by clean-import; records the call so the
	// test can tell which copy ran, and whether it ran at all.
	const stub = "#!/bin/sh\nprintf '%s\\n' \"$0\" >> \"$GUARD_MARKER\"\n"

	runCleanImport := func(t *testing.T, gopath, extraPathDir string) (marker string, gopathBin []os.DirEntry) {
		t.Helper()

		marker = filepath.Join(t.TempDir(), "calls")
		path := sandboxPath

		if extraPathDir != "" {
			path = extraPathDir + string(os.PathListSeparator) + path
		}

		// GOPATH is read back by the Makefile via `go env GOPATH`.
		//
		// GOMODCACHE is kept out of that throwaway GOPATH on purpose. Only
		// GOPATH/bin is under test; pkg/mod would just inherit it. When the
		// generated go.mod targets a Go newer than the host toolchain, the go
		// command materializes the matching toolchain there, and a module
		// cache is written read-only — t.TempDir cleanup then fails with
		// "permission denied". Reusing the real cache also avoids
		// re-downloading that toolchain for every subtest.
		env := append(os.Environ(),
			"GOPATH="+gopath,
			"GOMODCACHE="+goModCache,
			"PATH="+path,
			"GUARD_MARKER="+marker,
		)

		cmd := exec.Command("make", "clean-import")
		cmd.Dir = tmpDir
		cmd.Env = env

		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("make clean-import failed: %v\n%s", err, out)
		}

		entries, err := os.ReadDir(filepath.Join(gopath, "bin"))
		if err != nil {
			t.Fatalf("Error reading %s/bin: %v", gopath, err)
		}

		return marker, entries
	}

	t.Run("tool installed in GOPATH/bin is callable although GOPATH/bin is not on PATH", func(t *testing.T) {
		gopath := t.TempDir()
		writeStub(t, filepath.Join(gopath, "bin", "goimports"), stub)

		marker, _ := runCleanImport(t, gopath, "")

		if calls := readCalls(t, marker); len(calls) != 1 {
			t.Fatalf("goimports from GOPATH/bin was called %d times, want 1 (calls: %v)", len(calls), calls)
		}
	})

	t.Run("tool already on PATH is used as is, nothing is installed", func(t *testing.T) {
		gopath := t.TempDir()

		if err := os.MkdirAll(filepath.Join(gopath, "bin"), 0o755); err != nil {
			t.Fatalf("Error creating GOPATH/bin: %v", err)
		}

		bakedDir := t.TempDir()
		writeStub(t, filepath.Join(bakedDir, "goimports"), stub)

		marker, gopathBin := runCleanImport(t, gopath, bakedDir)

		calls := readCalls(t, marker)
		if len(calls) != 1 || !strings.HasPrefix(calls[0], bakedDir) {
			t.Errorf("clean-import must call the goimports from %s exactly once, calls: %v", bakedDir, calls)
		}

		// No `go install` ran, so nothing landed in GOPATH/bin that could
		// shadow the binary baked into the image.
		if len(gopathBin) != 0 {
			t.Errorf("GOPATH/bin must stay empty when the tool is already on PATH, got %v", gopathBin)
		}
	})
}

// hostGoModCache reports the module cache of the host toolchain, so a test
// running the go command under a throwaway GOPATH can keep writing there
// instead of filling (and then failing to clean up) its temporary directory.
func hostGoModCache(t *testing.T) string {
	t.Helper()

	out, err := exec.Command("go", "env", "GOMODCACHE").Output()
	if err != nil {
		t.Fatalf("Error reading GOMODCACHE: %v", err)
	}

	cache := strings.TrimSpace(string(out))
	if cache == "" {
		t.Fatal("go env GOMODCACHE is empty")
	}

	return cache
}

// pathWithoutTool returns PATH with every directory containing an executable
// named tool removed. ok is false when that would also remove a tool the
// generated Makefile itself needs.
func pathWithoutTool(t *testing.T, tool string) (path string, ok bool) {
	t.Helper()

	kept := make([]string, 0)

	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			continue
		}

		if isExecutable(filepath.Join(dir, tool)) {
			continue
		}

		kept = append(kept, dir)
	}

	for _, needed := range []string{"go", "make", "git", "sh"} {
		found := false

		for _, dir := range kept {
			if isExecutable(filepath.Join(dir, needed)) {
				found = true

				break
			}
		}

		if !found {
			return "", false
		}
	}

	return strings.Join(kept, string(os.PathListSeparator)), true
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)

	return err == nil && !info.IsDir() && info.Mode().Perm()&0o111 != 0
}

func writeStub(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("Error creating %s: %v", filepath.Dir(path), err)
	}

	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("Error writing stub %s: %v", path, err)
	}
}

func readCalls(t *testing.T, marker string) []string {
	t.Helper()

	data, err := os.ReadFile(marker)
	if os.IsNotExist(err) {
		return nil
	}

	if err != nil {
		t.Fatalf("Error reading marker %s: %v", marker, err)
	}

	// One path per line: a temp directory may contain spaces.
	calls := make([]string, 0)

	for _, line := range strings.Split(string(data), "\n") {
		if line != "" {
			calls = append(calls, line)
		}
	}

	return calls
}
