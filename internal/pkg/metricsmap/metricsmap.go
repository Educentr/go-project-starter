// Package metricsmap parses and validates the metrics map — the declaration of
// every Prometheus metric a generated service is allowed to emit.
//
// The map is a contract artifact: it lives in a contract repository and is
// referenced from project.yaml via `metrics.path` in the same URI form as
// `rest[].path` (see internal/pkg/specsource). Its shape is a flat list:
//
//	metrics:
//	  - name: spawn_errors_total
//	    type: CounterVec
//	    labels: reason, stage
//	    help: >-
//	      Spawn attempts that failed, by reason and stage.
//
// `name` carries the FULL Prometheus metric name — the one that ends up in
// /metrics and in dashboards. The namespace/subsystem split of
// prometheus.CounterOpts is deliberately not reproduced here: a name assembled
// at runtime is a name no external tool can find in the sources.
package metricsmap

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Labels is the label-name list of a metric series. It accepts both a YAML
// sequence and a comma-separated scalar, because the declaration form in the
// original request writes `labels: reason, stage`.
type Labels []string

// Objective is a single Summary quantile target: the quantile and the allowed
// relative error.
type Objective struct {
	Quantile float64
	Error    float64
}

// Metric is one entry of the map.
type Metric struct {
	// Name is the full Prometheus metric name. Required.
	Name string `yaml:"name"`
	// Type is one of the Type* constants. Required.
	Type string `yaml:"type"`
	// Labels holds the label names of the series. Required for the *Vec types,
	// forbidden for the others.
	Labels Labels `yaml:"labels"`
	// Help is the text Prometheus exposes alongside the metric. Required —
	// the map is the one place where that prose is visible for review.
	Help string `yaml:"help"`
	// Buckets overrides prometheus.DefBuckets. Histogram/HistogramVec only.
	Buckets []float64 `yaml:"buckets"`
	// Objectives sets the Summary quantile targets. Summary/SummaryVec only.
	// Absent means nil objectives — client_golang then reports only _sum and
	// _count, which is its own default.
	Objectives []Objective `yaml:"objectives"`
	// External marks a metric that is emitted by a third-party library
	// (client_golang's Go runtime collector, ogen's HTTP instrumentation, …)
	// rather than by the service. No collector is generated for it, but it
	// still occupies its name in the merged namespace and still gets dashboard
	// panels.
	External bool `yaml:"external"`

	// Owner names the generator component that constructs the collector, for
	// metrics the generator emits from a template of its own (the Kafka
	// producer, for instance, needs one collector per producer and therefore
	// cannot share a package-level variable). Empty means the service owns it
	// and the collector goes into the generated collector package.
	//
	// Reserved for the generator's internal map; rejected in a service map.
	Owner string `yaml:"owner"`

	// Requires names the project capability the declaration depends on, so
	// that a metric nothing in the project can emit neither reserves its name
	// nor appears on a dashboard. Empty means unconditional.
	//
	// Reserved for the generator's internal map; rejected in a service map.
	Requires string `yaml:"requires"`
}

// Map is a parsed metrics map.
type Map struct {
	Metrics []Metric `yaml:"metrics"`
}

// Metric type constants. These mirror the client_golang constructor names so
// that a reader of the map knows exactly which collector they get.
const (
	TypeCounter      = "Counter"
	TypeCounterVec   = "CounterVec"
	TypeGauge        = "Gauge"
	TypeGaugeVec     = "GaugeVec"
	TypeHistogram    = "Histogram"
	TypeHistogramVec = "HistogramVec"
	TypeSummary      = "Summary"
	TypeSummaryVec   = "SummaryVec"
)

// Metric kinds — the Prometheus kind behind a type, with the Vec distinction
// dropped. Used to pick the options struct, the bucket/objective rules and the
// canonical dashboard panel shape.
const (
	KindCounter   = "counter"
	KindGauge     = "gauge"
	KindHistogram = "histogram"
	KindSummary   = "summary"
)

// Metric owners. Empty owner means the service.
const (
	// OwnerKafkaProducer marks metrics constructed by the generated Kafka
	// producer driver.
	OwnerKafkaProducer = "kafka_producer"
)

// Project capabilities a declaration can depend on via `requires`.
const (
	// CapabilityKafkaProducer is held by projects with at least one Kafka
	// producer.
	CapabilityKafkaProducer = "kafka_producer"
	// CapabilityRESTServer is held by projects serving at least one ogen REST
	// transport.
	CapabilityRESTServer = "rest_server"
	// CapabilityRESTClient is held by projects with at least one ogen REST
	// client transport.
	CapabilityRESTClient = "rest_client"
)

const (
	// objectivePairSize is the number of yaml.Node entries one
	// `quantile: error` pair occupies in a mapping node's Content slice (the
	// key and the value).
	objectivePairSize = 2

	// errLabelsCtx wraps a decode failure of the `labels` field.
	errLabelsCtx = "labels: %w"
)

