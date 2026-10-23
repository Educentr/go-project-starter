package test

import (
	"encoding/json"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// serviceMetricPanels returns the Service Metrics row of a generated
// dashboard, as panel title -> queries, plus whether the row is there at all.
func serviceMetricPanels(t *testing.T, raw string) (map[string][]string, bool) {
	t.Helper()

	var dashboard struct {
		Panels []struct {
			Title  string `json:"title"`
			Panels []struct {
				Title   string `json:"title"`
				Targets []struct {
					Expr string `json:"expr"`
				} `json:"targets"`
			} `json:"panels"`
		} `json:"panels"`
	}

	if err := json.Unmarshal([]byte(raw), &dashboard); err != nil {
		t.Fatalf("generated dashboard is not valid JSON: %v", err)
	}

	for _, row := range dashboard.Panels {
		if row.Title != "Service Metrics" {
			continue
		}

		panels := make(map[string][]string, len(row.Panels))

		for _, panel := range row.Panels {
			exprs := make([]string, 0, len(panel.Targets))
			for _, target := range panel.Targets {
				exprs = append(exprs, target.Expr)
			}

			panels[panel.Title] = exprs
		}

		return panels, true
	}

	return nil, false
}

// appCollectorBlock returns the body of one application's entry in the
// generated collectorsByApp map.
func appCollectorBlock(t *testing.T, metricsFile, app string) string {
	t.Helper()

	head := "\t\"" + app + "\": {"

	start := strings.Index(metricsFile, head)
	if start < 0 {
		t.Fatalf("collectorsByApp should carry an entry for %q:\n%s", app, metricsFile)
	}

	rest := metricsFile[start+len(head):]

	end := strings.Index(rest, "\n\t},")
	if end < 0 {
		t.Fatalf("entry for %q in collectorsByApp is not terminated:\n%s", app, metricsFile)
	}

	return rest[:end]
}

// TestGenerateMetricsMapPerApp covers the `apps` field on a service that is
// three processes: the collectors of an application are the ones it publishes,
// not the whole map.
//
// Registering the whole map in every process puts series on /metrics that this
// process never increments. An always-zero series is indistinguishable from a
// broken feature, so the "zero is normal, growth is a defect" reading holds in
// neither direction — nine such series was the defect this field removes.
func TestGenerateMetricsMapPerApp(t *testing.T) {
	tmpDir := generateFromConfig(t, "metrics-map-apps", "Generate multi-application project with a metrics map")

	t.Run("collectors", func(t *testing.T) { assertPerAppCollectors(t, tmpDir) })
	t.Run("dashboards", func(t *testing.T) { assertPerAppDashboards(t, tmpDir) })
}

// assertPerAppCollectors checks the generated collector package and the
// registration call in each generated main.
func assertPerAppCollectors(t *testing.T, tmpDir string) {
	t.Helper()

	metricsFile := readGenerated(t, tmpDir, "internal", "pkg", "metrics", "psg_metrics_gen.go")

	if _, err := parser.ParseFile(token.NewFileSet(), "psg_metrics_gen.go", metricsFile, parser.SkipObjectResolution); err != nil {
		t.Fatalf("generated collector package is not valid Go: %v", err)
	}

	// Every declaration still becomes a variable: `apps` splits registration,
	// not the namespace. The map is the surface of the service either way.
	for _, varName := range []string{"SpawnErrorsTotal", "QueueDepth", "AdminActionsTotal", "RestartsTotal"} {
		if !strings.Contains(metricsFile, "var "+varName+" = ") {
			t.Errorf("every declaration should still get a collector variable, missing %s", varName)
		}
	}

	// One entry per application with a /metrics endpoint, holding exactly the
	// collectors that application publishes.
	wantPerApp := map[string]struct {
		have    []string
		haveNot []string
	}{
		"api": {
			have:    []string{"SpawnErrorsTotal", "QueueDepth", "RestartsTotal"},
			haveNot: []string{"AdminActionsTotal"},
		},
		"admin": {
			have:    []string{"QueueDepth", "AdminActionsTotal", "RestartsTotal"},
			haveNot: []string{"SpawnErrorsTotal"},
		},
	}

	for app, want := range wantPerApp {
		block := appCollectorBlock(t, metricsFile, app)

		for _, varName := range want.have {
			if !strings.Contains(block, "\t\t"+varName+",") {
				t.Errorf("application %q should publish %s, got:%s", app, varName, block)
			}
		}

		for _, varName := range want.haveNot {
			if strings.Contains(block, "\t\t"+varName+",") {
				t.Errorf("application %q must not publish %s — it never increments it, so the series would be always zero:%s",
					app, varName, block)
			}
		}
	}

	// An application without a sys transport has no registry at all, so it is
	// not a key of the map: RegisterFor on it is an error, not an empty
	// registration that reads as success.
	if strings.Contains(metricsFile, `"worker": {`) {
		t.Error("an application without a /metrics endpoint must not appear in collectorsByApp")
	}

	// Each generated main registers under its own name.
	for _, app := range []string{"api", "admin"} {
		mainFile := readGenerated(t, tmpDir, "cmd", app, "psg_main_gen.go")

		if !strings.Contains(mainFile, `metrics.RegisterFor("`+app+`", application.GetMetrics())`) {
			t.Errorf("generated main of %q should register that application's collectors", app)
		}
	}

	workerMain := readGenerated(t, tmpDir, "cmd", "worker", "psg_main_gen.go")

	if strings.Contains(workerMain, "metrics.RegisterFor(") {
		t.Error("an application without a /metrics endpoint should not register collectors")
	}

	if strings.Contains(workerMain, `"github.com/test/metricsappstest/internal/pkg/metrics"`) {
		t.Error("an application that does not register collectors must not import the package")
	}
}

// assertPerAppDashboards pins the other half: a panel for a metric the process
// cannot emit reads empty forever, which is the same false "all good" as an
// always-zero series on /metrics.
func assertPerAppDashboards(t *testing.T, tmpDir string) {
	t.Helper()

	apiPanels, ok := serviceMetricPanels(t, readGenerated(t, tmpDir, "configs", "grafana", "dashboards", "api.json"))
	if !ok {
		t.Fatal("api dashboard should carry a Service Metrics row built from the declarations")
	}

	adminPanels, ok := serviceMetricPanels(t, readGenerated(t, tmpDir, "configs", "grafana", "dashboards", "admin.json"))
	if !ok {
		t.Fatal("admin dashboard should carry a Service Metrics row built from the declarations")
	}

	// A dashboard follows the Grafana datasource, not the sys transport, so an
	// application with no /metrics endpoint still gets one. Its Service
	// Metrics row would be panels nothing ever scrapes — and the user cannot
	// scope them away either, because `apps: [worker]` is a hard error.
	if _, present := serviceMetricPanels(t, readGenerated(t, tmpDir, "configs", "grafana", "dashboards", "worker.json")); present {
		t.Error("an application without a /metrics endpoint must get no Service Metrics row: nothing scrapes it")
	}

	for _, want := range []string{"spawn_errors_total", "queue_depth", "restarts_total"} {
		if _, ok := apiPanels[want]; !ok {
			t.Errorf("api dashboard should chart %s", want)
		}
	}

	if _, ok := apiPanels["admin_actions_total"]; ok {
		t.Error("api dashboard must not chart admin_actions_total — the api process never emits it")
	}

	for _, want := range []string{"queue_depth", "admin_actions_total", "restarts_total"} {
		if _, ok := adminPanels[want]; !ok {
			t.Errorf("admin dashboard should chart %s", want)
		}
	}

	if _, ok := adminPanels["spawn_errors_total"]; ok {
		t.Error("admin dashboard must not chart spawn_errors_total — the admin process never emits it")
	}

	// The Kafka producer is attached to the api application only. Its
	// declarations come from the generator's internal map, where `apps` cannot
	// be written — the owning component answers the same question.
	if _, ok := apiPanels["kafka_producer_publish_total"]; !ok {
		t.Error("api dashboard should chart the Kafka producer metrics — that application runs the producer")
	}

	if _, ok := adminPanels["kafka_producer_publish_total"]; ok {
		t.Error("admin dashboard must not chart the Kafka producer metrics — that application runs no producer")
	}
}

// generateWithMetricsMap reuses the multi-application fixture with a different
// metrics map, so that the only difference from the passing case is the map.
func generateWithMetricsMap(t *testing.T, mapYAML, msg string) (string, error) {
	t.Helper()

	curDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting current directory: %v", err)
	}

	fixture := filepath.Join(curDir, "..", "test", "docker-integration", "configs", "metrics-map-apps")
	configDir := t.TempDir()

	for _, name := range []string{"project.yaml", "api.swagger.yml"} {
		data, rerr := os.ReadFile(filepath.Join(fixture, name))
		if rerr != nil {
			t.Fatalf("Error reading fixture %s: %v", name, rerr)
		}

		if werr := os.WriteFile(filepath.Join(configDir, name), data, 0o600); werr != nil {
			t.Fatalf("Error writing %s: %v", name, werr)
		}
	}

	if werr := os.WriteFile(filepath.Join(configDir, "metrics-map.yaml"), []byte(mapYAML), 0o600); werr != nil {
		t.Fatalf("Error writing metrics map: %v", werr)
	}

	return ExecCommand(filepath.Join(curDir, ".."), "go", []string{
		"run", filepath.Join(curDir, "..", "cmd", "go-project-starter", "main.go"),
		"--target", t.TempDir(),
		"--configDir", configDir,
		"--config", "project.yaml",
	}, msg)
}

