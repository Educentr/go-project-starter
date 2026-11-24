package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGenerateWebApp covers the whole web-application feature on a project
// that declares two frontends: the make targets, the client codegen fed by the
// project's own spec copies, and the frontend build stage of each image.
func TestGenerateWebApp(t *testing.T) {
	tmpDir := generateFromConfig(t, "web-app", "Generate project with web applications")

	makefile := readGenerated(t, tmpDir, "Makefile")

	t.Run("frontend with install, build and codegen", func(t *testing.T) {
		_, recipe, found := makeTarget(t, makefile, "install-web-admin")
		if !found {
			t.Fatal("install-web-admin target is missing")
		}

		// Every command runs in the frontend directory: that is where the
		// frontend's own manifest and its node_modules live.
		if len(recipe) != 1 || recipe[0] != "cd web/admin && npm ci" {
			t.Errorf("install recipe = %v, want one 'cd web/admin && npm ci' line", recipe)
		}

		_, recipe, found = makeTarget(t, makefile, "build-web-admin")
		if !found {
			t.Fatal("build-web-admin target is missing")
		}

		if len(recipe) != 1 || recipe[0] != "cd web/admin && npm run build" {
			t.Errorf("build recipe = %v", recipe)
		}

		_, recipe, found = makeTarget(t, makefile, "generate-web-admin")
		if !found {
			t.Fatal("generate-web-admin target is missing")
		}

		// The paths are the generator's own copies of the declared inputs,
		// relative to the frontend directory — no second copy of a spec the
		// generator already fetches.
		//
		// Each channel runs its own tool: openapi-typescript over the OpenAPI
		// spec, json2ts over the JSON Schema. One tool over both would fail on
		// the format it does not read, and generate-web-admin is a
		// prerequisite of `generate` — the whole `make generate` would fail
		// with it.
		want := []string{
			"cd web/admin && npx openapi-typescript ../../api/rest/api/v1/api.swagger.yml -o src/api/api.swagger.ts",
			"cd web/admin && npx json2ts ../../api/schema/models/user.schema.json -o src/models/user.schema.d.ts",
		}

		if len(recipe) != len(want) {
			t.Fatalf("generate recipe = %v, want %v", recipe, want)
		}

		for i := range want {
			if recipe[i] != want[i] {
				t.Errorf("generate recipe[%d] = %q, want %q", i, recipe[i], want[i])
			}
		}

		// Comparing the recipe text says nothing about whether the command
		// would find anything to read. The spec path is relative to the
		// frontend directory, because that is where the command runs, so it is
		// resolved from there — against the project the generator just wrote.
		for _, line := range recipe {
			spec := specArg(t, line)

			if _, err := os.Stat(filepath.Join(tmpDir, "web", "admin", spec)); err != nil {
				t.Errorf("codegen reads %q, which the generator did not produce: %v", spec, err)
			}
		}
	})

	// A frontend declaring only `build` must not produce an empty install- or
	// generate- target: an empty recipe looks like a step that ran.
	t.Run("frontend with build only", func(t *testing.T) {
		_, recipe, found := makeTarget(t, makefile, "build-web-landing")
		if !found {
			t.Fatal("build-web-landing target is missing")
		}

		if len(recipe) != 1 || recipe[0] != "cd web/landing && npm run build" {
			t.Errorf("build recipe = %v", recipe)
		}

		for _, absent := range []string{"install-web-landing", "generate-web-landing"} {
			if _, _, found := makeTarget(t, makefile, absent); found {
				t.Errorf("%s should not be generated for a build-only frontend", absent)
			}
		}
	})

	t.Run("wired into the built-in targets", func(t *testing.T) {
		prereqs, _, found := makeTarget(t, makefile, "install-tools")
		if !found {
			t.Fatal("install-tools target is missing")
		}

		if !strings.Contains(prereqs, "install-web-admin") {
			t.Errorf("install-tools prerequisites = %q, want install-web-admin", prereqs)
		}

		if strings.Contains(prereqs, "install-web-landing") {
			t.Errorf("install-tools must not depend on a frontend with no install step: %q", prereqs)
		}

		// The whole point of declaring the codegen: it runs as part of
		// `make generate`, and so of `make regenerate` through it, instead of
		// waiting for someone to remember an npm script.
		prereqs, _, found = makeTarget(t, makefile, "generate")
		if !found {
			t.Fatal("generate target is missing")
		}

		if !strings.Contains(prereqs, "generate-web-admin") {
			t.Errorf("generate prerequisites = %q, want generate-web-admin", prereqs)
		}

		// The client is generated from the specs the built-in generators have
		// just refreshed, so it has to come after them.
		if idx := strings.Index(prereqs, "generate-web-admin"); idx >= 0 {
			if ogen := strings.Index(prereqs, "generate-ogen"); ogen < 0 || ogen > idx {
				t.Errorf("generate should run the built-in generators first: %q", prereqs)
			}
		}

		if strings.Contains(prereqs, "generate-web-landing") {
			t.Errorf("generate must not depend on a frontend with no codegen: %q", prereqs)
		}
	})

	// The frontend is built in the image, not taken from the host: that is
	// what makes `make docker-<app>` work on a machine with no node, which is
	// what CI runs on.
	t.Run("frontend build stage in the image that serves it", func(t *testing.T) {
		dockerfile := readGenerated(t, tmpDir, "Dockerfile-api")

		for _, want := range []string{
			"FROM node:22-alpine AS web-admin",
			"RUN cd web/admin && npm ci",
			"RUN cd web/admin && npm run build",
			"COPY --from=web-admin /src/static/ /static/",
		} {
			if !strings.Contains(dockerfile, want) {
				t.Errorf("Dockerfile-api should contain %q", want)
			}
		}

		// A frontend lands only in the image whose static transport serves its
		// output directory.
		if strings.Contains(dockerfile, "web-landing") {
			t.Error("Dockerfile-api carries the build stage of a frontend it does not serve")
		}

		// The application declares no platforms, so the stage carries no pin:
		// the build passes no --platform, every stage is built for the host
		// anyway, and the Dockerfile of an existing project is unchanged.
		if strings.Contains(dockerfile, "BUILDPLATFORM") {
			t.Error("Dockerfile-api pins a stage to the build platform without docker.platforms")
		}

		landing := readGenerated(t, tmpDir, "Dockerfile-landing")

		for _, want := range []string{
			// The landing application declares platforms, so its frontend
			// stage is pinned to the build host. Unpinned, buildx would build
			// it once per target platform, under emulation of each — the QEMU
			// the cross-compiling builder stage exists to avoid.
			"FROM --platform=$BUILDPLATFORM node:20-alpine AS web-landing",
			"COPY --from=web-landing /src/public_web/ /public_web/",
		} {
			if !strings.Contains(landing, want) {
				t.Errorf("Dockerfile-landing should contain %q", want)
			}
		}

		if strings.Contains(landing, "npm ci") {
			t.Error("Dockerfile-landing has an install step for a frontend that declares none")
		}
	})

	// The combination of `web` and `docker.platforms`: every stage that runs a
	// command has to be pinned to the build host, otherwise the image loses the
	// no-emulation invariant the platforms key is built on.
	t.Run("frontend stage of a multi-platform image is pinned to the build host", func(t *testing.T) {
		landing := readGenerated(t, tmpDir, "Dockerfile-landing")

		// Every stage of the file that is not the runtime stage: the frontend
		// build and the Go builder both run commands, so both are pinned.
		var unpinned []string

		for _, line := range strings.Split(landing, "\n") {
			if !strings.HasPrefix(line, "FROM ") || !strings.Contains(line, " AS ") {
				continue
			}

			if !strings.HasPrefix(line, "FROM --platform=$BUILDPLATFORM ") {
				unpinned = append(unpinned, line)
			}
		}

		if len(unpinned) > 0 {
			t.Errorf("stages of a multi-platform image run under emulation: %q\n%s", unpinned, landing)
		}

		// The pin is what makes the frontend build run once; the result is
		// still copied into the image built for every declared platform.
		if !strings.Contains(landing, "RUN cd web/landing && npm run build") {
			t.Errorf("Dockerfile-landing does not build the frontend:\n%s", landing)
		}

		// The frontend reaches the image through a COPY of its output
		// directory only, so whatever the node stage installed for the build
		// host — native modules included — stays in that stage.
		if strings.Contains(landing, "COPY --from=web-landing /src/node_modules") {
			t.Errorf("the node toolchain must not reach the image:\n%s", landing)
		}

		// The image is still a cross-built index: the frontend pin must not
		// have displaced the builder's own cross-compilation.
		for _, want := range []string{
			"FROM --platform=$BUILDPLATFORM golang:",
			"\nARG TARGETARCH\n",
			"GOARCH=${TARGETARCH:?",
		} {
			if !strings.Contains(landing, want) {
				t.Errorf("Dockerfile-landing misses %q:\n%s", want, landing)
			}
		}
	})

	// The starter owns the build wiring, not the frontend itself: nothing is
	// created under the declared source directory.
	t.Run("frontend sources are not created by the starter", func(t *testing.T) {
		if _, err := os.Stat(filepath.Join(tmpDir, "web")); err == nil {
			t.Error("the starter must not create the frontend source tree")
		}

		// The output directory is the static drop-zone, which does exist — it
		// has to be in the build context for the image's ADD.
		if _, err := os.Stat(filepath.Join(tmpDir, "static", ".keep")); err != nil {
			t.Errorf("static drop-zone is missing: %v", err)
		}
	})

	t.Run("build output is ignored, the drop-zone marker is not", func(t *testing.T) {
		gitignore := readGenerated(t, tmpDir, ".gitignore")

		for _, want := range []string{"node_modules/", "static/*", "!static/.keep", "public_web/*"} {
			if !strings.Contains(gitignore, want) {
				t.Errorf(".gitignore should contain %q", want)
			}
		}
	})
}

