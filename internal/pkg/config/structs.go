package config

import (
	"path/filepath"
	"strings"

	"github.com/Educentr/go-project-starter/internal/pkg/ds"
	"github.com/Educentr/go-project-starter/internal/pkg/loggers"
	"github.com/Educentr/go-project-starter/internal/pkg/tools"
	"github.com/pkg/errors"
)

// ArtifactType represents a build artifact type
type ArtifactType string

// PackageUploadType represents package upload storage type
type PackageUploadType string

// PackageUploadConfig contains package upload configuration.
// Connection details (endpoint, bucket, credentials) are passed via CI/CD variables.
type PackageUploadConfig struct {
	Type PackageUploadType `mapstructure:"type"` // minio, aws, rsync
}

// PackagingConfig contains system package configuration for nfpm
type PackagingConfig struct {
	Maintainer  string              `mapstructure:"maintainer"`
	Description string              `mapstructure:"description"`
	Homepage    string              `mapstructure:"homepage"`
	License     string              `mapstructure:"license"`
	Vendor      string              `mapstructure:"vendor"`
	InstallDir  string              `mapstructure:"install_dir"`
	ConfigDir   string              `mapstructure:"config_dir"`
	Upload      PackageUploadConfig `mapstructure:"upload"`
}

// Kafka driver and type constants
const (
	KafkaTypeProducer    = "producer"
	KafkaTypeConsumer    = "consumer"
	KafkaDriverSegmentio = "segmentio"
	KafkaDriverCustom    = "custom"
)

// Artifact type constants
const (
	ArtifactDocker ArtifactType = "docker"
	ArtifactDeb    ArtifactType = "deb"
	ArtifactRPM    ArtifactType = "rpm"
	ArtifactAPK    ArtifactType = "apk"
)

// Package upload type constants
const (
	PackageUploadMinio PackageUploadType = "minio"
	PackageUploadAWS   PackageUploadType = "aws"
	PackageUploadRsync PackageUploadType = "rsync"
)

