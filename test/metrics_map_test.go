package test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// generateFromConfig runs the generator over one of the test config
// directories and returns the target dir.
func generateFromConfig(t *testing.T, configName, msg string) string {
	t.Helper()

	curDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting current directory: %v", err)
	}

	configDir := filepath.Join(curDir, "..", "test", "docker-integration", "configs", configName)
	tmpDir := t.TempDir()

	out, err := ExecCommand(filepath.Join(curDir, ".."), "go", []string{
		"run", filepath.Join(curDir, "..", "cmd", "go-project-starter", "main.go"),
		"--target", tmpDir,
		"--configDir", configDir,
		"--config", "project.yaml",
	}, msg+" ("+tmpDir+")")
	if err != nil {
		t.Fatalf("Error creating project: %s\n%s", err, out)
	}

	return tmpDir
}

func readGenerated(t *testing.T, targetDir string, parts ...string) string {
	t.Helper()

	path := filepath.Join(append([]string{targetDir}, parts...)...)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Error reading %s: %v", filepath.Join(parts...), err)
	}

	return string(data)
}

// TestGenerateMetricsMap covers the whole metrics-map feature on a project
// that declares one: the collector package, its registration, and the
// validator.
func TestGenerateMetricsMap(t *testing.T) {
	tmpDir := generateFromConfig(t, "metrics-map", "Generate project with a metrics map")

	metricsFile := readGenerated(t, tmpDir, "internal", "pkg", "metrics", "psg_metrics_gen.go")

	// One exported variable per declared metric, with the full Prometheus name
	// and the declared labels. The full name matters: a name assembled at
	// runtime is a name no dashboard or alert can find in the sources.
	wantVars := map[string]string{
		"SpawnErrorsTotal":    `prometheus.NewCounterVec(`,
		"QueueDepth":          `prometheus.NewGauge(`,
		"TaskDurationSeconds": `prometheus.NewHistogramVec(`,
		"BatchSize":           `prometheus.NewSummary(`,
		"RestartsTotal":       `prometheus.NewCounter(`,
	}

	for varName, constructor := range wantVars {
		if !strings.Contains(metricsFile, "var "+varName+" = "+constructor) {
			t.Errorf("expected generated variable %s built with %s", varName, constructor)
		}
	}

	for _, want := range []string{
		`Name: "spawn_errors_total"`,
		`[]string{"reason", "stage"}`,
		`Buckets: []float64{0.05, 0.25, 1, 5, 30}`,
		`Objectives: map[float64]float64{0.5: 0.05, 0.99: 0.001}`,
	} {
		if !strings.Contains(metricsFile, want) {
			t.Errorf("generated collector package should contain %q", want)
		}
	}

	// A histogram with no declared buckets must still get usable ones.
	if !strings.Contains(metricsFile, "Buckets: prometheus.DefBuckets") &&
		strings.Contains(metricsFile, "NewHistogram(") {
		t.Error("an unbucketed histogram should fall back to prometheus.DefBuckets")
	}

	// External declarations are declared, not constructed: the OTEL SDK emits
	// this one.
	if strings.Contains(metricsFile, "OtelSdkExporterSpansTotal") {
		t.Error("external metrics must not get a generated collector")
	}

	// This project HAS a Kafka producer, so the Kafka declarations are merged
	// into the namespace — but their collectors belong to the driver (the
	// producer name is a ConstLabel), not to the shared package.
	if strings.Contains(metricsFile, "KafkaProducerPublish") {
		t.Error("Kafka producer collectors must not land in the shared collector package")
	}

	if _, err := os.Stat(filepath.Join(tmpDir,
		"pkg", "drivers", "kafka", "events_producer", "psg_metrics_gen.go")); err != nil {
		t.Errorf("the Kafka producer's own metrics file should still be generated: %v", err)
	}

	// Explicit registry, no promauto: nothing may reach the default registerer
	// on its own.
	for _, want := range []string{
		"func Register(reg prometheus.Registerer) error",
		"func Collectors() []prometheus.Collector",
		"func Unregister(reg prometheus.Registerer)",
	} {
		if !strings.Contains(metricsFile, want) {
			t.Errorf("generated collector package should declare %q", want)
		}
	}

	if strings.Contains(metricsFile, "promauto") {
		t.Error("generated collectors must not use promauto — the registerer is explicit")
	}

	// Registration is wired into the generated main next to InitMetrics.
	mainFile := readGenerated(t, tmpDir, "cmd", "api", "psg_main_gen.go")

	if !strings.Contains(mainFile, "metrics.Register(application.GetMetrics())") {
		t.Error("generated main should register the declared collectors")
	}

	if !strings.Contains(mainFile, `"github.com/test/metricstest/internal/pkg/metrics"`) {
		t.Error("generated main should import the generated collector package")
	}

	// The second application has no sys transport, so no /metrics endpoint and
	// no registry to register into. It must get neither the call nor the
	// import — an import it never uses would not compile.
	workerMain := readGenerated(t, tmpDir, "cmd", "worker", "psg_main_gen.go")

	if strings.Contains(workerMain, "metrics.Register(") {
		t.Error("an application without a /metrics endpoint should not register collectors")
	}

	if strings.Contains(workerMain, `"github.com/test/metricstest/internal/pkg/metrics"`) {
		t.Error("an application that does not register collectors must not import the package")
	}

	// Validator: standalone config, the rule in the main lint config, and the
	// Makefile target.
	metricsLint := readGenerated(t, tmpDir, "configs", "golangci-metrics.yml")

	for _, want := range []string{
		"- forbidigo",
		`^prometheus\.New(Counter|Gauge|Histogram|Summary|Untyped)(Vec|Func)?$`,
		`^prometheus\.New(Desc|ConstMetric|ConstHistogram|ConstSummary|InvalidMetric|MetricWithTimestamp)$`,
		`^promauto\.`,
		"path: (internal/pkg/metrics/|pkg/drivers/kafka/)",
	} {
		if !strings.Contains(metricsLint, want) {
			t.Errorf("configs/golangci-metrics.yml should contain %q", want)
		}
	}

	// `generated: lax` would skip every psg_*_gen.go — including the
	// hand-written section below the disclaimer marker, which is exactly where
	// handlers live. The validator config must keep those files in scope.
	if !strings.Contains(metricsLint, "generated: disable") {
		t.Error("the metrics validator must not exempt generated files: user code lives below the disclaimer marker")
	}

	mainLint := readGenerated(t, tmpDir, "configs", "golangci-lint.yml")

	if !strings.Contains(mainLint, "- forbidigo") {
		t.Error("configs/golangci-lint.yml should enable forbidigo when a metrics map is configured")
	}

	if !strings.Contains(mainLint, `^promauto\.`) {
		t.Error("configs/golangci-lint.yml should carry the metrics validator patterns")
	}

	makefile := readGenerated(t, tmpDir, "Makefile")

	if !strings.Contains(makefile, "metrics-validate:") {
		t.Error("Makefile should expose a metrics-validate target")
	}

	if !strings.Contains(makefile, "--config=./configs/golangci-metrics.yml") {
		t.Error("metrics-validate should use the standalone validator config, not a flag")
	}
}

