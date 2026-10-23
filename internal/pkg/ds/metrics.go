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
//
// Apps describes the project's applications, in config order. It is what turns
// the service-wide namespace into a per-process one: the map declares the
// surface of a service, while a collector is registered — and a dashboard
// panel is read — in one process at a time. It is filled once the applications
// are built, after the map itself is parsed.
type MetricsConfig struct {
	Enabled bool
	Metrics []metricsmap.Metric
	Apps    []metricsmap.AppScope
}

// MetricsAppCollectors binds an application to the generated collectors it
// registers.
type MetricsAppCollectors struct {
	App     string
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
var metricsDir = filepath.Join("internal", "pkg", MetricsPackage)

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

// ForApp returns the declarations the named application publishes: the ones
// whose `apps` list names it (or is empty), minus the generator-owned ones
// whose component this application does not run.
//
// This is the per-process view of the map. It is what the dashboard is built
// from: a panel for a metric the process cannot emit reads empty forever, and
// an always-empty panel is indistinguishable from a broken feature.
//
// An application the generator did not resolve, or one without a /metrics
// endpoint, publishes nothing: Apps is filled from the project's own
// application list, and an application with no sys transport is never scraped,
// so every panel built for it could only read empty.
func (m MetricsConfig) ForApp(name string) []metricsmap.Metric {
	scope, ok := m.publishingScope(name)
	if !ok {
		return nil
	}

	out := make([]metricsmap.Metric, 0, len(m.Metrics))

	for _, metric := range m.Metrics {
		if scope.Publishes(metric) {
			out = append(out, metric)
		}
	}

	return out
}

// CollectorsByApp lists, for every application that exposes /metrics, the
// collectors generated for it — applications in config order, collectors in
// declaration order.
func (m MetricsConfig) CollectorsByApp() []MetricsAppCollectors {
	out := make([]MetricsAppCollectors, 0, len(m.Apps))

	for _, scope := range m.Apps {
		if !scope.ExposesMetrics {
			continue
		}

		out = append(out, MetricsAppCollectors{App: scope.Name, Metrics: m.generatedFor(scope)})
	}

	return out
}

// HasGeneratedFor reports whether the named application registers at least one
// generated collector. The generated main uses it to decide whether to import
// the collector package at all: an application whose every declaration belongs
// to a sibling would otherwise import a package it never calls.
//
// It answers over the same set CollectorsByApp emits, so the import guard and
// the map it guards cannot disagree.
func (m MetricsConfig) HasGeneratedFor(name string) bool {
	scope, ok := m.publishingScope(name)
	if !ok {
		return false
	}

	return len(m.generatedFor(scope)) > 0
}

// generatedFor returns the collectors the application registers: the
// declarations it publishes, minus the ones no collector is generated for
// (external declarations, and the ones a generator component constructs
// itself).
func (m MetricsConfig) generatedFor(scope metricsmap.AppScope) []metricsmap.Metric {
	out := make([]metricsmap.Metric, 0, len(m.Metrics))

	for _, metric := range m.Metrics {
		if metric.Generated() && scope.Publishes(metric) {
			out = append(out, metric)
		}
	}

	return out
}

// publishingScope returns the resolved description of an application that
// actually publishes metrics.
func (m MetricsConfig) publishingScope(name string) (metricsmap.AppScope, bool) {
	scope, ok := m.appScope(name)
	if !ok || !scope.ExposesMetrics {
		return metricsmap.AppScope{}, false
	}

	return scope, true
}

// appScope returns the resolved description of one application.
func (m MetricsConfig) appScope(name string) (metricsmap.AppScope, bool) {
	for _, scope := range m.Apps {
		if scope.Name == name {
			return scope, true
		}
	}

	return metricsmap.AppScope{}, false
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
