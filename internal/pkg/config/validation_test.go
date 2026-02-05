package config

import (
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
