package grafana_test

import (
	"testing"

	"github.com/Educentr/go-project-starter/internal/pkg/grafana"
	"github.com/Educentr/go-project-starter/internal/pkg/metricsmap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeclaredMetricPanels_CanonicalShapePerType(t *testing.T) {
	tests := []struct {
		name    string
		metric  metricsmap.Metric
		exprs   []string
		legends []string
	}{
		{
			name: "labelled counter charts a rate broken down by its labels",
			metric: metricsmap.Metric{
				Name:   "spawn_errors_total",
				Type:   metricsmap.TypeCounterVec,
				Labels: metricsmap.Labels{"reason", "stage"},
			},
			exprs:   []string{"sum by(reason, stage) (rate(spawn_errors_total[$__rate_interval]))"},
			legends: []string{"{{reason}} - {{stage}}"},
		},
		{
			name: "unlabelled counter charts a plain rate",
			metric: metricsmap.Metric{
				Name: "restarts_total",
				Type: metricsmap.TypeCounter,
			},
			exprs:   []string{"sum(rate(restarts_total[$__rate_interval]))"},
			legends: []string{"__auto"},
		},
		{
			name: "gauge charts the value itself, not a rate",
			metric: metricsmap.Metric{
				Name: "queue_depth",
				Type: metricsmap.TypeGauge,
			},
			exprs:   []string{"queue_depth"},
			legends: []string{"__auto"},
		},
		{
			name: "labelled gauge sums by its labels",
			metric: metricsmap.Metric{
				Name:   "queue_depth",
				Type:   metricsmap.TypeGaugeVec,
				Labels: metricsmap.Labels{"queue"},
			},
			exprs:   []string{"sum by(queue) (queue_depth)"},
			legends: []string{"{{queue}}"},
		},
		{
			name: "histogram charts p99 and p95 off the _bucket series, grouped by le",
			metric: metricsmap.Metric{
				Name:   "task_duration_seconds",
				Type:   metricsmap.TypeHistogramVec,
				Labels: metricsmap.Labels{"kind"},
			},
			exprs: []string{
				"histogram_quantile(0.99, sum by(le, kind) (rate(task_duration_seconds_bucket[$__rate_interval])))",
				"histogram_quantile(0.95, sum by(le, kind) (rate(task_duration_seconds_bucket[$__rate_interval])))",
			},
			legends: []string{"p99 {{kind}}", "p95 {{kind}}"},
		},
		{
			name: "summary with objectives reads the quantile label, not histogram_quantile",
			metric: metricsmap.Metric{
				Name:       "batch_size",
				Type:       metricsmap.TypeSummary,
				Objectives: []metricsmap.Objective{{Quantile: 0.99, Error: 0.001}},
			},
			exprs:   []string{"sum by(quantile) (batch_size)"},
			legends: []string{"q{{quantile}}"},
		},
		{
			name: "summary without objectives falls back to the average it actually carries",
			metric: metricsmap.Metric{
				Name: "batch_size",
				Type: metricsmap.TypeSummary,
			},
			exprs: []string{
				"sum(rate(batch_size_sum[$__rate_interval])) / sum(rate(batch_size_count[$__rate_interval]))",
			},
			legends: []string{"avg"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			panels := grafana.DeclaredMetricPanels([]metricsmap.Metric{tt.metric})
			require.Len(t, panels, 1)

			panel := panels[0]
			assert.Equal(t, tt.metric.Name, panel.Title)
			assert.Equal(t, "timeseries", panel.Type)
			assert.Equal(t, grafana.DatasourcePrometheus, panel.Datasource)

			require.Len(t, panel.Targets, len(tt.exprs))

			for i, wantExpr := range tt.exprs {
				assert.Equal(t, wantExpr, panel.Targets[i].Expr)
				assert.Equal(t, tt.legends[i], panel.Targets[i].LegendFormat)
			}
		})
	}
}