// TestGenerateRejectsBadAppReference pins that an `apps` entry the project
// cannot satisfy stops generation. Tolerating one would move the metric
// nowhere while the declaration reads as if it had moved it somewhere — the
// failure mode the map exists to remove.
//
// Both halves ride one map because the check reports every problem at once;
// the wording of each message is pinned at unit level in
// internal/pkg/metricsmap/apps_test.go, so what this costs a generator run for
// is the end-to-end claim: the error reaches the binary's exit code.
func TestGenerateRejectsBadAppReference(t *testing.T) {
	out, err := generateWithMetricsMap(t,
		"metrics:\n"+
			// Misspelled: no such application.
			"  - name: restarts_total\n"+
			"    type: Counter\n"+
			"    help: Process restarts since start.\n"+
			"    apps: [apii]\n"+
			// Real application, but it has no sys transport and therefore no
			// /metrics endpoint to register into.
			"  - name: spawn_errors_total\n"+
			"    type: Counter\n"+
			"    help: Spawn failures.\n"+
			"    apps: [worker]\n",
		"Generate project whose metrics map carries unsatisfiable apps references")

	if err == nil {
		t.Fatalf("generation succeeded on apps entries the project cannot satisfy\n%s", out)
	}

	for _, want := range []string{
		`apps names "apii", which is not an application of this project`,
		"(declared applications: admin, api, worker)",
		`application "worker" has no sys transport`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("generator error should mention %q, got:\n%s", want, out)
		}
	}
}
