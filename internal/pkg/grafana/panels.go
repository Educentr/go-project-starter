package grafana

// PanelTarget defines a query target for a panel.
type PanelTarget struct {
	Expr         string // PromQL or LogQL expression
	LegendFormat string // Legend template
	RefID        string // Reference ID (A, B, C...)
}

// LogsPanelOptions contains options specific to logs panels.
type LogsPanelOptions struct {
	ShowTime    bool
	WrapMessage bool
	SortOrder   string // "Descending", "Ascending"
}

// Panel defines a dashboard panel.
type Panel struct {
	Title       string
	Type        string // "timeseries", "logs"
	Width       int    // Grid width (1-24)
	Height      int    // Grid height
	Targets     []PanelTarget
	Datasource  string // "prometheus" or "loki"
	LogsOptions *LogsPanelOptions
}

// Row groups panels into collapsible sections.
type Row struct {
	Title     string
	Collapsed bool
	Panels    []Panel
}

// Panel dimension constants.
const (
	panelWidthHalf = 12
	panelWidthFull = 24
	panelHeightS   = 8
	panelHeightM   = 10
)

// DefaultGoRuntimePanels returns standard Go runtime metrics panels.
func DefaultGoRuntimePanels() []Panel {
	return []Panel{
		{
			Title:      "Go GC",
			Type:       "timeseries",
			Width:      panelWidthHalf,
			Height:     panelHeightS,
			Datasource: "prometheus",
			Targets: []PanelTarget{
				{
					Expr:         `sum by(instance) (rate(go_gc_duration_seconds_sum[5m]))`,
					LegendFormat: "__auto",
					RefID:        "A",
				},
			},
		},
		{
			Title:      "Goroutines",
			Type:       "timeseries",
			Width:      panelWidthHalf,
			Height:     panelHeightS,
			Datasource: "prometheus",
			Targets: []PanelTarget{
				{
					Expr:         `go_goroutines`,
					LegendFormat: "__auto",
					RefID:        "A",
				},
			},
		},
		{
			Title:      "CPU",
			Type:       "timeseries",
			Width:      panelWidthHalf,
			Height:     panelHeightS,
			Datasource: "prometheus",
			Targets: []PanelTarget{
				{
					Expr:         `sum by(instance) (rate(process_cpu_seconds_total[5m]))`,
					LegendFormat: "__auto",
					RefID:        "A",
				},
			},
		},
		{
			Title:      "Memory Alloc",
			Type:       "timeseries",
			Width:      panelWidthHalf,
			Height:     panelHeightS,
			Datasource: "prometheus",
			Targets: []PanelTarget{
				{
					Expr:         `sum by(instance) (rate(go_memstats_alloc_bytes[5m]))`,
					LegendFormat: "__auto",
					RefID:        "A",
				},
			},
		},
	}
}

// DefaultHTTPMetricsPanels returns HTTP metrics panels.
func DefaultHTTPMetricsPanels() []Panel {
	return []Panel{
		{
			Title:      "HTTP Status Codes",
			Type:       "timeseries",
			Width:      panelWidthFull,
			Height:     panelHeightM,
			Datasource: "prometheus",
			Targets: []PanelTarget{
				{
					Expr:         `sum by(http_response_status_code) (increase(ogen_server_request_count_total[$__rate_interval]))`,
					LegendFormat: "{{http_response_status_code}}",
					RefID:        "A",
				},
			},
		},
		{
			Title:      "500 Errors",
			Type:       "timeseries",
			Width:      panelWidthHalf,
			Height:     panelHeightM,
			Datasource: "prometheus",
			Targets: []PanelTarget{
				{
					Expr: `sum by(http_request_method, http_route) ` +
						`(increase(ogen_server_errors_count_total{http_response_status_code="500"}[$__rate_interval]))`,
					LegendFormat: "{{http_request_method}} - {{http_route}}",
					RefID:        "A",
				},
			},
		},
		{
			Title:      "4xx Errors",
			Type:       "timeseries",
			Width:      panelWidthHalf,
			Height:     panelHeightM,
			Datasource: "prometheus",
			Targets: []PanelTarget{
				{
					Expr: `sum by(http_request_method, http_route, http_response_status_code) ` +
						`(increase(ogen_server_errors_count_total{http_response_status_code=~"4.."}[$__rate_interval]))`,
					LegendFormat: "{{http_request_method}} - {{http_route}} {{http_response_status_code}}",
					RefID:        "A",
				},
			},
		},
		{
			Title:      "Requests by Route",
			Type:       "timeseries",
			Width:      panelWidthHalf,
			Height:     panelHeightM,
			Datasource: "prometheus",
			Targets: []PanelTarget{
				{
					Expr:         `sum by(http_request_method, http_route) (increase(ogen_server_request_count_total[$__rate_interval]))`,
					LegendFormat: "{{http_request_method}} - {{http_route}}",
					RefID:        "A",
				},
			},
		},
		{
			Title:      "Request Latency (ms)",
			Type:       "timeseries",
			Width:      panelWidthHalf,
			Height:     panelHeightM,
			Datasource: "prometheus",
			Targets: []PanelTarget{
				{
					Expr: `histogram_quantile(0.99, sum by(le, http_route) ` +
						`(rate(ogen_server_duration_milliseconds_bucket[$__rate_interval])))`,
					LegendFormat: "p99 {{http_route}}",
					RefID:        "A",
				},
				{
					Expr: `histogram_quantile(0.95, sum by(le, http_route) ` +
						`(rate(ogen_server_duration_milliseconds_bucket[$__rate_interval])))`,
					LegendFormat: "p95 {{http_route}}",
					RefID:        "B",
				},
			},
		},
	}
}

// DefaultLogsPanels returns Loki log panels.
func DefaultLogsPanels(appName string) []Panel {
	return []Panel{
		{
			Title:      "Application Logs",
			Type:       "logs",
			Width:      panelWidthFull,
			Height:     panelHeightM,
			Datasource: "loki",
			Targets: []PanelTarget{
				{
					Expr:  `{service_name=~"backend-api-1"} | json | logfmt | drop __error__, __error_details__ | level = `,
					RefID: "A",
				},
			},
			LogsOptions: &LogsPanelOptions{ShowTime: true, WrapMessage: true, SortOrder: "Descending"},
		},
		{
			Title:      "Error Logs",
			Type:       "logs",
			Width:      panelWidthFull,
			Height:     panelHeightM,
			Datasource: "loki",
			Targets: []PanelTarget{
				{
					Expr:  `{service_name=~"backend-api-1"} | json | logfmt | drop __error__, __error_details__ | message = "Failed to create widget token"`,
					RefID: "A",
				},
			},
			LogsOptions: &LogsPanelOptions{ShowTime: true, WrapMessage: true, SortOrder: "Descending"},
		},
	}
}