// metricFields is the closed set of keys a map entry may carry. Enforced
// explicitly because yaml.Decoder.KnownFields does not reach into a custom
// UnmarshalYAML, and a silently ignored key is a declaration that never takes
// effect — exactly the failure mode the map exists to remove.
var metricFields = map[string]struct{}{
	"name": {}, "type": {}, "labels": {}, "help": {},
	"buckets": {}, "objectives": {}, "external": {},
	"owner": {}, "requires": {},
}

// UnmarshalYAML accepts either a sequence of strings or a single scalar holding
// a comma-separated list.
func (l *Labels) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		var raw string
		if err := value.Decode(&raw); err != nil {
			return fmt.Errorf(errLabelsCtx, err)
		}

		*l = splitLabels(raw)

		return nil
	case yaml.SequenceNode:
		var raw []string
		if err := value.Decode(&raw); err != nil {
			return fmt.Errorf(errLabelsCtx, err)
		}

		out := make(Labels, 0, len(raw))

		for _, item := range raw {
			out = append(out, strings.TrimSpace(item))
		}

		*l = out

		return nil
	default:
		return fmt.Errorf("%w: labels: expected a list or a comma-separated string, got %s", ErrMalformedMap, nodeKindName(value.Kind))
	}
}

// UnmarshalYAML accepts the client_golang-shaped mapping of quantile to allowed
// error (`{0.5: 0.05, 0.99: 0.001}`) and keeps it ordered by quantile as
// written.
func (o *Objective) UnmarshalYAML(value *yaml.Node) error {
	// A single objective is never written on its own in the map; the mapping
	// form is handled by Metric.UnmarshalYAML. This method exists so that an
	// explicit list form (`- quantile: 0.99` / `error: 0.001`) also works.
	var explicit struct {
		Quantile *float64 `yaml:"quantile"`
		Error    *float64 `yaml:"error"`
	}

	if err := value.Decode(&explicit); err != nil {
		return fmt.Errorf("objectives: %w", err)
	}

	if explicit.Quantile == nil {
		return fmt.Errorf("%w: objectives: missing quantile", ErrMalformedMap)
	}

	o.Quantile = *explicit.Quantile

	if explicit.Error != nil {
		o.Error = *explicit.Error
	}

	return nil
}

// UnmarshalYAML decodes a metric entry. It is hand-rolled to support the
// `objectives: {0.99: 0.001}` mapping form, which has no struct equivalent,
// and to reject unknown keys.
func (m *Metric) UnmarshalYAML(value *yaml.Node) error {
	type plain struct {
		Name       string    `yaml:"name"`
		Type       string    `yaml:"type"`
		Labels     Labels    `yaml:"labels"`
		Help       string    `yaml:"help"`
		Buckets    []float64 `yaml:"buckets"`
		Objectives yaml.Node `yaml:"objectives"`
		External   bool      `yaml:"external"`
		Owner      string    `yaml:"owner"`
		Requires   string    `yaml:"requires"`
	}

	if err := rejectUnknownFields(value, metricFields); err != nil {
		return err
	}

	var p plain
	if err := value.Decode(&p); err != nil {
		return err
	}

	m.Name = p.Name
	m.Type = p.Type
	m.Labels = p.Labels
	m.Help = p.Help
	m.Buckets = p.Buckets
	m.External = p.External
	m.Owner = p.Owner
	m.Requires = p.Requires

	objectives, err := decodeObjectives(&p.Objectives)
	if err != nil {
		return fmt.Errorf("metric %q: %w", p.Name, err)
	}

	m.Objectives = objectives

	return nil
}

// Parse decodes a metrics map from YAML and validates it. The returned map is
// safe to hand to code generation: every entry has a usable name, type, label
// set and help text, and no two entries share a name.
//
// Parse accepts the generator-internal `owner` and `requires` fields; use
// ParseService for a map that comes from a service's contract repository.
func Parse(data []byte) (*Map, error) {
	var m Map

	if len(strings.TrimSpace(string(data))) == 0 {
		// Without this the yaml decoder reports a bare "EOF", which says
		// nothing about what to do.
		return nil, fmt.Errorf("%w: the file is empty", ErrInvalidMap)
	}

	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)

	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMalformedMap, err)
	}

	if err := m.Validate(); err != nil {
		return nil, err
	}

	return &m, nil
}

// ParseService is Parse plus the rule that a service map may not use the
// generator-internal `owner` and `requires` fields.
func ParseService(data []byte) (*Map, error) {
	m, err := Parse(data)
	if err != nil {
		return nil, err
	}

	if err := m.validateNoReservedFields(); err != nil {
		return nil, err
	}

	return m, nil
}