type (
	// Main contains the main project configuration settings.
	//
	// YAML example:
	//
	//	main:
	//	  name: myproject           # Project name (used in paths, Docker images)
	//	  registry_type: github     # github, digitalocean, aws, selfhosted
	//	  logger: zerolog           # Logger type (only zerolog supported)
	//	  author: "Your Name"       # Author for generated files
	//	  use_active_record: true   # Enable PostgreSQL ActiveRecord generation
	//	  dev_stand: true           # Generate docker-compose-dev.yaml with OnlineConf
	//	  skip_service_init: false  # Skip Service layer generation
	//
	// See docs/configuration/main.md for full documentation.
	Main struct {
		// Name is the project name, used in paths and Docker images. Required.
		Name string `mapstructure:"name"`
		// RegistryType specifies the container registry type: github, digitalocean, aws, or selfhosted.
		RegistryType string `mapstructure:"registry_type"`
		// Logger specifies the logger type. Currently only "zerolog" is supported.
		Logger string `mapstructure:"logger"`
		// Author is used in generated file headers.
		Author string `mapstructure:"author"`
		// SkipServiceInit disables Service layer generation.
		SkipServiceInit bool `mapstructure:"skip_service_init"`
		// UseActiveRecord enables PostgreSQL ActiveRecord code generation.
		UseActiveRecord bool `mapstructure:"use_active_record"`
		// DevStand enables docker-compose-dev.yaml generation with OnlineConf.
		DevStand  bool `mapstructure:"dev_stand"`
		LoggerObj ds.Logger
		TargetDir string
		ConfigDir string
	}

	// GrafanaDatasource represents a single Grafana datasource configuration
	GrafanaDatasource struct {
		Name      string `mapstructure:"name"`
		Type      string `mapstructure:"type"`   // prometheus, loki
		Access    string `mapstructure:"access"` // proxy, direct
		URL       string `mapstructure:"url"`
		IsDefault bool   `mapstructure:"isDefault"`
		Editable  bool   `mapstructure:"editable"`
	}

	// Grafana represents global Grafana configuration
	Grafana struct {
		Datasources []GrafanaDatasource `mapstructure:"datasources"`
	}

	// AppGrafana represents per-application Grafana settings
	AppGrafana struct {
		Datasources []string `mapstructure:"datasources"` // references by name
	}

	Scheduler struct {
		Enabled bool `mapstructure:"enabled"`
	}

	// Git contains Git repository configuration.
	//
	// YAML example:
	//
	//	git:
	//	  repo: https://github.com/org/repo
	//	  module_path: github.com/org/repo
	//	  private_repos: github.com/myorg/*
	//
	// See docs/configuration/main.md for full documentation.
	Git struct {
		// Repo is the Git repository URL. Required.
		Repo string `mapstructure:"repo"`
		// ModulePath is the Go module path. Required.
		ModulePath string `mapstructure:"module_path"`
		// PrivateRepos is a comma-separated list of private modules for GOPRIVATE.
		PrivateRepos string `mapstructure:"private_repos"`
	}

	// Tools contains version settings for tools used during generation and build.
	//
	// YAML example:
	//
	//	tools:
	//	  golang_version: "1.24"
	//	  ogen_version: "v0.78.0"
	//	  argen_version: "v1.0.0"
	//	  golangci_version: "1.55.2"
	//	  protobuf_version: "1.7.0"
	//	  go_jsonschema_version: "v0.16.0"
	//
	// See docs/configuration/main.md for full documentation.
	Tools struct {
		// ProtobufVersion is the protoc-gen-go version. Default: 1.7.0
		ProtobufVersion string `mapstructure:"protobuf_version"`
		// GolangVersion is the Go version. Default: 1.24
		GolangVersion string `mapstructure:"golang_version"`
		// OgenVersion is the ogen version. Default: v0.78.0
		OgenVersion string `mapstructure:"ogen_version"`
		// ArgenVersion is the argen (ActiveRecord) version. Default: v1.0.0
		ArgenVersion string `mapstructure:"argen_version"`
		// GolangciVersion is the golangci-lint version. Default: 1.55.2
		GolangciVersion string `mapstructure:"golangci_version"`
		// RuntimeVersion is the go-project-starter-runtime version. Auto-set.
		RuntimeVersion string `mapstructure:"runtime_version"`
		// GoJSONSchemaVersion is the go-jsonschema version. Default: v0.16.0
		GoJSONSchemaVersion string `mapstructure:"go_jsonschema_version"`
		// GoatVersion is the GOAT test framework version. Auto-set.
		GoatVersion string `mapstructure:"goat_version"`
		// GoatServicesVersion is the GOAT services version. Auto-set.
		GoatServicesVersion string `mapstructure:"goat_services_version"`
	}

	AuthParams struct {
		Transport string `mapstructure:"transport"`
		Type      string `mapstructure:"type"`
	}

	// Rest contains REST API transport configuration.
	//
	// YAML example:
	//
	//	rest:
	//	  - name: api
	//	    path: [./api/openapi.yaml]
	//	    generator_type: ogen       # ogen, template, ogen_client
	//	    port: 8080
	//	    version: v1
	//	    health_check_path: /health
	//
	//	  - name: system
	//	    generator_type: template
	//	    generator_template: sys
	//	    port: 9090
	//	    version: v1
	//
	//	  - name: external_api
	//	    generator_type: ogen_client
	//	    path: [./api/external.yaml]
	//	    instantiation: dynamic     # static or dynamic (ogen_client only)
	//	    auth_params:
	//	      transport: header
	//	      type: apikey
	//
	// See docs/configuration/transports.md for full documentation.
	Rest struct {
		// Name is the transport name. Required.
		Name string `mapstructure:"name"`
		// Path contains paths to OpenAPI specs. Required for ogen/ogen_client.
		Path []string `mapstructure:"path"`
		// APIPrefix is the URL prefix for the API.
		APIPrefix string `mapstructure:"api_prefix"`
		// Port is the HTTP port. Required (except for template sys).
		Port uint `mapstructure:"port"`
		// Version is the API version (v1, v2, etc). Required.
		Version string `mapstructure:"version"`
		// PublicService marks the service as public (no auth).
		PublicService bool `mapstructure:"public_service"`
		// GeneratorType is the generator type: ogen, template, or ogen_client. Required.
		GeneratorType string `mapstructure:"generator_type"`
		// HealthCheckPath is the path for health checks.
		HealthCheckPath string `mapstructure:"health_check_path"`
		// GeneratorTemplate is the template name (for generator_type: template).
		GeneratorTemplate string `mapstructure:"generator_template"`
		// GeneratorParams are additional generator parameters.
		GeneratorParams map[string]string `mapstructure:"generator_params"`
		// AuthParams are authentication parameters (for ogen_client).
		AuthParams AuthParams `mapstructure:"auth_params"`
		// EmptyConfigAvailable allows empty OnlineConf configuration.
		EmptyConfigAvailable bool `mapstructure:"empty_config_available"`
		// Instantiation mode: "static" (default) or "dynamic". Only for ogen_client.
		// Dynamic mode creates a new client instance for each request.
		Instantiation string `mapstructure:"instantiation"`
	}

	// Worker contains background worker configuration.
	//
	// YAML example:
	//
	//	worker:
	//	  - name: telegram_bot
	//	    generator_type: template
	//	    generator_template: telegram  # telegram or daemon
	//
	// See docs/configuration/workers.md for full documentation.
	Worker struct {
		// Name is the unique worker name. Required.
		Name string `mapstructure:"name"`
		// Path contains paths to specification files. Optional.
		Path []string `mapstructure:"path"`
		// Version is the worker version. Optional.
		Version string `mapstructure:"version"`
		// GeneratorType must be "template". Required.
		GeneratorType string `mapstructure:"generator_type"`
		// GeneratorTemplate is the template name: telegram or daemon. Required.
		GeneratorTemplate string `mapstructure:"generator_template"`
		// GeneratorParams contains additional generator parameters. Optional.
		GeneratorParams map[string]string `mapstructure:"generator_params"`
	}

	// CLI represents a command-line interface transport configuration.
	// CLI is a transport type like REST/GRPC, but requires interactive user communication.
	// It works like a shell: first word is command, rest are arguments.
	//
	// YAML example:
	//
	//	cli:
	//	  - name: admin
	//	    generator_type: template
	//	    generator_template: cli
	//
	// See docs/configuration/transports.md for full documentation.
	CLI struct {
		// Name is the unique CLI name. Required.
		Name string `mapstructure:"name"`
		// Path contains paths to CLI spec files. Optional.
		Path []string `mapstructure:"path"`
		// GeneratorType must be "template". Required.
		GeneratorType string `mapstructure:"generator_type"`
		// GeneratorTemplate is the CLI template name. Required.
		GeneratorTemplate string `mapstructure:"generator_template"`
		// GeneratorParams contains additional generator parameters. Optional.
		GeneratorParams map[string]string `mapstructure:"generator_params"`
	}

	// JSONSchemaItem represents a single JSON schema file with its identifier
	JSONSchemaItem struct {
		ID   string `mapstructure:"id"`   // Unique identifier for referencing from kafka topics
		Path string `mapstructure:"path"` // Path to JSON schema file
		Type string `mapstructure:"type"` // Generated Go type name (e.g. "AbonentUserSchemaJson")
	}

	// JSONSchema represents a JSON Schema configuration for generating Go structs.
	// Similar to OpenAPI/gRPC but generates only data structures with validation.
	//
	// YAML example:
	//
	//	jsonschema:
	//	  - name: models
	//	    schemas:
	//	      - id: user
	//	        path: ./schemas/user.json
	//	        type: UserSchema  # optional, auto-generated if empty
	//	    package: models       # optional, defaults to name
	//
	// See docs/reference/yaml-schema.md for full documentation.
	JSONSchema struct {
		// Name is the unique identifier for the schema set. Required.
		Name string `mapstructure:"name"`
		// Path contains paths to JSON schema files. Deprecated: use Schemas instead.
		Path []string `mapstructure:"path"`
		// Schemas contains individual schema files with IDs. Recommended.
		Schemas []JSONSchemaItem `mapstructure:"schemas"`
		// Package overrides the Go package name. Optional, defaults to Name.
		Package string `mapstructure:"package"`
	}

	// KafkaEvent represents a Kafka event with typed messages.
	// Event name is used for Go method generation and as default topic name.
	// Topic can be overridden per environment via OnlineConf.
	KafkaEvent struct {
		Name   string `mapstructure:"name"`   // Event name (used for method naming and as default topic name)
		Schema string `mapstructure:"schema"` // Optional: package.TypeName (e.g. "tb.AbonentUserSchemaJson"), empty for raw []byte
	}

	// Kafka represents Kafka producer/consumer configuration.
	//
	// YAML example:
	//
	//	kafka:
	//	  - name: events_producer
	//	    type: producer
	//	    driver: segmentio         # segmentio (default) or custom
	//	    client: main_kafka        # Client name for OnlineConf path
	//	    events:
	//	      - name: user_events
	//	        schema: models.user   # Optional: JSON Schema reference
	//
	//	  - name: order_consumer
	//	    type: consumer
	//	    driver: segmentio
	//	    client: main_kafka
	//	    group: my_group           # Required for consumers
	//	    events:
	//	      - name: order_events
	//
	// See docs/configuration/transports.md for full documentation.
	Kafka struct {
		// Name is the unique name for reference from applications. Required.
		Name string `mapstructure:"name"`
		// Type is "producer" or "consumer". Required.
		Type string `mapstructure:"type"`
		// Driver is the Kafka driver: segmentio (default) or custom.
		Driver string `mapstructure:"driver"`
		// DriverImport is the import path for custom driver.
		DriverImport string `mapstructure:"driver_import"`
		// DriverPackage is the package name for custom driver.
		DriverPackage string `mapstructure:"driver_package"`
		// DriverObj is the struct name for custom driver.
		DriverObj string `mapstructure:"driver_obj"`
		// Client is the client name used in OnlineConf paths. Required.
		Client string `mapstructure:"client"`
		// Group is the consumer group. Required for consumers.
		Group string `mapstructure:"group"`
		// Events is the list of events to publish/consume. Required.
		Events []KafkaEvent `mapstructure:"events"`
	}

	KafkaList []Kafka

	// Grpc contains gRPC service configuration.
	//
	// YAML example:
	//
	//	grpc:
	//	  - name: users
	//	    path: ./api/users.proto
	//	    port: 9000
	//	    generator_type: buf_client
	//
	// See docs/configuration/transports.md for full documentation.
	Grpc struct {
		// Name is the unique gRPC service name. Required.
		Name string `mapstructure:"name"`
		// Path is the path to .proto file. Required.
		Path string `mapstructure:"path"`
		// Short is a short name for package naming. Optional.
		Short string `mapstructure:"short"`
		// Port is the gRPC port. Required.
		Port uint `mapstructure:"port"`
		// GeneratorType is the generator type: buf_client. Required.
		GeneratorType string `mapstructure:"generator_type"`
		// BufLocalPlugins enables local buf plugins instead of remote. Optional.
		BufLocalPlugins bool `mapstructure:"buf_local_plugins"`
		// EmptyConfigAvailable allows empty OnlineConf configuration. Optional.
		EmptyConfigAvailable bool `mapstructure:"empty_config_available"`
	}

	Ws struct {
		Name string `mapstructure:"name"`
		Path string `mapstructure:"path"`
		Port uint   `mapstructure:"port"`
	}

	Repository struct {
		Name     string   `mapstructure:"name"`
		TypeDB   TypeDB   `mapstructure:"type_db"`
		DriverDB DriverDB `mapstructure:"driver_db"`
	}

	Consumer struct {
		Name    string  `mapstructure:"name"`
		Path    string  `mapstructure:"path"`
		Backend BufType `mapstructure:"backend"`
		Group   string  `mapstructure:"group"`
		Topic   string  `mapstructure:"topic"`
	}

	// Driver contains custom driver configuration for external integrations.
	//
	// YAML example:
	//
	//	driver:
	//	  - name: s3
	//	    import: github.com/myorg/drivers/s3
	//	    package: s3
	//	    obj_name: Client
	//
	// See docs/configuration/applications.md for full documentation.
	Driver struct {
		// Name is the unique driver name. Required.
		Name string `mapstructure:"name"`
		// Import is the Go import path. Required.
		Import string `mapstructure:"import"`
		// Package is the Go package name. Required.
		Package string `mapstructure:"package"`
		// ObjName is the driver struct name. Required.
		ObjName string `mapstructure:"obj_name"`
		// ServiceInjection is custom code to inject into Service. Optional.
		ServiceInjection string `mapstructure:"service_injection"`
	}

	RestList       []Rest
	GrpcList       []Grpc
	WsList         []Ws
	RepositoryList []Repository
	WorkerList     []Worker
	CLIList        []CLI
	JSONSchemaList []JSONSchema
	ConsumerList   []Consumer
	DriverList     []Driver

	AppDriver struct {
		Name   string   `mapstructure:"name"`
		Params []string `mapstructure:"params"`
	}

	DeployVolume struct {
		Path  string `mapstructure:"path"`
		Mount string `mapstructure:"mount"`
	}

	AppDeploy struct {
		Volumes []DeployVolume `mapstructure:"volumes"`
	}

	// GoatTestsConfig represents extended GOAT tests configuration
	GoatTestsConfig struct {
		Enabled    bool   `mapstructure:"enabled"`
		BinaryPath string `mapstructure:"binary_path"` // Path to test binary (default: /tmp/{app_name})
	}

	// AppTransportConfig holds per-application transport configuration overrides
	AppTransportConfig struct {
		Instantiation string `mapstructure:"instantiation"` // "static" or "dynamic" - overrides REST-level setting
	}

	// AppTransport references a transport with optional per-app config overrides
	AppTransport struct {
		Name   string             `mapstructure:"name"`
		Config AppTransportConfig `mapstructure:"config"`
	}

	// Application contains configuration for an atomic deployment unit.
	// Each application becomes a separate binary/container that can be scaled independently.
	//
	// YAML example:
	//
	//	applications:
	//	  - name: api
	//	    transport:
	//	      - name: api
	//	      - name: sys
	//	    driver: [postgres, redis]
	//	    goat_tests: true
	//
	//	  - name: workers
	//	    worker: [telegram_bot]
	//	    kafka: [order_events]
	//
	//	  - name: cli-app
	//	    cli: admin  # CLI is exclusive with transport/worker
	//
	// See docs/configuration/applications.md for full documentation.
	Application struct {
		// Name is the application name (= container name). Required.
		Name string `mapstructure:"name"`
		// TransportListRaw is raw YAML data (string[] or object[]).
		TransportListRaw interface{} `mapstructure:"transport"`
		// TransportList is the normalized transport list (populated after loading).
		TransportList []AppTransport `mapstructure:"-"`
		// HasDeprecatedFormat is true if old string[] format was used.
		HasDeprecatedFormat bool `mapstructure:"-"`
		// DriverList contains drivers for this application.
		DriverList []AppDriver `mapstructure:"driver"`
		// WorkerList contains worker names for this application.
		WorkerList []string `mapstructure:"worker"`
		// KafkaList contains kafka producer/consumer names.
		KafkaList []string `mapstructure:"kafka"`
		// CLI is the CLI transport name. Exclusive with transport/worker.
		CLI string `mapstructure:"cli"`
		// Deploy contains deployment settings (volumes).
		Deploy AppDeploy `mapstructure:"deploy"`
		// UseActiveRecord overrides the global use_active_record setting.
		UseActiveRecord *bool `mapstructure:"use_active_record"`
		// DependsOnDockerImages lists Docker images to pre-pull.
		DependsOnDockerImages []string `mapstructure:"depends_on_docker_images"`
		// UseEnvs enables environment variable usage.
		UseEnvs *bool `mapstructure:"use_envs"`
		// Grafana contains Grafana dashboard settings.
		Grafana AppGrafana `mapstructure:"grafana"`
		// GoatTests enables GOAT integration tests (simple flag).
		GoatTests *bool `mapstructure:"goat_tests"`
		// GoatTestsConfig contains extended GOAT tests configuration.
		GoatTestsConfig *GoatTestsConfig `mapstructure:"goat_tests_config"`
		// Artifacts overrides global artifacts for this application.
		Artifacts []ArtifactType `mapstructure:"artifacts"`
	}

	Docker struct {
		ImagePrefix string `mapstructure:"image_prefix"`
	}

	LogCollector struct {
		Type       string            `mapstructure:"type"`
		Parameters map[string]string `mapstructure:"parameters"`
	}

	Deploy struct {
		LogCollector LogCollector `mapstructure:"log_collector"`
	}

	Config struct {
		BasePath       string
		ConfigFilePath string          // Full path to the config file
		Main           Main            `mapstructure:"main"`
		Deploy         Deploy          `mapstructure:"deploy"`
		PostGenerate   []string        `mapstructure:"post_generate"`
		Git            Git             `mapstructure:"git"`
		Tools          Tools           `mapstructure:"tools"`
		RepositoryList RepositoryList  `mapstructure:"repository"`
		Scheduler      Scheduler       `mapstructure:"scheduler"`
		RestList       RestList        `mapstructure:"rest"`
		WorkerList     WorkerList      `mapstructure:"worker"`
		CLIList        CLIList         `mapstructure:"cli"`
		JSONSchemaList JSONSchemaList  `mapstructure:"jsonschema"`
		KafkaList      KafkaList       `mapstructure:"kafka"`
		GrpcList       GrpcList        `mapstructure:"grpc"`
		WsList         WsList          `mapstructure:"ws"`
		ConsumerList   ConsumerList    `mapstructure:"consumer"`
		DriverList     DriverList      `mapstructure:"driver"`
		Applications   []Application   `mapstructure:"applications"`
		Docker         Docker          `mapstructure:"docker"`
		Grafana        Grafana         `mapstructure:"grafana"`
		Artifacts      []ArtifactType  `mapstructure:"artifacts"`
		Packaging      PackagingConfig `mapstructure:"packaging"`

		RestMap              map[string]Rest
		GrpcMap              map[string]Grpc
		DriverMap            map[string]Driver
		WorkerMap            map[string]Worker
		CLIMap               map[string]CLI
		JSONSchemaMap        map[string]JSONSchema
		KafkaMap             map[string]Kafka
		GrafanaDatasourceMap map[string]GrafanaDatasource
	}
)

