package metricsmap_test

import (
	"errors"
	"testing"

	"github.com/Educentr/go-project-starter/internal/pkg/metricsmap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParse_GaugeSet covers the shape of a state-family declaration: it is an
// ordinary entry of the map, read with the same fields as any other, and the
// collector for it is generated.
func TestParse_GaugeSet(t *testing.T) {
	m, err := metricsmap.ParseService([]byte(`
metrics:
  - name: fleet_state
    type: GaugeSet
    labels: node, zone
    help: State of each known fleet node.
    apps: api
`))
	require.NoError(t, err)
	require.Len(t, m.Metrics, 1)

	metric := m.Metrics[0]

	assert.True(t, metric.IsGaugeSet())
	assert.True(t, metric.HasLabels())
	assert.False(t, metric.IsVec(), "a state family is not one of the client_golang *Vec constructors")
	assert.True(t, metric.Generated(), "the collector of a state family is generated")
	assert.Equal(t, metricsmap.KindGauge, metric.Kind(),
		"a state family is a gauge everywhere Prometheus and Grafana look")
	assert.Equal(t, metricsmap.Apps{"api"}, metric.Apps, "`apps` works as it does for any other entry")
	assert.Equal(t, []string{"node", "zone"}, []string(metric.Labels))
}

// TestGaugeSet_PublishesLikeAnyOtherEntry pins that the per-application split
// (`apps`, TOOLS-74) means for a state family exactly what it means for every
// other type: the question is answered by the declaration's `apps` list and
// the application's export channel, never by the type.
func TestGaugeSet_PublishesLikeAnyOtherEntry(t *testing.T) {
	named := metricsmap.Metric{
		Name:   "fleet_state",
		Type:   metricsmap.TypeGaugeSet,
		Labels: metricsmap.Labels{"node"},
		Help:   "State of each known fleet node.",
		Apps:   metricsmap.Apps{"supervisor"},
	}

	unnamed := named
	unnamed.Apps = nil

	scraped := metricsmap.AppScope{Name: "api", ExposesMetrics: true}
	addressed := metricsmap.AppScope{Name: "supervisor", ExposesMetrics: false}

	assert.False(t, scraped.Publishes(named), "a named declaration is published by the applications it names")
	assert.True(t, addressed.Publishes(named),
		"an application named by the declaration publishes it even without a /metrics endpoint")
	assert.True(t, scraped.Publishes(unnamed), "a declaration naming nobody is published by every scraped application")
	assert.False(t, addressed.Publishes(unnamed),
		"a declaration naming nobody must not leak into a process with no endpoint")
}

// TestGaugeSet_GeneratedNames pins the identifiers the collector package gets
// from one declaration. They are part of the contract with hand-written code —
// the tick names the sample type — so a change here is a breaking change for
// every service that declares a state family.
func TestGaugeSet_GeneratedNames(t *testing.T) {
	metric := metricsmap.Metric{
		Name:   "fleet_state",
		Type:   metricsmap.TypeGaugeSet,
		Labels: metricsmap.Labels{"node", "zone"},
		Help:   "State of each known fleet node.",
	}

	assert.Equal(t, "FleetState", metric.VarName())
	assert.Equal(t, "FleetStateSample", metric.SampleTypeName())
	assert.Equal(t, "FleetStateFunc", metric.SourceTypeName())
	assert.Equal(t, "GaugeSet[FleetStateSample]", metric.SetType())
	assert.Equal(t, "*GaugeSet[FleetStateSample]", metric.GoType())
	assert.Equal(t, "Value", metric.ValueField())

	assert.Equal(t, []metricsmap.SampleField{
		{Name: "Node", Label: "node"},
		{Name: "Zone", Label: "zone"},
	}, metric.SampleFields(), "sample fields keep declaration order: Collect hands the values to the descriptor in it")

	// No client_golang constructor stands behind a state family, and the
	// template must not be able to render half of one by accident.
	assert.Empty(t, metric.Constructor())
	assert.Empty(t, metric.OptsType())

	assert.Nil(t, metricsmap.Metric{Name: "queue_depth", Type: metricsmap.TypeGaugeVec}.SampleFields(),
		"only a state family has a sample type")
}

// TestGaugeSet_LabelsAreRequired pins that a state family without labels is
// rejected, with the hint that one series is a Gauge.
func TestGaugeSet_LabelsAreRequired(t *testing.T) {
	_, err := metricsmap.ParseService([]byte(`
metrics:
  - name: fleet_state
    type: GaugeSet
    help: State of the fleet.
`))
	require.Error(t, err)
	assert.True(t, errors.Is(err, metricsmap.ErrInvalidMap))
	assert.Contains(t, err.Error(), "type GaugeSet requires labels")
	assert.Contains(t, err.Error(), "use Gauge")
}

// TestGaugeSet_ExclusiveWithExternalAndOwner pins the two combinations that
// would generate nothing while reading as a declaration: `external` and
// `owner` both mean "the collector is built elsewhere", and GaugeSet is a
// statement about how the generated package builds it.
func TestGaugeSet_ExclusiveWithExternalAndOwner(t *testing.T) {
	t.Run("external", func(t *testing.T) {
		_, err := metricsmap.ParseService([]byte(`
metrics:
  - name: fleet_state
    type: GaugeSet
    labels: node
    help: State of each known fleet node.
    external: true
`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "type GaugeSet and external are mutually exclusive")
	})

	t.Run("owner", func(t *testing.T) {
		// `owner` is reserved for the generator's own map, so the rule is
		// checked through Parse, which accepts the field.
		err := (&metricsmap.Map{Metrics: []metricsmap.Metric{{
			Name:   "fleet_state",
			Type:   metricsmap.TypeGaugeSet,
			Labels: metricsmap.Labels{"node"},
			Help:   "State of each known fleet node.",
			Owner:  metricsmap.OwnerKafkaProducer,
		}}}).Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "type GaugeSet and owner are mutually exclusive")
	})
}

// TestGaugeSet_LabelsMustSurviveBecomingFields pins the price of generating a
// struct per family instead of taking a []string of label values: the labels
// have to produce distinct Go field names, and none of them may take the field
// that carries the value.
//
// Without these rules the generated package does not compile, and it is not a
// package the service wrote — the map is where the error has to surface.
func TestGaugeSet_LabelsMustSurviveBecomingFields(t *testing.T) {
	tests := []struct {
		name   string
		labels metricsmap.Labels
		want   string
	}{
		{
			name:   "two labels fold to one field",
			labels: metricsmap.Labels{"node_id", "node__id"},
			want:   `both generate the sample field NodeId of FleetStateSample`,
		},
		{
			name:   "label takes the value field",
			labels: metricsmap.Labels{"value"},
			want:   `generates the sample field Value of FleetStateSample, which is the field carrying the value`,
		},
		{
			name:   "label already written as the value field",
			labels: metricsmap.Labels{"Value"},
			want:   `generates the sample field Value of FleetStateSample`,
		},
		{
			name:   "field name would start with a digit",
			labels: metricsmap.Labels{"_1st_try"},
			want:   `generates the sample field 1stTry of FleetStateSample, which is not a Go identifier`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := (&metricsmap.Map{Metrics: []metricsmap.Metric{{
				Name:   "fleet_state",
				Type:   metricsmap.TypeGaugeSet,
				Labels: tt.labels,
				Help:   "State of each known fleet node.",
			}}}).Validate()
			require.Error(t, err)
			assert.True(t, errors.Is(err, metricsmap.ErrInvalidMap))
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

// TestGaugeSet_AcceptedLabelsGenerateDistinctFields asserts the property the
// rules above exist for, over the label names a real map carries: every
// accepted declaration produces a sample struct whose fields are valid Go
// identifiers, distinct, and none of them the value field.
func TestGaugeSet_AcceptedLabelsGenerateDistinctFields(t *testing.T) {
	labelSets := []metricsmap.Labels{
		{"node", "zone"},
		{"account_id", "kind"},
		{"a"},
		{"_leading", "trailing_"},
		{"value_kind", "values"},
		{"node_id", "nodeid"},
		{"x1", "x2"},
	}

	for _, labels := range labelSets {
		metric := metricsmap.Metric{
			Name:   "fleet_state",
			Type:   metricsmap.TypeGaugeSet,
			Labels: labels,
			Help:   "State of each known fleet node.",
		}

		err := (&metricsmap.Map{Metrics: []metricsmap.Metric{metric}}).Validate()
		if err != nil {
			t.Errorf("labels %v were rejected: %v", labels, err)

			continue
		}

		fields := metric.SampleFields()
		require.Len(t, fields, len(labels))

		seen := make(map[string]string, len(fields))

		for _, field := range fields {
			assert.NotEqual(t, metricsmap.SampleValueField, field.Name,
				"labels %v: field of %q collides with the value field", labels, field.Label)

			if prev, dup := seen[field.Name]; dup {
				t.Errorf("labels %v: %q and %q both produce field %s", labels, prev, field.Label, field.Name)
			}

			seen[field.Name] = field.Label
		}
	}
}

// TestGaugeSet_BucketsAndObjectivesStayRejected pins that the gauge rules
// apply: a state family is a gauge, so histogram buckets and summary
// objectives are as wrong on it as they are on a Gauge.
func TestGaugeSet_BucketsAndObjectivesStayRejected(t *testing.T) {
	err := (&metricsmap.Map{Metrics: []metricsmap.Metric{{
		Name:       "fleet_state",
		Type:       metricsmap.TypeGaugeSet,
		Labels:     metricsmap.Labels{"node"},
		Help:       "State of each known fleet node.",
		Buckets:    []float64{1, 2},
		Objectives: []metricsmap.Objective{{Quantile: 0.99, Error: 0.001}},
	}}}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "buckets are only valid for Histogram/HistogramVec, not GaugeSet")
	assert.Contains(t, err.Error(), "objectives are only valid for Summary/SummaryVec, not GaugeSet")
}
