package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makeRecipe returns the lines of the given make target: its comment block, the
// target line and the recipe, so a test can assert on one target without
// matching the rest of the Makefile.
func makeRecipe(t *testing.T, makefile, target string) string {
	t.Helper()

	lines := strings.Split(makefile, "\n")

	start := -1

	for i, line := range lines {
		if strings.HasPrefix(line, target+":") {
			start = i

			break
		}
	}

	if start < 0 {
		t.Fatalf("Makefile has no target %q", target)
	}

	// Walk back over the comment block and the .PHONY declaration of the
	// target: both are generated with it and carry the flags' rationale.
	for start > 0 && (strings.HasPrefix(lines[start-1], "#") || strings.HasPrefix(lines[start-1], ".PHONY:")) {
		start--
	}

	end := start

	for end < len(lines) && !(end > start && lines[end] == "") {
		end++
	}

	return strings.Join(lines[start:end], "\n")
}

// TestGenerateDockerPlatforms covers the applications[].docker.platforms key on
// a project that declares it for two of its three applications: a multi-platform
// image (an index), a single cross-built platform and an application without the
// key at all.
//
// The config is test/docker-integration/configs/docker-platforms.
func TestGenerateDockerPlatforms(t *testing.T) {
	tmpDir := generateFromConfig(t, "docker-platforms", "Generate project with docker platforms")

	// --- Dockerfile of the multi-platform application -----------------------
	dockerfile := readGenerated(t, tmpDir, "Dockerfile-cli")

	builder, final, ok := strings.Cut(dockerfile, "\nFROM alpine\n")
	if !ok {
		t.Fatalf("Dockerfile-cli has no final stage:\n%s", dockerfile)
	}

	// The builder runs on the build host and cross-compiles: Go does that
	// itself, so the build of the binary needs no emulation.
	if !strings.Contains(builder, "FROM --platform=$BUILDPLATFORM golang:") {
		t.Errorf("builder stage does not pin itself to the build platform:\n%s", builder)
	}

	if !strings.Contains(builder, "\nARG TARGETARCH\n") {
		t.Errorf("builder stage does not declare ARG TARGETARCH:\n%s", builder)
	}

	// The architecture has to reach the Go build, and an empty TARGETARCH has
	// to fail the build instead of silently producing a binary for the
	// architecture of the builder.
	if !strings.Contains(builder, "GOARCH=${TARGETARCH:?") {
		t.Errorf("builder stage does not pass TARGETARCH to the Go build:\n%s", builder)
	}

	if !strings.Contains(builder, "make build_in_docker-cli") {
		t.Errorf("builder stage does not build the application:\n%s", builder)
	}

	// The runtime stage must not RUN anything: a RUN there runs for the target
	// architecture and so would need emulation of it.
	if strings.Contains(final, "RUN ") {
		t.Errorf("runtime stage of a cross-built image must not RUN anything:\n%s", final)
	}

	if !strings.Contains(final, "COPY --from=builder /etc/onlineconf /etc/onlineconf") {
		t.Errorf("runtime stage does not get /etc/onlineconf from the builder stage:\n%s", final)
	}

	if !strings.Contains(builder, "RUN mkdir -p /etc/onlineconf") {
		t.Errorf("builder stage does not create /etc/onlineconf for the runtime stage:\n%s", builder)
	}

	// --- Dockerfile of the application without the key ----------------------
	// Untouched: the historical single-platform build.
	plain := readGenerated(t, tmpDir, "Dockerfile-api")

	if strings.Contains(plain, "BUILDPLATFORM") || strings.Contains(plain, "TARGETARCH") {
		t.Errorf("Dockerfile-api declares no platforms but cross-compiles:\n%s", plain)
	}

	// /etc/onlineconf is created in the builder stage and copied into the
	// runtime stage in every project, with or without platforms.
	if !strings.Contains(plain, "\nRUN mkdir -p /etc/onlineconf\n") ||
		!strings.Contains(plain, "COPY --from=builder /etc/onlineconf /etc/onlineconf") {
		t.Errorf("Dockerfile-api does not carry /etc/onlineconf from the builder stage:\n%s", plain)
	}

	// --- Makefile -----------------------------------------------------------
	makefile := readGenerated(t, tmpDir, "Makefile")

	// The buildx builder: the default "docker" builder can build neither for
	// another platform nor an image index.
	if !strings.Contains(makefile, "\nBUILDX_BUILDER ?= $(SERVICE_NAME)-builder\n") {
		t.Error("Makefile does not declare BUILDX_BUILDER")
	}

	builderTarget := makeRecipe(t, makefile, "buildx-builder")

	for _, want := range []string{
		"docker buildx inspect $(BUILDX_BUILDER)",
		"docker buildx create --name $(BUILDX_BUILDER) --driver docker-container",
	} {
		if !strings.Contains(builderTarget, want) {
			t.Errorf("buildx-builder target misses %q:\n%s", want, builderTarget)
		}
	}

	// Several platforms: one buildx build of both, kept in the builder cache,
	// and a push that publishes the index under one tag.
	multi := makeRecipe(t, makefile, "docker-cli")

	for _, want := range []string{
		"docker-cli: buildx-builder",
		"docker buildx build .",
		"--platform linux/amd64,linux/arm64",
		"--output type=cacheonly",
		"-f ./Dockerfile-cli",
		"-t $(REGISTRY_SERVER)/$(SERVICE_NAME)-cli:${DOCKER_TAG}",
	} {
		if !strings.Contains(multi, want) {
			t.Errorf("docker-cli recipe misses %q:\n%s", want, multi)
		}
	}

	if strings.Contains(multi, "--load") {
		t.Errorf("a multi-platform build cannot --load an image index:\n%s", multi)
	}

	multiPush := makeRecipe(t, makefile, "docker-cli-push")

	for _, want := range []string{
		"docker-cli-push: buildx-builder",
		"docker buildx build .",
		"--platform linux/amd64,linux/arm64",
		"--push",
		"-t $(REGISTRY_SERVER)/$(SERVICE_NAME)-cli:${DOCKER_TAG}",
	} {
		if !strings.Contains(multiPush, want) {
			t.Errorf("docker-cli-push recipe misses %q:\n%s", want, multiPush)
		}
	}

	if strings.Contains(multiPush, "docker push ") {
		t.Errorf("an image index cannot be pushed from the local image store:\n%s", multiPush)
	}

	// One platform: a plain image, so it lands in the local store and the push
	// stays the `docker push` of the historical recipe.
	single := makeRecipe(t, makefile, "docker-tool")

	for _, want := range []string{
		"docker-tool: buildx-builder",
		"--platform linux/arm64",
		"--load",
	} {
		if !strings.Contains(single, want) {
			t.Errorf("docker-tool recipe misses %q:\n%s", want, single)
		}
	}

	singlePush := makeRecipe(t, makefile, "docker-tool-push")

	if !strings.Contains(singlePush, "docker push $(REGISTRY_SERVER)/$(SERVICE_NAME)-tool:${DOCKER_TAG}") {
		t.Errorf("docker-tool-push should push the loaded image:\n%s", singlePush)
	}

	if strings.Contains(singlePush, "buildx") {
		t.Errorf("docker-tool-push should not rebuild the image:\n%s", singlePush)
	}

	// The application without the key keeps the historical recipes.
	plainBuild := makeRecipe(t, makefile, "docker-api")

	if !strings.Contains(plainBuild, "DOCKER_BUILDKIT=1 docker build . -f ./Dockerfile-api --rm") {
		t.Errorf("docker-api recipe should be unchanged:\n%s", plainBuild)
	}

	if strings.Contains(plainBuild, "buildx") || strings.Contains(plainBuild, "--platform") {
		t.Errorf("docker-api declares no platforms and must not use buildx:\n%s", plainBuild)
	}
}

