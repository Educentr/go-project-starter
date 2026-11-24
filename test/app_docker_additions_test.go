package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGenerateAppDockerAdditions covers the project-declared additions to the
// runtime stage of an application image — `copy`, `env`, `fetch` and
// `stages` — on the consumer case of Educentr/go-project-starter#14: a project
// directory with an ENV pointing at it, a release binary fetched by URL with
// the architecture in its file name and a digest per platform, and a CLI from
// npm installed in a stage of its own.
//
// The test config is test/docker-integration/configs/app-docker-additions.
func TestGenerateAppDockerAdditions(t *testing.T) {
	tmpDir := generateProject(t, "app-docker-additions")

	dockerfile := readGenerated(t, tmpDir, "Dockerfile-api")

	builder, final, ok := strings.Cut(dockerfile, "\nFROM ubuntu:24.04\n")
	if !ok {
		t.Fatalf("Dockerfile-api has no runtime stage:\n%s", dockerfile)
	}

	// A project directory is copied straight from the build context into the
	// runtime stage. Nothing about it reaches the builder, where a destination
	// like /usr/local would overwrite the Go toolchain and then be copied on.
	if strings.Contains(builder, "/opt/api/template") {
		t.Errorf("builder stage carries the project directory of a copy entry:\n%s", builder)
	}

	// A digest per platform: a stage per declared architecture plus a selector
	// that resolves TARGETARCH, because a Dockerfile cannot choose between
	// digests inside one stage.
	for _, want := range []string{
		"\nFROM scratch AS fetch-tool-amd64\n",
		"\nFROM scratch AS fetch-tool-arm64\n",
		"\nFROM fetch-tool-$TARGETARCH AS fetch-tool\n",
		"ADD --checksum=sha256:1111111111111111111111111111111111111111111111111111111111111111 --chmod=0755 " +
			"https://github.com/org/tool/releases/download/v1.2.3/tool-linux-${TARGETARCH} /fetched/tool\n",
		"ADD --checksum=sha256:2222222222222222222222222222222222222222222222222222222222222222 --chmod=0755 " +
			"https://github.com/org/tool/releases/download/v1.2.3/tool-linux-${TARGETARCH} /fetched/tool\n",
	} {
		if !strings.Contains(dockerfile, want) {
			t.Errorf("Dockerfile-api does not contain %q:\n%s", want, dockerfile)
		}
	}

	// The auxiliary stage is pinned to the platform of the build host, and the
	// target architecture is available to its commands as an ARG — so a
	// multi-platform build runs it once, natively, and anything
	// architecture-dependent has to name the target explicitly.
	for _, want := range []string{
		"\nFROM --platform=$BUILDPLATFORM node:22-alpine AS stage-npmtools\n",
		"\nARG TARGETARCH\nARG TARGETOS\nWORKDIR /opt/npm\n",
		"\nRUN npm install -g --prefix /opt/npm --os=${TARGETOS} " +
			"--cpu=$([ \"${TARGETARCH}\" = amd64 ] && echo x64 || echo \"${TARGETARCH}\") some-cli@3.3.3\n",
	} {
		if !strings.Contains(dockerfile, want) {
			t.Errorf("Dockerfile-api does not contain %q:\n%s", want, dockerfile)
		}
	}

	assertStagesPrecedeBuilder(t, dockerfile, "fetch-tool-amd64", "fetch-tool-arm64", "fetch-tool", "stage-npmtools")

	// What the new keys add to the runtime stage is copies and one ENV — no
	// RUN. The only RUN of this stage is the package install, which is what
	// keeps a shell-less base image buildable and a cross-built image free of
	// emulation.
	if got := strings.Count(final, "\nRUN "); got != 1 {
		t.Errorf("runtime stage has %d RUN instructions, want exactly 1 (the package install):\n%s", got, final)
	}

	for _, want := range []string{
		"\nCOPY template /opt/api/template\n",
		"\nCOPY --from=fetch-tool /fetched/tool /usr/local/bin/tool\n",
		"\nCOPY --from=stage-npmtools /opt/npm /usr/local\n",
		"\nENV API_TEMPLATE_DIR=\"/opt/api/template\" \\\n    API_TOOL=\"/usr/local/bin/tool\"\n",
	} {
		if !strings.Contains(final, want) {
			t.Errorf("runtime stage does not contain %q:\n%s", want, final)
		}
	}

	// One ENV instruction, so one layer.
	if got := strings.Count(final, "\nENV "); got != 1 {
		t.Errorf("runtime stage has %d ENV instructions, want exactly 1:\n%s", got, final)
	}

	assertFetchStageHasNoRun(t, dockerfile)
	assertSingleDigestFetch(t, tmpDir)
	assertHistoricalRuntimeStage(t, tmpDir)
}

