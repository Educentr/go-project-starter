package grafana

import (
	"strings"

	"github.com/Educentr/go-project-starter/internal/pkg/metricsmap"
)

const (
	// serviceMetricsRowTitle is the row title of the declared-metrics section.
	serviceMetricsRowTitle = "Service Metrics"

	// Quantiles charted for histograms and summaries.
	quantileP99 = "0.99"
	quantileP95 = "0.95"

	// legendAuto lets Grafana derive the series legend itself — right for a
	// metric with no labels to name.
	legendAuto = "__auto"
)

// dedicatedRowPrefixes lists the metric name prefixes already charted by the
// generator's hand-built rows. A declaration for one of these still reserves
// its name and documents its help text; it just does not get a second,
// poorer panel.
var dedicatedRowPrefixes = []string{
	"go_",
	"process_",
	"ogen_server_",
	"ogen_client_",
}

// DeclaredMetricPanels builds the canonical panel for each declared metric:
// a rate for counters, the current value for gauges, quantiles for histograms
// and summaries.
//
// Canonical means "the shape that is right for the metric type", which is all
// that can be known without knowing the installation. Thresholds and alert
// rules are deliberately absent: a threshold is tied to the capacity of a
// particular installation, and a generated one would be invented.
//
// Metrics the generator already charts by hand — the Go runtime row and the
// HTTP server/client rows — are skipped here, so a declaration does not
// produce a second panel for a metric that already has a better one.
func DeclaredMetricPanels(metrics []metricsmap.Metric) []Panel {
	panels := make([]Panel, 0, len(metrics))

	for _, metric := range metrics {
		if hasDedicatedRow(metric.Name) {
			continue
		}

		targets := canonicalTargets(metric)
		if len(targets) == 0 {
			continue
		}

		panels = append(panels, Panel{
			Title:      metric.Name,
			Type:       "timeseries",
			Width:      panelWidthHalf,
			Height:     panelHeightS,
			Datasource: DatasourcePrometheus,
			Targets:    targets,
		})
	}

	return panels
}

// DeclaredMetricsRow wraps DeclaredMetricPanels in a collapsed row, or returns
// false when there is nothing to chart.
func DeclaredMetricsRow(metrics []metricsmap.Metric) (Row, bool) {
	panels := DeclaredMetricPanels(metrics)
	if len(panels) == 0 {
		return Row{}, false
	}

	return Row{
		Title:     serviceMetricsRowTitle,
		Collapsed: true,
		Panels:    panels,
	}, true
}

// canonicalTargets returns the PromQL for a metric, by type.
func canonicalTargets(metric metricsmap.Metric) []PanelTarget {
	switch metric.Kind() {
	case metricsmap.KindCounter:
		return counterTargets(metric)
	case metricsmap.KindGauge:
		return gaugeTargets(metric)
	case metricsmap.KindHistogram:
		return quantileTargets(metric.Name+"_bucket", metric.Labels)
	case metricsmap.KindSummary:
		return summaryTargets(metric)
	default:
		return nil
	}
}

// counterTargets charts the per-second rate, broken down by the declared
// labels. A raw counter is a monotonically rising line nobody reads; the rate
// is what the declaration implies.
func counterTargets(metric metricsmap.Metric) []PanelTarget {
	expr := "sum(rate(" + metric.Name + "[$__rate_interval]))"
	legend := legendAuto

	if len(metric.Labels) > 0 {
		expr = "sum by(" + strings.Join(metric.Labels, ", ") + ") (rate(" + metric.Name + "[$__rate_interval]))"
		legend = labelLegend(metric.Labels)
	}

	return []PanelTarget{{Expr: expr, LegendFormat: legend, RefID: "A"}}
}

// gaugeTargets charts the value itself — a gauge is already a level.
func gaugeTargets(metric metricsmap.Metric) []PanelTarget {
	expr := metric.Name
	legend := legendAuto

	if len(metric.Labels) > 0 {
		expr = "sum by(" + strings.Join(metric.Labels, ", ") + ") (" + metric.Name + ")"
		legend = labelLegend(metric.Labels)
	}

	return []PanelTarget{{Expr: expr, LegendFormat: legend, RefID: "A"}}
}

// quantileTargets charts p99 and p95 off a histogram's _bucket series. `le` is
// always part of the grouping — histogram_quantile needs it.
func quantileTargets(bucketMetric string, labels metricsmap.Labels) []PanelTarget {
	grouping := append([]string{"le"}, labels...)
	by := "sum by(" + strings.Join(grouping, ", ") + ") (rate(" + bucketMetric + "[$__rate_interval]))"

	legendSuffix := ""
	if len(labels) > 0 {
		legendSuffix = " " + labelLegend(labels)
	}

	return []PanelTarget{
		{
			Expr:         "histogram_quantile(" + quantileP99 + ", " + by + ")",
			LegendFormat: "p99" + legendSuffix,
			RefID:        "A",
		},
		{
			Expr:         "histogram_quantile(" + quantileP95 + ", " + by + ")",
			LegendFormat: "p95" + legendSuffix,
			RefID:        "B",
		},
	}
}

// summaryTargets charts the declared quantiles directly — a Summary computes
// them client-side and exposes them on the `quantile` label, so
// histogram_quantile does not apply.
//
// With no declared objectives client_golang exposes only _sum and _count, so
// the panel falls back to the average over the interval: that is all the
// metric actually carries.
func summaryTargets(metric metricsmap.Metric) []PanelTarget {
	if len(metric.Objectives) == 0 {
		return []PanelTarget{{
			Expr: "sum(rate(" + metric.Name + "_sum[$__rate_interval])) / " +
				"sum(rate(" + metric.Name + "_count[$__rate_interval]))",
			LegendFormat: "avg",
			RefID:        "A",
		}}
	}

	grouping := append([]string{"quantile"}, metric.Labels...)

	legend := "q{{quantile}}"
	if len(metric.Labels) > 0 {
		legend += " " + labelLegend(metric.Labels)
	}

	return []PanelTarget{{
		Expr:         "sum by(" + strings.Join(grouping, ", ") + ") (" + metric.Name + ")",
		LegendFormat: legend,
		RefID:        "A",
	}}
}

// labelLegend renders the Grafana legend template for a label list:
// {{reason}} - {{stage}}.
func labelLegend(labels metricsmap.Labels) string {
	parts := make([]string, 0, len(labels))
	for _, label := range labels {
		parts = append(parts, "{{"+label+"}}")
	}

	return strings.Join(parts, " - ")
}

func hasDedicatedRow(name string) bool {
	for _, prefix := range dedicatedRowPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}

	return false
}