// TestGenerateMetricsMapDashboard pins §2.6: the dev dashboard's rows now
// carry queries derived from the declarations instead of being empty.
func TestGenerateMetricsMapDashboard(t *testing.T) {
	tmpDir := generateFromConfig(t, "metrics-map", "Generate project with a metrics map")

	raw := readGenerated(t, tmpDir, "configs", "grafana", "dashboards", "api.json")

	// The dashboard is consumed by Grafana, so a template that renders
	// syntactically broken JSON must fail here rather than at provisioning.
	var dashboard struct {
		Panels []struct {
			Title  string `json:"title"`
			Type   string `json:"type"`
			Panels []struct {
				Title   string `json:"title"`
				Type    string `json:"type"`
				Targets []struct {
					Expr         string `json:"expr"`
					LegendFormat string `json:"legendFormat"`
				} `json:"targets"`
			} `json:"panels"`
		} `json:"panels"`
	}

	if err := json.Unmarshal([]byte(raw), &dashboard); err != nil {
		t.Fatalf("generated dashboard is not valid JSON: %v", err)
	}

	var metricPanels map[string][]string

	for _, row := range dashboard.Panels {
		if row.Title != "Service Metrics" {
			continue
		}

		metricPanels = make(map[string][]string, len(row.Panels))

		for _, panel := range row.Panels {
			exprs := make([]string, 0, len(panel.Targets))
			for _, target := range panel.Targets {
				exprs = append(exprs, target.Expr)
			}

			metricPanels[panel.Title] = exprs
		}
	}

	if metricPanels == nil {
		t.Fatal("dashboard should carry a Service Metrics row built from the declarations")
	}

	// Canonical shape per metric type: rate for counters, value for gauges,
	// quantiles for histograms and summaries.
	wantExprs := map[string]string{
		"spawn_errors_total":    "sum by(reason, stage) (rate(spawn_errors_total[$__rate_interval]))",
		"queue_depth":           "queue_depth",
		"task_duration_seconds": "histogram_quantile(0.99, sum by(le, kind, outcome) (rate(task_duration_seconds_bucket[$__rate_interval])))",
		"batch_size":            "sum by(quantile) (batch_size)",
		"restarts_total":        "sum(rate(restarts_total[$__rate_interval]))",
		// Declared but library-emitted: no collector, but still charted —
		// charting it is why a service declares a library's metric.
		"otel_sdk_exporter_spans_total": "sum by(outcome) (rate(otel_sdk_exporter_spans_total[$__rate_interval]))",
		// Declared by the generator's internal map, applicable because this
		// project has a Kafka producer.
		"kafka_producer_publish_total": "sum by(topic) (rate(kafka_producer_publish_total[$__rate_interval]))",
	}

	for metric, wantExpr := range wantExprs {
		exprs, ok := metricPanels[metric]
		if !ok {
			t.Errorf("Service Metrics row should carry a panel for %s", metric)

			continue
		}

		found := false

		for _, expr := range exprs {
			if expr == wantExpr {
				found = true

				break
			}
		}

		if !found {
			t.Errorf("panel %s: expected query %q, got %v", metric, wantExpr, exprs)
		}
	}

	// Metrics with a hand-built row must not get a second, poorer panel.
	for _, skipped := range []string{"go_goroutines", "ogen_server_request_count_total"} {
		if _, ok := metricPanels[skipped]; ok {
			t.Errorf("%s already has a dedicated row; it must not be duplicated in Service Metrics", skipped)
		}
	}

	// Out of scope by §3: no thresholds, no alert rules.
	if strings.Contains(raw, "alert") {
		t.Error("the generated dashboard must not carry alert rules — thresholds belong to an installation")
	}
}