// assertStagesPrecedeBuilder checks that every generated auxiliary stage is
// declared above the builder stage. A `COPY --from` in the runtime stage only
// resolves to a stage declared earlier in the file, so the position is part of
// the contract and not cosmetic.
func assertStagesPrecedeBuilder(t *testing.T, dockerfile string, stages ...string) {
	t.Helper()

	builderAt := strings.Index(dockerfile, "AS builder\n")
	if builderAt < 0 {
		t.Fatalf("Dockerfile has no builder stage:\n%s", dockerfile)
	}

	for _, stage := range stages {
		at := strings.Index(dockerfile, "AS "+stage+"\n")
		if at < 0 {
			t.Errorf("Dockerfile declares no stage %q:\n%s", stage, dockerfile)
			continue
		}

		if at > builderAt {
			t.Errorf("stage %q is declared below the builder stage, a COPY --from would not resolve it", stage)
		}
	}
}

// assertFetchStageHasNoRun checks that a fetch stage carries nothing but the
// download. It is built for the target platform, so a RUN in it would need
// emulation — the invariant the download exists to avoid.
func assertFetchStageHasNoRun(t *testing.T, dockerfile string) {
	t.Helper()

	_, fetchStages, ok := strings.Cut(dockerfile, "\nFROM scratch AS fetch-tool-amd64\n")
	if !ok {
		t.Fatalf("Dockerfile-api has no fetch stage:\n%s", dockerfile)
	}

	fetchStages, _, ok = strings.Cut(fetchStages, "\nFROM fetch-tool-$TARGETARCH")
	if !ok {
		t.Fatalf("Dockerfile-api has no variant selector:\n%s", dockerfile)
	}

	if strings.Contains(fetchStages, "RUN ") {
		t.Errorf("fetch stage contains a RUN, which a multi-platform build would have to emulate:\n%s", fetchStages)
	}
}

// assertSingleDigestFetch covers the other checksum form: one digest for a URL
// that does not depend on the platform, with an explicit file mode. The
// application declares no packages either, so its runtime stage has to stay
// free of RUN altogether.
func assertSingleDigestFetch(t *testing.T, tmpDir string) {
	t.Helper()

	dockerfile := readGenerated(t, tmpDir, "Dockerfile-fixed")

	want := "\nFROM scratch AS fetch-helper\nARG TARGETARCH\nARG TARGETOS\n" +
		"ADD --checksum=sha256:3333333333333333333333333333333333333333333333333333333333333333 " +
		"--chmod=0700 https://example.com/helper /fetched/helper\n"
	if !strings.Contains(dockerfile, want) {
		t.Errorf("Dockerfile-fixed does not contain %q:\n%s", want, dockerfile)
	}

	// No variant stage and no selector: one digest covers every build.
	for _, unwanted := range []string{"fetch-helper-", "$TARGETARCH AS"} {
		if strings.Contains(dockerfile, unwanted) {
			t.Errorf("Dockerfile-fixed contains %q for a single digest:\n%s", unwanted, dockerfile)
		}
	}

	_, final, ok := strings.Cut(dockerfile, "\nFROM alpine\n")
	if !ok {
		t.Fatalf("Dockerfile-fixed has no runtime stage:\n%s", dockerfile)
	}

	if strings.Contains(final, "\nRUN ") {
		t.Errorf("runtime stage of an application with a fetch entry contains a RUN:\n%s", final)
	}

	if !strings.Contains(final, "\nCOPY --from=fetch-helper /fetched/helper /usr/local/bin/helper\n") {
		t.Errorf("runtime stage does not copy the fetched file:\n%s", final)
	}
}

