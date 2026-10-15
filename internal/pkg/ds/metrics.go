package ds

import (
	"path/filepath"

	"github.com/Educentr/go-project-starter/internal/pkg/metricsmap"
)

// MetricsPackage is the Go package name of the generated collector package.
const MetricsPackage = "metrics"

// metricsDir is the project-relative directory of the generated collector
// package. It sits under internal/pkg/ because it is generated core logic with
// no config dependency, following the repository's three-layer split.
var metricsDir = filepath.Join("internal", "pkg", MetricsPackage)

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
