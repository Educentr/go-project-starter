package grafana

import "strings"

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
	UID       string // explicit uid from the config, or the generated "ds-" + lowercase(name)
}

// Config holds resolved Grafana configuration.
type Config struct {
	Datasources []Datasource
}

// Datasource type constants.
const (
	DatasourcePrometheus = "prometheus"
	DatasourceLoki       = "loki"
)

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

// GetDashboardRows returns all dashboard rows based on configured datasources and transports.
func (g Config) GetDashboardRows(appName string, transports []TransportInfo) []Row {
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
	}

	return rows
}

// MaxDatasourceUIDLen is the length limit Grafana puts on a datasource uid.
const MaxDatasourceUIDLen = 40

// GenerateDatasourceUID generates a deterministic UID from datasource name.
func GenerateDatasourceUID(name string) string {
	return "ds-" + strings.ToLower(strings.ReplaceAll(name, " ", "-"))
}

// ResolveDatasourceUID returns the uid to emit for a datasource: the explicit
// one when the config sets it, the generated one otherwise.
//
// Both the provisioning file and the dashboards must go through this, or the
// panels reference a uid the provisioned datasource does not have.
func ResolveDatasourceUID(explicitUID, name string) string {
	if explicitUID != "" {
		return explicitUID
	}

	return GenerateDatasourceUID(name)
}

// IsValidDatasourceUID reports whether uid satisfies Grafana's constraints:
// non-empty, at most MaxDatasourceUIDLen characters, and built only from
// letters, digits, "-" and "_".
func IsValidDatasourceUID(uid string) bool {
	if uid == "" || len(uid) > MaxDatasourceUIDLen {
		return false
	}

	for _, r := range uid {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-',
			r == '_':
		default:
			return false
		}
	}

	return true
}
