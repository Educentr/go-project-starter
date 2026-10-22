package config

import (
	"strings"
	"testing"
)

func TestMain_IsValid(t *testing.T) {
	tests := []struct {
		name    string
		main    Main
		wantOK  bool
		wantMsg string
	}{
		{
			name: "valid config with zerolog",
			main: Main{
				Name:         "myproject",
				Logger:       "zerolog",
				RegistryType: "github",
			},
			wantOK: true,
		},
		{
			name: "valid config with logrus",
			main: Main{
				Name:         "myproject",
				Logger:       "logrus",
				RegistryType: "github",
			},
			wantOK: true,
		},
		{
			name: "valid config with digitalocean registry",
			main: Main{
				Name:         "myproject",
				Logger:       "zerolog",
				RegistryType: "digitalocean",
			},
			wantOK: true,
		},
		{
			name: "valid config with aws registry",
			main: Main{
				Name:         "myproject",
				Logger:       "zerolog",
				RegistryType: "aws",
			},
			wantOK: true,
		},
		{
			name: "valid config with selfhosted registry",
			main: Main{
				Name:         "myproject",
				Logger:       "zerolog",
				RegistryType: "selfhosted",
			},
			wantOK: true,
		},
		{
			name: "empty name",
			main: Main{
				Name:         "",
				Logger:       "zerolog",
				RegistryType: "github",
			},
			wantOK:  false,
			wantMsg: "Empty name",
		},
		{
			name: "invalid logger",
			main: Main{
				Name:         "myproject",
				Logger:       "slog",
				RegistryType: "github",
			},
			wantOK:  false,
			wantMsg: "invalid logger",
		},
		{
			name: "empty logger",
			main: Main{
				Name:         "myproject",
				Logger:       "",
				RegistryType: "github",
			},
			wantOK:  false,
			wantMsg: "invalid logger",
		},
		{
			name: "empty registry type",
			main: Main{
				Name:         "myproject",
				Logger:       "zerolog",
				RegistryType: "",
			},
			wantOK:  false,
			wantMsg: "RegistryType not set ",
		},
		{
			name: "invalid registry type",
			main: Main{
				Name:         "myproject",
				Logger:       "zerolog",
				RegistryType: "invalid",
			},
			wantOK:  false,
			wantMsg: "RegistryType value can be 'github', 'digitalocean', 'aws', or 'selfhosted', invalid RegistryType value invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOK, gotMsg := tt.main.IsValid()

			if gotOK != tt.wantOK {
				t.Errorf("Main.IsValid() ok = %v, want %v", gotOK, tt.wantOK)
			}

			if !tt.wantOK && gotMsg != tt.wantMsg {
				t.Errorf("Main.IsValid() msg = %q, want %q", gotMsg, tt.wantMsg)
			}
		})
	}
}

func TestGit_IsValid(t *testing.T) {
	tests := []struct {
		name    string
		git     Git
		wantOK  bool
		wantMsg string
	}{
		{
			name: "valid git config",
			git: Git{
				ModulePath: "github.com/org/repo",
				Repo:       "https://github.com/org/repo",
			},
			wantOK: true,
		},
		{
			name: "empty module path",
			git: Git{
				ModulePath: "",
				Repo:       "https://github.com/org/repo",
			},
			wantOK:  false,
			wantMsg: "Empty module path",
		},
		{
			name: "empty repo",
			git: Git{
				ModulePath: "github.com/org/repo",
				Repo:       "",
			},
			wantOK:  false,
			wantMsg: "Empty repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOK, gotMsg := tt.git.IsValid()

			if gotOK != tt.wantOK {
				t.Errorf("Git.IsValid() ok = %v, want %v", gotOK, tt.wantOK)
			}

			if !tt.wantOK && gotMsg != tt.wantMsg {
				t.Errorf("Git.IsValid() msg = %q, want %q", gotMsg, tt.wantMsg)
			}
		})
	}
}

func TestWorker_IsValid(t *testing.T) {
	tests := []struct {
		name    string
		worker  Worker
		wantOK  bool
		wantMsg string
	}{
		{
			name: "valid template worker",
			worker: Worker{
				Name:              "telegram_bot",
				GeneratorType:     "template",
				GeneratorTemplate: "telegram",
			},
			wantOK: true,
		},
		{
			name: "valid daemon worker",
			worker: Worker{
				Name:              "background_worker",
				GeneratorType:     "template",
				GeneratorTemplate: "daemon",
			},
			wantOK: true,
		},
		{
			name: "empty name",
			worker: Worker{
				Name:              "",
				GeneratorType:     "template",
				GeneratorTemplate: "telegram",
			},
			wantOK:  false,
			wantMsg: "Empty name",
		},
		{
			name: "invalid generator type",
			worker: Worker{
				Name:              "worker",
				GeneratorType:     "ogen",
				GeneratorTemplate: "telegram",
			},
			wantOK:  false,
			wantMsg: "Invalid generator type",
		},
		{
			name: "empty generator template",
			worker: Worker{
				Name:          "worker",
				GeneratorType: "template",
			},
			wantOK:  false,
			wantMsg: "Empty generator template",
		},
		{
			name: "template with generator params",
			worker: Worker{
				Name:              "worker",
				GeneratorType:     "template",
				GeneratorTemplate: "telegram",
				GeneratorParams:   map[string]string{"key": "value"},
			},
			wantOK:  false,
			wantMsg: "Generator params not supported",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOK, gotMsg := tt.worker.IsValid("")

			if gotOK != tt.wantOK {
				t.Errorf("Worker.IsValid() ok = %v, want %v", gotOK, tt.wantOK)
			}

			if !tt.wantOK && gotMsg != tt.wantMsg {
				t.Errorf("Worker.IsValid() msg = %q, want %q", gotMsg, tt.wantMsg)
			}
		})
	}
}