// TestGenerateWithoutDockerPlatforms pins the no-op case: a project that
// declares no platform gets neither the buildx builder nor a buildx call, so
// regenerating an existing project changes nothing about how it is built.
func TestGenerateWithoutDockerPlatforms(t *testing.T) {
	tmpDir := generateProject(t, "docker-build-args")

	makefile := readGenerated(t, tmpDir, "Makefile")

	for _, unwanted := range []string{"BUILDX_BUILDER", "buildx-builder", "docker buildx build", "--platform"} {
		if strings.Contains(makefile, unwanted) {
			t.Errorf("Makefile contains %q without docker.platforms", unwanted)
		}
	}

	dockerfile := readGenerated(t, tmpDir, "Dockerfile-api")

	for _, unwanted := range []string{"BUILDPLATFORM", "TARGETARCH"} {
		if strings.Contains(dockerfile, unwanted) {
			t.Errorf("Dockerfile-api contains %q without docker.platforms", unwanted)
		}
	}
}

// TestGenerateDockerPlatformsWithoutImage verifies that platforms declared for
// an application that builds no docker artifact fail the generation. The key
// shapes the recipes of the image, so there would be nothing at all to generate
// from it — the silent no-op the generation has to rule out.
func TestGenerateDockerPlatformsWithoutImage(t *testing.T) {
	curDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting current directory: %v", err)
	}

	fixture := filepath.Join(curDir, "..", "test", "docker-integration", "configs", "docker-platforms")
	configDir := t.TempDir()

	data, err := os.ReadFile(filepath.Join(fixture, "api.swagger.yml"))
	if err != nil {
		t.Fatalf("Error reading fixture spec: %v", err)
	}

	if werr := os.WriteFile(filepath.Join(configDir, "api.swagger.yml"), data, 0o600); werr != nil {
		t.Fatalf("Error writing spec: %v", werr)
	}

	// Same project as the fixture, but the application that declares platforms
	// builds a deb package instead of an image.
	project, err := os.ReadFile(filepath.Join(fixture, "project.yaml"))
	if err != nil {
		t.Fatalf("Error reading fixture config: %v", err)
	}

	broken := strings.Replace(string(project),
		"  - name: cli\n    transport:\n      - name: sys\n    docker:\n",
		"  - name: cli\n    artifacts: [deb]\n    transport:\n      - name: sys\n    docker:\n", 1)
	if broken == string(project) {
		t.Fatal("fixture config changed shape, the artifacts override was not applied")
	}

	if werr := os.WriteFile(filepath.Join(configDir, "project.yaml"), []byte(broken), 0o600); werr != nil {
		t.Fatalf("Error writing config: %v", werr)
	}

	targetDir := t.TempDir()

	out, err := ExecCommand(filepath.Join(curDir, ".."), "go", []string{
		"run", filepath.Join(curDir, "..", "cmd", "go-project-starter", "main.go"),
		"--target", targetDir,
		"--configDir", configDir,
		"--config", "project.yaml",
	}, "Generate project with platforms on an application without an image")

	if err == nil {
		t.Fatalf("generation succeeded with platforms declared for an application that builds no image\n%s", out)
	}

	for _, want := range []string{"cli", "docker artifact"} {
		if !strings.Contains(out, want) {
			t.Errorf("generator error should mention %q, got:\n%s", want, out)
		}
	}
}
