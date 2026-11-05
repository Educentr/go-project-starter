package metricsmap

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Set is a named group of declared metrics taking part in a merge. The name is
// used in collision messages so that the reader learns which two declarations
// clashed.
type Set struct {
	// Origin names the source of the declarations, e.g. "service metrics map"
	// or "go-project-starter internal metrics".
	Origin string
	// Metrics holds the declarations.
	Metrics []Metric
}

// Sentinel errors. Use errors.Is to distinguish.
var (
	// ErrInvalidMap reports a metrics map that cannot be used for generation.
	ErrInvalidMap = errors.New("invalid metrics map")
	// ErrDuplicateName reports the same metric name declared twice inside one
	// map.
	ErrDuplicateName = errors.New("duplicate metric name")
	// ErrNameCollision reports the same metric name declared by two different
	// sets being merged (typically the service map and the generator's own
	// internal set).
	ErrNameCollision = errors.New("metric name collision")
	// ErrMalformedMap reports YAML that cannot be decoded into a metrics map
	// at all — a wrong node kind or an unknown field.
	ErrMalformedMap = errors.New("malformed metrics map")
	// ErrInternalMapDefect reports a mismatch between the generator's embedded
	// internal map and the Go code that reads it. It is a defect in
	// go-project-starter, not in the user's project.
	ErrInternalMapDefect = errors.New("internal metrics map defect")
)