type (
	BufType  string
	TypeDB   string
	DriverDB string
)

const (
	defaultGolangVersion       = "1.24"
	defaultProtobufVersion     = "1.7.0"
	defaultGolangciVersion     = "1.55.2"
	defaultOgenVersion         = "v0.78.0"
	defaultArgenVersion        = "v1.0.0"
	defaultGoJSONSchemaVersion = "v0.16.0"

	errInstantiationOnlyOgenClient = "instantiation is only supported for ogen_client"

	// Generator type constants
	GeneratorTypeOgenClient = "ogen_client"

	// Instantiation mode constants
	InstantiationStatic  = "static"
	InstantiationDynamic = "dynamic"
)

var (
	ErrInvalidConfig = errors.New("invalid config")
)

func (c *Config) SetTargetDir(dir string)     { c.Main.TargetDir = dir }
func (c *Config) SetBaseConfigDir(dir string) { c.Main.ConfigDir = dir }

func (m Main) IsValid() (bool, string) {
	if len(m.Name) == 0 {
		return false, "Empty name"
	}

	_, ex := loggers.LoggerMapping[m.Logger]
	if !ex {
		return false, "invalid logger"
	}

	if len(m.RegistryType) == 0 {
		return false, "RegistryType not set " + m.RegistryType
	}

	validRegistryTypes := map[string]bool{
		"digitalocean": true,
		"github":       true,
		"aws":          true,
		"selfhosted":   true,
	}

	if !validRegistryTypes[m.RegistryType] {
		return false, "RegistryType value can be 'github', 'digitalocean', 'aws', or 'selfhosted', invalid RegistryType value " + m.RegistryType
	}

	return true, ""
}