// assertHistoricalRuntimeStage pins the no-op case: an application that
// declares none of the new keys gets the runtime stage every generated project
// had before they existed, instruction for instruction.
func assertHistoricalRuntimeStage(t *testing.T, tmpDir string) {
	t.Helper()

	dockerfile := readGenerated(t, tmpDir, "Dockerfile-plain")

	_, final, ok := strings.Cut(dockerfile, "\nFROM alpine\n")
	if !ok {
		t.Fatalf("Dockerfile-plain has no runtime stage:\n%s", dockerfile)
	}

	want := "COPY --from=builder /service /service\n" +
		"COPY --from=builder /public /public\n" +
		"# Copying the directory from a stage keeps the ownership and mode it has there\n" +
		"# (root:root, 0755) — the same a `mkdir -p` in this stage would have given.\n" +
		"COPY --from=builder /etc/onlineconf /etc/onlineconf\n" +
		"CMD [\"/service\"]\n"
	if !strings.HasPrefix(final, want) {
		t.Errorf("Dockerfile-plain runtime stage is not the historical one:\nwant prefix:\n%s\ngot:\n%s", want, final)
	}

	// None of the stages or instructions the new keys generate reaches a
	// project that declares none of them.
	for _, unwanted := range []string{"AS fetch-", "AS stage-", "\nENV API_", "/fetched/"} {
		if strings.Contains(dockerfile, unwanted) {
			t.Errorf("Dockerfile-plain contains %q without the keys that generate it:\n%s", unwanted, dockerfile)
		}
	}
}

// TestGenerateWithoutDockerAdditions pins the no-op case on a project that
// declares no runtime addition at all: regenerating an existing project adds
// neither a stage nor an instruction to any of its Dockerfiles.
func TestGenerateWithoutDockerAdditions(t *testing.T) {
	tmpDir := generateProject(t, "app-docker-runtime")

	for _, app := range []string{"api", "slim", "plain"} {
		dockerfile := readGenerated(t, tmpDir, "Dockerfile-"+app)

		for _, unwanted := range []string{"AS fetch-", "AS stage-", "/fetched/", "--checksum=", "--chmod="} {
			if strings.Contains(dockerfile, unwanted) {
				t.Errorf("Dockerfile-%s contains %q without the keys that generate it:\n%s", app, unwanted, dockerfile)
			}
		}

		// ENV is generated in the builder stage (CGO_ENABLED) in every
		// project; the runtime stage gets one only from the `env` key.
		_, final, ok := strings.Cut(dockerfile, "\nFROM ")
		if !ok {
			t.Fatalf("Dockerfile-%s has no stage at all:\n%s", app, dockerfile)
		}

		if runtime := final[strings.LastIndex(final, "\nFROM "):]; strings.Contains(runtime, "\nENV ") {
			t.Errorf("runtime stage of Dockerfile-%s has an ENV without the env key:\n%s", app, runtime)
		}
	}
}

// TestGenerateDockerAdditionsWithoutImage verifies that an addition declared
// for an application that builds no docker artifact fails the generation. The
// keys shape the Dockerfile of the application and nothing else, so there would
// be nothing at all to generate from them — the same silent no-op the platform
// list rules out.
func TestGenerateDockerAdditionsWithoutImage(t *testing.T) {
	curDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting current directory: %v", err)
	}

	fixture := filepath.Join(curDir, "..", "test", "docker-integration", "configs", "app-docker-additions")
	configDir := t.TempDir()

	data, err := os.ReadFile(filepath.Join(fixture, "api.swagger.yml"))
	if err != nil {
		t.Fatalf("Error reading fixture spec: %v", err)
	}

	if werr := os.WriteFile(filepath.Join(configDir, "api.swagger.yml"), data, 0o600); werr != nil {
		t.Fatalf("Error writing spec: %v", werr)
	}

	// Same project as the fixture, but the application that declares a fetch
	// entry builds a deb package instead of an image.
	project, err := os.ReadFile(filepath.Join(fixture, "project.yaml"))
	if err != nil {
		t.Fatalf("Error reading fixture config: %v", err)
	}

	broken := strings.Replace(string(project),
		"  - name: fixed\n    transport:\n",
		"  - name: fixed\n    artifacts: [deb]\n    transport:\n", 1)
	if broken == string(project) {
		t.Fatal("fixture config changed shape, the artifacts override was not applied")
	}

	if werr := os.WriteFile(filepath.Join(configDir, "project.yaml"), []byte(broken), 0o600); werr != nil {
		t.Fatalf("Error writing config: %v", werr)
	}

	out, err := ExecCommand(filepath.Join(curDir, ".."), "go", []string{
		"run", filepath.Join(curDir, "..", "cmd", "go-project-starter", "main.go"),
		"--target", t.TempDir(),
		"--configDir", configDir,
		"--config", "project.yaml",
	}, "Generate project with a fetch entry on an application without an image")

	if err == nil {
		t.Fatalf("generation succeeded with a fetch entry declared for an application that builds no image\n%s", out)
	}

	for _, want := range []string{"fixed", "docker artifact"} {
		if !strings.Contains(out, want) {
			t.Errorf("generator error should mention %q, got:\n%s", want, out)
		}
	}
}
