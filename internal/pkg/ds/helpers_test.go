package ds

import (
	"testing"
)

func TestTransports_Add(t *testing.T) {
	tests := []struct {
		name        string
		existing    Transports
		addName     string
		addTransp   Transport
		wantError   bool
		wantErrText string
	}{
		{
			name:      "add to empty map",
			existing:  make(Transports),
			addName:   "api",
			addTransp: Transport{Name: "api"},
			wantError: false,
		},
		{
			name: "add new transport",
			existing: Transports{
				"existing": Transport{Name: "existing"},
			},
			addName:   "new",
			addTransp: Transport{Name: "new"},
			wantError: false,
		},
		{
			name: "add duplicate returns error",
			existing: Transports{
				"api": Transport{Name: "api"},
			},
			addName:     "api",
			addTransp:   Transport{Name: "api"},
			wantError:   true,
			wantErrText: "transport api already exists",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.existing.Add(tt.addName, tt.addTransp)

			if tt.wantError {
				if err == nil {
					t.Errorf("Transports.Add() error = nil, want error")

					return
				}

				if err.Error() != tt.wantErrText {
					t.Errorf("Transports.Add() error = %q, want %q", err.Error(), tt.wantErrText)
				}

				return
			}

			if err != nil {
				t.Errorf("Transports.Add() error = %v, want nil", err)
				return
			}

			if _, exists := tt.existing[tt.addName]; !exists {
				t.Errorf("Transports.Add() did not add transport %q", tt.addName)
			}
		})
	}
}

func TestWorkers_Add(t *testing.T) {
	tests := []struct {
		name        string
		existing    Workers
		addName     string
		addWorker   Worker
		wantError   bool
		wantErrText string
	}{
		{
			name:      "add to empty map",
			existing:  make(Workers),
			addName:   "telegram",
			addWorker: Worker{Name: "telegram"},
			wantError: false,
		},
		{
			name: "add new worker",
			existing: Workers{
				"existing": Worker{Name: "existing"},
			},
			addName:   "new",
			addWorker: Worker{Name: "new"},
			wantError: false,
		},
		{
			name: "add duplicate returns error",
			existing: Workers{
				"telegram": Worker{Name: "telegram"},
			},
			addName:     "telegram",
			addWorker:   Worker{Name: "telegram"},
			wantError:   true,
			wantErrText: "worker telegram already exists",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.existing.Add(tt.addName, tt.addWorker)

			if tt.wantError {
				if err == nil {
					t.Errorf("Workers.Add() error = nil, want error")

					return
				}

				if err.Error() != tt.wantErrText {
					t.Errorf("Workers.Add() error = %q, want %q", err.Error(), tt.wantErrText)
				}

				return
			}

			if err != nil {
				t.Errorf("Workers.Add() error = %v, want nil", err)
				return
			}

			if _, exists := tt.existing[tt.addName]; !exists {
				t.Errorf("Workers.Add() did not add worker %q", tt.addName)
			}
		})
	}
}