func (g Git) IsValid() (bool, string) {
	if len(g.ModulePath) == 0 {
		return false, "Empty module path"
	}

	if len(g.Repo) == 0 {
		return false, "Empty repo"
	}

	return true, ""
}

func (r Rest) IsValid(baseConfigDir string) (bool, string) {
	if len(r.Name) == 0 {
		return false, "Empty name"
	}

	if r.Name == "sys" {
		return true, ""
	}

	if len(r.Path) == 0 {
		return false, "Empty path"
	}

	for _, p := range r.Path {
		absPath := filepath.Join(baseConfigDir, p)

		if tools.FileExists(absPath) != tools.ErrExist {
			return false, "Invalid path: " + p
		}
	}

	switch r.GeneratorType {
	case "ogen":
		if len(r.GeneratorTemplate) != 0 {
			return false, "Invalid generator template for type ogen"
		}

		if len(r.GeneratorParams) != 0 {
			for k := range r.GeneratorParams {
				switch k {
				case "auth_handler":
				default:
					return false, "Invalid generator params"
				}
			}
		}

		if r.Instantiation != "" {
			return false, errInstantiationOnlyOgenClient
		}
	case "template":
		if len(r.GeneratorTemplate) == 0 {
			return false, "Empty generator template"
		}

		if len(r.GeneratorParams) != 0 {
			return false, "Generator params not supported"
		}

		if r.Instantiation != "" {
			return false, errInstantiationOnlyOgenClient
		}
	case "ogen_client":
		if len(r.GeneratorTemplate) != 0 {
			return false, "Generator template not supported"
		}
		if len(r.GeneratorParams) != 0 {
			for k := range r.GeneratorParams {
				switch k {
				case "auth_type":
					return false, "don't use auth_type in generator params. User auth_params instead"
				default:
					return false, "Invalid generator params"
				}
			}
		}
		// Validate instantiation: only "static" or "dynamic" allowed
		if r.Instantiation != "" &&
			r.Instantiation != InstantiationStatic &&
			r.Instantiation != InstantiationDynamic {
			return false, "instantiation must be 'static' or 'dynamic'"
		}
	case "auth_params":
		if len(r.AuthParams.Transport) == 0 || len(r.AuthParams.Type) == 0 {
			return false, "Empty transport or type"
		}
		if r.AuthParams.Transport != "header" {
			return false, "Invalid transport"
		}
		if r.AuthParams.Type != "apikey" {
			return false, "Invalid type"
		}
	default:
		return false, "Invalid generator type"
	}

	return true, ""
}

