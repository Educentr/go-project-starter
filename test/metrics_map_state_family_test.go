package test

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// TestGenerateMetricsMapStateFamily covers a declaration of type GaugeSet
// (TOOLS-125 / issue #148): a gauge family the tick knows the whole truth
// about, so its set of series is published as a unit instead of being mutated
// series by series.
//
// What the generator owes such a declaration is everything the map owes any
// other — the name, the help text, the labels and the registration — plus the
// collector, because this is the one shape client_golang has no constructor
// for. Before the type existed those 24 families were hand-written collectors
// declared `external: true`, i.e. recorded in the map as somebody else's
// metrics.
func TestGenerateMetricsMapStateFamily(t *testing.T) {
	tmpDir := generateFromConfig(t, "metrics-map", "Generate project with a state family")

	metricsFile := readGenerated(t, tmpDir, "internal", "pkg", "metrics", "psg_metrics_gen.go")

	// The strongest check of the whole block: the package must parse. The
	// generated sample struct is the only place in this package where a
	// declaration produces a type rather than a variable.
	if _, err := parser.ParseFile(token.NewFileSet(), "psg_metrics_gen.go", metricsFile, parser.SkipObjectResolution); err != nil {
		t.Fatalf("generated collector package is not valid Go: %v", err)
	}

	// The machinery is one generic collector for the whole package, emitted
	// once however many families there are.
	for _, want := range []string{
		"type gaugeSetSample interface {",
		"type GaugeSet[S gaugeSetSample] struct {",
		"src  atomic.Pointer[func() []S]",
		"func (g *GaugeSet[S]) SetFunc(src func() []S) {",
		"func (g *GaugeSet[S]) Set(samples []S) {",
		"func (g *GaugeSet[S]) Describe(ch chan<- *prometheus.Desc) {",
		"func (g *GaugeSet[S]) Collect(ch chan<- prometheus.Metric) {",
		`"sync/atomic"`,
	} {
		if !strings.Contains(metricsFile, want) {
			t.Errorf("generated collector package should contain %q", want)
		}
	}

	if got := strings.Count(metricsFile, "type GaugeSet[S gaugeSetSample] struct {"); got != 1 {
		t.Errorf("the GaugeSet machinery should be emitted once, got %d copies", got)
	}

	// Per family: the sample struct, its accessors, the source type and the
	// collector variable. One string field per declared label, in declared
	// order, plus the value — which is what makes a sample with the wrong
	// number of label values unwritable rather than a runtime error.
	for _, want := range []string{
		"type FleetStateSample struct {",
		"\t// Node is the value of the node label.\n\tNode string",
		"\t// Zone is the value of the zone label.\n\tZone string",
		"\t// Value is the value of the series.\n\tValue float64",
		"func (s FleetStateSample) labelValues() []string {\n\treturn []string{s.Node, s.Zone}\n}",
		"func (s FleetStateSample) value() float64 {\n\treturn s.Value\n}",
		"type FleetStateFunc func() []FleetStateSample",
		"var FleetState = &GaugeSet[FleetStateSample]{",
		`		"fleet_state",`,
		`		"State of each known fleet node, by node and zone.",`,
		`		[]string{"node", "zone"},`,
	} {
		if !strings.Contains(metricsFile, want) {
			t.Errorf("generated collector package should contain %q", want)
		}
	}

	// A second family gets its own sample type. Its label is two words, so it
	// also pins how a label name becomes a field name: the separators fold
	// away, nothing else is rewritten.
	for _, want := range []string{
		"type CredentialProbeStateSample struct {",
		"\tAccountId string",
		"var CredentialProbeState = &GaugeSet[CredentialProbeStateSample]{",
	} {
		if !strings.Contains(metricsFile, want) {
			t.Errorf("generated collector package should contain %q", want)
		}
	}

	// A state family is registered by the generated code, like every other
	// declaration — that is the half the hand-written collectors could not
	// have, because the generator registering a GaugeVec of the same name
	// first is what made them fail with AlreadyRegisteredError.
	block := appCollectorBlock(t, metricsFile, "api")

	for _, want := range []string{"FleetState,", "CredentialProbeState,"} {
		if !strings.Contains(block, want) {
			t.Errorf("collectorsByApp entry for api should carry %q, got:\n%s", want, block)
		}
	}

	// No metric constructor is called for it: the family is a Desc plus a
	// Collector, so `make metrics-validate` stays satisfiable by a tick that
	// only builds samples.
	if strings.Contains(metricsFile, "prometheus.NewGaugeSet") {
		t.Error("GaugeSet has no client_golang constructor; the template must not render one")
	}

	// The dashboard panel is the gauge one: a state family is a gauge in every
	// way Grafana cares about.
	dashboard := readGenerated(t, tmpDir, "configs", "grafana", "dashboards", "api.json")

	for _, want := range []string{
		`"expr": "sum by(node, zone) (fleet_state)"`,
		`"expr": "sum by(account_id, kind) (credential_probe_state)"`,
	} {
		if !strings.Contains(dashboard, want) {
			t.Errorf("dashboard should chart the state family with the canonical gauge query %q", want)
		}
	}
}

// TestGenerateWithoutStateFamilyLeavesPackageUnchanged pins the other half of
// the feature: a map that declares no state family gets the collector package
// it got before the type existed, down to the import block. An unconditional
// emit would put a generic collector and a sync/atomic import into every
// project that has a metrics map, for nobody.
func TestGenerateWithoutStateFamilyLeavesPackageUnchanged(t *testing.T) {
	tmpDir := generateFromConfig(t, "metrics-map-apps", "Generate project without a state family")

	metricsFile := readGenerated(t, tmpDir, "internal", "pkg", "metrics", "psg_metrics_gen.go")

	for _, forbidden := range []string{
		"GaugeSet", "gaugeSetSample", "sync/atomic", "Sample struct", "labelValues()",
	} {
		if strings.Contains(metricsFile, forbidden) {
			t.Errorf("a project with no state family must not carry %q in its collector package", forbidden)
		}
	}
}

// TestGenerateRejectsStateFamilyLabelTakingTheValueField pins where a label
// that cannot become a sample field is reported: at the map, during
// generation, not as a compile error in a package the service did not write.
//
// `value` is a legal Prometheus label name, so nothing upstream rejects it;
// the field generated from it collides with the field carrying the value of
// the series. That a rejected map leaves no collector package behind is pinned
// by TestGenerateRejectsNameThatIsNoGoIdentifier — the rejection path is the
// same one.
func TestGenerateRejectsStateFamilyLabelTakingTheValueField(t *testing.T) {
	out, err := generateWithMetricsMap(t,
		"metrics:\n"+
			"  - name: fleet_state\n"+
			"    type: GaugeSet\n"+
			"    labels: value\n"+
			"    help: Legal label name, no sample field left for the value.\n",
		"Generate project with a state family label taking the value field")

	if err == nil {
		t.Fatalf("generation succeeded on a label that cannot become a sample field; "+
			"the generated package would not compile\n%s", out)
	}

	// The message names the generated identifier the label collides with, which
	// is how the reader gets from it back to the declaration — the entry itself
	// is pointed at by index, as every problem in a map is.
	for _, want := range []string{"metrics[0]", "sample field Value of FleetStateSample", "rename the label"} {
		if !strings.Contains(out, want) {
			t.Errorf("generator error should mention %q, got:\n%s", want, out)
		}
	}
}