func TestTransport_IsDynamic(t *testing.T) {
	tests := []struct {
		name      string
		transport Transport
		want      bool
	}{
		{
			name:      "empty instantiation is static",
			transport: Transport{},
			want:      false,
		},
		{
			name:      "static instantiation",
			transport: Transport{Instantiation: "static"},
			want:      false,
		},
		{
			name:      "dynamic instantiation",
			transport: Transport{Instantiation: "dynamic"},
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.transport.IsDynamic(); got != tt.want {
				t.Errorf("Transport.IsDynamic() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTransport_HasAuthParams(t *testing.T) {
	tests := []struct {
		name      string
		transport Transport
		want      bool
	}{
		{
			name:      "empty auth params",
			transport: Transport{},
			want:      false,
		},
		{
			name:      "empty auth type",
			transport: Transport{AuthParams: AuthParams{Transport: "header"}},
			want:      false,
		},
		{
			name:      "has auth params",
			transport: Transport{AuthParams: AuthParams{Transport: "header", Type: "apikey"}},
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.transport.HasAuthParams(); got != tt.want {
				t.Errorf("Transport.HasAuthParams() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApp_GetRestTransport(t *testing.T) {
	app := App{
		Transports: Transports{
			"api":          Transport{Name: "api", Type: RestTransportType},
			"sys":          Transport{Name: "sys", Type: RestTransportType},
			"grpc_service": Transport{Name: "grpc_service", Type: GrpcTransportType},
		},
	}

	got := app.GetRestTransport()

	if len(got) != 2 {
		t.Errorf("App.GetRestTransport() returned %d transports, want 2", len(got))
	}

	// Results should be sorted by name
	if len(got) >= 2 && got[0].Name != "api" {
		t.Errorf("App.GetRestTransport() not sorted, got %q first", got[0].Name)
	}
}

func TestApp_GetGrpcTransport(t *testing.T) {
	app := App{
		Transports: Transports{
			"api":    Transport{Name: "api", Type: RestTransportType},
			"users":  Transport{Name: "users", Type: GrpcTransportType},
			"orders": Transport{Name: "orders", Type: GrpcTransportType},
		},
	}

	got := app.GetGrpcTransport()

	if len(got) != 2 {
		t.Errorf("App.GetGrpcTransport() returned %d transports, want 2", len(got))
	}
}

func TestApp_HasOgenClients(t *testing.T) {
	tests := []struct {
		name string
		app  App
		want bool
	}{
		{
			name: "no transports",
			app:  App{},
			want: false,
		},
		{
			name: "no ogen_client transports",
			app: App{
				Transports: Transports{
					"api": Transport{Name: "api", GeneratorType: "ogen"},
				},
			},
			want: false,
		},
		{
			name: "has ogen_client transport",
			app: App{
				Transports: Transports{
					"external": Transport{Name: "external", GeneratorType: "ogen_client"},
				},
			},
			want: true,
		},
		{
			name: "mixed transports with ogen_client",
			app: App{
				Transports: Transports{
					"api":      Transport{Name: "api", GeneratorType: "ogen"},
					"external": Transport{Name: "external", GeneratorType: "ogen_client"},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.app.HasOgenClients(); got != tt.want {
				t.Errorf("App.HasOgenClients() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApp_GetOgenClients(t *testing.T) {
	app := App{
		Transports: Transports{
			"api":       Transport{Name: "api", GeneratorType: "ogen"},
			"external1": Transport{Name: "external1", GeneratorType: "ogen_client"},
			"external2": Transport{Name: "external2", GeneratorType: "ogen_client"},
		},
	}

	got := app.GetOgenClients()

	if len(got) != 2 {
		t.Errorf("App.GetOgenClients() returned %d clients, want 2", len(got))
	}

	// Results should be sorted by name
	if len(got) >= 2 && got[0].Name != "external1" {
		t.Errorf("App.GetOgenClients() not sorted, got %q first", got[0].Name)
	}
}

func TestApps_IsTransportOptional(t *testing.T) {
	apps := Apps{
		{
			Name: "app1",
			Transports: Transports{
				"api":      Transport{Name: "api", Optional: false},
				"external": Transport{Name: "external", Optional: true},
			},
		},
		{
			Name: "app2",
			Transports: Transports{
				"api": Transport{Name: "api", Optional: false},
			},
		},
	}

	tests := []struct {
		name   string
		transp string
		want   bool
	}{
		{
			name:   "transport is optional in at least one app",
			transp: "external",
			want:   true,
		},
		{
			name:   "transport is not optional",
			transp: "api",
			want:   false,
		},
		{
			name:   "transport does not exist",
			transp: "unknown",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := apps.IsTransportOptional(tt.transp); got != tt.want {
				t.Errorf("Apps.IsTransportOptional(%q) = %v, want %v", tt.transp, got, tt.want)
			}
		})
	}
}

func TestApps_IsDriverOptional(t *testing.T) {
	apps := Apps{
		{
			Name: "app1",
			Drivers: Drivers{
				"postgres": Driver{Name: "postgres", Optional: false},
				"redis":    Driver{Name: "redis", Optional: true},
			},
		},
		{
			Name: "app2",
			Drivers: Drivers{
				"postgres": Driver{Name: "postgres", Optional: false},
			},
		},
	}

	tests := []struct {
		name   string
		driver string
		want   bool
	}{
		{
			name:   "driver is optional in at least one app",
			driver: "redis",
			want:   true,
		},
		{
			name:   "driver is not optional",
			driver: "postgres",
			want:   false,
		},
		{
			name:   "driver does not exist",
			driver: "unknown",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := apps.IsDriverOptional(tt.driver); got != tt.want {
				t.Errorf("Apps.IsDriverOptional(%q) = %v, want %v", tt.driver, got, tt.want)
			}
		})
	}
}

func TestApps_IsKafkaOptional(t *testing.T) {
	apps := Apps{
		{
			Name: "app1",
			Kafka: KafkaConfigs{
				"events":   KafkaConfig{Name: "events", Optional: false},
				"optional": KafkaConfig{Name: "optional", Optional: true},
			},
		},
	}

	tests := []struct {
		name  string
		kafka string
		want  bool
	}{
		{
			name:  "kafka is optional",
			kafka: "optional",
			want:  true,
		},
		{
			name:  "kafka is not optional",
			kafka: "events",
			want:  false,
		},
		{
			name:  "kafka does not exist",
			kafka: "unknown",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := apps.IsKafkaOptional(tt.kafka); got != tt.want {
				t.Errorf("Apps.IsKafkaOptional(%q) = %v, want %v", tt.kafka, got, tt.want)
			}
		})
	}
}

func TestApps_HasActiveRecord(t *testing.T) {
	tests := []struct {
		name string
		apps Apps
		want bool
	}{
		{
			name: "no apps",
			apps: Apps{},
			want: false,
		},
		{
			name: "no active record",
			apps: Apps{
				{Name: "app1", UseActiveRecord: false},
				{Name: "app2", UseActiveRecord: false},
			},
			want: false,
		},
		{
			name: "one app with active record",
			apps: Apps{
				{Name: "app1", UseActiveRecord: false},
				{Name: "app2", UseActiveRecord: true},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.apps.HasActiveRecord(); got != tt.want {
				t.Errorf("Apps.HasActiveRecord() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApps_HasGoatTests(t *testing.T) {
	tests := []struct {
		name string
		apps Apps
		want bool
	}{
		{
			name: "no apps",
			apps: Apps{},
			want: false,
		},
		{
			name: "no goat tests",
			apps: Apps{
				{Name: "app1", GoatTests: false},
			},
			want: false,
		},
		{
			name: "has goat tests",
			apps: Apps{
				{Name: "app1", GoatTests: true},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.apps.HasGoatTests(); got != tt.want {
				t.Errorf("Apps.HasGoatTests() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApp_IsCLI(t *testing.T) {
	tests := []struct {
		name string
		app  App
		want bool
	}{
		{
			name: "not CLI app",
			app:  App{Name: "api"},
			want: false,
		},
		{
			name: "CLI app",
			app:  App{Name: "cli", CLI: &CLIApp{Name: "admin"}},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.app.IsCLI(); got != tt.want {
				t.Errorf("App.IsCLI() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApp_GetCLITransport(t *testing.T) {
	tests := []struct {
		name string
		app  App
		want *CLIApp
	}{
		{
			name: "not CLI app",
			app:  App{Name: "api"},
			want: nil,
		},
		{
			name: "CLI app",
			app:  App{Name: "cli", CLI: &CLIApp{Name: "admin"}},
			want: &CLIApp{Name: "admin"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.app.GetCLITransport()

			if tt.want == nil && got != nil {
				t.Errorf("App.GetCLITransport() = %v, want nil", got)
			}

			if tt.want != nil && (got == nil || got.Name != tt.want.Name) {
				t.Errorf("App.GetCLITransport() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApp_HasSysTransport(t *testing.T) {
	tests := []struct {
		name string
		app  App
		want bool
	}{
		{
			name: "no transports",
			app:  App{},
			want: false,
		},
		{
			name: "no sys transport",
			app: App{
				Transports: Transports{
					"api": Transport{Name: "api", GeneratorType: "ogen"},
				},
			},
			want: false,
		},
		{
			name: "has sys transport",
			app: App{
				Transports: Transports{
					"sys": Transport{Name: "sys", GeneratorType: "template", GeneratorTemplate: "sys"},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.app.HasSysTransport(); got != tt.want {
				t.Errorf("App.HasSysTransport() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Note: Kafka-related tests (TestKafkaConfig_*, TestApp_*KafkaProducers) are in kafka_test.go

func TestArtifactsConfig_HasDocker(t *testing.T) {
	tests := []struct {
		name   string
		config ArtifactsConfig
		want   bool
	}{
		{
			name:   "empty",
			config: ArtifactsConfig{},
			want:   false,
		},
		{
			name:   "has docker",
			config: ArtifactsConfig{Types: []ArtifactType{ArtifactDocker}},
			want:   true,
		},
		{
			name:   "no docker",
			config: ArtifactsConfig{Types: []ArtifactType{ArtifactDeb}},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.HasDocker(); got != tt.want {
				t.Errorf("ArtifactsConfig.HasDocker() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestArtifactsConfig_HasPackaging(t *testing.T) {
	tests := []struct {
		name   string
		config ArtifactsConfig
		want   bool
	}{
		{
			name:   "empty",
			config: ArtifactsConfig{},
			want:   false,
		},
		{
			name:   "docker only",
			config: ArtifactsConfig{Types: []ArtifactType{ArtifactDocker}},
			want:   false,
		},
		{
			name:   "has deb",
			config: ArtifactsConfig{Types: []ArtifactType{ArtifactDeb}},
			want:   true,
		},
		{
			name:   "has rpm",
			config: ArtifactsConfig{Types: []ArtifactType{ArtifactRPM}},
			want:   true,
		},
		{
			name:   "has apk",
			config: ArtifactsConfig{Types: []ArtifactType{ArtifactAPK}},
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.HasPackaging(); got != tt.want {
				t.Errorf("ArtifactsConfig.HasPackaging() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestArtifactsConfig_Upload(t *testing.T) {
	tests := []struct {
		name   string
		config ArtifactsConfig
		method string
		want   bool
	}{
		{
			name: "has minio upload with packaging",
			config: ArtifactsConfig{
				Types:     []ArtifactType{ArtifactDeb},
				Packaging: PackagingConfig{Upload: PackageUploadConfig{Type: PackageUploadMinio}},
			},
			method: "IsMinio",
			want:   true,
		},
		{
			name: "has aws upload with packaging",
			config: ArtifactsConfig{
				Types:     []ArtifactType{ArtifactRPM},
				Packaging: PackagingConfig{Upload: PackageUploadConfig{Type: PackageUploadAWS}},
			},
			method: "IsAWS",
			want:   true,
		},
		{
			name: "has rsync upload with packaging",
			config: ArtifactsConfig{
				Types:     []ArtifactType{ArtifactAPK},
				Packaging: PackagingConfig{Upload: PackageUploadConfig{Type: PackageUploadRsync}},
			},
			method: "IsRsync",
			want:   true,
		},
		{
			name: "upload without packaging returns false",
			config: ArtifactsConfig{
				Types:     []ArtifactType{ArtifactDocker},
				Packaging: PackagingConfig{Upload: PackageUploadConfig{Type: PackageUploadMinio}},
			},
			method: "IsMinio",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got bool

			switch tt.method {
			case "IsMinio":
				got = tt.config.IsMinio()
			case "IsAWS":
				got = tt.config.IsAWS()
			case "IsRsync":
				got = tt.config.IsRsync()
			}

			if got != tt.want {
				t.Errorf("ArtifactsConfig.%s() = %v, want %v", tt.method, got, tt.want)
			}
		})
	}
}

func TestArtifactsConfig_IsS3Compatible(t *testing.T) {
	tests := []struct {
		name   string
		config ArtifactsConfig
		want   bool
	}{
		{
			name:   "empty",
			config: ArtifactsConfig{},
			want:   false,
		},
		{
			name: "minio is s3 compatible",
			config: ArtifactsConfig{
				Types:     []ArtifactType{ArtifactDeb},
				Packaging: PackagingConfig{Upload: PackageUploadConfig{Type: PackageUploadMinio}},
			},
			want: true,
		},
		{
			name: "aws is s3 compatible",
			config: ArtifactsConfig{
				Types:     []ArtifactType{ArtifactDeb},
				Packaging: PackagingConfig{Upload: PackageUploadConfig{Type: PackageUploadAWS}},
			},
			want: true,
		},
		{
			name: "rsync is not s3 compatible",
			config: ArtifactsConfig{
				Types:     []ArtifactType{ArtifactDeb},
				Packaging: PackagingConfig{Upload: PackageUploadConfig{Type: PackageUploadRsync}},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.IsS3Compatible(); got != tt.want {
				t.Errorf("ArtifactsConfig.IsS3Compatible() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTransports_GetUniqueTypes(t *testing.T) {
	transports := Transports{
		"api1": Transport{Name: "api1", Type: RestTransportType, GeneratorType: "ogen"},
		"api2": Transport{Name: "api2", Type: RestTransportType, GeneratorType: "ogen"},
		"sys":  Transport{Name: "sys", Type: RestTransportType, GeneratorType: "template"},
		"grpc": Transport{Name: "grpc", Type: GrpcTransportType, GeneratorType: "buf_client"},
	}

	got := transports.GetUniqueTypes()

	if _, exists := got[RestTransportType]; !exists {
		t.Error("GetUniqueTypes() should have RestTransportType")
	}

	if _, exists := got[GrpcTransportType]; !exists {
		t.Error("GetUniqueTypes() should have GrpcTransportType")
	}

	// Check that ogen generators are grouped
	if len(got[RestTransportType]["ogen"]) != 2 {
		t.Errorf("GetUniqueTypes() should have 2 ogen transports, got %d", len(got[RestTransportType]["ogen"]))
	}
}

func TestWorkers_GetUniqueTypes(t *testing.T) {
	workers := Workers{
		"telegram": Worker{Name: "telegram", GeneratorType: "template"},
		"daemon1":  Worker{Name: "daemon1", GeneratorType: "template"},
	}

	got := workers.GetUniqueTypes()

	if _, exists := got["template"]; !exists {
		t.Error("GetUniqueTypes() should have template type")
	}

	if len(got["template"]) != 2 {
		t.Errorf("GetUniqueTypes() should have 2 template workers, got %d", len(got["template"]))
	}
}