func (w Worker) IsValid(_ string) (bool, string) {
	if len(w.Name) == 0 {
		return false, "Empty name"
	}

	switch w.GeneratorType {
	case "template":
		if len(w.GeneratorTemplate) == 0 {
			return false, "Empty generator template"
		}
		if len(w.GeneratorParams) != 0 {
			return false, "Generator params not supported"
		}
	default:
		return false, "Invalid generator type"
	}

	return true, ""
}

func (g Grpc) IsValid(baseConfigDir string) (bool, string) {
	if len(g.Name) == 0 {
		return false, "Empty name"
	}

	if len(g.Path) == 0 {
		return false, "Empty path"
	}

	absPath := filepath.Join(baseConfigDir, g.Path)

	if tools.FileExists(absPath) != tools.ErrExist {
		return false, "Invalid path: " + g.Path
	}

	switch g.GeneratorType {
	case "buf_client":
		// valid
	case "buf_server":
		return false, "buf_server not yet implemented"
	case "":
		return false, "generator_type is required for gRPC"
	default:
		return false, "invalid generator_type: " + g.GeneratorType
	}

	return true, ""
}

func (d Driver) IsValid() (bool, string) {
	if len(d.Name) == 0 {
		return false, "Empty name"
	}

	if len(d.Import) == 0 {
		return false, "Empty import"
	}

	if len(d.Package) == 0 {
		return false, "Empty package"
	}

	if len(d.ObjName) == 0 {
		return false, "Empty object name"
	}

	return true, ""
}