func TestDriver_IsValid(t *testing.T) {
	tests := []struct {
		name    string
		driver  Driver
		wantOK  bool
		wantMsg string
	}{
		{
			name: "valid driver",
			driver: Driver{
				Name:    "s3",
				Import:  "github.com/myorg/drivers/s3",
				Package: "s3",
				ObjName: "Client",
			},
			wantOK: true,
		},
		{
			name: "valid driver with service injection",
			driver: Driver{
				Name:             "postgres",
				Import:           "github.com/myorg/pg",
				Package:          "pg",
				ObjName:          "DB",
				ServiceInjection: "db.SetLogger(logger)",
			},
			wantOK: true,
		},
		{
			name: "empty name",
			driver: Driver{
				Name:    "",
				Import:  "github.com/myorg/drivers/s3",
				Package: "s3",
				ObjName: "Client",
			},
			wantOK:  false,
			wantMsg: "Empty name",
		},
		{
			name: "empty import",
			driver: Driver{
				Name:    "s3",
				Import:  "",
				Package: "s3",
				ObjName: "Client",
			},
			wantOK:  false,
			wantMsg: "Empty import",
		},
		{
			name: "empty package",
			driver: Driver{
				Name:    "s3",
				Import:  "github.com/myorg/drivers/s3",
				Package: "",
				ObjName: "Client",
			},
			wantOK:  false,
			wantMsg: "Empty package",
		},
		{
			name: "empty object name",
			driver: Driver{
				Name:    "s3",
				Import:  "github.com/myorg/drivers/s3",
				Package: "s3",
				ObjName: "",
			},
			wantOK:  false,
			wantMsg: "Empty object name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOK, gotMsg := tt.driver.IsValid()

			if gotOK != tt.wantOK {
				t.Errorf("Driver.IsValid() ok = %v, want %v", gotOK, tt.wantOK)
			}

			if !tt.wantOK && gotMsg != tt.wantMsg {
				t.Errorf("Driver.IsValid() msg = %q, want %q", gotMsg, tt.wantMsg)
			}
		})
	}
}

func TestCLI_IsValid(t *testing.T) {
	tests := []struct {
		name    string
		cli     CLI
		wantOK  bool
		wantMsg string
	}{
		{
			name: "valid CLI",
			cli: CLI{
				Name: "admin",
			},
			wantOK: true,
		},
		{
			name: "empty name",
			cli: CLI{
				Name: "",
			},
			wantOK:  false,
			wantMsg: "Empty name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOK, gotMsg := tt.cli.IsValid()

			if gotOK != tt.wantOK {
				t.Errorf("CLI.IsValid() ok = %v, want %v", gotOK, tt.wantOK)
			}

			if !tt.wantOK && gotMsg != tt.wantMsg {
				t.Errorf("CLI.IsValid() msg = %q, want %q", gotMsg, tt.wantMsg)
			}
		})
	}
}

func TestApplication_IsValid(t *testing.T) {
	tests := []struct {
		name    string
		app     Application
		wantOK  bool
		wantMsg string
	}{
		{
			name: "valid app with transport",
			app: Application{
				Name: "api",
				TransportList: []AppTransport{
					{Name: "api_v1"},
				},
			},
			wantOK: true,
		},
		{
			name: "valid app with multiple transports",
			app: Application{
				Name: "api",
				TransportList: []AppTransport{
					{Name: "api_v1"},
					{Name: "sys"},
				},
			},
			wantOK: true,
		},
		{
			name: "valid CLI app",
			app: Application{
				Name: "cli-app",
				CLI:  "admin",
			},
			wantOK: true,
		},
		{
			name: "valid app with transport config",
			app: Application{
				Name: "api",
				TransportList: []AppTransport{
					{
						Name: "external_api",
						Config: AppTransportConfig{
							Instantiation: "dynamic",
						},
					},
				},
			},
			wantOK: true,
		},
		{
			name: "empty name",
			app: Application{
				Name: "",
				TransportList: []AppTransport{
					{Name: "api_v1"},
				},
			},
			wantOK:  false,
			wantMsg: "Empty name",
		},
		{
			name: "no transports and not CLI",
			app: Application{
				Name:          "api",
				TransportList: nil,
			},
			wantOK:  false,
			wantMsg: "Application must have at least one transport or be a CLI app",
		},
		{
			name: "empty transport list and not CLI",
			app: Application{
				Name:          "api",
				TransportList: []AppTransport{},
			},
			wantOK:  false,
			wantMsg: "Application must have at least one transport or be a CLI app",
		},
		{
			name: "CLI with transports",
			app: Application{
				Name: "cli-app",
				CLI:  "admin",
				TransportList: []AppTransport{
					{Name: "api_v1"},
				},
			},
			wantOK:  false,
			wantMsg: "CLI application cannot have transports",
		},
		{
			name: "CLI with workers",
			app: Application{
				Name:       "cli-app",
				CLI:        "admin",
				WorkerList: []string{"telegram"},
			},
			wantOK:  false,
			wantMsg: "CLI application cannot have workers",
		},
		{
			name: "transport with empty name",
			app: Application{
				Name: "api",
				TransportList: []AppTransport{
					{Name: ""},
				},
			},
			wantOK:  false,
			wantMsg: "Transport name cannot be empty",
		},
		{
			name: "transport with invalid instantiation",
			app: Application{
				Name: "api",
				TransportList: []AppTransport{
					{
						Name: "api",
						Config: AppTransportConfig{
							Instantiation: "invalid",
						},
					},
				},
			},
			wantOK:  false,
			wantMsg: "transport api: instantiation must be 'static' or 'dynamic'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOK, gotMsg := tt.app.IsValid()

			if gotOK != tt.wantOK {
				t.Errorf("Application.IsValid() ok = %v, want %v", gotOK, tt.wantOK)
			}

			if !tt.wantOK && gotMsg != tt.wantMsg {
				t.Errorf("Application.IsValid() msg = %q, want %q", gotMsg, tt.wantMsg)
			}
		})
	}
}

