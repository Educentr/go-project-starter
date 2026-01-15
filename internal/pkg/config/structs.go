package config

import (
	"path/filepath"
	"strings"

	"github.com/Educentr/go-project-starter/internal/pkg/ds"
	"github.com/Educentr/go-project-starter/internal/pkg/loggers"
	"github.com/Educentr/go-project-starter/internal/pkg/tools"
	"github.com/pkg/errors"
)

// Kafka driver and type constants
const (
	KafkaTypeProducer    = "producer"
	KafkaTypeConsumer    = "consumer"
	KafkaDriverSegmentio = "segmentio"
	KafkaDriverCustom    = "custom"
)

type (
	Main struct {
		Name            string `mapstructure:"name"`
		RegistryType    string `mapstructure:"registry_type"`
		Logger          string `mapstructure:"logger"`
		Author          string `mapstructure:"author"`
		SkipServiceInit bool   `mapstructure:"skip_service_init"`
		UseActiveRecord bool   `mapstructure:"use_active_record"`
		DevStand        bool   `mapstructure:"dev_stand"`
		LoggerObj       ds.Logger
		TargetDir       string
		ConfigDir       string
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

	Git struct {
		Repo         string `mapstructure:"repo"`
		ModulePath   string `mapstructure:"module_path"`
		PrivateRepos string `mapstructure:"private_repos"`
		// ProjectID  uint   `mapstructure:"project_id"` // Todo
	}

	Tools struct {
		ProtobufVersion     string `mapstructure:"protobuf_version"`
		GolangVersion       string `mapstructure:"golang_version"`
		OgenVersion         string `mapstructure:"ogen_version"`
		ArgenVersion        string `mapstructure:"argen_version"`
		GolangciVersion     string `mapstructure:"golangci_version"`
		RuntimeVersion      string `mapstructure:"runtime_version"`
		GoJSONSchemaVersion string `mapstructure:"go_jsonschema_version"`
		GoatVersion         string `mapstructure:"goat_version"`
		GoatServicesVersion string `mapstructure:"goat_services_version"`
	}

	AuthParams struct {
		Transport string `mapstructure:"transport"`
		Type      string `mapstructure:"type"`
	}

	Rest struct {
		Name                 string            `mapstructure:"name"`
		Path                 []string          `mapstructure:"path"`
		APIPrefix            string            `mapstructure:"api_prefix"`
		Port                 uint              `mapstructure:"port"`
		Version              string            `mapstructure:"version"`
		PublicService        bool              `mapstructure:"public_service"`
		GeneratorType        string            `mapstructure:"generator_type"`
		HealthCheckPath      string            `mapstructure:"health_check_path"`
		GeneratorTemplate    string            `mapstructure:"generator_template"`
		GeneratorParams      map[string]string `mapstructure:"generator_params"`
		AuthParams           AuthParams        `mapstructure:"auth_params"`
		EmptyConfigAvailable bool              `mapstructure:"empty_config_available"`
	}

	Worker struct {
		Name              string            `mapstructure:"name"`
		Path              []string          `mapstructure:"path"`
		Version           string            `mapstructure:"version"`
		GeneratorType     string            `mapstructure:"generator_type"`
		GeneratorTemplate string            `mapstructure:"generator_template"`
		GeneratorParams   map[string]string `mapstructure:"generator_params"`
	}

	// CLI represents a command-line interface transport configuration.
	// CLI is a transport type like REST/GRPC, but requires interactive user communication.
	// It works like a shell: first word is command, rest are arguments.
	CLI struct {
		Name              string            `mapstructure:"name"`
		Path              []string          `mapstructure:"path"`               // Path to CLI spec files (optional)
		GeneratorType     string            `mapstructure:"generator_type"`     // template
		GeneratorTemplate string            `mapstructure:"generator_template"` // cli template name
		GeneratorParams   map[string]string `mapstructure:"generator_params"`
	}

	// JSONSchemaItem represents a single JSON schema file with its identifier
	JSONSchemaItem struct {
		ID   string `mapstructure:"id"`   // Unique identifier for referencing from kafka topics
		Path string `mapstructure:"path"` // Path to JSON schema file
		Type string `mapstructure:"type"` // Generated Go type name (e.g. "AbonentUserSchemaJson")
	}

	// JSONSchema represents a JSON Schema configuration for generating Go structs.
	// Similar to OpenAPI/gRPC but generates only data structures with validation.
	JSONSchema struct {
		Name    string           `mapstructure:"name"`    // Unique identifier for the schema set
		Path    []string         `mapstructure:"path"`    // Legacy: Paths to JSON schema files (deprecated, use schemas)
		Schemas []JSONSchemaItem `mapstructure:"schemas"` // New: Individual schema files with IDs
		Package string           `mapstructure:"package"` // Optional: override package name (default: schema name)
	}

	// KafkaEvent represents a Kafka event with typed messages.
	// Event name is used for Go method generation and as default topic name.
	// Topic can be overridden per environment via OnlineConf.
	KafkaEvent struct {
		Name   string `mapstructure:"name"`   // Event name (used for method naming and as default topic name)
		Schema string `mapstructure:"schema"` // Optional: package.TypeName (e.g. "tb.AbonentUserSchemaJson"), empty for raw []byte
	}

	// Kafka represents Kafka producer/consumer configuration
	Kafka struct {
		Name          string        `mapstructure:"name"`           // Unique name for reference
		Type          string        `mapstructure:"type"`           // producer, consumer
		Driver        string        `mapstructure:"driver"`         // segmentio (default), custom
		DriverImport  string        `mapstructure:"driver_import"`  // For custom: import path
		DriverPackage string        `mapstructure:"driver_package"` // For custom: package name
		DriverObj     string        `mapstructure:"driver_obj"`     // For custom: struct name
		Client        string        `mapstructure:"client"`         // Client name for OC path
		Group         string        `mapstructure:"group"`          // Consumer group (for consumer type)
		Events        []KafkaEvent  `mapstructure:"events"`         // List of events to publish/consume
	}

	KafkaList []Kafka

	Grpc struct {
		Name                 string `mapstructure:"name"`
		Path                 string `mapstructure:"path"`
		Short                string `mapstructure:"short"`
		Port                 uint   `mapstructure:"port"`
		GeneratorType        string `mapstructure:"generator_type"`
		BufLocalPlugins      bool   `mapstructure:"buf_local_plugins"`
		EmptyConfigAvailable bool   `mapstructure:"empty_config_available"`
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

	Driver struct {
		Name             string `mapstructure:"name"`
		Import           string `mapstructure:"import"`
		Package          string `mapstructure:"package"`
		ObjName          string `mapstructure:"obj_name"`
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

	Application struct {
		Name                  string           `mapstructure:"name"`
		TransportList         []string         `mapstructure:"transport"`
		DriverList            []AppDriver      `mapstructure:"driver"`
		WorkerList            []string         `mapstructure:"worker"`
		KafkaList             []string         `mapstructure:"kafka"` // References to kafka producers/consumers by name
		CLI                   string           `mapstructure:"cli"`   // CLI app name (only one per application, exclusive with transport/worker)
		Deploy                AppDeploy        `mapstructure:"deploy"`
		UseActiveRecord       *bool            `mapstructure:"use_active_record"`
		DependsOnDockerImages []string         `mapstructure:"depends_on_docker_images"`
		UseEnvs               *bool            `mapstructure:"use_envs"`
		Grafana               AppGrafana       `mapstructure:"grafana"`
		GoatTests             *bool            `mapstructure:"goat_tests"`        // Enable GOAT integration tests generation (simple flag)
		GoatTestsConfig       *GoatTestsConfig `mapstructure:"goat_tests_config"` // Extended GOAT tests configuration
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
		ConfigFilePath string         // Full path to the config file
		Main           Main           `mapstructure:"main"`
		Deploy         Deploy         `mapstructure:"deploy"`
		PostGenerate   []string       `mapstructure:"post_generate"`
		Git            Git            `mapstructure:"git"`
		Tools          Tools          `mapstructure:"tools"`
		RepositoryList RepositoryList `mapstructure:"repository"`
		Scheduler      Scheduler      `mapstructure:"scheduler"`
		RestList       RestList       `mapstructure:"rest"`
		WorkerList     WorkerList     `mapstructure:"worker"`
		CLIList        CLIList        `mapstructure:"cli"`
		JSONSchemaList JSONSchemaList `mapstructure:"jsonschema"`
		KafkaList      KafkaList      `mapstructure:"kafka"`
		GrpcList       GrpcList       `mapstructure:"grpc"`
		WsList         WsList         `mapstructure:"ws"`
		ConsumerList   ConsumerList   `mapstructure:"consumer"`
		DriverList     DriverList     `mapstructure:"driver"`
		Applications   []Application  `mapstructure:"applications"`
		Docker         Docker         `mapstructure:"docker"`
		Grafana        Grafana        `mapstructure:"grafana"`

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
	defaultGolangVersion       = "1.20"
	defaultProtobufVersion     = "1.7.0"
	defaultGolangciVersion     = "1.55.2"
	defaultOgenVersion         = "v0.78.0"
	defaultArgenVersion        = "v1.0.0"
	defaultGoJSONSchemaVersion = "v0.16.0"
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
	case "template":
		if len(r.GeneratorTemplate) == 0 {
			return false, "Empty generator template"
		}
		if len(r.GeneratorParams) != 0 {
			return false, "Generator params not supported"
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
