package test

import (
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

	// Kafka producer metrics are declared internally but constructed by the
	// Kafka driver, and this project has no Kafka producer at all.
	if strings.Contains(metricsFile, "KafkaProducerPublish") {
		t.Error("Kafka producer collectors must not land in the shared collector package")
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
