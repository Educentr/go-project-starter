package metricsmap_test

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/Educentr/go-project-starter/internal/pkg/metricsmap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse_MinimalMap(t *testing.T) {
	// Exactly the shape from the request (§2.1), including the comma-separated
	// labels scalar and the folded help block.
	m, err := metricsmap.Parse([]byte(`
metrics:
  - name: "spawn_errors_total"
    type: CounterVec
    labels: reason, stage
    help: >-
      Spawn attempts that failed, by reason and stage.
`))
	require.NoError(t, err)
	require.Len(t, m.Metrics, 1)

	got := m.Metrics[0]
	assert.Equal(t, "spawn_errors_total", got.Name)
	assert.Equal(t, metricsmap.TypeCounterVec, got.Type)
	assert.Equal(t, metricsmap.Labels{"reason", "stage"}, got.Labels)
	assert.Equal(t, "Spawn attempts that failed, by reason and stage.", got.Help)
	assert.True(t, got.IsVec())
	assert.True(t, got.Generated())
	assert.Equal(t, "counter", got.Kind())
}

func TestParse_LabelsAsSequence(t *testing.T) {
	m, err := metricsmap.Parse([]byte(`
metrics:
  - name: spawn_errors_total
    type: CounterVec
    labels:
      - reason
      - stage
    help: Spawn failures.
`))
	require.NoError(t, err)
	assert.Equal(t, metricsmap.Labels{"reason", "stage"}, m.Metrics[0].Labels)
}

func TestParse_HistogramBuckets(t *testing.T) {
	m, err := metricsmap.Parse([]byte(`
metrics:
  - name: request_duration_seconds
    type: HistogramVec
    labels: route
    help: Request duration.
    buckets: [0.01, 0.1, 1, 10]
`))
	require.NoError(t, err)
	assert.Equal(t, []float64{0.01, 0.1, 1, 10}, m.Metrics[0].Buckets)
	assert.Equal(t, "histogram", m.Metrics[0].Kind())
}

func TestParse_SummaryObjectivesMapping(t *testing.T) {
	m, err := metricsmap.Parse([]byte(`
metrics:
  - name: batch_size
    type: Summary
    help: Batch size.
    objectives: {0.5: 0.05, 0.99: 0.001}
`))
	require.NoError(t, err)
	require.Len(t, m.Metrics[0].Objectives, 2)
	assert.Equal(t, 0.5, m.Metrics[0].Objectives[0].Quantile)
	assert.InDelta(t, 0.05, m.Metrics[0].Objectives[0].Error, 1e-9)
	assert.Equal(t, 0.99, m.Metrics[0].Objectives[1].Quantile)
}

func TestParse_SummaryObjectivesExplicitList(t *testing.T) {
	m, err := metricsmap.Parse([]byte(`
metrics:
  - name: batch_size
    type: SummaryVec
    labels: kind
    help: Batch size.
    objectives:
      - quantile: 0.99
        error: 0.001
`))
	require.NoError(t, err)
	require.Len(t, m.Metrics[0].Objectives, 1)
	assert.Equal(t, 0.99, m.Metrics[0].Objectives[0].Quantile)
}

func TestParse_ExternalMetricIsNotGenerated(t *testing.T) {
	m, err := metricsmap.Parse([]byte(`
metrics:
  - name: go_goroutines
    type: Gauge
    help: Number of goroutines.
    external: true
  - name: own_total
    type: Counter
    help: Own counter.
`))
	require.NoError(t, err)

	generatable := m.Generatable()
	require.Len(t, generatable, 1)
	assert.Equal(t, "own_total", generatable[0].Name)
}