func TestApplication_NormalizeTransports(t *testing.T) {
	tests := []struct {
		name      string
		rawData   any
		wantList  []AppTransport
		wantError bool
	}{
		{
			name:      "nil raw data",
			rawData:   nil,
			wantList:  nil,
			wantError: false,
		},
		{
			name: "object format with name only",
			rawData: []any{
				map[string]any{"name": "api"},
			},
			wantList: []AppTransport{
				{Name: "api"},
			},
			wantError: false,
		},
		{
			name: "object format with config",
			rawData: []any{
				map[string]any{
					"name": "api",
					"config": map[string]any{
						"instantiation": "dynamic",
						"optional":      true,
					},
				},
			},
			wantList: []AppTransport{
				{
					Name: "api",
					Config: AppTransportConfig{
						Instantiation: "dynamic",
						Optional:      true,
					},
				},
			},
			wantError: false,
		},
		{
			name: "multiple transports",
			rawData: []any{
				map[string]any{"name": "api"},
				map[string]any{"name": "sys"},
			},
			wantList: []AppTransport{
				{Name: "api"},
				{Name: "sys"},
			},
			wantError: false,
		},
		{
			name:      "not an array",
			rawData:   "api",
			wantError: true,
		},
		{
			name: "string format (deprecated)",
			rawData: []any{
				"api",
			},
			wantError: true,
		},
		{
			name: "object without name",
			rawData: []any{
				map[string]any{"config": map[string]any{}},
			},
			wantError: true,
		},
		{
			name: "object with empty name",
			rawData: []any{
				map[string]any{"name": ""},
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := Application{TransportListRaw: tt.rawData}
			err := app.NormalizeTransports()

			if tt.wantError {
				if err == nil {
					t.Errorf("Application.NormalizeTransports() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Errorf("Application.NormalizeTransports() error = %v, want nil", err)

				return
			}

			if len(app.TransportList) != len(tt.wantList) {
				t.Errorf("Application.NormalizeTransports() len = %d, want %d", len(app.TransportList), len(tt.wantList))

				return
			}

			for i, want := range tt.wantList {
				got := app.TransportList[i]

				if got.Name != want.Name {
					t.Errorf("TransportList[%d].Name = %q, want %q", i, got.Name, want.Name)
				}

				if got.Config.Instantiation != want.Config.Instantiation {
					t.Errorf("TransportList[%d].Config.Instantiation = %q, want %q", i, got.Config.Instantiation, want.Config.Instantiation)
				}

				if got.Config.Optional != want.Config.Optional {
					t.Errorf("TransportList[%d].Config.Optional = %v, want %v", i, got.Config.Optional, want.Config.Optional)
				}
			}
		})
	}
}

func TestApplication_NormalizeKafka(t *testing.T) {
	tests := []struct {
		name      string
		rawData   any
		wantList  []AppKafka
		wantError bool
	}{
		{
			name:      "nil raw data",
			rawData:   nil,
			wantList:  nil,
			wantError: false,
		},
		{
			name: "string format",
			rawData: []any{
				"events_producer",
			},
			wantList: []AppKafka{
				{Name: "events_producer"},
			},
			wantError: false,
		},
		{
			name: "object format",
			rawData: []any{
				map[string]any{
					"name":     "events_producer",
					"optional": true,
				},
			},
			wantList: []AppKafka{
				{Name: "events_producer", Optional: true},
			},
			wantError: false,
		},
		{
			name: "mixed formats",
			rawData: []any{
				"required_kafka",
				map[string]any{"name": "optional_kafka", "optional": true},
			},
			wantList: []AppKafka{
				{Name: "required_kafka"},
				{Name: "optional_kafka", Optional: true},
			},
			wantError: false,
		},
		{
			name:      "not an array",
			rawData:   "kafka",
			wantError: true,
		},
		{
			name: "object without name",
			rawData: []any{
				map[string]any{"optional": true},
			},
			wantError: true,
		},
		{
			name: "invalid item type",
			rawData: []any{
				123,
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := Application{KafkaListRaw: tt.rawData}
			err := app.NormalizeKafka()

			if tt.wantError {
				if err == nil {
					t.Errorf("Application.NormalizeKafka() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Errorf("Application.NormalizeKafka() error = %v, want nil", err)

				return
			}

			if len(app.KafkaList) != len(tt.wantList) {
				t.Errorf("Application.NormalizeKafka() len = %d, want %d", len(app.KafkaList), len(tt.wantList))

				return
			}

			for i, want := range tt.wantList {
				got := app.KafkaList[i]

				if got.Name != want.Name {
					t.Errorf("KafkaList[%d].Name = %q, want %q", i, got.Name, want.Name)
				}

				if got.Optional != want.Optional {
					t.Errorf("KafkaList[%d].Optional = %v, want %v", i, got.Optional, want.Optional)
				}
			}
		})
	}
}

func TestGrafanaDatasource_IsValid(t *testing.T) {
	tests := []struct {
		name    string
		ds      GrafanaDatasource
		wantOK  bool
		wantMsg string
	}{
		{
			name: "valid prometheus datasource",
			ds: GrafanaDatasource{
				Name: "prometheus",
				Type: "prometheus",
				URL:  "http://prometheus:9090",
			},
			wantOK: true,
		},
		{
			name: "valid loki datasource",
			ds: GrafanaDatasource{
				Name: "loki",
				Type: "loki",
				URL:  "http://loki:3100",
			},
			wantOK: true,
		},
		{
			name: "valid datasource with access mode",
			ds: GrafanaDatasource{
				Name:   "prometheus",
				Type:   "prometheus",
				URL:    "http://prometheus:9090",
				Access: "proxy",
			},
			wantOK: true,
		},
		{
			name: "valid datasource with direct access",
			ds: GrafanaDatasource{
				Name:   "prometheus",
				Type:   "prometheus",
				URL:    "http://prometheus:9090",
				Access: "direct",
			},
			wantOK: true,
		},
		{
			name: "empty name",
			ds: GrafanaDatasource{
				Name: "",
				Type: "prometheus",
				URL:  "http://prometheus:9090",
			},
			wantOK:  false,
			wantMsg: "Empty datasource name",
		},
		{
			name: "empty type",
			ds: GrafanaDatasource{
				Name: "prometheus",
				Type: "",
				URL:  "http://prometheus:9090",
			},
			wantOK:  false,
			wantMsg: "Empty datasource type for prometheus",
		},
		{
			name: "invalid type",
			ds: GrafanaDatasource{
				Name: "influx",
				Type: "influxdb",
				URL:  "http://influx:8086",
			},
			wantOK:  false,
			wantMsg: "Invalid datasource type: influxdb (supported: prometheus, loki)",
		},
		{
			name: "invalid access mode",
			ds: GrafanaDatasource{
				Name:   "prometheus",
				Type:   "prometheus",
				URL:    "http://prometheus:9090",
				Access: "invalid",
			},
			wantOK:  false,
			wantMsg: "Invalid access mode: invalid (supported: proxy, direct)",
		},
		{
			name: "empty URL",
			ds: GrafanaDatasource{
				Name: "prometheus",
				Type: "prometheus",
				URL:  "",
			},
			wantOK:  false,
			wantMsg: "Empty URL for datasource prometheus",
		},
		{
			name: "valid explicit uid",
			ds: GrafanaDatasource{
				Name: "Prometheus",
				Type: "prometheus",
				URL:  "http://victoriametrics:8428",
				UID:  "victoriametrics",
			},
			wantOK: true,
		},
		{
			name: "uid with dash and underscore",
			ds: GrafanaDatasource{
				Name: "Prometheus",
				Type: "prometheus",
				URL:  "http://prometheus:9090",
				UID:  "shared_metrics-1",
			},
			wantOK: true,
		},
		{
			name: "uid with forbidden character",
			ds: GrafanaDatasource{
				Name: "Prometheus",
				Type: "prometheus",
				URL:  "http://prometheus:9090",
				UID:  "victoria.metrics",
			},
			wantOK:  false,
			wantMsg: "Invalid uid for datasource Prometheus: victoria.metrics (expected 1-40 characters of a-z, A-Z, 0-9, '-', '_')",
		},
		{
			name: "uid longer than 40 characters",
			ds: GrafanaDatasource{
				Name: "Prometheus",
				Type: "prometheus",
				URL:  "http://prometheus:9090",
				UID:  "01234567890123456789012345678901234567890",
			},
			wantOK: false,
			wantMsg: "Invalid uid for datasource Prometheus: 01234567890123456789012345678901234567890 " +
				"(expected 1-40 characters of a-z, A-Z, 0-9, '-', '_')",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOK, gotMsg := tt.ds.IsValid()

			if gotOK != tt.wantOK {
				t.Errorf("GrafanaDatasource.IsValid() ok = %v, want %v", gotOK, tt.wantOK)
			}

			if !tt.wantOK && gotMsg != tt.wantMsg {
				t.Errorf("GrafanaDatasource.IsValid() msg = %q, want %q", gotMsg, tt.wantMsg)
			}
		})
	}
}

func TestGrafana_IsValid(t *testing.T) {
	tests := []struct {
		name    string
		grafana Grafana
		wantOK  bool
		wantMsg string
	}{
		{
			name:    "empty grafana config is valid",
			grafana: Grafana{},
			wantOK:  true,
		},
		{
			name: "single valid datasource",
			grafana: Grafana{
				Datasources: []GrafanaDatasource{
					{Name: "prometheus", Type: "prometheus", URL: "http://prometheus:9090"},
				},
			},
			wantOK: true,
		},
		{
			name: "multiple valid datasources",
			grafana: Grafana{
				Datasources: []GrafanaDatasource{
					{Name: "prometheus", Type: "prometheus", URL: "http://prometheus:9090"},
					{Name: "loki", Type: "loki", URL: "http://loki:3100"},
				},
			},
			wantOK: true,
		},
		{
			name: "single default datasource",
			grafana: Grafana{
				Datasources: []GrafanaDatasource{
					{Name: "prometheus", Type: "prometheus", URL: "http://prometheus:9090", IsDefault: true},
					{Name: "loki", Type: "loki", URL: "http://loki:3100"},
				},
			},
			wantOK: true,
		},
		{
			name: "invalid datasource",
			grafana: Grafana{
				Datasources: []GrafanaDatasource{
					{Name: "", Type: "prometheus", URL: "http://prometheus:9090"},
				},
			},
			wantOK:  false,
			wantMsg: "Empty datasource name",
		},
		{
			name: "duplicate datasource names",
			grafana: Grafana{
				Datasources: []GrafanaDatasource{
					{Name: "prometheus", Type: "prometheus", URL: "http://prometheus1:9090"},
					{Name: "prometheus", Type: "prometheus", URL: "http://prometheus2:9090"},
				},
			},
			wantOK:  false,
			wantMsg: "Duplicate datasource name: prometheus",
		},
		{
			name: "multiple default datasources",
			grafana: Grafana{
				Datasources: []GrafanaDatasource{
					{Name: "prometheus", Type: "prometheus", URL: "http://prometheus:9090", IsDefault: true},
					{Name: "loki", Type: "loki", URL: "http://loki:3100", IsDefault: true},
				},
			},
			wantOK:  false,
			wantMsg: "Only one datasource can be default",
		},
		{
			name: "explicit uid alongside a generated one",
			grafana: Grafana{
				Datasources: []GrafanaDatasource{
					{Name: "Prometheus", Type: "prometheus", URL: "http://victoriametrics:8428", UID: "victoriametrics"},
					{Name: "Loki", Type: "loki", URL: "http://loki:3100"},
				},
			},
			wantOK: true,
		},
		{
			name: "duplicate explicit uids",
			grafana: Grafana{
				Datasources: []GrafanaDatasource{
					{Name: "Prometheus", Type: "prometheus", URL: "http://prometheus:9090", UID: "shared"},
					{Name: "Loki", Type: "loki", URL: "http://loki:3100", UID: "shared"},
				},
			},
			wantOK:  false,
			wantMsg: "Duplicate datasource uid shared: used by Prometheus and Loki",
		},
		{
			// The collision that only shows up after resolution: Loki's explicit
			// uid is exactly what Prometheus' name generates.
			name: "explicit uid collides with another datasource's generated uid",
			grafana: Grafana{
				Datasources: []GrafanaDatasource{
					{Name: "Prometheus", Type: "prometheus", URL: "http://prometheus:9090"},
					{Name: "Loki", Type: "loki", URL: "http://loki:3100", UID: "ds-prometheus"},
				},
			},
			wantOK:  false,
			wantMsg: "Duplicate datasource uid ds-prometheus: used by Prometheus and Loki",
		},
		{
			// Mirror of the case above with the explicit uid declared first, to
			// pin that detection does not depend on declaration order.
			name: "generated uid collides with an earlier explicit uid",
			grafana: Grafana{
				Datasources: []GrafanaDatasource{
					{Name: "Loki", Type: "loki", URL: "http://loki:3100", UID: "ds-prometheus"},
					{Name: "Prometheus", Type: "prometheus", URL: "http://prometheus:9090"},
				},
			},
			wantOK:  false,
			wantMsg: "Duplicate datasource uid ds-prometheus: used by Loki and Prometheus",
		},
		{
			name: "invalid uid is reported from the datasource check",
			grafana: Grafana{
				Datasources: []GrafanaDatasource{
					{Name: "Prometheus", Type: "prometheus", URL: "http://prometheus:9090", UID: "bad uid"},
				},
			},
			wantOK:  false,
			wantMsg: "Invalid uid for datasource Prometheus: bad uid (expected 1-40 characters of a-z, A-Z, 0-9, '-', '_')",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOK, gotMsg := tt.grafana.IsValid()

			if gotOK != tt.wantOK {
				t.Errorf("Grafana.IsValid() ok = %v, want %v", gotOK, tt.wantOK)
			}

			if !tt.wantOK && gotMsg != tt.wantMsg {
				t.Errorf("Grafana.IsValid() msg = %q, want %q", gotMsg, tt.wantMsg)
			}
		})
	}
}

func TestGrafanaAnnotation_IsValid(t *testing.T) {
	tests := []struct {
		name    string
		ann     GrafanaAnnotation
		wantOK  bool
		wantMsg string
	}{
		{
			name:   "minimal annotation",
			ann:    GrafanaAnnotation{Name: "Deploys", Tags: []string{"deploy"}},
			wantOK: true,
		},
		{
			name: "every field spelled out",
			ann: GrafanaAnnotation{
				Name:       "Deploys / Restarts / Crashes",
				IconColor:  "rgba(0, 211, 255, 1)",
				Datasource: "grafana",
				Tags:       []string{"deploy", "restart", "crash"},
				MatchAny:   true,
				Hide:       true,
				Limit:      50,
				Enable:     boolPtr(false),
			},
			wantOK: true,
		},
		{
			name: "comma-separated tags arrive with spaces viper does not trim",
			ann:  GrafanaAnnotation{Name: "Deploys", Tags: []string{"deploy", " restart", " crash"}},

			wantOK: true,
		},
		{
			name:    "empty name",
			ann:     GrafanaAnnotation{Tags: []string{"deploy"}},
			wantOK:  false,
			wantMsg: "Empty annotation name",
		},
		{
			name:    "name of the built-in entry is reserved",
			ann:     GrafanaAnnotation{Name: "Annotations & Alerts", Tags: []string{"deploy"}},
			wantOK:  false,
			wantMsg: "Annotation name Annotations & Alerts is reserved for the built-in entry",
		},
		{
			name:    "no tags",
			ann:     GrafanaAnnotation{Name: "Deploys"},
			wantOK:  false,
			wantMsg: "No tags for annotation Deploys",
		},
		{
			name:    "tags that are blank after trimming count as none",
			ann:     GrafanaAnnotation{Name: "Deploys", Tags: []string{"  ", ""}},
			wantOK:  false,
			wantMsg: "No tags for annotation Deploys",
		},
		{
			name:    "unsupported datasource",
			ann:     GrafanaAnnotation{Name: "Deploys", Datasource: "prometheus", Tags: []string{"deploy"}},
			wantOK:  false,
			wantMsg: "Invalid annotation datasource: prometheus for Deploys (supported: grafana)",
		},
		{
			name:    "negative limit",
			ann:     GrafanaAnnotation{Name: "Deploys", Tags: []string{"deploy"}, Limit: -1},
			wantOK:  false,
			wantMsg: "Negative limit for annotation Deploys: -1",
		},
		{
			name:    "control character in name would break the dashboard JSON",
			ann:     GrafanaAnnotation{Name: "Dep\nloys", Tags: []string{"deploy"}},
			wantOK:  false,
			wantMsg: "Control character in name of annotation Dep\nloys",
		},
		{
			name:    "control character in icon_color would break the dashboard JSON",
			ann:     GrafanaAnnotation{Name: "Deploys", IconColor: "yel\tlow", Tags: []string{"deploy"}},
			wantOK:  false,
			wantMsg: "Control character in icon_color of annotation Deploys",
		},
		{
			name:    "control character in a tag would break the dashboard JSON",
			ann:     GrafanaAnnotation{Name: "Deploys", Tags: []string{"dep\rloy"}},
			wantOK:  false,
			wantMsg: "Control character in tag of annotation Deploys",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOK, gotMsg := tt.ann.IsValid()

			if gotOK != tt.wantOK {
				t.Errorf("GrafanaAnnotation.IsValid() ok = %v, want %v (msg %q)", gotOK, tt.wantOK, gotMsg)
			}

			if !tt.wantOK && gotMsg != tt.wantMsg {
				t.Errorf("GrafanaAnnotation.IsValid() msg = %q, want %q", gotMsg, tt.wantMsg)
			}
		})
	}
}

// TestGrafanaAnnotation_Resolved covers the defaults applied between
// project.yaml and the dashboard: an omitted field must not render as an empty
// colour or a zero limit, both of which Grafana treats as "draw nothing".
func TestGrafanaAnnotation_Resolved(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		ann := GrafanaAnnotation{Name: "Deploys", Tags: []string{"deploy"}}

		if got := ann.ResolvedIconColor(); got != "yellow" {
			t.Errorf("ResolvedIconColor() = %q, want %q", got, "yellow")
		}

		if got := ann.ResolvedLimit(); got != 100 {
			t.Errorf("ResolvedLimit() = %d, want 100", got)
		}

		if !ann.ResolvedEnable() {
			t.Error("ResolvedEnable() = false, want true for an omitted enable")
		}
	})

	t.Run("explicit values win", func(t *testing.T) {
		ann := GrafanaAnnotation{
			Name:      "Deploys",
			IconColor: "red",
			Limit:     7,
			Enable:    boolPtr(false),
			Tags:      []string{"deploy"},
		}

		if got := ann.ResolvedIconColor(); got != "red" {
			t.Errorf("ResolvedIconColor() = %q, want %q", got, "red")
		}

		if got := ann.ResolvedLimit(); got != 7 {
			t.Errorf("ResolvedLimit() = %d, want 7", got)
		}

		if ann.ResolvedEnable() {
			t.Error("ResolvedEnable() = true, want false for an explicit enable: false")
		}
	})

	// Viper splits a comma-separated scalar on "," and leaves the spaces in
	// place; untrimmed, " restart" would match no event at all.
	t.Run("tags are trimmed and blanks dropped", func(t *testing.T) {
		ann := GrafanaAnnotation{Name: "Deploys", Tags: []string{"deploy", " restart", "", "  crash  ", "   "}}

		want := []string{"deploy", "restart", "crash"}
		got := ann.ResolvedTags()

		if len(got) != len(want) {
			t.Fatalf("ResolvedTags() = %q, want %q", got, want)
		}

		for i := range want {
			if got[i] != want[i] {
				t.Errorf("ResolvedTags()[%d] = %q, want %q", i, got[i], want[i])
			}
		}
	})
}

