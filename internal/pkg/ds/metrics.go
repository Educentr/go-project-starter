package ds

import (
	"path/filepath"

	"github.com/Educentr/go-project-starter/internal/pkg/metricsmap"
)

// MetricsConfig carries the resolved metric declarations into templates.
//
// Enabled mirrors the presence of `metrics.path` in project.yaml. When it is
// false the whole feature is off and Metrics is empty: no collector package is
// generated, the metrics validator is not wired into the lint config, and the
// dashboard gets no metric panels.
//
// Metrics holds the merged namespace — the generator's own declarations plus
// the service's. Entries with External set are declared but not constructed
// (a library emits them); they are present so that a service cannot shadow
// their names and so that the dashboard can still chart them.
type MetricsConfig struct {
	Enabled bool
	Metrics []metricsmap.Metric
}

// ForbidRule is one forbidigo rule: a regexp over the called identifier plus
// the message the developer sees. Shared by the generated validators (metrics
// map, OnlineConf key map), which differ in their patterns, not in their shape.
type ForbidRule struct {
	Pattern string
	Msg     string
}

// MetricForbidRule is one forbidigo rule of the metrics validator.
type MetricForbidRule = ForbidRule

// KafkaProducerMetrics carries the Kafka producer's metric declarations into
// its driver template. The producer needs one collector per producer instance
// (the producer name is a ConstLabel), so the collectors cannot be the
// package-level variables of the generated collector package — but their
// names, help texts and label sets still come from the single declaration.
type KafkaProducerMetrics struct {
	PublishTotal    metricsmap.Metric
	PublishErrors   metricsmap.Metric
	PublishDuration metricsmap.Metric
}

const (
	// MetricsPackage is the Go package name of the generated collector package.
	MetricsPackage = "metrics"

	// ProducerLabel is the ConstLabel naming the producer instance a Kafka
	// collector belongs to.
	ProducerLabel = "producer"

	// metricsValidatorMsg is the common tail of every validator message. Kept
	// in one place so the two generated lint configs cannot drift.
	metricsValidatorMsg = "declare the metric in the metrics map instead — " +
		"the generated collector package is the only place metrics are constructed"
)

// metricsDir is the project-relative directory of the generated collector
// package. It sits under internal/pkg/ because it is generated core logic with
// no config dependency, following the repository's three-layer split.
var metricsDir = filepath.Join(internalDir, pkgDir, MetricsPackage) //nolint:gochecknoglobals // filepath.Join of constants; read-only

// IsEnabled reports whether a metrics map is configured.
func (m MetricsConfig) IsEnabled() bool {
	return m.Enabled
}

// Generated returns the declarations a collector variable is generated for.
func (m MetricsConfig) Generated() []metricsmap.Metric {
	out := make([]metricsmap.Metric, 0, len(m.Metrics))

	for _, metric := range m.Metrics {
		if metric.Generated() {
			out = append(out, metric)
		}
	}

	return out
}

// HasGenerated reports whether at least one collector will be generated.
// A map consisting solely of external declarations is legitimate — it still
// populates the dashboard and the namespace — but produces no collectors.
func (m MetricsConfig) HasGenerated() bool {
	for _, metric := range m.Metrics {
		if metric.Generated() {
			return true
		}
	}

	return false
}

// ForbiddenConstructors returns the metrics validator's rules: the collector
// constructors that may not appear outside the generated collector package.
//
// Only constructor CALLS are forbidden, deliberately. The options literals
// (prometheus.CounterOpts{…}) are left alone because on their own they emit
// nothing — without a constructor call no metric comes out of them, so banning
// the calls closes the hole completely. That includes the positional
// prometheus.NewDesc form used by hand-written collectors with their own
// Collect/Describe, which a rule aimed only at the Opts structs would miss.
func (m MetricsConfig) ForbiddenConstructors() []MetricForbidRule {
	return []MetricForbidRule{
		{
			Pattern: `^prometheus\.New(Counter|Gauge|Histogram|Summary|Untyped)(Vec|Func)?$`,
			Msg:     "metric constructors are not allowed here: " + metricsValidatorMsg,
		},
		{
			Pattern: `^prometheus\.New(Desc|ConstMetric|ConstHistogram|ConstSummary|InvalidMetric|MetricWithTimestamp)$`,
			Msg:     "low-level metric constructors are not allowed here: " + metricsValidatorMsg,
		},
		{
			Pattern: `^promauto\.`,
			Msg:     "promauto registers metrics into a global registry: " + metricsValidatorMsg,
		},
	}
}

// OwnedBy returns the declarations constructed by the named generator
// component.
func (m MetricsConfig) OwnedBy(owner string) []metricsmap.Metric {
	out := make([]metricsmap.Metric, 0, len(m.Metrics))

	for _, metric := range m.Metrics {
		if metric.OwnedBy(owner) {
			out = append(out, metric)
		}
	}

	return out
}

// NewKafkaProducerMetrics resolves the Kafka producer declarations from the
// generator's internal map. An error here means this repository's embedded map
// and its Go callers have drifted apart.
func NewKafkaProducerMetrics() (KafkaProducerMetrics, error) {
	var out KafkaProducerMetrics

	for _, target := range []struct {
		name string
		dst  *metricsmap.Metric
	}{
		{metricsmap.KafkaPublishTotal, &out.PublishTotal},
		{metricsmap.KafkaPublishErrors, &out.PublishErrors},
		{metricsmap.KafkaPublishDuration, &out.PublishDuration},
	} {
		metric, err := metricsmap.MustFindInternal(target.name)
		if err != nil {
			return KafkaProducerMetrics{}, err
		}

		*target.dst = metric
	}

	return out, nil
}

// GetTargetGeneratePath returns the directory of the generated collector
// package.
func (m MetricsConfig) GetTargetGeneratePath(targetDir string) string {
	return filepath.Join(targetDir, metricsDir)
}

// GetPackagePath returns the project-relative directory of the generated
// collector package, with forward slashes — suitable for import paths and for
// lint-config path patterns.
func (m MetricsConfig) GetPackagePath() string {
	return filepath.ToSlash(metricsDir)
}

// GetPackageName returns the Go package name of the generated collector
// package.
func (m MetricsConfig) GetPackageName() string {
	return MetricsPackage
}