// Load reads and parses a service's metrics map.
func Load(path string) (*Map, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read metrics map %s: %w", path, err)
	}

	m, err := ParseService(data)
	if err != nil {
		return nil, fmt.Errorf("metrics map %s: %w", path, err)
	}

	return m, nil
}

// IsVec reports whether the metric's type is one of the *Vec constructors,
// i.e. whether it carries labels.
func (m Metric) IsVec() bool {
	switch m.Type {
	case TypeCounterVec, TypeGaugeVec, TypeHistogramVec, TypeSummaryVec:
		return true
	default:
		return false
	}
}

// Generated reports whether the collector for this metric belongs in the
// generated collector package. External declarations exist for the namespace
// and the dashboard only; owned declarations are constructed by the generator
// component named in Owner.
func (m Metric) Generated() bool {
	return !m.External && m.Owner == ""
}

// OwnedBy reports whether the metric is constructed by the named generator
// component.
func (m Metric) OwnedBy(owner string) bool {
	return !m.External && m.Owner == owner
}

// Applies reports whether the declaration is relevant to a project holding the
// given capabilities. A declaration with no `requires` always applies.
func (m Metric) Applies(capabilities map[string]bool) bool {
	if m.Requires == "" {
		return true
	}

	return capabilities[m.Requires]
}

// Kind returns the Prometheus metric kind ("counter", "gauge", "histogram",
// "summary") regardless of whether the type is a Vec. Used to pick the
// canonical dashboard panel shape.
func (m Metric) Kind() string {
	switch m.Type {
	case TypeCounter, TypeCounterVec:
		return KindCounter
	case TypeGauge, TypeGaugeVec:
		return KindGauge
	case TypeHistogram, TypeHistogramVec:
		return KindHistogram
	case TypeSummary, TypeSummaryVec:
		return KindSummary
	default:
		return ""
	}
}

// Generatable returns the subset of entries a collector is generated for.
func (m *Map) Generatable() []Metric {
	if m == nil {
		return nil
	}

	out := make([]Metric, 0, len(m.Metrics))

	for _, metric := range m.Metrics {
		if metric.Generated() {
			out = append(out, metric)
		}
	}

	return out
}

// rejectUnknownFields fails on any mapping key outside allowed, naming the
// offending key and the line it sits on.
func rejectUnknownFields(value *yaml.Node, allowed map[string]struct{}) error {
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("%w: expected a mapping, got %s", ErrMalformedMap, nodeKindName(value.Kind))
	}

	for i := 0; i < len(value.Content)-1; i += 2 {
		key := value.Content[i].Value
		if _, ok := allowed[key]; !ok {
			return fmt.Errorf("%w: line %d: unknown field %q", ErrMalformedMap, value.Content[i].Line, key)
		}
	}

	return nil
}

// splitLabels turns "reason, stage" into []string{"reason", "stage"}. An empty
// or whitespace-only input yields no labels rather than one empty label.
func splitLabels(raw string) Labels {
	parts := strings.Split(raw, ",")
	out := make(Labels, 0, len(parts))

	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		out = append(out, p)
	}

	if len(out) == 0 {
		return nil
	}

	return out
}

// decodeObjectives accepts the `{quantile: error}` mapping form as well as the
// explicit list form. A zero node (field absent) yields nil.
func decodeObjectives(node *yaml.Node) ([]Objective, error) {
	if node == nil || node.Kind == 0 {
		return nil, nil
	}

	switch node.Kind {
	case yaml.MappingNode:
		if len(node.Content)%objectivePairSize != 0 {
			return nil, fmt.Errorf("%w: objectives: malformed mapping", ErrMalformedMap)
		}

		out := make([]Objective, 0, len(node.Content)/objectivePairSize)

		for i := 0; i < len(node.Content); i += objectivePairSize {
			var quantile, allowedError float64

			if err := node.Content[i].Decode(&quantile); err != nil {
				return nil, fmt.Errorf("objectives: quantile %q: %w", node.Content[i].Value, err)
			}

			if err := node.Content[i+1].Decode(&allowedError); err != nil {
				return nil, fmt.Errorf("objectives: error for quantile %v: %w", quantile, err)
			}

			out = append(out, Objective{Quantile: quantile, Error: allowedError})
		}

		return out, nil
	case yaml.SequenceNode:
		var out []Objective
		if err := node.Decode(&out); err != nil {
			return nil, err
		}

		return out, nil
	default:
		return nil, fmt.Errorf("%w: objectives: expected a mapping of quantile to error, got %s", ErrMalformedMap, nodeKindName(node.Kind))
	}
}

func nodeKindName(kind yaml.Kind) string {
	switch kind {
	case yaml.DocumentNode:
		return "document"
	case yaml.SequenceNode:
		return "list"
	case yaml.MappingNode:
		return "mapping"
	case yaml.ScalarNode:
		return "scalar"
	case yaml.AliasNode:
		return "alias"
	default:
		return "nothing"
	}
}
