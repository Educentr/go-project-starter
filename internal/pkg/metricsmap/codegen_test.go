package metricsmap_test

import (
	"go/token"
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
		// A leading separator before a digit promotes that digit to the front.
		// VarName reports it as-is; rejecting it is Validate's job, so that the
		// error names the declaration rather than the generated file. See
		// TestValidate_AcceptedNamesAreDeclarableInGo.
		{"_123_total", "123Total"},
	}

	for _, tt := range tests {
		t.Run(tt.metricName, func(t *testing.T) {
			assert.Equal(t, tt.want, metricsmap.Metric{Name: tt.metricName}.VarName())
		})
	}
}

// TestValidate_AcceptedNamesAreDeclarableInGo pins the invariant the generated
// collector package rests on: every name Validate accepts must be usable as
// `var <VarName> = prometheus.New…`. It asserts the property over awkward names
// rather than a list of known-bad ones on purpose — the digit-start case got
// through a guard that only rejected names folding away to nothing, and the next
// such name would get through a guard that only rejects this one.
func TestValidate_AcceptedNamesAreDeclarableInGo(t *testing.T) {
	// Every name here is legal under the Prometheus grammar
	// (^[a-zA-Z_:][a-zA-Z0-9_:]*$), so each one reaches the identifier checks.
	names := []string{
		"spawn_errors_total", "queue_depth", "job:latency:p99",
		"_leading_underscore", "_a_total", "A", "_",
		"::", "_:_", "_123_total", "::9x", "__0_seconds",
		"_1", ":2:", "go_gc_duration_seconds",
	}

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			metric := metricsmap.Metric{Name: name, Type: "Counter", Help: "Awkward but legal name."}

			err := (&metricsmap.Map{Metrics: []metricsmap.Metric{metric}}).Validate()
			if err != nil {
				// Rejected — nothing is generated from it, invariant holds.
				assert.ErrorIs(t, err, metricsmap.ErrInvalidMap)

				return
			}

			assert.True(t, token.IsIdentifier(metric.VarName()),
				"Validate accepted %q, but it generates %q which is not a Go identifier",
				name, metric.VarName())
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
