package test

import (
	"strings"
	"testing"
)

// TestGenerateMetricsInitializedWithoutSysTransport is the regression test for
// the startup panic.
//
// The metrics initialization used to be generated only for an application with
// a sys transport, on the reasoning that there was no /metrics endpoint to
// serve without one. But the registry is not the endpoint's: the REST
// middleware registers its collectors into it while the transport initializes,
// so an application whose registry was never created died on a nil dereference
// before serving its first request — and the generator accepted that
// configuration without a word.
func TestGenerateMetricsInitializedWithoutSysTransport(t *testing.T) {
	tmpDir := generateFromConfig(t, "metrics-map", "Generate project with an application without sys")

	for _, app := range []string{"api", "worker"} {
		mainFile := readGenerated(t, tmpDir, "cmd", app, "psg_main_gen.go")

		if !strings.Contains(mainFile, "application.InitMetrics(mainCtx)") {
			t.Errorf("application %q must initialize the metrics registry: the code writing metrics gets it on init", app)
		}
	}

	// The resolved channel is stated in the generated main, so a reviewer of
	// the generated project sees which of the three cases this application is
	// without going back to project.yaml.
	apiMain := readGenerated(t, tmpDir, "cmd", "api", "psg_main_gen.go")
	workerMain := readGenerated(t, tmpDir, "cmd", "worker", "psg_main_gen.go")

	if !strings.Contains(apiMain, "Export channel of this application: pull") {
		t.Error("an application with a sys transport exports its metrics by pull")
	}

	if !strings.Contains(workerMain, "Export channel of this application: none") {
		t.Error("an application without a sys transport has no export channel")
	}
}

// TestGenerateMetricsWithoutMetricsMap checks the same fix on a project that
// declares no metrics map at all: the registry is still initialized, and
// nothing of the collector package leaks in — an unused import would not
// compile.
func TestGenerateMetricsWithoutMetricsMap(t *testing.T) {
	tmpDir := generateFromConfig(t, "rest-client-only", "Generate project without a metrics map or a sys transport")

	mainFile := readGenerated(t, tmpDir, "cmd", "api", "psg_main_gen.go")

	if !strings.Contains(mainFile, "application.InitMetrics(mainCtx)") {
		t.Error("the metrics registry is initialized in every application, whatever its transports")
	}

	if strings.Contains(mainFile, "metrics.Register(") {
		t.Error("a project without a metrics map has no generated collectors to register")
	}

	if strings.Contains(mainFile, `/internal/pkg/metrics"`) {
		t.Error("a project without a metrics map must not import the collector package")
	}
}
