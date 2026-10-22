package grafana

import (
	"regexp"
	"strings"

	"github.com/Educentr/go-project-starter/internal/pkg/metricsmap"
)

// TransportInfo contains transport information for dashboard generation.
type TransportInfo struct {
	Name          string
	GeneratorType string // "ogen", "ogen_client", "template"
}

// Datasource represents a resolved Grafana datasource for templates.
type Datasource struct {
	Name      string
	Type      string
	Access    string
	URL       string
	IsDefault bool
	Editable  bool
	UID       string // resolved: explicit uid from config, or "ds-" + lowercase(name)
}

// Annotation is a resolved annotation query for the dashboard template. It
// renders into an entry of the dashboard's .annotations.list[] and is what
// makes the events a service posts to Grafana's /api/annotations actually show
// up as vertical lines across the panels: without a matching query the events
// are stored but never drawn.
//
// Only tag queries against the built-in annotation store are modelled — see
// AnnotationDatasourceGrafana.
type Annotation struct {
	// Name labels the toggle in the dashboard header.
	Name string
	// IconColor is the colour of the marker line, in the form Grafana accepts
	// it ("yellow", "rgba(0, 211, 255, 1)", ...).
	IconColor string
	// Tags selects which events the query pulls in.
	Tags []string
	// MatchAny selects OR over Tags instead of AND.
	MatchAny bool
	// Hide hides the toggle in the dashboard header (the query still runs).
	Hide bool
	// Enable is the initial state of the toggle.
	Enable bool
	// Limit caps how many events a single query pulls into the view.
	Limit int
}

// Config holds resolved Grafana configuration.
type Config struct {
	Datasources []Datasource
	Annotations []Annotation
}

// Datasource type constants.
const (
	DatasourcePrometheus = "prometheus"
	DatasourceLoki       = "loki"
)

// Annotation query constants.
const (
	// AnnotationDatasourceGrafana is the only datasource an annotation query
	// may name: Grafana's built-in annotation store (uid "-- Grafana --"), the
	// one a service posts events to via /api/annotations. Querying prometheus
	// or loki for annotations needs a wholly different, expression-based
	// target, so it is not another value of the same field.
	AnnotationDatasourceGrafana = "grafana"
	// BuiltInAnnotationName is the name of the stock builtIn entry the
	// dashboard template always emits; a declared annotation cannot reuse it.
	BuiltInAnnotationName = "Annotations & Alerts"
	// DefaultAnnotationIconColor is the marker colour used when icon_color is
	// omitted — Grafana draws nothing for an empty iconColor.
	DefaultAnnotationIconColor = "yellow"
	// DefaultAnnotationLimit is the event cap used when limit is omitted. It
	// matches the default Grafana's own UI writes into a tag query.
	DefaultAnnotationLimit = 100
)

// MaxDatasourceUIDLen is the longest datasource uid Grafana accepts.
const MaxDatasourceUIDLen = 40

// datasourceUIDRe mirrors Grafana's uid charset: latin letters, digits, "-" and "_".
var datasourceUIDRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// HasDatasourceType checks if config has a datasource of the given type.
func (g Config) HasDatasourceType(dsType string) bool {
	for _, ds := range g.Datasources {
		if ds.Type == dsType {
			return true
		}
	}

	return false
}

// GetDatasourceUID returns the UID of the first datasource of the given type.
func (g Config) GetDatasourceUID(dsType string) string {
	for _, ds := range g.Datasources {
		if ds.Type == dsType {
			return ds.UID
		}
	}

	return ""
}

// GetDatasourceByType returns the first datasource of the given type.
func (g Config) GetDatasourceByType(dsType string) *Datasource {
	for i, ds := range g.Datasources {
		if ds.Type == dsType {
			return &g.Datasources[i]
		}
	}

	return nil
}

// HasDatasources returns true if there are any datasources configured.
func (g Config) HasDatasources() bool {
	return len(g.Datasources) > 0
}

// HasPrometheus returns true if prometheus datasource is configured.
func (g Config) HasPrometheus() bool {
	return g.HasDatasourceType(DatasourcePrometheus)
}

// HasLoki returns true if loki datasource is configured.
func (g Config) HasLoki() bool {
	return g.HasDatasourceType(DatasourceLoki)
}

// GetDashboardRows returns all dashboard rows based on configured datasources,
// transports, and the metrics the service declared.
//
// The declared metrics come last, in their own row: the rows above are
// hand-built and know more about their metrics than a canonical shape derived
// from a type can.
func (g Config) GetDashboardRows(appName string, transports []TransportInfo, declared []metricsmap.Metric) []Row {
	var rows []Row

	// 1. Logs row (if Loki datasource exists)
	if g.HasDatasourceType(DatasourceLoki) {
		rows = append(rows, Row{
			Title:     "Logs",
			Collapsed: true,
			Panels:    DefaultLogsPanels(appName),
		})
	}

	// 2. Go Runtime row (if Prometheus datasource exists)
	if g.HasDatasourceType(DatasourcePrometheus) {
		rows = append(rows, Row{
			Title:     "Go Runtime",
			Collapsed: true,
			Panels:    DefaultGoRuntimePanels(),
		})

		// 3. Http Server rows for each ogen transport (excluding template type)
		for _, t := range transports {
			if t.GeneratorType == "ogen" {
				rows = append(rows, Row{
					Title:     "Http Server: " + t.Name,
					Collapsed: true,
					Panels:    DefaultHTTPServerPanels(t.Name),
				})
			}
		}

		// 4. Http Client rows for each ogen_client transport
		for _, t := range transports {
			if t.GeneratorType == "ogen_client" {
				rows = append(rows, Row{
					Title:     "Http Client: " + t.Name,
					Collapsed: true,
					Panels:    DefaultHTTPClientPanels(t.Name),
				})
			}
		}

		// 5. Service metrics row, from the declarations in the metrics map.
		if row, ok := DeclaredMetricsRow(declared); ok {
			rows = append(rows, row)
		}
	}

	return rows
}

// GenerateDatasourceUID generates a deterministic UID from datasource name.
func GenerateDatasourceUID(name string) string {
	return "ds-" + strings.ToLower(strings.ReplaceAll(name, " ", "-"))
}

// ResolveDatasourceUID returns the uid a datasource is provisioned with: the
// explicit uid when set, otherwise the one derived from the name. This is the
// single source of truth for the uid that lands both in the provisioning file
// and in every dashboard panel, so an explicit uid cannot drift between them.
func ResolveDatasourceUID(uid, name string) string {
	if uid != "" {
		return uid
	}

	return GenerateDatasourceUID(name)
}

// ValidateDatasourceUID reports whether uid is accepted by Grafana: 1 to
// MaxDatasourceUIDLen characters of latin letters, digits, "-" and "_".
func ValidateDatasourceUID(uid string) bool {
	if uid == "" || len(uid) > MaxDatasourceUIDLen {
		return false
	}

	return datasourceUIDRe.MatchString(uid)
}