// ToDo WS
func (w Ws) IsValid(baseConfigDir string) (bool, string) {
	if len(w.Name) == 0 {
		return false, "Empty name"
	}

	if len(w.Path) == 0 {
		return false, "Empty path"
	}

	if tools.FileExists(filepath.Join(baseConfigDir, w.Path)) != tools.ErrExist {
		return false, "Invalid path: " + w.Path
	}

	return true, ""
}

// NormalizeTransports converts TransportListRaw to TransportList
// Supports both old format ([]string) and new format ([]AppTransport)
//
// Deprecated: The string array format for transports is deprecated and will be removed in v0.12.0.
// Use the new object format instead:
//
//	transport:
//	  - name: transport_name
//	    config:
//	      instantiation: dynamic
func (a *Application) NormalizeTransports() error {
	if a.TransportListRaw == nil {
		a.TransportList = nil

		return nil
	}

	rawList, ok := a.TransportListRaw.([]interface{})
	if !ok {
		return errors.New("transport must be an array")
	}

	a.TransportList = make([]AppTransport, 0, len(rawList))
	a.HasDeprecatedFormat = false

	for i, item := range rawList {
		switch v := item.(type) {
		case string:
			// Old format: just transport name
			// DEPRECATED: Will be removed in v0.12.0
			a.TransportList = append(a.TransportList, AppTransport{Name: v})
			a.HasDeprecatedFormat = true

		case map[string]interface{}:
			// New format: object with name and optional config
			name, ok := v["name"].(string)
			if !ok || name == "" {
				return errors.Errorf("transport[%d]: name is required", i)
			}

			appTransport := AppTransport{Name: name}

			// Parse config if present
			if configRaw, exists := v["config"]; exists {
				if configMap, ok := configRaw.(map[string]interface{}); ok {
					if inst, ok := configMap["instantiation"].(string); ok {
						appTransport.Config.Instantiation = inst
					}
				}
			}

			a.TransportList = append(a.TransportList, appTransport)

		default:
			return errors.Errorf("transport[%d]: invalid format, expected string or object", i)
		}
	}

	return nil
}