func TestGrafana_IsValid_Annotations(t *testing.T) {
	datasources := []GrafanaDatasource{
		{Name: "Prometheus", Type: "prometheus", URL: "http://prometheus:9090"},
	}

	tests := []struct {
		name    string
		grafana Grafana
		wantOK  bool
		wantMsg string
	}{
		{
			name: "annotations alongside a datasource",
			grafana: Grafana{
				Datasources: datasources,
				Annotations: []GrafanaAnnotation{
					{Name: "Deploys", Tags: []string{"deploy"}},
					{Name: "Migrations", Tags: []string{"migration"}},
				},
			},
			wantOK: true,
		},
		{
			name: "duplicate annotation name",
			grafana: Grafana{
				Datasources: datasources,
				Annotations: []GrafanaAnnotation{
					{Name: "Deploys", Tags: []string{"deploy"}},
					{Name: "Deploys", Tags: []string{"restart"}},
				},
			},
			wantOK:  false,
			wantMsg: "Duplicate annotation name: Deploys",
		},
		{
			name: "invalid annotation is reported from the annotation check",
			grafana: Grafana{
				Datasources: datasources,
				Annotations: []GrafanaAnnotation{{Name: "Deploys"}},
			},
			wantOK:  false,
			wantMsg: "No tags for annotation Deploys",
		},
		{
			// No datasource means no dashboard is generated, so the
			// annotations would be dropped without a word — the exact silent
			// failure this section exists to remove.
			name: "annotations without a datasource have nowhere to go",
			grafana: Grafana{
				Annotations: []GrafanaAnnotation{{Name: "Deploys", Tags: []string{"deploy"}}},
			},
			wantOK:  false,
			wantMsg: "Annotations declared without any datasource: no dashboard is generated to hold them",
		},
		{
			name:    "no datasources and no annotations stays valid",
			grafana: Grafana{},
			wantOK:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOK, gotMsg := tt.grafana.IsValid()

			if gotOK != tt.wantOK {
				t.Errorf("Grafana.IsValid() ok = %v, want %v (msg %q)", gotOK, tt.wantOK, gotMsg)
			}

			if !tt.wantOK && gotMsg != tt.wantMsg {
				t.Errorf("Grafana.IsValid() msg = %q, want %q", gotMsg, tt.wantMsg)
			}
		})
	}
}

