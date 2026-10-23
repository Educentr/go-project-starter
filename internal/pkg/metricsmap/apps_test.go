package metricsmap_test

import (
	"errors"
	"testing"

	"github.com/Educentr/go-project-starter/internal/pkg/metricsmap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse_AppsInBothWrittenForms(t *testing.T) {
	m, err := metricsmap.Parse([]byte(`
metrics:
  - name: spawn_errors_total
    type: Counter
    help: Spawn failures.
    apps: api, worker
  - name: queue_depth
    type: Gauge
    help: Queue depth.
    apps:
      - api
  - name: restarts_total
    type: Counter
    help: Restarts.
`))
	require.NoError(t, err)

	assert.Equal(t, metricsmap.Apps{"api", "worker"}, m.Metrics[0].Apps)
	assert.Equal(t, metricsmap.Apps{"api"}, m.Metrics[1].Apps)
	assert.Empty(t, m.Metrics[2].Apps)
}

func TestParse_AppsIsAllowedInAServiceMap(t *testing.T) {
	// Unlike `owner` and `requires`, `apps` is the service's own answer to
	// which of its processes publishes a metric.
	_, err := metricsmap.ParseService([]byte(`
metrics:
  - name: restarts_total
    type: Counter
    help: Restarts.
    apps: [api]
`))
	require.NoError(t, err)
}

// TestParse_EmptyAppsIsRejected pins the asymmetry with `labels`: an empty
// label list means "no labels", but an empty `apps` list would mean "published
// by EVERY application" — the opposite of what the line reads as.
func TestParse_EmptyAppsIsRejected(t *testing.T) {
	for name, apps := range map[string]string{
		"empty scalar":    `apps: ""`,
		"only separators": `apps: ","`,
		"empty sequence":  `apps: []`,
		"only whitespace": `apps: " "`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := metricsmap.Parse([]byte(`
metrics:
  - name: restarts_total
    type: Counter
    help: Restarts.
    ` + apps + "\n"))
			require.Error(t, err)
			assert.ErrorIs(t, err, metricsmap.ErrMalformedMap)
			assert.Contains(t, err.Error(), "declared but names no application")
		})
	}
}

func TestValidate_AppsShape(t *testing.T) {
	for name, tc := range map[string]struct {
		yaml string
		want string
	}{
		"empty entry": {
			yaml: `
metrics:
  - name: restarts_total
    type: Counter
    help: Restarts.
    apps:
      - ""
`,
			want: "apps contains an empty application name",
		},
		"duplicate entry": {
			yaml: `
metrics:
  - name: restarts_total
    type: Counter
    help: Restarts.
    apps: [api, api]
`,
			want: `application "api" is listed twice in apps`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := metricsmap.Parse([]byte(tc.yaml))
			require.Error(t, err)
			assert.ErrorIs(t, err, metricsmap.ErrInvalidMap)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestPublishedBy(t *testing.T) {
	assigned := metricsmap.Metric{Name: "a", Apps: metricsmap.Apps{"api"}}
	assert.True(t, assigned.PublishedBy("api"))
	assert.False(t, assigned.PublishedBy("worker"))

	// No `apps` means every application — the behavior from before the field
	// existed, so an existing map keeps generating what it generated.
	unassigned := metricsmap.Metric{Name: "a"}
	assert.True(t, unassigned.PublishedBy("api"))
	assert.True(t, unassigned.PublishedBy("worker"))
}

func TestAppScope_Publishes(t *testing.T) {
	producer := metricsmap.AppScope{
		Name:           "api",
		ExposesMetrics: true,
		Components:     map[string]bool{metricsmap.OwnerKafkaProducer: true},
	}
	plain := metricsmap.AppScope{Name: "admin", ExposesMetrics: true}

	owned := metricsmap.Metric{Name: "kafka_producer_publish_total", Owner: metricsmap.OwnerKafkaProducer}

	// An owned declaration belongs to the applications that run the component
	// constructing it, which the service's `apps` list cannot state.
	assert.True(t, producer.Publishes(owned))
	assert.False(t, plain.Publishes(owned))

	shared := metricsmap.Metric{Name: "restarts_total"}
	assert.True(t, producer.Publishes(shared))
	assert.True(t, plain.Publishes(shared))

	assigned := metricsmap.Metric{Name: "admin_actions_total", Apps: metricsmap.Apps{"admin"}}
	assert.False(t, producer.Publishes(assigned))
	assert.True(t, plain.Publishes(assigned))
}

func TestValidateApps(t *testing.T) {
	scopes := []metricsmap.AppScope{
		{Name: "api", ExposesMetrics: true},
		{Name: "worker"},
	}

	require.NoError(t, metricsmap.ValidateApps([]metricsmap.Metric{
		{Name: "restarts_total"},
		{Name: "spawn_errors_total", Apps: metricsmap.Apps{"api"}},
	}, scopes))

	err := metricsmap.ValidateApps([]metricsmap.Metric{
		{Name: "spawn_errors_total", Apps: metricsmap.Apps{"apii"}},
		{Name: "restarts_total", Apps: metricsmap.Apps{"worker"}},
	}, scopes)
	require.Error(t, err)
	assert.True(t, errors.Is(err, metricsmap.ErrUnknownApp))

	// Both problems at once, and the hint spells the real names out so the
	// reader does not have to open project.yaml.
	assert.Contains(t, err.Error(), `apps names "apii", which is not an application of this project`)
	assert.Contains(t, err.Error(), "(declared applications: api, worker)")
	assert.Contains(t, err.Error(), `application "worker" has no sys transport`)

	// Metric NAME, not index: this check runs on the merged set, where the
	// position counts the generator's own declarations first and so points at
	// a line the service's map does not have.
	assert.Contains(t, err.Error(), `metric "spawn_errors_total":`)
	assert.NotContains(t, err.Error(), "metrics[")
}

// TestInternalMapDoesNotUseApps pins that the generator's own map cannot carry
// `apps`: it names applications of the USER's project, so an entry here would
// be reported against every project as that user's config error.
func TestInternalMapDoesNotUseApps(t *testing.T) {
	internal, err := metricsmap.Internal()
	require.NoError(t, err)

	for _, metric := range internal.Metrics {
		assert.Empty(t, metric.Apps, "internal declaration %q must not set apps", metric.Name)
	}
}

func TestKnownOwners_IsSortedAndComplete(t *testing.T) {
	assert.Equal(t, []string{metricsmap.OwnerKafkaProducer}, metricsmap.KnownOwners())
}