func (a Application) IsValid() (bool, string) {
	if len(a.Name) == 0 {
		return false, "Empty name"
	}

	// CLI apps are exclusive - cannot have transports or workers
	if a.CLI != "" {
		if len(a.TransportList) > 0 {
			return false, "CLI application cannot have transports"
		}

		if len(a.WorkerList) > 0 {
			return false, "CLI application cannot have workers"
		}

		return true, ""
	}

	// Non-CLI apps must have at least one transport
	if len(a.TransportList) == 0 {
		return false, "Application must have at least one transport or be a CLI app"
	}

	// Validate transport configs
	for _, t := range a.TransportList {
		if t.Name == "" {
			return false, "Transport name cannot be empty"
		}

		// Validate instantiation if specified
		if t.Config.Instantiation != "" &&
			t.Config.Instantiation != InstantiationStatic &&
			t.Config.Instantiation != InstantiationDynamic {
			return false, "transport " + t.Name + ": instantiation must be 'static' or 'dynamic'"
		}
	}

	return true, ""
}

// CLI validation
func (c CLI) IsValid() (bool, string) {
	if len(c.Name) == 0 {
		return false, "Empty name"
	}
	return true, ""
}

// JSONSchema validation
func (j JSONSchema) IsValid(baseConfigDir string) (bool, string) {
	if len(j.Name) == 0 {
		return false, "Empty name"
	}

	// Support both legacy Path[] and new Schemas[] format
	if len(j.Schemas) > 0 {
		for _, s := range j.Schemas {
			if s.ID == "" {
				return false, "Schema item missing id"
			}
			if s.Path == "" {
				return false, "Schema item missing path"
			}
			// Type is optional - will be auto-calculated from filename if empty
			absPath := filepath.Join(baseConfigDir, s.Path)
			if !errors.Is(tools.FileExists(absPath), tools.ErrExist) {
				return false, "Invalid path: " + s.Path
			}
		}
	} else {
		if len(j.Path) == 0 {
			return false, "Empty path"
		}

		for _, p := range j.Path {
			absPath := filepath.Join(baseConfigDir, p)

			if !errors.Is(tools.FileExists(absPath), tools.ErrExist) {
				return false, "Invalid path: " + p
			}
		}
	}

	return true, ""
}

// IsValid validates KafkaEvent configuration
func (e KafkaEvent) IsValid(jsonSchemaMap map[string]JSONSchema) (bool, string) {
	if len(e.Name) == 0 {
		return false, "Empty event name"
	}

	// Schema is optional - if empty, event uses raw []byte
	// If set, format should be "schemaset.schemaid"
	if e.Schema != "" && jsonSchemaMap != nil {
		parts := strings.SplitN(e.Schema, ".", 2)
		if len(parts) != 2 {
			return false, "Invalid schema format: expected 'schemaset.schemaid', got: " + e.Schema
		}

		schemaSetName := parts[0]
		schemaID := parts[1]

		schemaSet, exists := jsonSchemaMap[schemaSetName]
		if !exists {
			return false, "Unknown jsonschema reference: " + schemaSetName
		}

		// Find schema item by ID
		found := false
		for _, item := range schemaSet.Schemas {
			if item.ID == schemaID {
				found = true
				break
			}
		}

		if !found {
			return false, "Unknown schema id '" + schemaID + "' in schema set '" + schemaSetName + "'"
		}
	}

	return true, ""
}