// TestGenerateWithoutMetricsMap pins §2.3 of the request: no map means nothing
// is switched on. A half-on transitional mode would leave traces in generated
// code that nobody cleans up later.
func TestGenerateWithoutMetricsMap(t *testing.T) {
	tmpDir := generateFromConfig(t, "grafana", "Generate project without a metrics map")

	if _, err := os.Stat(filepath.Join(tmpDir, "internal", "pkg", "metrics")); !os.IsNotExist(err) {
		t.Error("no metrics map should mean no generated collector package")
	}

	if _, err := os.Stat(filepath.Join(tmpDir, "configs", "golangci-metrics.yml")); !os.IsNotExist(err) {
		t.Error("no metrics map should mean no metrics validator config")
	}

	mainLint := readGenerated(t, tmpDir, "configs", "golangci-lint.yml")

	if strings.Contains(mainLint, "forbidigo") {
		t.Error("no metrics map should mean no forbidigo rule in the lint config")
	}

	makefile := readGenerated(t, tmpDir, "Makefile")

	if strings.Contains(makefile, "metrics-validate") {
		t.Error("no metrics map should mean no metrics-validate target")
	}

	mainFile := readGenerated(t, tmpDir, "cmd", "api", "psg_main_gen.go")

	if strings.Contains(mainFile, "metrics.Register(") {
		t.Error("no metrics map should mean no registration call in the generated main")
	}
}

// TestGenerateKafkaProducerMetricsFromDeclaration pins that the Kafka
// producer's metrics come from the internal declaration — names, help texts
// and labels in one place — while the collectors stay per-producer because the
// producer name is a ConstLabel.
func TestGenerateKafkaProducerMetricsFromDeclaration(t *testing.T) {
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
	}, "Generate example project with a Kafka producer ("+tmpDir+")")
	if err != nil {
		t.Fatalf("Error creating project: %s\n%s", err, out)
	}

	kafkaMetrics := readGenerated(t, tmpDir,
		"pkg", "drivers", "kafka", "events_producer", "psg_metrics_gen.go")

	// Names and label sets must be exactly the declared ones — this is the
	// contract consumers' dashboards already read.
	for _, want := range []string{
		`Name:    "kafka_producer_publish_duration_seconds"`,
		`Name: "kafka_producer_publish_total"`,
		`Name: "kafka_producer_publish_errors_total"`,
		`[]string{"topic"}`,
		`[]string{"topic", "error_type"}`,
		`Buckets: prometheus.DefBuckets`,
	} {
		if !strings.Contains(kafkaMetrics, want) {
			t.Errorf("generated Kafka metrics should contain %q", want)
		}
	}

	// The producer name stays a ConstLabel — that is why these collectors
	// cannot be package-level variables in the shared collector package.
	if !strings.Contains(kafkaMetrics, `"producer": producerName`) {
		t.Error("the producer name should remain a ConstLabel")
	}

	// This project has no metrics map, so nothing else is switched on.
	if _, err := os.Stat(filepath.Join(tmpDir, "internal", "pkg", "metrics")); !os.IsNotExist(err) {
		t.Error("the Kafka declaration alone must not switch on the collector package")
	}
}