// Prometheus identifier grammar, from
// https://prometheus.io/docs/concepts/data_model/.
var (
	metricNameRx = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`)
	labelNameRx  = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
)

// knownOwners is the closed set of generator components that may own a metric.
var knownOwners = map[string]struct{}{ //nolint:gochecknoglobals // fixed lookup table, written as a literal and read-only afterwards
	OwnerKafkaProducer: {},
}

// knownCapabilities is the closed set of project capabilities a declaration may
// depend on.
var knownCapabilities = map[string]struct{}{ //nolint:gochecknoglobals // fixed lookup table, written as a literal and read-only afterwards
	CapabilityKafkaProducer: {},
	CapabilityRESTServer:    {},
	CapabilityRESTClient:    {},
}

// knownTypes is the closed set of accepted `type` values.
var knownTypes = map[string]struct{}{ //nolint:gochecknoglobals // fixed lookup table, written as a literal and read-only afterwards
	TypeCounter:      {},
	TypeCounterVec:   {},
	TypeGauge:        {},
	TypeGaugeVec:     {},
	TypeHistogram:    {},
	TypeHistogramVec: {},
	TypeSummary:      {},
	TypeSummaryVec:   {},
}

// Validate checks the whole map: every entry on its own, plus uniqueness of
// names across the map. All problems are reported at once so that a bad map
// takes one round trip to fix, not one per entry.
func (m *Map) Validate() error {
	if m == nil || len(m.Metrics) == 0 {
		return fmt.Errorf("%w: no metrics declared", ErrInvalidMap)
	}

	var problems []string

	seen := make(map[string]int, len(m.Metrics))

	for i, metric := range m.Metrics {
		for _, msg := range metric.problems() {
			problems = append(problems, fmt.Sprintf("metrics[%d]: %s", i, msg))
		}

		if metric.Name == "" {
			continue
		}

		if first, dup := seen[metric.Name]; dup {
			problems = append(problems, fmt.Sprintf("metrics[%d]: %s: %q (first declared at metrics[%d])",
				i, ErrDuplicateName, metric.Name, first))

			continue
		}

		seen[metric.Name] = i
	}

	if len(problems) > 0 {
		return problemList(ErrInvalidMap, problems)
	}

	return nil
}

// validateNoReservedFields rejects the generator-internal fields in a service
// map. They decide where a collector is constructed and when a declaration
// applies — questions only the generator's own map gets to answer.
func (m *Map) validateNoReservedFields() error {
	var problems []string

	for i, metric := range m.Metrics {
		if metric.Owner != "" {
			problems = append(problems, fmt.Sprintf("metrics[%d] (%s): `owner` is reserved for the generator's internal map", i, metric.Name))
		}

		if metric.Requires != "" {
			problems = append(problems, fmt.Sprintf("metrics[%d] (%s): `requires` is reserved for the generator's internal map", i, metric.Name))
		}
	}

	if len(problems) > 0 {
		return problemList(ErrInvalidMap, problems)
	}

	return nil
}

// problems returns every rule the entry breaks, as human-readable messages.
func (m Metric) problems() []string {
	var problems []string

	switch {
	case m.Name == "":
		problems = append(problems, "name is required")
	case !metricNameRx.MatchString(m.Name):
		problems = append(problems, fmt.Sprintf("name %q is not a valid Prometheus metric name", m.Name))
	case m.VarName() == "":
		// Prometheus accepts a name built only from '_' and ':', but there is
		// no Go identifier left after folding those away — the generated
		// package would not compile.
		problems = append(problems, fmt.Sprintf(
			"name %q leaves no Go identifier once '_' and ':' are folded away; it needs at least one letter or digit", m.Name))
	case !startsWithLetter(m.VarName()):
		// Prometheus allows a leading '_' or ':' before a digit, Go does not
		// allow an identifier to start with one. Folding the separators away
		// promotes the digit to the front, so the generated
		// `var <VarName> = prometheus.New…` would not compile.
		problems = append(problems, fmt.Sprintf(
			"name %q generates the Go variable %s, which starts with a digit; "+
				"the first character after any leading '_' or ':' must be a letter", m.Name, m.VarName()))
	}

	if m.Help == "" {
		problems = append(problems, "help is required — it is the text Prometheus exposes to the reader")
	}

	if m.Owner != "" {
		if _, ok := knownOwners[m.Owner]; !ok {
			problems = append(problems, fmt.Sprintf("owner %q is unknown", m.Owner))
		}

		if m.External {
			problems = append(problems,
				"external and owner are mutually exclusive: either a library emits the metric "+
					"or a generator component constructs it")
		}
	}

	if m.Requires != "" {
		if _, ok := knownCapabilities[m.Requires]; !ok {
			problems = append(problems, fmt.Sprintf("requires %q is not a known project capability", m.Requires))
		}
	}

	if _, ok := knownTypes[m.Type]; !ok {
		problems = append(problems, fmt.Sprintf("type %q is unknown, expected one of %s", m.Type, strings.Join(KnownTypes(), ", ")))

		// Without a known type the remaining per-type rules are meaningless.
		return problems
	}

	problems = append(problems, m.labelProblems()...)
	problems = append(problems, m.bucketProblems()...)
	problems = append(problems, m.objectiveProblems()...)

	return problems
}

func (m Metric) labelProblems() []string {
	var problems []string

	if m.IsVec() && len(m.Labels) == 0 {
		problems = append(problems, fmt.Sprintf("type %s requires labels; use %s for a metric without labels",
			m.Type, strings.TrimSuffix(m.Type, "Vec")))
	}

	if !m.IsVec() && len(m.Labels) > 0 {
		problems = append(problems, fmt.Sprintf("type %s takes no labels; use %sVec to declare a series with labels", m.Type, m.Type))
	}

	seen := make(map[string]struct{}, len(m.Labels))

	for _, label := range m.Labels {
		if !labelNameRx.MatchString(label) {
			problems = append(problems, fmt.Sprintf("label %q is not a valid Prometheus label name", label))

			continue
		}

		if strings.HasPrefix(label, "__") {
			problems = append(problems, fmt.Sprintf("label %q uses the __ prefix reserved for Prometheus internals", label))

			continue
		}

		if _, dup := seen[label]; dup {
			problems = append(problems, fmt.Sprintf("label %q is declared twice", label))

			continue
		}

		seen[label] = struct{}{}
	}

	return problems
}

func (m Metric) bucketProblems() []string {
	if len(m.Buckets) == 0 {
		return nil
	}

	if m.Kind() != KindHistogram {
		return []string{fmt.Sprintf("buckets are only valid for Histogram/HistogramVec, not %s", m.Type)}
	}

	var problems []string

	for i := 1; i < len(m.Buckets); i++ {
		if m.Buckets[i] <= m.Buckets[i-1] {
			problems = append(problems, fmt.Sprintf("buckets must be strictly increasing, got %v after %v",
				m.Buckets[i], m.Buckets[i-1]))

			break
		}
	}

	return problems
}

func (m Metric) objectiveProblems() []string {
	if len(m.Objectives) == 0 {
		return nil
	}

	if m.Kind() != KindSummary {
		return []string{fmt.Sprintf("objectives are only valid for Summary/SummaryVec, not %s", m.Type)}
	}

	var problems []string

	for _, o := range m.Objectives {
		if o.Quantile <= 0 || o.Quantile >= 1 {
			problems = append(problems, fmt.Sprintf("objective quantile %v is outside the open interval (0, 1)", o.Quantile))
		}

		if o.Error < 0 {
			problems = append(problems, fmt.Sprintf("objective for quantile %v has a negative allowed error %v", o.Quantile, o.Error))
		}
	}

	return problems
}

// KnownTypes returns the accepted `type` values, sorted, for error messages.
func KnownTypes() []string {
	out := make([]string, 0, len(knownTypes))

	for t := range knownTypes {
		out = append(out, t)
	}

	sort.Strings(out)

	return out
}

// Merge folds several sets into one namespace. A name declared by two sets is
// an error, never an override: the name in /metrics is what dashboards and
// alerts look at, so letting it depend on merge order would make the artifact
// unreviewable.
//
// The returned slice preserves the input order (sets in the order given,
// entries in declaration order within each set).
func Merge(sets ...Set) ([]Metric, error) {
	type provenance struct {
		origin string
		index  int
	}

	owner := make(map[string]provenance)

	var (
		merged   []Metric
		problems []string
	)

	for _, set := range sets {
		for i, metric := range set.Metrics {
			if prev, taken := owner[metric.Name]; taken {
				problems = append(problems, fmt.Sprintf("%q declared by both %s[%d] and %s[%d]",
					metric.Name, prev.origin, prev.index, set.Origin, i))

				continue
			}

			owner[metric.Name] = provenance{origin: set.Origin, index: i}

			merged = append(merged, metric)
		}
	}

	if len(problems) > 0 {
		return nil, problemList(ErrNameCollision, problems)
	}

	return merged, nil
}

// problemList wraps sentinel with every collected problem on its own indented
// line. All problems are reported together on purpose: fixing a bad map should
// take one round trip, not one per entry.
func problemList(sentinel error, problems []string) error {
	return fmt.Errorf("%w:\n  %s", sentinel, strings.Join(problems, "\n  "))
}