// IsValid validates Kafka configuration
func (k Kafka) IsValid(jsonSchemaMap map[string]JSONSchema) (bool, string) {
	if len(k.Name) == 0 {
		return false, "Empty name"
	}

	if k.Type != KafkaTypeProducer && k.Type != KafkaTypeConsumer {
		return false, "Invalid type: must be 'producer' or 'consumer'"
	}

	// Validate driver
	driver := k.Driver
	if driver == "" {
		driver = KafkaDriverSegmentio // default
	}

	switch driver {
	case KafkaDriverSegmentio:
		// ok, will be auto-generated
	case KafkaDriverCustom:
		if k.DriverImport == "" || k.DriverPackage == "" || k.DriverObj == "" {
			return false, "Custom driver requires driver_import, driver_package, driver_obj"
		}
	default:
		return false, "Invalid driver: must be 'segmentio' or 'custom'"
	}

	if len(k.Client) == 0 {
		return false, "Empty client"
	}

	if k.Type == KafkaTypeConsumer && len(k.Group) == 0 {
		return false, "Consumer requires group"
	}

	if len(k.Events) == 0 {
		return false, "Empty events"
	}

	for _, event := range k.Events {
		if ok, msg := event.IsValid(jsonSchemaMap); !ok {
			return false, "event " + event.Name + ": " + msg
		}
	}

	return true, ""
}

// ToDo Repository
func (r Repository) IsValid() (bool, string) {
	if len(r.Name) == 0 {
		return false, "Empty name"
	}

	if len(r.TypeDB) == 0 || len(r.DriverDB) == 0 {
		return false, "Empty type or driver"
	}

	return true, ""
}

// ToDo Consumer
func (c Consumer) IsValid() (bool, string) {
	if len(c.Name) == 0 {
		return false, "Empty name"
	}

	if len(c.Path) == 0 || len(c.Backend) == 0 || len(c.Group) == 0 || len(c.Topic) == 0 {
		return false, "Empty path, backend, group or topic"
	}

	return true, ""
}

// IsValid validates Grafana datasource configuration
func (d GrafanaDatasource) IsValid() (bool, string) {
	if len(d.Name) == 0 {
		return false, "Empty datasource name"
	}

	if len(d.Type) == 0 {
		return false, "Empty datasource type for " + d.Name
	}

	switch d.Type {
	case "prometheus", "loki":
		// valid types
	default:
		return false, "Invalid datasource type: " + d.Type + " (supported: prometheus, loki)"
	}

	if d.Access != "" && d.Access != "proxy" && d.Access != "direct" {
		return false, "Invalid access mode: " + d.Access + " (supported: proxy, direct)"
	}

	if len(d.URL) == 0 {
		return false, "Empty URL for datasource " + d.Name
	}

	return true, ""
}

// IsValid validates Grafana configuration
func (g Grafana) IsValid() (bool, string) {
	if len(g.Datasources) == 0 {
		return true, "" // Empty grafana config is valid
	}

	seenNames := make(map[string]struct{})
	hasDefault := false

	for _, ds := range g.Datasources {
		if ok, msg := ds.IsValid(); !ok {
			return false, msg
		}

		if _, exists := seenNames[ds.Name]; exists {
			return false, "Duplicate datasource name: " + ds.Name
		}
		seenNames[ds.Name] = struct{}{}

		if ds.IsDefault {
			if hasDefault {
				return false, "Only one datasource can be default"
			}
			hasDefault = true
		}
	}

	return true, ""
}

// IsValid validates PackageUploadConfig
func (u PackageUploadConfig) IsValid() (bool, string) {
	if u.Type == "" {
		return true, "" // Empty type means upload is disabled
	}

	validTypes := map[PackageUploadType]bool{
		PackageUploadMinio: true,
		PackageUploadAWS:   true,
		PackageUploadRsync: true,
	}

	if !validTypes[u.Type] {
		return false, "packaging.upload.type must be 'minio', 'aws', or 'rsync'"
	}

	return true, ""
}

// IsEnabled returns true if upload is configured
func (u PackageUploadConfig) IsEnabled() bool {
	return u.Type != ""
}

// IsValid validates PackagingConfig
func (p PackagingConfig) IsValid() (bool, string) {
	if p.Maintainer == "" {
		return false, "packaging.maintainer is required"
	}

	if p.Description == "" {
		return false, "packaging.description is required"
	}

	// Validate upload config if enabled
	if ok, msg := p.Upload.IsValid(); !ok {
		return false, msg
	}

	return true, ""
}

// HasPackaging checks if any system package artifacts are configured
func HasPackaging(artifacts []ArtifactType) bool {
	for _, a := range artifacts {
		if a == ArtifactDeb || a == ArtifactRPM || a == ArtifactAPK {
			return true
		}
	}

	return false
}

// ValidateArtifacts validates artifacts configuration
func ValidateArtifacts(artifacts []ArtifactType, packaging PackagingConfig) (bool, string) {
	validTypes := map[ArtifactType]bool{
		ArtifactDocker: true,
		ArtifactDeb:    true,
		ArtifactRPM:    true,
		ArtifactAPK:    true,
	}

	for _, a := range artifacts {
		if !validTypes[a] {
			return false, "Invalid artifact type: " + string(a)
		}
	}

	// If system packages are configured, packaging section is required
	if HasPackaging(artifacts) {
		if ok, msg := packaging.IsValid(); !ok {
			return false, msg
		}
	}

	return true, ""
}
