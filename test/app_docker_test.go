package test

import (
	"strings"
	"testing"
)

// TestGenerateAppDockerRuntime verifies that the docker section of an
// application shapes the runtime stage of its Dockerfile: base image, runtime
// packages, working directory and start instruction — while an application
// without the section keeps the historical stage.
//
// The test config (test/docker-integration/configs/app-docker-runtime) declares
// the section in full for the "api" application, a shell-less base image for
// "slim" and nothing for "plain".
func TestGenerateAppDockerRuntime(t *testing.T) {
	tmpDir := generateProject(t, "app-docker-runtime")

	dockerfile := readGenerated(t, tmpDir, "Dockerfile-api")

	_, final, ok := strings.Cut(dockerfile, "\nFROM ubuntu:24.04\n")
	if !ok {
		t.Fatalf("Dockerfile-api runtime stage does not use the declared base image:\n%s", dockerfile)
	}

	// Runtime packages: one instruction, installed with the package manager of
	// the declared base image, index dropped in the same layer.
	if got := strings.Count(final, "\nRUN set -eux"); got != 1 {
		t.Errorf("runtime stage has %d package install instructions, want exactly 1:\n%s", got, final)
	}

	for _, want := range []string{
		"apt-get install -y --no-install-recommends git curl ca-certificates",
		"rm -rf /var/lib/apt/lists/*",
		"\nWORKDIR /workspace\n",
		"\nENTRYPOINT [\"/service\", \"--verbose\"]\n",
	} {
		if !strings.Contains(final, want) {
			t.Errorf("runtime stage does not contain %q:\n%s", want, final)
		}
	}

	// The packages are installed before the binary is copied, so the layer
	// survives a code change.
	if strings.Index(final, "apt-get install") > strings.Index(final, "COPY --from=builder /service") {
		t.Errorf("runtime stage installs the packages below the binary copy:\n%s", final)
	}

	// An entrypoint replaces the default command: Docker would otherwise pass
	// it to the entrypoint as an argument.
	if strings.Contains(final, "CMD [") {
		t.Errorf("runtime stage keeps a CMD next to the declared ENTRYPOINT:\n%s", final)
	}

	// A shell-less base image without packages: the runtime stage must have no
	// RUN at all, otherwise the image does not build (there is no /bin/sh to
	// run it with). The config directory therefore arrives from the builder
	// stage instead of being created here.
	slim := readGenerated(t, tmpDir, "Dockerfile-slim")

	_, slimFinal, ok := strings.Cut(slim, "\nFROM gcr.io/distroless/static\n")
	if !ok {
		t.Fatalf("Dockerfile-slim runtime stage does not use the declared base image:\n%s", slim)
	}

	if strings.Contains(slimFinal, "\nRUN ") {
		t.Errorf("runtime stage of a shell-less base image contains a RUN:\n%s", slimFinal)
	}

	if !strings.Contains(slimFinal, "\nCOPY --from=builder /etc/onlineconf /etc/onlineconf\n") {
		t.Errorf("Dockerfile-slim runtime stage does not copy the config directory:\n%s", slimFinal)
	}

	// The directory is created in the builder stage, which is where the copy
	// above takes it from.
	slimBuilder, _, _ := strings.Cut(slim, "\nFROM gcr.io/distroless/static\n")
	if !strings.Contains(slimBuilder, "\nRUN mkdir -p /etc/onlineconf\n") {
		t.Errorf("builder stage does not create the config directory:\n%s", slimBuilder)
	}

	// The application without the section keeps the stage every generated
	// project had before it existed.
	plain := readGenerated(t, tmpDir, "Dockerfile-plain")

	_, plainFinal, ok := strings.Cut(plain, "\nFROM alpine\n")
	if !ok {
		t.Fatalf("Dockerfile-plain runtime stage does not use the default base image:\n%s", plain)
	}

	if !strings.HasPrefix(plainFinal, "COPY --from=builder /service /service\n") {
		t.Errorf("Dockerfile-plain runtime stage has an instruction before the binary copy:\n%s", plainFinal)
	}

	if !strings.Contains(plainFinal, "\nCMD [\"/service\"]\n") {
		t.Errorf("Dockerfile-plain runtime stage does not start the service binary:\n%s", plainFinal)
	}

	for _, unwanted := range []string{"WORKDIR", "ENTRYPOINT", "apk add", "\nRUN "} {
		if strings.Contains(plainFinal, unwanted) {
			t.Errorf("Dockerfile-plain runtime stage has an unexpected %q:\n%s", unwanted, plainFinal)
		}
	}
}