func boolPtr(b bool) *bool { return &b }

func TestPackageUploadConfig_IsValid(t *testing.T) {
	tests := []struct {
		name    string
		config  PackageUploadConfig
		wantOK  bool
		wantMsg string
	}{
		{
			name:   "empty type is valid (disabled)",
			config: PackageUploadConfig{},
			wantOK: true,
		},
		{
			name:   "minio type",
			config: PackageUploadConfig{Type: PackageUploadMinio},
			wantOK: true,
		},
		{
			name:   "aws type",
			config: PackageUploadConfig{Type: PackageUploadAWS},
			wantOK: true,
		},
		{
			name:   "rsync type",
			config: PackageUploadConfig{Type: PackageUploadRsync},
			wantOK: true,
		},
		{
			name:    "invalid type",
			config:  PackageUploadConfig{Type: "invalid"},
			wantOK:  false,
			wantMsg: "packaging.upload.type must be 'minio', 'aws', or 'rsync'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOK, gotMsg := tt.config.IsValid()

			if gotOK != tt.wantOK {
				t.Errorf("PackageUploadConfig.IsValid() ok = %v, want %v", gotOK, tt.wantOK)
			}

			if !tt.wantOK && gotMsg != tt.wantMsg {
				t.Errorf("PackageUploadConfig.IsValid() msg = %q, want %q", gotMsg, tt.wantMsg)
			}
		})
	}
}