func TestDeclaredMetricPanels_SkipsMetricsWithADedicatedRow(t *testing.T) {
	// The Go runtime and HTTP rows are hand-built and know more about their
	// metrics than a shape derived from a type. A declaration for one of them
	// still reserves the name; it must not produce a second, poorer panel.
	panels := grafana.DeclaredMetricPanels([]metricsmap.Metric{
		{Name: "go_goroutines", Type: metricsmap.TypeGauge, External: true},
		{Name: "process_open_fds", Type: metricsmap.TypeGauge, External: true},
		{
			Name:     "ogen_server_request_count_total",
			Type:     metricsmap.TypeCounterVec,
			Labels:   metricsmap.Labels{"http_route"},
			External: true,
		},
		{
			Name:     "ogen_client_duration_milliseconds",
			Type:     metricsmap.TypeHistogramVec,
			Labels:   metricsmap.Labels{"client_name"},
			External: true,
		},
		{Name: "queue_depth", Type: metricsmap.TypeGauge},
	})

	require.Len(t, panels, 1)
	assert.Equal(t, "queue_depth", panels[0].Title)
}

func TestDeclaredMetricPanels_ChartsExternalDeclarations(t *testing.T) {
	// An external metric gets no collector but does get a panel — charting it
	// is the whole reason a service declares a library's metric.
	panels := grafana.DeclaredMetricPanels([]metricsmap.Metric{{
		Name:     "otel_sdk_exporter_spans_total",
		Type:     metricsmap.TypeCounterVec,
		Labels:   metricsmap.Labels{"outcome"},
		External: true,
	}})

	require.Len(t, panels, 1)
	assert.Contains(t, panels[0].Targets[0].Expr, "otel_sdk_exporter_spans_total")
}

func TestDeclaredMetricPanels_NoThresholdsAreInvented(t *testing.T) {
	// Out of scope by §3 of the request: a threshold is tied to the capacity
	// of a particular installation, so a generated one would be made up.
	panels := grafana.DeclaredMetricPanels([]metricsmap.Metric{
		{Name: "queue_depth", Type: metricsmap.TypeGauge},
	})
	require.Len(t, panels, 1)

	for _, target := range panels[0].Targets {
		assert.NotContains(t, target.Expr, ">")
		assert.NotContains(t, target.Expr, "<")
	}
}

func TestDeclaredMetricsRow(t *testing.T) {
	_, ok := grafana.DeclaredMetricsRow(nil)
	assert.False(t, ok, "no declarations means no row")

	_, ok = grafana.DeclaredMetricsRow([]metricsmap.Metric{
		{Name: "go_goroutines", Type: metricsmap.TypeGauge, External: true},
	})
	assert.False(t, ok, "a row of nothing but skipped metrics must not be emitted")

	row, ok := grafana.DeclaredMetricsRow([]metricsmap.Metric{
		{Name: "queue_depth", Type: metricsmap.TypeGauge},
	})
	require.True(t, ok)
	assert.Equal(t, "Service Metrics", row.Title)
	assert.True(t, row.Collapsed)
	assert.Len(t, row.Panels, 1)
}

func TestGetDashboardRows_DeclaredMetricsComeLast(t *testing.T) {
	cfg := grafana.Config{
		Datasources: []grafana.Datasource{
			{Name: "Prometheus", Type: grafana.DatasourcePrometheus, UID: "ds-prometheus"},
		},
	}

	rows := cfg.GetDashboardRows(
		"api",
		[]grafana.TransportInfo{{Name: "api", GeneratorType: "ogen"}},
		[]metricsmap.Metric{{Name: "queue_depth", Type: metricsmap.TypeGauge}},
	)

	require.NotEmpty(t, rows)
	assert.Equal(t, "Service Metrics", rows[len(rows)-1].Title)
	assert.Equal(t, "Go Runtime", rows[0].Title)
}

func TestGetDashboardRows_NoPrometheusNoMetricRow(t *testing.T) {
	// Every declared-metric panel is PromQL; without a Prometheus datasource
	// there is nothing to point it at.
	cfg := grafana.Config{
		Datasources: []grafana.Datasource{
			{Name: "Loki", Type: grafana.DatasourceLoki, UID: "ds-loki"},
		},
	}

	rows := cfg.GetDashboardRows("api", nil, []metricsmap.Metric{
		{Name: "queue_depth", Type: metricsmap.TypeGauge},
	})

	for _, row := range rows {
		assert.NotEqual(t, "Service Metrics", row.Title)
	}
}
