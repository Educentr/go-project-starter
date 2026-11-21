package test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Educentr/go-project-starter/internal/pkg/metricsmap"
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

	// The identifiers a state family adds to the package are part of a
	// namespace nothing else may land in, so the list a declaration is refused
	// against has to cover everything the template declares.
	assertDeclarationsCollideWithNothing(t, metricsFile)
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

// assertDeclarationsCollideWithNothing keeps the collector template and the
// list of identifiers a declaration may not generate
// (metricsmap.ReservedIdents) in step.
//
// Every identifier a declaration generates is exported — folding a Prometheus
// name's separators away upper-cases the first character — and so is every
// identifier of the package the reader of /metrics meets. A declaration whose
// name lands on one of the package's own (`gauge_set` against the generic
// collector, `register` against RegisterFor's sibling) yields a package that
// does not compile, which is why such a name is refused while the map is read.
// This is the other side of that rule: every exported identifier the template
// puts in the package has to be on the list it is refused against, or derived
// from a declaration of the map. An export added to the template without the
// list growing fails here, not in a user's project.
func assertDeclarationsCollideWithNothing(t *testing.T, metricsFile string) {
	t.Helper()

	file, err := parser.ParseFile(token.NewFileSet(), "psg_metrics_gen.go", metricsFile,
		parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("generated collector package does not parse: %v", err)
	}

	accounted := make(map[string]bool)
	for _, ident := range metricsmap.ReservedIdents(true) {
		accounted[ident] = true
	}

	// The other legitimate source of an exported identifier here: the map the
	// fixture generated from. Read from the same file the generator read, so
	// that a declaration added to the fixture does not have to be repeated
	// here.
	curDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting current directory: %v", err)
	}

	serviceMap, err := metricsmap.Load(filepath.Join(curDir, "..", "test", "docker-integration",
		"configs", "metrics-map", "metrics-map.yaml"))
	if err != nil {
		t.Fatalf("Error loading the fixture metrics map: %v", err)
	}

	for _, metric := range serviceMap.Metrics {
		accounted[metric.VarName()] = true
		accounted[metric.SampleTypeName()] = true
		accounted[metric.SourceTypeName()] = true
	}

	for _, ident := range exportedTopLevelIdents(file) {
		if accounted[ident] {
			continue
		}

		t.Errorf("the generated collector package declares %q, which is neither in "+
			"metricsmap.ReservedIdents() nor generated from a declaration of the map: a metrics map "+
			"could name a metric after it and the package would not compile", ident)
	}
}

// exportedTopLevelIdents returns the names of the file's exported
// package-level declarations — the namespace a declaration of the map lands
// in. Unexported ones are out of reach of a declaration and are left out.
func exportedTopLevelIdents(file *ast.File) []string {
	var idents []string

	add := func(name string) {
		if name != "_" && ast.IsExported(name) {
			idents = append(idents, name)
		}
	}

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			// A method is namespaced by its receiver, so only a plain function
			// can be collided with.
			if d.Recv == nil {
				add(d.Name.Name)
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					add(s.Name.Name)
				case *ast.ValueSpec:
					for _, name := range s.Names {
						add(name.Name)
					}
				}
			}
		}
	}

	return idents
}

// TestGenerateRejectsStateFamilyIdentCollision pins the second half of what a
// state family costs the identifier namespace of the collector package: the
// declaration generates two types besides its collector variable, and the
// generic collector itself appears as soon as one family is declared. A name
// landing on any of them has to be refused while the map is read — before the
// check existed the generator printed `done` over a package that redeclares a
// type, which is a build failure in a project the service did not write.
func TestGenerateRejectsStateFamilyIdentCollision(t *testing.T) {
	family := "metrics:\n" +
		"  - name: fleet_state\n" +
		"    type: GaugeSet\n" +
		"    labels: node, zone\n" +
		"    help: State of each known fleet node, by node and zone.\n"

	tests := []struct {
		name  string
		entry string
		want  string
	}{
		{
			name: "name reaching the sample type of the family",
			entry: "  - name: fleet_state_sample\n" +
				"    type: Gauge\n" +
				"    help: Collision probe.\n",
			want: `"fleet_state_sample" generates the variable FleetStateSample, ` +
				`which is already the sample type of "fleet_state"`,
		},
		{
			name: "name reaching the generic collector of the package",
			entry: "  - name: gauge_set\n" +
				"    type: Gauge\n" +
				"    help: Collision probe.\n",
			want: `"gauge_set" generates the variable GaugeSet, ` +
				`which the generated collector package declares itself`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := generateWithMetricsMap(t, family+tt.entry,
				"Generate project whose metrics map collides with a state family identifier")

			if err == nil {
				t.Fatalf("generation succeeded on a colliding identifier; "+
					"the generated collector package would not compile\n%s", out)
			}

			if !strings.Contains(out, tt.want) {
				t.Errorf("generator error should mention %q, got:\n%s", tt.want, out)
			}
		})
	}
}
