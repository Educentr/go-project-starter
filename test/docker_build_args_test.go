package test

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestGenerateDockerBuildArgs verifies that the docker.build_args and
// docker.labels sections produce everything an image needs to carry its own
// identity: the ARG declarations of both stages, the LABEL instruction of the
// final stage, the --build-arg flags of the docker-<app> recipe, the Makefile
// defaults and the docker-compose-dev build args.
//
// The test config (test/docker-integration/configs/docker-build-args) declares
// IMAGE_VERSION in the default (final) stage without a default value and
// IMAGE_REVISION in both stages with a Makefile default.
func TestGenerateDockerBuildArgs(t *testing.T) {
	tmpDir := generateProject(t, "docker-build-args")

	dockerfile := readGenerated(t, tmpDir, "Dockerfile-api")

	builder, final, ok := strings.Cut(dockerfile, "\nFROM alpine\n")
	if !ok {
		t.Fatalf("Dockerfile-api has no final stage:\n%s", dockerfile)
	}

	// Builder stage: only the argument that asks for it.
	if !strings.Contains(builder, "\nARG IMAGE_REVISION\n") {
		t.Errorf("builder stage does not declare ARG IMAGE_REVISION:\n%s", builder)
	}

	if strings.Contains(builder, "ARG IMAGE_VERSION") {
		t.Errorf("builder stage declares ARG IMAGE_VERSION, which is final-stage only:\n%s", builder)
	}

	// Final stage: both arguments, because both are referenced by labels.
	for _, arg := range []string{"IMAGE_VERSION", "IMAGE_REVISION"} {
		if !strings.Contains(final, "\nARG "+arg+"\n") {
			t.Errorf("final stage does not declare ARG %s:\n%s", arg, final)
		}
	}

	// One LABEL instruction (one layer) carrying every declared label.
	if got := strings.Count(final, "\nLABEL "); got != 1 {
		t.Errorf("final stage has %d LABEL instructions, want exactly 1:\n%s", got, final)
	}

	for _, pair := range []string{
		`org.opencontainers.image.revision="${IMAGE_REVISION}"`,
		`org.opencontainers.image.vendor="Educentr"`,
		`org.opencontainers.image.version="${IMAGE_VERSION}"`,
	} {
		if !strings.Contains(final, pair) {
			t.Errorf("final stage does not label %s:\n%s", pair, final)
		}
	}

	// The ARG block must stay below the COPY layers: an ARG invalidates the
	// build cache of everything after it, and a version changes every commit.
	if strings.Index(final, "ARG IMAGE_VERSION") < strings.LastIndex(final, "COPY --from=builder") {
		t.Errorf("final stage declares its ARGs above the COPY layers:\n%s", final)
	}

	makefile := readGenerated(t, tmpDir, "Makefile")

	// Makefile defaults: `?=` so an environment or command line value wins.
	for _, assignment := range []string{
		"\nIMAGE_VERSION ?=\n",
		"\nIMAGE_REVISION ?= $(shell git rev-parse HEAD)\n",
	} {
		if !strings.Contains(makefile, assignment) {
			t.Errorf("Makefile does not declare %q", assignment)
		}
	}

	// The docker-<app> recipe passes every declared argument.
	recipe := ""

	for _, line := range strings.Split(makefile, "\n") {
		if strings.Contains(line, "docker build .") {
			recipe = line

			break
		}
	}

	if recipe == "" {
		t.Fatal("Makefile has no docker build recipe")
	}

	for _, flag := range []string{
		`--build-arg IMAGE_VERSION="$(IMAGE_VERSION)"`,
		`--build-arg IMAGE_REVISION="$(IMAGE_REVISION)"`,
	} {
		if !strings.Contains(recipe, flag) {
			t.Errorf("docker build recipe misses %s:\n%s", flag, recipe)
		}
	}

	// docker-compose-dev takes the values from the environment.
	var compose struct {
		Services map[string]struct {
			Build struct {
				Args []string `yaml:"args"`
			} `yaml:"build"`
		} `yaml:"services"`
	}

	raw := readGenerated(t, tmpDir, "docker-compose-dev.yaml")
	if err := yaml.Unmarshal([]byte(raw), &compose); err != nil {
		t.Fatalf("docker-compose-dev.yaml is not valid YAML: %v\n%s", err, raw)
	}

	args := strings.Join(compose.Services["api"].Build.Args, ",")

	for _, arg := range []string{"IMAGE_VERSION=${IMAGE_VERSION:-}", "IMAGE_REVISION=${IMAGE_REVISION:-}"} {
		if !strings.Contains(args, arg) {
			t.Errorf("docker-compose-dev.yaml build args = %q, want it to contain %q", args, arg)
		}
	}
}

// TestGenerateDockerBuildArgsAbsent verifies that a project without
// docker.build_args keeps the historical output: no extra ARG, no LABEL and a
// docker build recipe without --build-arg.
func TestGenerateDockerBuildArgsAbsent(t *testing.T) {
	// github-ci-defaults declares no docker section at all.
	tmpDir := generateProject(t, "github-ci-defaults")

	dockerfile := readGenerated(t, tmpDir, "Dockerfile-api")

	if strings.Contains(dockerfile, "LABEL ") {
		t.Errorf("Dockerfile-api has a LABEL without docker.labels:\n%s", dockerfile)
	}

	// The two ARGs of the builder stage the template always declares.
	if got := strings.Count(dockerfile, "\nARG "); got != 2 {
		t.Errorf("Dockerfile-api has %d ARG declarations, want the 2 of the template:\n%s", got, dockerfile)
	}

	if makefile := readGenerated(t, tmpDir, "Makefile"); strings.Contains(makefile, "--build-arg") {
		t.Error("Makefile passes --build-arg without docker.build_args")
	}
}