func TestParse_UnknownFieldIsRejected(t *testing.T) {
	// A typo in a field name must not be swallowed: the map is a contract and
	// a silently ignored key is a declaration that does not take effect.
	_, err := metricsmap.Parse([]byte(`
metrics:
  - name: own_total
    type: Counter
    help: Own counter.
    lables: oops
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lables")
}

func TestValidate_Problems(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantMsg string
	}{
		{
			name:    "empty map",
			yaml:    "metrics: []",
			wantMsg: "no metrics declared",
		},
		{
			name:    "missing name",
			yaml:    "metrics:\n  - type: Counter\n    help: No name.",
			wantMsg: "name is required",
		},
		{
			name:    "invalid metric name",
			yaml:    "metrics:\n  - name: has-a-dash\n    type: Counter\n    help: Bad name.",
			wantMsg: "is not a valid Prometheus metric name",
		},
		{
			// Prometheus accepts this name; Go has no identifier left after
			// folding '_' and ':' away, so the generated package would not
			// compile. Caught here rather than by the Go compiler on
			// `var  = prometheus.NewCounter(`.
			name:    "name that leaves no Go identifier",
			yaml:    "metrics:\n  - name: \"_\"\n    type: Counter\n    help: Only separators.",
			wantMsg: "leaves no Go identifier",
		},
		{
			name:    "name of only colons",
			yaml:    "metrics:\n  - name: \"::\"\n    type: Counter\n    help: Only separators.",
			wantMsg: "leaves no Go identifier",
		},
		{
			// Prometheus accepts a leading '_' before a digit; folding the
			// separator away promotes the digit to the front, and Go has no
			// identifier starting with a digit. Caught here rather than by the
			// Go compiler on `var 123Total = prometheus.NewCounter(`.
			name:    "name whose identifier would start with a digit",
			yaml:    "metrics:\n  - name: _123_total\n    type: Counter\n    help: Leading separator then digits.",
			wantMsg: "which starts with a digit",
		},
		{
			name:    "colon-prefixed name whose identifier would start with a digit",
			yaml:    "metrics:\n  - name: \":9x\"\n    type: Counter\n    help: Leading colon then a digit.",
			wantMsg: "which starts with a digit",
		},
		{
			name:    "empty file",
			yaml:    "   \n\n",
			wantMsg: "the file is empty",
		},
		{
			name:    "missing help",
			yaml:    "metrics:\n  - name: own_total\n    type: Counter",
			wantMsg: "help is required",
		},
		{
			name:    "unknown type",
			yaml:    "metrics:\n  - name: own_total\n    type: Countre\n    help: Typo.",
			wantMsg: `type "Countre" is unknown`,
		},
		{
			name:    "vec without labels",
			yaml:    "metrics:\n  - name: own_total\n    type: CounterVec\n    help: No labels.",
			wantMsg: "type CounterVec requires labels; use Counter",
		},
		{
			name:    "non-vec with labels",
			yaml:    "metrics:\n  - name: own_total\n    type: Counter\n    labels: reason\n    help: Stray labels.",
			wantMsg: "type Counter takes no labels; use CounterVec",
		},
		{
			name:    "invalid label name",
			yaml:    "metrics:\n  - name: own_total\n    type: CounterVec\n    labels: not-a-label\n    help: Bad label.",
			wantMsg: "is not a valid Prometheus label name",
		},
		{
			name:    "reserved label prefix",
			yaml:    "metrics:\n  - name: own_total\n    type: CounterVec\n    labels: __name\n    help: Reserved.",
			wantMsg: "reserved for Prometheus internals",
		},
		{
			name:    "duplicate label",
			yaml:    "metrics:\n  - name: own_total\n    type: CounterVec\n    labels: reason, reason\n    help: Dup label.",
			wantMsg: `label "reason" is declared twice`,
		},
		{
			name:    "buckets on a counter",
			yaml:    "metrics:\n  - name: own_total\n    type: Counter\n    help: Not a histogram.\n    buckets: [1, 2]",
			wantMsg: "buckets are only valid for Histogram/HistogramVec",
		},
		{
			name:    "buckets not increasing",
			yaml:    "metrics:\n  - name: own_seconds\n    type: Histogram\n    help: Histogram.\n    buckets: [1, 1, 2]",
			wantMsg: "buckets must be strictly increasing",
		},
		{
			name:    "objectives on a histogram",
			yaml:    "metrics:\n  - name: own_seconds\n    type: Histogram\n    help: Histogram.\n    objectives: {0.99: 0.001}",
			wantMsg: "objectives are only valid for Summary/SummaryVec",
		},
		{
			name:    "objective quantile out of range",
			yaml:    "metrics:\n  - name: own_size\n    type: Summary\n    help: Summary.\n    objectives: {1.5: 0.001}",
			wantMsg: "outside the open interval",
		},
		{
			name: "duplicate metric name",
			yaml: "metrics:\n  - name: own_total\n    type: Counter\n    help: First.\n" +
				"  - name: own_total\n    type: Gauge\n    help: Second.",
			wantMsg: "duplicate metric name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := metricsmap.Parse([]byte(tt.yaml))
			require.Error(t, err)
			assert.ErrorIs(t, err, metricsmap.ErrInvalidMap)
			assert.Contains(t, err.Error(), tt.wantMsg)
		})
	}
}

func TestValidate_ReportsEveryProblemAtOnce(t *testing.T) {
	// Fixing a bad map should take one round trip, not one per entry.
	_, err := metricsmap.Parse([]byte(`
metrics:
  - name: has-a-dash
    type: Counter
    help: Bad name.
  - name: own_total
    type: Countre
    help: Bad type.
  - name: third
    type: Counter
`))
	require.Error(t, err)

	msg := err.Error()
	assert.Contains(t, msg, "metrics[0]")
	assert.Contains(t, msg, "metrics[1]")
	assert.Contains(t, msg, "metrics[2]")
}

func TestMerge_DisjointSets(t *testing.T) {
	merged, err := metricsmap.Merge(
		metricsmap.Set{Origin: "a", Metrics: []metricsmap.Metric{{Name: "a_total"}}},
		metricsmap.Set{Origin: "b", Metrics: []metricsmap.Metric{{Name: "b_total"}}},
	)
	require.NoError(t, err)
	require.Len(t, merged, 2)
	assert.Equal(t, "a_total", merged[0].Name)
	assert.Equal(t, "b_total", merged[1].Name)
}

func TestMerge_CollisionIsAnErrorNotAnOverride(t *testing.T) {
	_, err := metricsmap.Merge(
		metricsmap.Set{Origin: "internal", Metrics: []metricsmap.Metric{{Name: "go_goroutines"}}},
		metricsmap.Set{Origin: "service", Metrics: []metricsmap.Metric{{Name: "go_goroutines"}}},
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, metricsmap.ErrNameCollision)
	assert.Contains(t, err.Error(), "internal[0]")
	assert.Contains(t, err.Error(), "service[0]")
}

func TestInternal_IsValidAndCoversKafkaProducer(t *testing.T) {
	internal, err := metricsmap.Internal()
	require.NoError(t, err, "the embedded internal map must parse and validate")
	require.NotEmpty(t, internal.Metrics)

	for _, name := range []string{
		metricsmap.KafkaPublishTotal,
		metricsmap.KafkaPublishErrors,
		metricsmap.KafkaPublishDuration,
	} {
		metric, err := metricsmap.MustFindInternal(name)
		require.NoError(t, err)
		assert.NotEmpty(t, metric.Help)
		assert.True(t, metric.OwnedBy(metricsmap.OwnerKafkaProducer),
			"%s is constructed by the generated Kafka producer, not by the shared collector package", name)
		assert.False(t, metric.Generated(),
			"%s must not land in the shared collector package — it needs one collector per producer", name)
		assert.Equal(t, metricsmap.CapabilityKafkaProducer, metric.Requires,
			"%s must not apply to projects without a Kafka producer", name)
	}

	// The Go runtime and ogen sets must stay declaration-only: nothing in the
	// generator constructs them.
	for _, name := range []string{"go_goroutines", "ogen_server_request_count_total"} {
		metric, ok := internal.Find(name)
		require.True(t, ok, "%s must be declared", name)
		assert.False(t, metric.Generated(), "%s is emitted by a library, not by generated code", name)
		assert.True(t, metric.External, "%s is emitted by a library", name)
	}
}

func TestInternal_RejectsReservedFieldsInAServiceMap(t *testing.T) {
	// `owner` and `requires` decide where a collector is constructed and when
	// a declaration applies — the service's contract repository does not get
	// to answer either question.
	for _, field := range []string{"owner: kafka_producer", "requires: kafka_producer"} {
		t.Run(field, func(t *testing.T) {
			_, err := metricsmap.ParseService([]byte(`
metrics:
  - name: own_total
    type: Counter
    help: Own counter.
    ` + field + `
`))
			require.Error(t, err)
			assert.ErrorIs(t, err, metricsmap.ErrInvalidMap)
			assert.Contains(t, err.Error(), "reserved for the generator's internal map")
		})
	}
}

func TestValidate_ExternalAndOwnerAreMutuallyExclusive(t *testing.T) {
	_, err := metricsmap.Parse([]byte(`
metrics:
  - name: own_total
    type: Counter
    help: Own counter.
    external: true
    owner: kafka_producer
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

func TestValidate_UnknownOwnerAndCapability(t *testing.T) {
	_, err := metricsmap.Parse([]byte(`
metrics:
  - name: own_total
    type: Counter
    help: Own counter.
    owner: nobody
    requires: telepathy
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `owner "nobody" is unknown`)
	assert.Contains(t, err.Error(), `requires "telepathy" is not a known project capability`)
}

func TestMergeWithInternal_ServiceCollidingWithInternalFails(t *testing.T) {
	service, err := metricsmap.ParseService([]byte(`
metrics:
  - name: go_goroutines
    type: Gauge
    help: Shadowing an internal name.
`))
	require.NoError(t, err)

	_, err = metricsmap.MergeWithInternal(service, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, metricsmap.ErrNameCollision)
	assert.Contains(t, err.Error(), metricsmap.OriginInternal)
	assert.Contains(t, err.Error(), metricsmap.OriginService)
}

func TestMergeWithInternal_DropsDeclarationsTheProjectCannotEmit(t *testing.T) {
	// A project with no Kafka producer and no REST transports must not carry
	// those names at all: they would reserve names against the service and
	// produce dashboard panels that can only read empty.
	bare, err := metricsmap.MergeWithInternal(nil, nil)
	require.NoError(t, err)

	for _, metric := range bare {
		assert.Empty(t, metric.Requires, "%s should have been filtered out", metric.Name)
	}

	assert.False(t, containsName(bare, metricsmap.KafkaPublishTotal))
	assert.False(t, containsName(bare, "ogen_server_request_count_total"))
	assert.True(t, containsName(bare, "go_goroutines"), "runtime metrics are unconditional")

	// With the capabilities present, the same names come back.
	full, err := metricsmap.MergeWithInternal(nil, map[string]bool{
		metricsmap.CapabilityKafkaProducer: true,
		metricsmap.CapabilityRESTServer:    true,
	})
	require.NoError(t, err)
	assert.True(t, containsName(full, metricsmap.KafkaPublishTotal))
	assert.True(t, containsName(full, "ogen_server_request_count_total"))
	assert.False(t, containsName(full, "ogen_client_request_count_total"), "no client transport declared")
}

func TestMergeWithInternal_FilteredNameIsFreeForTheService(t *testing.T) {
	// The flip side of filtering: a project without a Kafka producer may
	// declare its own metric under that name without a spurious collision.
	service, err := metricsmap.ParseService([]byte(`
metrics:
  - name: kafka_producer_publish_total
    type: Counter
    help: Our own hand-rolled publish counter.
`))
	require.NoError(t, err)

	_, err = metricsmap.MergeWithInternal(service, nil)
	require.NoError(t, err)

	_, err = metricsmap.MergeWithInternal(service, map[string]bool{metricsmap.CapabilityKafkaProducer: true})
	require.Error(t, err, "with a producer present the name is taken")
	assert.ErrorIs(t, err, metricsmap.ErrNameCollision)
}

func containsName(metrics []metricsmap.Metric, name string) bool {
	for _, metric := range metrics {
		if metric.Name == name {
			return true
		}
	}

	return false
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := metricsmap.Load("/nonexistent/metrics-map.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read metrics map")
}

func TestLoad_ReadsFromDisk(t *testing.T) {
	path := writeTempMap(t, `
metrics:
  - name: own_total
    type: Counter
    help: Own counter.
`)

	m, err := metricsmap.Load(path)
	require.NoError(t, err)
	require.Len(t, m.Metrics, 1)
	assert.Equal(t, "own_total", m.Metrics[0].Name)
}

func TestLoad_InvalidMapNamesTheFile(t *testing.T) {
	path := writeTempMap(t, "metrics:\n  - name: own_total\n    type: Counter")

	_, err := metricsmap.Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), path, "the error must say which file to fix")
	assert.True(t, errors.Is(err, metricsmap.ErrInvalidMap))
}

func TestKnownTypes_IsSortedAndComplete(t *testing.T) {
	types := metricsmap.KnownTypes()
	require.Len(t, types, 8)
	assert.Equal(t, strings.Join([]string{
		"Counter", "CounterVec", "Gauge", "GaugeVec",
		"Histogram", "HistogramVec", "Summary", "SummaryVec",
	}, ","), strings.Join(types, ","))
}

func writeTempMap(t *testing.T, content string) string {
	t.Helper()

	path := t.TempDir() + "/metrics-map.yaml"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}
