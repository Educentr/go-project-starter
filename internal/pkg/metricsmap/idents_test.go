package metricsmap_test

import (
	"testing"

	"github.com/Educentr/go-project-starter/internal/pkg/metricsmap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateIdents_CollisionAfterFolding(t *testing.T) {
	// "queue:depth" and "queue_depth" are two distinct Prometheus metrics that
	// fold to one Go identifier. Generating one variable for both would make
	// one of the two declarations silently unreachable.
	err := metricsmap.ValidateIdents([]metricsmap.Metric{
		{Name: "queue_depth", Type: metricsmap.TypeGauge},
		{Name: "queue:depth", Type: metricsmap.TypeGauge},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, metricsmap.ErrIdentCollision)
	assert.Contains(t, err.Error(), `"queue_depth" and "queue:depth" both generate the variable QueueDepth`)
}

func TestValidateIdents_IgnoresNonGeneratedEntries(t *testing.T) {
	// External and generator-owned declarations produce nothing in the
	// collector package, so they cannot collide inside it.
	err := metricsmap.ValidateIdents([]metricsmap.Metric{
		{Name: "queue_depth", Type: metricsmap.TypeGauge},
		{Name: "queue:depth", Type: metricsmap.TypeGauge, External: true},
		{Name: "queue::depth", Type: metricsmap.TypeGauge, Owner: metricsmap.OwnerKafkaProducer},
	})
	require.NoError(t, err)
}

func TestValidateIdents_InternalSetIsClean(t *testing.T) {
	internal, err := metricsmap.Internal()
	require.NoError(t, err)
	require.NoError(t, metricsmap.ValidateIdents(internal.Metrics))
}

// TestValidateIdents_StateFamilyTypes covers what a state family adds to the
// namespace: besides its collector variable it generates the struct of its
// samples and the type of its source, so a name of one declaration can reach
// the types of another. Before the check existed such a map generated `done`
// and a collector package that redeclares a type.
func TestValidateIdents_StateFamilyTypes(t *testing.T) {
	family := metricsmap.Metric{
		Name:   "fleet_state",
		Type:   metricsmap.TypeGaugeSet,
		Labels: metricsmap.Labels{"node"},
	}

	tests := []struct {
		name    string
		metrics []metricsmap.Metric
		want    string
	}{
		{
			name: "variable against the sample type of a family",
			metrics: []metricsmap.Metric{
				family,
				{Name: "fleet_state_sample", Type: metricsmap.TypeGauge},
			},
			want: `"fleet_state_sample" generates the variable FleetStateSample, ` +
				`which is already the sample type of "fleet_state"`,
		},
		{
			name: "variable against the source type of a family",
			metrics: []metricsmap.Metric{
				family,
				{Name: "fleet_state_func", Type: metricsmap.TypeCounter},
			},
			want: `"fleet_state_func" generates the variable FleetStateFunc, ` +
				`which is already the source type of "fleet_state"`,
		},
		{
			name: "sample type of one family against the variable of another",
			metrics: []metricsmap.Metric{
				{Name: "fleet_state_sample", Type: metricsmap.TypeGauge},
				family,
			},
			want: `"fleet_state" generates the sample type FleetStateSample, ` +
				`which is already the variable of "fleet_state_sample"`,
		},
		{
			name: "two families, the name of one reaching the sample type of the other",
			metrics: []metricsmap.Metric{
				family,
				{Name: "fleet_state_sample", Type: metricsmap.TypeGaugeSet, Labels: metricsmap.Labels{"node"}},
			},
			want: `"fleet_state_sample" generates the variable FleetStateSample, ` +
				`which is already the sample type of "fleet_state"`,
		},
		{
			name: "two families whose derived names do not meet",
			metrics: []metricsmap.Metric{
				family,
				{Name: "fleet", Type: metricsmap.TypeGaugeSet, Labels: metricsmap.Labels{"node"}},
			},
			// Fleet, FleetSample, FleetFunc against FleetState,
			// FleetStateSample, FleetStateFunc: six identifiers, no pair.
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := metricsmap.ValidateIdents(tt.metrics)

			if tt.want == "" {
				require.NoError(t, err)

				return
			}

			require.Error(t, err)
			assert.ErrorIs(t, err, metricsmap.ErrIdentCollision)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

// TestValidateIdents_PackageOwnedNames covers the identifiers the collector
// package declares for itself. They were never reserved: a metric named
// `register` generated `var Register` against `func Register`, which is the
// same uncompilable package as a collision between two declarations.
func TestValidateIdents_PackageOwnedNames(t *testing.T) {
	for _, name := range []string{
		"collectors", "collectors_for", "err_unknown_app", "register", "register_for", "unregister",
	} {
		t.Run(name, func(t *testing.T) {
			err := metricsmap.ValidateIdents([]metricsmap.Metric{{Name: name, Type: metricsmap.TypeGauge}})
			require.Error(t, err)
			assert.ErrorIs(t, err, metricsmap.ErrIdentCollision)
			assert.Contains(t, err.Error(), "the generated collector package declares itself")
		})
	}
}

// TestValidateIdents_GaugeSetTypeIsReservedOnlyWhenEmitted pins that the
// reserved surface follows the generated package rather than being a fixed
// list: the generic collector is emitted only for a map that declares a state
// family, so only such a map may not call a metric `gauge_set`.
func TestValidateIdents_GaugeSetTypeIsReservedOnlyWhenEmitted(t *testing.T) {
	plain := []metricsmap.Metric{{Name: "gauge_set", Type: metricsmap.TypeGauge}}
	require.NoError(t, metricsmap.ValidateIdents(plain))

	withFamily := append(plain, metricsmap.Metric{
		Name:   "fleet_state",
		Type:   metricsmap.TypeGaugeSet,
		Labels: metricsmap.Labels{"node"},
	})

	err := metricsmap.ValidateIdents(withFamily)
	require.Error(t, err)
	assert.ErrorIs(t, err, metricsmap.ErrIdentCollision)
	assert.Contains(t, err.Error(),
		`"gauge_set" generates the variable GaugeSet, which the generated collector package declares itself`)
}

// TestReservedIdents_FollowsTheStateFamilyMachinery pins the shape of the list
// the collector template is checked against by
// assertDeclarationsCollideWithNothing.
func TestReservedIdents_FollowsTheStateFamilyMachinery(t *testing.T) {
	assert.Equal(t,
		[]string{"Collectors", "CollectorsFor", "ErrUnknownApp", "Register", "RegisterFor", "Unregister"},
		metricsmap.ReservedIdents(false))

	assert.Equal(t,
		[]string{"Collectors", "CollectorsFor", "ErrUnknownApp", "GaugeSet", "Register", "RegisterFor", "Unregister"},
		metricsmap.ReservedIdents(true))
}
