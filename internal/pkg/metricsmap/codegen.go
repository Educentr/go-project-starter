package metricsmap

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrVarNameCollision reports two declared names that sanitize to the same Go
// identifier (e.g. "queue:depth" and "queue_depth").
var ErrVarNameCollision = errors.New("metric variable name collision")

// VarName is the exported Go identifier of the generated collector variable.
// "spawn_errors_total" becomes "SpawnErrorsTotal".
//
// Prometheus allows ':' in metric names (the recording-rule convention) while
// Go does not allow it in identifiers, so ':' is folded to '_' the same way
// '_' is folded away. Two declarations that collide after folding are rejected
// by ValidateVarNames rather than silently producing one variable.
func (m Metric) VarName() string {
	var b strings.Builder

	upperNext := true

	for _, r := range m.Name {
		if r == '_' || r == ':' {
			upperNext = true

			continue
		}

		if upperNext {
			b.WriteString(strings.ToUpper(string(r)))

			upperNext = false

			continue
		}

		b.WriteRune(r)
	}

	name := b.String()
	if name == "" {
		return ""
	}

	// A declared name may legally start with '_' or a digit-free letter only,
	// so the first rune is always a letter here; nothing else to guard.
	return name
}

// Constructor returns the client_golang constructor call for the metric, e.g.
// "prometheus.NewCounterVec".
func (m Metric) Constructor() string {
	return "prometheus.New" + m.Type
}

// OptsType returns the client_golang options struct for the metric, e.g.
// "prometheus.CounterOpts".
func (m Metric) OptsType() string {
	switch m.Kind() {
	case KindCounter:
		return "prometheus.CounterOpts"
	case KindGauge:
		return "prometheus.GaugeOpts"
	case KindHistogram:
		return "prometheus.HistogramOpts"
	case KindSummary:
		return "prometheus.SummaryOpts"
	default:
		return ""
	}
}

// GoType returns the Go type of the generated collector variable, e.g.
// "*prometheus.CounterVec".
func (m Metric) GoType() string {
	return "*prometheus." + m.Type
}

// LabelsLiteral renders the label name list as a Go slice literal, or the empty
// string for a metric without labels (whose constructor takes no label argument).
func (m Metric) LabelsLiteral() string {
	if len(m.Labels) == 0 {
		return ""
	}

	quoted := make([]string, 0, len(m.Labels))
	for _, label := range m.Labels {
		quoted = append(quoted, strconv.Quote(label))
	}

	return "[]string{" + strings.Join(quoted, ", ") + "}"
}

// BucketsLiteral renders the histogram buckets as a Go expression. An
// unspecified bucket list falls back to prometheus.DefBuckets — a histogram
// with no buckets at all would observe nothing useful.
func (m Metric) BucketsLiteral() string {
	if m.Kind() != KindHistogram {
		return ""
	}

	if len(m.Buckets) == 0 {
		return "prometheus.DefBuckets"
	}

	parts := make([]string, 0, len(m.Buckets))
	for _, b := range m.Buckets {
		parts = append(parts, formatFloat(b))
	}

	return "[]float64{" + strings.Join(parts, ", ") + "}"
}

// ObjectivesLiteral renders the summary objectives as a Go map literal, or the
// empty string when none are declared. Empty means client_golang's own default
// (no quantiles — only _sum and _count), which is a deliberate choice: made-up
// quantile targets would be as wrong as made-up alert thresholds.
func (m Metric) ObjectivesLiteral() string {
	if m.Kind() != KindSummary || len(m.Objectives) == 0 {
		return ""
	}

	parts := make([]string, 0, len(m.Objectives))
	for _, o := range m.Objectives {
		parts = append(parts, formatFloat(o.Quantile)+": "+formatFloat(o.Error))
	}

	return "map[float64]float64{" + strings.Join(parts, ", ") + "}"
}

// ValidateVarNames checks that the declared names produce distinct Go
// identifiers. Called on the merged set, because a collision can be introduced
// by a service name meeting an internal one.
func ValidateVarNames(metrics []Metric) error {
	owner := make(map[string]string, len(metrics))

	var problems []string

	for _, metric := range metrics {
		if !metric.Generated() {
			continue
		}

		varName := metric.VarName()

		if prev, taken := owner[varName]; taken {
			problems = append(problems, fmt.Sprintf("%q and %q both generate the variable %s",
				prev, metric.Name, varName))

			continue
		}

		owner[varName] = metric.Name
	}

	if len(problems) > 0 {
		return problemList(ErrVarNameCollision, problems)
	}

	return nil
}

// formatFloat renders a float the way a human wrote it in YAML: 1 stays "1",
// 0.001 stays "0.001", no exponent noise.
func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'g', -1, 64)
}
