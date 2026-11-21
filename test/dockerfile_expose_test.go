package test

import (
	"reflect"
	"strings"
	"testing"
)

// exposedPorts returns the ports a Dockerfile declares, in order.
func exposedPorts(dockerfile string) []string {
	ports := make([]string, 0)

	for _, line := range strings.Split(dockerfile, "\n") {
		if fields := strings.Fields(line); len(fields) == 2 && fields[0] == "EXPOSE" {
			ports = append(ports, fields[1])
		}
	}

	return ports
}

// TestDockerfileExposesOwnServerPorts covers the EXPOSE directives of an
// application's image (TOOLS-96, issue #124 item 4).
//
// The directives used to be rendered from the project's whole set of REST
// transports, so every image announced every other application's ports, and a
// client transport — which dials a port rather than serving one — rendered
// EXPOSE 0. Nothing breaks on a wrong EXPOSE, which is why it survived: it is
// metadata, and `docker run -P`, image inspection and image scanners are the
// ones that read it and look for a port that is not there.
func TestDockerfileExposesOwnServerPorts(t *testing.T) {
	// combined: app `api` serves api (8080) and sys (8085) and holds a gRPC
	// client; app `bot` serves sys only.
	tmpDir := generateFromConfig(t, "combined", "Generate multi-app project for EXPOSE test")

	cases := map[string][]string{
		"Dockerfile-api": {"8080", "8085"},
		"Dockerfile-bot": {"8085"},
	}

	for name, want := range cases {
		if got := exposedPorts(readGenerated(t, tmpDir, name)); !reflect.DeepEqual(got, want) {
			t.Errorf("%s should expose %v, exposes %v", name, want, got)
		}
	}

	// An application whose only REST transport is a client serves nothing, so
	// its image announces nothing. This is the case that used to render
	// `EXPOSE 0`: a client carries no port because it is given one.
	clientDir := generateFromConfig(t, "rest-client-only", "Generate client-only project for EXPOSE test")

	if got := exposedPorts(readGenerated(t, clientDir, "Dockerfile-api")); len(got) != 0 {
		t.Errorf("a client-only application should expose no port, exposes %v", got)
	}
}
