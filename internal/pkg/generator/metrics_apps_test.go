package generator

import (
	"errors"
	"testing"

	"github.com/Educentr/go-project-starter/internal/pkg/ds"
	"github.com/Educentr/go-project-starter/internal/pkg/metricsmap"
)

// TestAppComponentsCoversEveryKnownOwner holds the link between the closed set
// of metric owners and the per-application map that decides who publishes an
// owned declaration.
//
// AppScope.Publishes reads a missing key as "no application publishes this",
// so an owner nobody maps would silently drop its metrics from every
// registration and every dashboard — no error, no warning, just a metric that
// never appears on /metrics.
func TestAppComponentsCoversEveryKnownOwner(t *testing.T) {
	components := appComponents(ds.App{})

	for _, owner := range metricsmap.KnownOwners() {
		if _, ok := components[owner]; !ok {
			t.Errorf("owner %q is declarable in the metrics map but appComponents never maps it; "+
				"its metrics would be published by no application", owner)
		}
	}
}

func TestResolveMetricsApps(t *testing.T) {
	g := &Generator{
		Applications: ds.Apps{
			{Name: "api", Transports: sysTransports()},
			{Name: "worker"},
		},
		Metrics: ds.MetricsConfig{
			Enabled: true,
			Metrics: []metricsmap.Metric{
				{Name: "restarts_total", Type: metricsmap.TypeCounter, Apps: metricsmap.Apps{"api"}},
			},
		},
	}

	if err := g.resolveMetricsApps(); err != nil {
		t.Fatalf("resolveMetricsApps: %v", err)
	}

	if len(g.Metrics.Apps) != 2 {
		t.Fatalf("expected one scope per application, got %d", len(g.Metrics.Apps))
	}

	if !g.Metrics.Apps[0].ExposesMetrics {
		t.Error("an application with a sys transport exposes /metrics")
	}

	if g.Metrics.Apps[1].ExposesMetrics {
		t.Error("an application without a sys transport exposes no /metrics")
	}
}

// TestResolveMetricsAppsRejectsUnknownApp pins that the reference check runs
// against the project's real applications.
func TestResolveMetricsAppsRejectsUnknownApp(t *testing.T) {
	g := &Generator{
		Applications: ds.Apps{{Name: "api", Transports: sysTransports()}},
		Metrics: ds.MetricsConfig{
			Enabled: true,
			Metrics: []metricsmap.Metric{
				{Name: "restarts_total", Type: metricsmap.TypeCounter, Apps: metricsmap.Apps{"apii"}},
			},
		},
	}

	err := g.resolveMetricsApps()
	if err == nil {
		t.Fatal("an apps entry naming no application of the project must stop generation")
	}

	if !errors.Is(err, metricsmap.ErrUnknownApp) {
		t.Errorf("expected ErrUnknownApp, got %v", err)
	}
}

// TestResolveMetricsAppsDedupesNames pins that a duplicate application name
// yields one scope. Two scopes would render a duplicate key in the generated
// collectorsByApp literal, which Go rejects — turning a degenerate config the
// generator already tolerates into a build failure of the generated project.
func TestResolveMetricsAppsDedupesNames(t *testing.T) {
	g := &Generator{
		Applications: ds.Apps{
			{Name: "api", Transports: sysTransports()},
			{Name: "api", Transports: sysTransports()},
		},
		Metrics: ds.MetricsConfig{Enabled: true},
	}

	if err := g.resolveMetricsApps(); err != nil {
		t.Fatalf("resolveMetricsApps: %v", err)
	}

	if len(g.Metrics.Apps) != 1 {
		t.Fatalf("expected one scope for a duplicated application name, got %d", len(g.Metrics.Apps))
	}
}

// sysTransports returns the transport set of an application serving /metrics.
func sysTransports() ds.Transports {
	return ds.Transports{
		"sys": {GeneratorType: "template", GeneratorTemplate: "sys"},
	}
}
