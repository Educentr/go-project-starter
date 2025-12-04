package grafana

import "strings"

// Datasource represents a resolved Grafana datasource for templates.
type Datasource struct {
	Name      string
	Type      string
	Access    string
	URL       string
	IsDefault bool
	Editable  bool
	UID       string // generated: "ds-" + lowercase(name)
}

// Config holds resolved Grafana configuration.
type Config struct {
	Datasources []Datasource
}

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

// GetDashboardRows returns all dashboard rows based on configured datasources.
func (g Config) GetDashboardRows(appName string) []Row {
	var rows []Row

	if g.HasDatasourceType("loki") {
		rows = append(rows, Row{
			Title:     "Logs",
			Collapsed: true,
			Panels:    DefaultLogsPanels(appName),
		})
	}

	if g.HasDatasourceType("prometheus") {
		rows = append(rows, Row{
			Title:     "Go Runtime",
			Collapsed: true,
			Panels:    DefaultGoRuntimePanels(),
		})
		rows = append(rows, Row{
			Title:     "HTTP Metrics",
			Collapsed: true,
			Panels:    DefaultHTTPMetricsPanels(),
		})
	}

	return rows
}

// GenerateDatasourceUID generates a deterministic UID from datasource name.
func GenerateDatasourceUID(name string) string {
	return "ds-" + strings.ToLower(strings.ReplaceAll(name, " ", "-"))
}
