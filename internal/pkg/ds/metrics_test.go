package ds_test

import (
	"testing"

	"github.com/Educentr/go-project-starter/internal/pkg/ds"
	"github.com/Educentr/go-project-starter/internal/pkg/metricsmap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// perAppConfig is a service of three processes: one serving /metrics and
// running a Kafka producer, one serving /metrics only, one serving neither but
// named by a declaration of its own.
func perAppConfig() ds.MetricsConfig {
	return ds.MetricsConfig{
		Enabled: true,
		Metrics: []metricsmap.Metric{
			{Name: "spawn_errors_total", Type: metricsmap.TypeCounter, Apps: metricsmap.Apps{"api"}},
			{Name: "admin_actions_total", Type: metricsmap.TypeCounter, Apps: metricsmap.Apps{"admin"}},
			{Name: "worker_runs_total", Type: metricsmap.TypeCounter, Apps: metricsmap.Apps{"worker"}},
			{Name: "restarts_total", Type: metricsmap.TypeCounter},
			{Name: "go_goroutines", Type: metricsmap.TypeGauge, External: true},
			{Name: "kafka_producer_publish_total", Type: metricsmap.TypeCounter, Owner: metricsmap.OwnerKafkaProducer},
		},
		Apps: []metricsmap.AppScope{
			{
				Name:           "api",
				ExposesMetrics: true,
				Components:     map[string]bool{metricsmap.OwnerKafkaProducer: true},
			},
			{Name: "admin", ExposesMetrics: true},
			{Name: "worker"},
		},
	}
}

func names(metrics []metricsmap.Metric) []string {
	out := make([]string, 0, len(metrics))
	for _, metric := range metrics {
		out = append(out, metric.Name)
	}

	return out
}

func TestMetricsConfig_ForApp(t *testing.T) {
	cfg := perAppConfig()

	// External declarations are part of the per-process view: the library that
	// emits them is linked into this process, and charting them is why the
	// service declared them.
	assert.Equal(t,
		[]string{"spawn_errors_total", "restarts_total", "go_goroutines", "kafka_producer_publish_total"},
		names(cfg.ForApp("api")))

	// No Kafka producer in this process, so no producer collector is ever
	// constructed here and a panel for it could only read empty.
	assert.Equal(t,
		[]string{"admin_actions_total", "restarts_total", "go_goroutines"},
		names(cfg.ForApp("admin")))

	// No /metrics endpoint: nothing scrapes this process, so a panel built for
	// it could only read empty — even for the declaration that names it. Its
	// collectors go to a registry of its own code, which no Prometheus target
	// reads.
	assert.Empty(t, cfg.ForApp("worker"))

	// An application the generator never resolved is not an application of
	// this project, so it publishes nothing.
	assert.Empty(t, cfg.ForApp("nosuchapp"))
}

func TestMetricsConfig_CollectorsByApp(t *testing.T) {
	byApp := perAppConfig().CollectorsByApp()

	require.Len(t, byApp, 3)

	assert.Equal(t, "api", byApp[0].App)
	assert.Equal(t, []string{"spawn_errors_total", "restarts_total"}, names(byApp[0].Metrics))

	// Neither external nor generator-owned declarations get a collector in the
	// shared package — the first is constructed by a library, the second by
	// its own template.
	assert.Equal(t, "admin", byApp[1].App)
	assert.Equal(t, []string{"admin_actions_total", "restarts_total"}, names(byApp[1].Metrics))

	// No /metrics endpoint, so it gets only what the map named it in: the
	// default surface is the service's server-side metrics, and handing those
	// to this process's private registry is the inverse of the defect the
	// field removes.
	assert.Equal(t, "worker", byApp[2].App)
	assert.Equal(t, []string{"worker_runs_total"}, names(byApp[2].Metrics))
}

func TestMetricsConfig_CollectorsByAppSkipsAppsWithNothingToRegister(t *testing.T) {
	// A map that names nobody: the applications with a /metrics endpoint are
	// still keys (RegisterFor answers over this set), the one without is not —
	// an empty entry would only make a typo in RegisterFor read as success.
	cfg := perAppConfig()
	cfg.Metrics = []metricsmap.Metric{{Name: "restarts_total", Type: metricsmap.TypeCounter}}

	byApp := cfg.CollectorsByApp()
	require.Len(t, byApp, 2)
	assert.Equal(t, "api", byApp[0].App)
	assert.Equal(t, "admin", byApp[1].App)
}

func TestMetricsConfig_HasGeneratedFor(t *testing.T) {
	cfg := perAppConfig()

	assert.True(t, cfg.HasGeneratedFor("api"))
	assert.True(t, cfg.HasGeneratedFor("admin"))

	// No /metrics endpoint, but the map named it, so the collector package
	// carries its entry. The generated main still registers nothing — it gates
	// the call on the sys transport, so the import it guards is not emitted
	// either.
	assert.True(t, cfg.HasGeneratedFor("worker"))

	assert.False(t, cfg.HasGeneratedFor("nosuchapp"))
}

func TestMetricsConfig_WithoutAppsEveryoneGetsEverything(t *testing.T) {
	// A map written before the field existed: every declaration goes to every
	// application that exposes /metrics, which is what it did before.
	cfg := ds.MetricsConfig{
		Enabled: true,
		Metrics: []metricsmap.Metric{
			{Name: "spawn_errors_total", Type: metricsmap.TypeCounter},
			{Name: "restarts_total", Type: metricsmap.TypeCounter},
		},
		Apps: []metricsmap.AppScope{
			{Name: "api", ExposesMetrics: true},
			{Name: "admin", ExposesMetrics: true},
		},
	}

	byApp := cfg.CollectorsByApp()
	require.Len(t, byApp, 2)

	for _, app := range byApp {
		assert.Equal(t, []string{"spawn_errors_total", "restarts_total"}, names(app.Metrics),
			"application %s", app.App)
	}
}
