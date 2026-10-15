package metricsmap_test

import (
	"testing"

	"github.com/Educentr/go-project-starter/internal/pkg/metricsmap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVarName(t *testing.T) {
	tests := []struct {
		metricName string
		want       string
	}{
		{"spawn_errors_total", "SpawnErrorsTotal"},
		{"queue_depth", "QueueDepth"},
		{"restarts", "Restarts"},
		{"go_gc_duration_seconds", "GoGcDurationSeconds"},
		// ':' is legal in a Prometheus name but not in a Go identifier, so it
		// folds the same way '_' does.
		{"job:latency:p99", "JobLatencyP99"},
		{"_leading_underscore", "LeadingUnderscore"},
	}

	for _, tt := range tests {
		t.Run(tt.metricName, func(t *testing.T) {
			assert.Equal(t, tt.want, metricsmap.Metric{Name: tt.metricName}.VarName())
		})
	}
}

func TestConstructorAndOptsAndGoType(t *testing.T) {
	tests := []struct {
		metricType  string
		constructor string
		opts        string
		goType      string
	}{
		{metricsmap.TypeCounter, "prometheus.NewCounter", "prometheus.CounterOpts", "*prometheus.Counter"},
		{metricsmap.TypeCounterVec, "prometheus.NewCounterVec", "prometheus.CounterOpts", "*prometheus.CounterVec"},
		{metricsmap.TypeGauge, "prometheus.NewGauge", "prometheus.GaugeOpts", "*prometheus.Gauge"},
		{metricsmap.TypeHistogramVec, "prometheus.NewHistogramVec", "prometheus.HistogramOpts", "*prometheus.HistogramVec"},
		{metricsmap.TypeSummary, "prometheus.NewSummary", "prometheus.SummaryOpts", "*prometheus.Summary"},
	}

	for _, tt := range tests {
		t.Run(tt.metricType, func(t *testing.T) {
			m := metricsmap.Metric{Type: tt.metricType}
			assert.Equal(t, tt.constructor, m.Constructor())
			assert.Equal(t, tt.opts, m.OptsType())
			assert.Equal(t, tt.goType, m.GoType())
		})
	}
}

func TestLabelsLiteral(t *testing.T) {
	assert.Empty(t, metricsmap.Metric{}.LabelsLiteral())
	assert.Equal(t,
		`[]string{"reason", "stage"}`,
		metricsmap.Metric{Labels: metricsmap.Labels{"reason", "stage"}}.LabelsLiteral())
}

func TestBucketsLiteral(t *testing.T) {
	// A histogram with no declared buckets must still get usable ones:
	// an empty bucket list would observe nothing.
	assert.Equal(t, "prometheus.DefBuckets",
		metricsmap.Metric{Type: metricsmap.TypeHistogram}.BucketsLiteral())

	assert.Equal(t, "[]float64{0.05, 0.25, 1, 5, 30}",
		metricsmap.Metric{
			Type:    metricsmap.TypeHistogramVec,
			Buckets: []float64{0.05, 0.25, 1, 5, 30},
		}.BucketsLiteral())

	// Non-histograms take no buckets at all.
	assert.Empty(t, metricsmap.Metric{Type: metricsmap.TypeCounter, Buckets: []float64{1}}.BucketsLiteral())
}

func TestObjectivesLiteral(t *testing.T) {
	// No declared objectives means client_golang's own default (only _sum and
	// _count): invented quantile targets would be as wrong as invented alert
	// thresholds.
	assert.Empty(t, metricsmap.Metric{Type: metricsmap.TypeSummary}.ObjectivesLiteral())

	assert.Equal(t, "map[float64]float64{0.5: 0.05, 0.99: 0.001}",
		metricsmap.Metric{
			Type: metricsmap.TypeSummary,
			Objectives: []metricsmap.Objective{
				{Quantile: 0.5, Error: 0.05},
				{Quantile: 0.99, Error: 0.001},
			},
		}.ObjectivesLiteral())

	assert.Empty(t, metricsmap.Metric{
		Type:       metricsmap.TypeCounter,
		Objectives: []metricsmap.Objective{{Quantile: 0.99}},
	}.ObjectivesLiteral())
}

func TestValidateVarNames_CollisionAfterFolding(t *testing.T) {
	// "queue:depth" and "queue_depth" are two distinct Prometheus metrics that
	// fold to one Go identifier. Generating one variable for both would make
	// one of the two declarations silently unreachable.
	err := metricsmap.ValidateVarNames([]metricsmap.Metric{
		{Name: "queue_depth", Type: metricsmap.TypeGauge},
		{Name: "queue:depth", Type: metricsmap.TypeGauge},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, metricsmap.ErrVarNameCollision)
	assert.Contains(t, err.Error(), "QueueDepth")
}

func TestValidateVarNames_IgnoresNonGeneratedEntries(t *testing.T) {
	// External and generator-owned declarations produce no variable, so they
	// cannot collide in the generated package.
	err := metricsmap.ValidateVarNames([]metricsmap.Metric{
		{Name: "queue_depth", Type: metricsmap.TypeGauge},
		{Name: "queue:depth", Type: metricsmap.TypeGauge, External: true},
		{Name: "queue::depth", Type: metricsmap.TypeGauge, Owner: metricsmap.OwnerKafkaProducer},
	})
	require.NoError(t, err)
}

func TestValidateVarNames_InternalSetIsClean(t *testing.T) {
	internal, err := metricsmap.Internal()
	require.NoError(t, err)
	require.NoError(t, metricsmap.ValidateVarNames(internal.Metrics))
}