func TestPackageUploadConfig_IsEnabled(t *testing.T) {
	tests := []struct {
		name   string
		config PackageUploadConfig
		want   bool
	}{
		{
			name:   "empty type is disabled",
			config: PackageUploadConfig{},
			want:   false,
		},
		{
			name:   "minio type is enabled",
			config: PackageUploadConfig{Type: PackageUploadMinio},
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.IsEnabled(); got != tt.want {
				t.Errorf("PackageUploadConfig.IsEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPackagingConfig_IsValid(t *testing.T) {
	tests := []struct {
		name    string
		config  PackagingConfig
		wantOK  bool
		wantMsg string
	}{
		{
			name: "valid config",
			config: PackagingConfig{
				Maintainer:  "Test <test@example.com>",
				Description: "Test package",
			},
			wantOK: true,
		},
		{
			name: "valid config with upload",
			config: PackagingConfig{
				Maintainer:  "Test <test@example.com>",
				Description: "Test package",
				Upload:      PackageUploadConfig{Type: PackageUploadMinio},
			},
			wantOK: true,
		},
		{
			name: "empty maintainer",
			config: PackagingConfig{
				Maintainer:  "",
				Description: "Test package",
			},
			wantOK:  false,
			wantMsg: "packaging.maintainer is required",
		},
		{
			name: "empty description",
			config: PackagingConfig{
				Maintainer:  "Test <test@example.com>",
				Description: "",
			},
			wantOK:  false,
			wantMsg: "packaging.description is required",
		},
		{
			name: "invalid upload config",
			config: PackagingConfig{
				Maintainer:  "Test <test@example.com>",
				Description: "Test package",
				Upload:      PackageUploadConfig{Type: "invalid"},
			},
			wantOK:  false,
			wantMsg: "packaging.upload.type must be 'minio', 'aws', or 'rsync'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOK, gotMsg := tt.config.IsValid()

			if gotOK != tt.wantOK {
				t.Errorf("PackagingConfig.IsValid() ok = %v, want %v", gotOK, tt.wantOK)
			}

			if !tt.wantOK && gotMsg != tt.wantMsg {
				t.Errorf("PackagingConfig.IsValid() msg = %q, want %q", gotMsg, tt.wantMsg)
			}
		})
	}
}

func TestHasPackaging(t *testing.T) {
	tests := []struct {
		name      string
		artifacts []ArtifactType
		want      bool
	}{
		{
			name:      "empty artifacts",
			artifacts: nil,
			want:      false,
		},
		{
			name:      "docker only",
			artifacts: []ArtifactType{ArtifactDocker},
			want:      false,
		},
		{
			name:      "deb artifact",
			artifacts: []ArtifactType{ArtifactDeb},
			want:      true,
		},
		{
			name:      "rpm artifact",
			artifacts: []ArtifactType{ArtifactRPM},
			want:      true,
		},
		{
			name:      "apk artifact",
			artifacts: []ArtifactType{ArtifactAPK},
			want:      true,
		},
		{
			name:      "docker and deb",
			artifacts: []ArtifactType{ArtifactDocker, ArtifactDeb},
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasPackaging(tt.artifacts); got != tt.want {
				t.Errorf("HasPackaging() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateArtifacts(t *testing.T) {
	validPackaging := PackagingConfig{
		Maintainer:  "Test <test@example.com>",
		Description: "Test package",
	}

	tests := []struct {
		name      string
		artifacts []ArtifactType
		packaging PackagingConfig
		wantOK    bool
		wantMsg   string
	}{
		{
			name:      "empty artifacts",
			artifacts: nil,
			packaging: PackagingConfig{},
			wantOK:    true,
		},
		{
			name:      "docker only without packaging",
			artifacts: []ArtifactType{ArtifactDocker},
			packaging: PackagingConfig{},
			wantOK:    true,
		},
		{
			name:      "deb with valid packaging",
			artifacts: []ArtifactType{ArtifactDeb},
			packaging: validPackaging,
			wantOK:    true,
		},
		{
			name:      "invalid artifact type",
			artifacts: []ArtifactType{"invalid"},
			packaging: PackagingConfig{},
			wantOK:    false,
			wantMsg:   "Invalid artifact type: invalid",
		},
		{
			name:      "deb without packaging",
			artifacts: []ArtifactType{ArtifactDeb},
			packaging: PackagingConfig{},
			wantOK:    false,
			wantMsg:   "packaging.maintainer is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOK, gotMsg := ValidateArtifacts(tt.artifacts, tt.packaging)

			if gotOK != tt.wantOK {
				t.Errorf("ValidateArtifacts() ok = %v, want %v", gotOK, tt.wantOK)
			}

			if !tt.wantOK && gotMsg != tt.wantMsg {
				t.Errorf("ValidateArtifacts() msg = %q, want %q", gotMsg, tt.wantMsg)
			}
		})
	}
}

func TestRepository_IsValid(t *testing.T) {
	tests := []struct {
		name    string
		repo    Repository
		wantOK  bool
		wantMsg string
	}{
		{
			name: "valid repository",
			repo: Repository{
				Name:     "users",
				TypeDB:   "postgres",
				DriverDB: "pgx",
			},
			wantOK: true,
		},
		{
			name: "empty name",
			repo: Repository{
				Name:     "",
				TypeDB:   "postgres",
				DriverDB: "pgx",
			},
			wantOK:  false,
			wantMsg: "Empty name",
		},
		{
			name: "empty type",
			repo: Repository{
				Name:     "users",
				TypeDB:   "",
				DriverDB: "pgx",
			},
			wantOK:  false,
			wantMsg: "Empty type or driver",
		},
		{
			name: "empty driver",
			repo: Repository{
				Name:     "users",
				TypeDB:   "postgres",
				DriverDB: "",
			},
			wantOK:  false,
			wantMsg: "Empty type or driver",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOK, gotMsg := tt.repo.IsValid()

			if gotOK != tt.wantOK {
				t.Errorf("Repository.IsValid() ok = %v, want %v", gotOK, tt.wantOK)
			}

			if !tt.wantOK && gotMsg != tt.wantMsg {
				t.Errorf("Repository.IsValid() msg = %q, want %q", gotMsg, tt.wantMsg)
			}
		})
	}
}

func TestConsumer_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		consumer Consumer
		wantOK   bool
		wantMsg  string
	}{
		{
			name: "valid consumer",
			consumer: Consumer{
				Name:    "events",
				Path:    "./events.proto",
				Backend: "kafka",
				Group:   "my_group",
				Topic:   "events_topic",
			},
			wantOK: true,
		},
		{
			name: "empty name",
			consumer: Consumer{
				Name:    "",
				Path:    "./events.proto",
				Backend: "kafka",
				Group:   "my_group",
				Topic:   "events_topic",
			},
			wantOK:  false,
			wantMsg: "Empty name",
		},
		{
			name: "empty path",
			consumer: Consumer{
				Name:    "events",
				Path:    "",
				Backend: "kafka",
				Group:   "my_group",
				Topic:   "events_topic",
			},
			wantOK:  false,
			wantMsg: "Empty path, backend, group or topic",
		},
		{
			name: "empty backend",
			consumer: Consumer{
				Name:    "events",
				Path:    "./events.proto",
				Backend: "",
				Group:   "my_group",
				Topic:   "events_topic",
			},
			wantOK:  false,
			wantMsg: "Empty path, backend, group or topic",
		},
		{
			name: "empty group",
			consumer: Consumer{
				Name:    "events",
				Path:    "./events.proto",
				Backend: "kafka",
				Group:   "",
				Topic:   "events_topic",
			},
			wantOK:  false,
			wantMsg: "Empty path, backend, group or topic",
		},
		{
			name: "empty topic",
			consumer: Consumer{
				Name:    "events",
				Path:    "./events.proto",
				Backend: "kafka",
				Group:   "my_group",
				Topic:   "",
			},
			wantOK:  false,
			wantMsg: "Empty path, backend, group or topic",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOK, gotMsg := tt.consumer.IsValid()

			if gotOK != tt.wantOK {
				t.Errorf("Consumer.IsValid() ok = %v, want %v", gotOK, tt.wantOK)
			}

			if !tt.wantOK && gotMsg != tt.wantMsg {
				t.Errorf("Consumer.IsValid() msg = %q, want %q", gotMsg, tt.wantMsg)
			}
		})
	}
}

func TestRest_IsValid(t *testing.T) {
	tests := []struct {
		name    string
		rest    Rest
		wantOK  bool
		wantMsg string
	}{
		{
			name:   "valid static transport without path or params",
			rest:   Rest{Name: "static", Port: 8086, GeneratorType: "template", GeneratorTemplate: "static"},
			wantOK: true,
		},
		{
			name:   "valid static transport with route and dir params",
			rest:   Rest{Name: "static", Port: 8086, GeneratorType: "template", GeneratorTemplate: "static", GeneratorParams: map[string]string{"route": "/assets/", "dir": "assets"}},
			wantOK: true,
		},
		{
			name:    "static transport with disallowed param key",
			rest:    Rest{Name: "static", GeneratorType: "template", GeneratorTemplate: "static", GeneratorParams: map[string]string{"foo": "bar"}},
			wantOK:  false,
			wantMsg: "Generator params not supported",
		},
		{
			name:    "static transport with invalid route",
			rest:    Rest{Name: "static", GeneratorType: "template", GeneratorTemplate: "static", GeneratorParams: map[string]string{"route": "static"}},
			wantOK:  false,
			wantMsg: "Invalid route: must start with '/'",
		},
		{
			name:    "static transport with invalid dir",
			rest:    Rest{Name: "static", GeneratorType: "template", GeneratorTemplate: "static", GeneratorParams: map[string]string{"dir": "1bad"}},
			wantOK:  false,
			wantMsg: "Invalid dir: must be a valid lowercase directory name",
		},
		{
			name:    "non-static template with params is rejected",
			rest:    Rest{Name: "metrics", GeneratorType: "template", GeneratorTemplate: "sys", GeneratorParams: map[string]string{"route": "/x/"}},
			wantOK:  false,
			wantMsg: "Generator params not supported",
		},
		{
			name:   "template transport with empty path is ok",
			rest:   Rest{Name: "metrics", GeneratorType: "template", GeneratorTemplate: "sys"},
			wantOK: true,
		},
		{
			name:    "ogen transport with empty path is rejected",
			rest:    Rest{Name: "api", GeneratorType: "ogen"},
			wantOK:  false,
			wantMsg: "Empty path",
		},
		{
			name:    "ogen_client transport with empty path is rejected",
			rest:    Rest{Name: "client", GeneratorType: "ogen_client"},
			wantOK:  false,
			wantMsg: "Empty path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOK, gotMsg := tt.rest.IsValid("")
			if gotOK != tt.wantOK {
				t.Errorf("Rest.IsValid() ok = %v, want %v (msg=%q)", gotOK, tt.wantOK, gotMsg)
			}

			if !tt.wantOK && gotMsg != tt.wantMsg {
				t.Errorf("Rest.IsValid() msg = %q, want %q", gotMsg, tt.wantMsg)
			}
		})
	}
}

func TestValidateSecretName(t *testing.T) {
	const field = "git.private_repos_token_secret"

	tests := []struct {
		name   string
		secret string
		wantOK bool
	}{
		{
			name:   "empty means not configured, caller applies its default",
			secret: "",
			wantOK: true,
		},
		{
			name:   "upper snake case",
			secret: "GO_MODULES_TOKEN",
			wantOK: true,
		},
		{
			name:   "legacy default",
			secret: "GH_PAT",
			wantOK: true,
		},
		{
			name:   "leading underscore",
			secret: "_TOKEN",
			wantOK: true,
		},
		{
			name:   "digits after the first character",
			secret: "TOKEN_V2",
			wantOK: true,
		},
		{
			name:   "lowercase is accepted",
			secret: "go_modules_token",
			wantOK: true,
		},
		{
			name:   "leading digit is rejected",
			secret: "2TOKEN",
			wantOK: false,
		},
		{
			name:   "dash is rejected",
			secret: "GO-MODULES-TOKEN",
			wantOK: false,
		},
		{
			name:   "dot is rejected",
			secret: "secrets.TOKEN",
			wantOK: false,
		},
		{
			name:   "whitespace is rejected",
			secret: "GO MODULES TOKEN",
			wantOK: false,
		},
		{
			name:   "expression injection is rejected",
			secret: "TOKEN }} ${{ secrets.OTHER",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOK, gotMsg := validateSecretName(field, tt.secret)

			if gotOK != tt.wantOK {
				t.Errorf("validateSecretName(%q) ok = %v, want %v (msg=%q)", tt.secret, gotOK, tt.wantOK, gotMsg)
			}

			if !tt.wantOK && !strings.Contains(gotMsg, field) {
				t.Errorf("validateSecretName(%q) msg = %q, want it to name the field %q", tt.secret, gotMsg, field)
			}
		})
	}
}