// specArg returns the spec path of a rendered codegen recipe line — the copy
// of the input the command is run over, written relative to the frontend
// directory.
func specArg(t *testing.T, line string) string {
	t.Helper()

	for _, field := range strings.Fields(line) {
		if strings.HasPrefix(field, "../") {
			return field
		}
	}

	t.Fatalf("recipe line %q carries no spec path", line)

	return ""
}

// TestGenerateWithoutWebApp_OutputUnchanged pins the regression the feature
// must not cause: a project that declares no frontend gets a Makefile,
// Dockerfile and .gitignore with no trace of it.
func TestGenerateWithoutWebApp_OutputUnchanged(t *testing.T) {
	tmpDir := generateFromConfig(t, "static", "Generate project without web applications")

	makefile := readGenerated(t, tmpDir, "Makefile")

	for _, absent := range []string{"# Frontend '", "install-web-", "build-web-", "generate-web-"} {
		if strings.Contains(makefile, absent) {
			t.Errorf("Makefile carries %q for a project that declares no frontend", absent)
		}
	}

	prereqs, _, found := makeTarget(t, makefile, "install-tools")
	if !found {
		t.Fatal("install-tools target is missing")
	}

	if !strings.HasSuffix(prereqs, "install-goimports") {
		t.Errorf("install-tools prerequisites = %q, want them to end with install-goimports", prereqs)
	}

	dockerfile := readGenerated(t, tmpDir, "Dockerfile-web")

	if strings.Contains(dockerfile, "AS web-") || strings.Contains(dockerfile, "# Frontend '") {
		t.Error("Dockerfile carries a frontend build stage for a project that declares none")
	}

	// The project still serves static files the old way: a hand-filled
	// directory copied into the image.
	if !strings.Contains(dockerfile, "ADD static /static") {
		t.Error("Dockerfile should still copy the static directory into the image")
	}

	if strings.Contains(readGenerated(t, tmpDir, ".gitignore"), "node_modules/") {
		t.Error(".gitignore carries frontend entries for a project that declares no frontend")
	}
}
