package ds

import (
	"bytes"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Educentr/go-project-starter/internal/pkg/grafana"
)

type TransportType string

type LogCollectorType struct {
	Enabled    bool
	Type       string
	Parameters map[string]string
}

type DeployType struct {
	LogCollector LogCollectorType
}

//type WorkerType string

// JSONSchemaItem represents a single JSON schema file with its identifier
type JSONSchemaItem struct {
	ID   string // Unique identifier for referencing from kafka topics
	Path string // Path to JSON schema file (absolute)
	Type string // Generated Go type name (e.g. "AbonentUserSchemaJson")
}

// JSONSchema represents a JSON Schema configuration for code generation.
type JSONSchema struct {
	Name    string           // Unique identifier for the schema set
	Package string           // Package name for generated code
	Path    []string         // Legacy: Paths to JSON schema files (absolute)
	Schemas []JSONSchemaItem // New: Individual schema files with IDs
}

// JSONSchemas is a map of JSONSchema by name
type JSONSchemas map[string]JSONSchema

// KafkaTopic represents a Kafka topic with typed messages
type KafkaTopic struct {
	ID     string // Topic ID for OnlineConf path and method naming
	Name   string // Default topic name (can be overridden via OnlineConf)
	Schema string // Optional: package.TypeName (e.g. "tb.AbonentUserSchemaJson"), empty for raw []byte
	// Computed at generation time
	GoType   string // Full Go type (pkg.MessageType) or empty for []byte
	GoImport string // Import path for the message type or empty for []byte
}

// Kafka driver and type constants
const (
	KafkaTypeProducer    = "producer"
	KafkaTypeConsumer    = "consumer"
	KafkaDriverCustom    = "custom"
	KafkaObjNameProducer = "Producer"
)

// KafkaConfig represents Kafka producer/consumer configuration
//
//nolint:decorder // follows existing pattern - types after consts
type KafkaConfig struct {
	Name          string       // Unique name for reference
	Type          string       // producer, consumer
	Driver        string       // segmentio, custom
	DriverImport  string       // For custom driver: import path
	DriverPackage string       // For custom driver: package name
	DriverObj     string       // For custom driver: struct name
	ClientName    string       // Client name for OC path
	Group         string       // Consumer group (for consumer type)
	Topics        []KafkaTopic // Topics configuration
}

// KafkaConfigs is a map of KafkaConfig by name
//
//nolint:decorder // follows existing pattern
type KafkaConfigs map[string]KafkaConfig

// IsCustomDriver returns true if using custom driver
func (k KafkaConfig) IsCustomDriver() bool {
	return k.Driver == KafkaDriverCustom
}

// GetImport returns import path (generated or custom)
func (k KafkaConfig) GetImport(modulePath string) string {
	if k.IsCustomDriver() {
		return k.DriverImport
	}

	return modulePath + "/pkg/drivers/kafka/" + strings.ToLower(k.Name)
}

// GetPackage returns package name
func (k KafkaConfig) GetPackage() string {
	if k.IsCustomDriver() {
		return k.DriverPackage
	}

	return strings.ToLower(k.Name)
}

// GetObjName returns struct name
func (k KafkaConfig) GetObjName() string {
	if k.IsCustomDriver() {
		return k.DriverObj
	}

	return KafkaObjNameProducer
}

const (
	RestTransportType  TransportType = "rest"
	GrpcTransportType  TransportType = "grpc"
	KafkaTransportType TransportType = "kafka"
	CLITransportType   TransportType = "cli"

	// Path components for schema directories
	schemaDir = "schema"
	pkgDir    = "pkg"
	apiDir    = "api"

//	WorkerDaemonType WorkerType = "daemon"

// ServiceName  = "service_name"
// ConsumerName = "consumer_name"

// templateMainType    TemplateType = "main"
// templateRestType    TemplateType = "rest"
// templateSysType     TemplateType = "sys"
// templateGrpcType    TemplateType = "grpc"
// templateHandlerType TemplateType = "handlers"
)

type Files struct {
	SourceName  string
	DestName    string
	OldDestName string
	ParamsTmpl  any
	Code        *bytes.Buffer
}

type DeployParams struct {
	Volumes []DeployVolume
}

type DeployVolume struct {
	Path  string // Путь к папке на хосте
	Mount string // Путь к монтируемой папке в контейнере
}

// GoatTestsConfig represents extended GOAT tests configuration
type GoatTestsConfig struct {
	Enabled    bool
	BinaryPath string // Path to test binary (default: /tmp/{app_name})
}

type App struct {
	Name                  string
	Transports            Transports
	Drivers               Drivers
	Workers               Workers
	Kafka                 KafkaConfigs // Kafka producers/consumers for this app
	CLI                   *CLIApp      // CLI app config (exclusive with Transports/Workers)
	Deploy                DeployParams
	UseActiveRecord       bool
	DependsOnDockerImages []string
	UseEnvs               bool
	Grafana               grafana.Config
	GoatTests             bool             // Enable GOAT integration tests generation
	GoatTestsConfig       *GoatTestsConfig // Extended GOAT tests configuration
}

// CLIApp represents a CLI transport configuration
type CLIApp struct {
	Name              string
	Import            string // Import path for the CLI handler
	Init              string // Initialization code
	GeneratorType     string
	GeneratorTemplate string
	GeneratorParams   map[string]string
}

// IsCLI returns true if this is a CLI application
func (a App) IsCLI() bool {
	return a.CLI != nil
}

// GetCLITransport returns CLI transport if this is a CLI app
func (a App) GetCLITransport() *CLIApp {
	return a.CLI
}

// HasSysTransport returns true if application has a SYS transport configured
func (a App) HasSysTransport() bool {
	for _, transport := range a.Transports {
		if transport.GeneratorType == "template" && transport.GeneratorTemplate == "sys" {
			return true
		}
	}
	return false
}

type Transports map[string]Transport
type Drivers map[string]Driver
type Workers map[string]Worker

func (w Workers) Add(name string, worker Worker) error {
	if _, ex := w[name]; ex {
		return fmt.Errorf("worker %s already exists", name)
	}

	w[name] = worker

	return nil
}

func (w Workers) GetUniqueTypes() map[string][]Worker {
	uniqueTypes := make(map[string][]Worker)

	for _, work := range w {
		uniqueTypes[work.GeneratorType] = append(uniqueTypes[work.GeneratorType], work)
	}

	return uniqueTypes
}

func (ts Transports) Add(name string, transport Transport) error {
	if _, ex := ts[name]; ex {
		return fmt.Errorf("transport %s already exists", name)
	}

	ts[name] = transport

	return nil
}

func (ts Transports) GetUniqueTypes() map[TransportType]map[string][]Transport {
	uniqueTypes := make(map[TransportType]map[string][]Transport)

	for _, t := range ts {
		if _, ex := uniqueTypes[t.Type]; !ex {
			uniqueTypes[t.Type] = make(map[string][]Transport)
		}

		uniqueTypes[t.Type][t.GeneratorType] = append(uniqueTypes[t.Type][t.GeneratorType], t)
	}

	return uniqueTypes
}

type Driver struct {
	Name         string   // Название драйвера, просто строка
	Import       string   // Путь к драйверу, например github.com/ovysilov/driver
	Package      string   // Название пакета драйвера, например driver
	ObjName      string   // Название объекта драйвера, например Driver
	CreateParams []string // Параметры для создания драйвера, те параметры, что будут переданы в функцию Create
	// ToDo это должно быть конфигурацией самого драйвера, а не конфигурацией приложения.
	// Т.е. это должно находиться в папке рядом с шаблонами, а не в конфигурационном файле генерируемого приложения.
	// Это единая настройка на все приложения, которые используют этот драйвер.
	ServiceInjection string // Структура которая будет добавлена в Service
}

type AuthParams struct {
	Transport string
	Type      string
}

type Transport struct {
	Name            string
	PkgName         string
	Import          []string // ToDo точно ли нужен срез?
	PublicService   bool
	Init            string
	HealthCheckPath string
	// Handler        Handler
	Type                 TransportType
	GeneratorType        string
	GeneratorTemplate    string
	AuthParams           AuthParams
	GeneratorParams      map[string]string
	SpecPath             []string
	ApiVersion           string // перенесено из Hendler
	Port                 string // перенесено из Hendler
	EmptyConfigAvailable bool
	BufLocalPlugins      bool // Use local buf instead of docker for proto generation
}

// HasAuthParams returns true if transport has authentication parameters configured
func (t Transport) HasAuthParams() bool {
	return t.AuthParams.Type != ""
}

type Worker struct {
	Import            []string // Imports for main.go (worker initialization)
	Name              string
	GeneratorType     string
	GeneratorTemplate string
	GeneratorParams   map[string]string
}

type Apps []App

// HasActiveRecord returns true if any application uses ActiveRecord
func (a Apps) HasActiveRecord() bool {
	for _, app := range a {
		if app.UseActiveRecord {
			return true
		}
	}

	return false
}

// HasGoatTests returns true if any application has GOAT tests generation enabled
func (a Apps) HasGoatTests() bool {
	for _, app := range a {
		if app.GoatTests {
			return true
		}
	}

	return false
}

// HasOgenClients returns true if app has any ogen_client transports (external API clients that need mocks)
func (a App) HasOgenClients() bool {
	for _, transport := range a.Transports {
		if transport.GeneratorType == "ogen_client" {
			return true
		}
	}

	return false
}

// GetOgenClients returns all ogen_client transports sorted by name
func (a App) GetOgenClients() []Transport {
	clients := make([]Transport, 0)

	for _, transport := range a.Transports {
		if transport.GeneratorType == "ogen_client" {
			clients = append(clients, transport)
		}
	}

	sort.Slice(clients, func(i, j int) bool {
		return strings.Compare(clients[i].Name, clients[j].Name) < 0
	})

	return clients
}

func (a App) TransportImports() []string {
	imports := make([]string, 0)

	for _, transport := range a.Transports {
		// Skip client transports - they are initialized in service, not in main
		if transport.GeneratorType != "ogen_client" && transport.GeneratorType != "buf_client" {
			imports = append(imports, transport.Import...)
		}
	}

	sort.Slice(imports, func(i, j int) bool {
		return strings.Compare(imports[i], imports[j]) < 0
	})

	return imports
}

func (a App) WorkerImports() []string {
	imports := make([]string, 0)

	for _, worker := range a.Workers {
		imports = append(imports, worker.Import...)
	}

	sort.Slice(imports, func(i, j int) bool {
		return strings.Compare(imports[i], imports[j]) < 0
	})

	return imports
}

// KafkaImports returns import paths for kafka producers
func (a App) KafkaImports(modulePath string) []string {
	imports := make([]string, 0)

	for _, kafka := range a.Kafka {
		if kafka.Type == KafkaTypeProducer {
			imports = append(imports, kafka.GetImport(modulePath))
		}
	}

	sort.Slice(imports, func(i, j int) bool {
		return strings.Compare(imports[i], imports[j]) < 0
	})

	return imports
}

// GetKafkaProducers returns all kafka producers for this app
func (a App) GetKafkaProducers() []KafkaConfig {
	producers := make([]KafkaConfig, 0)

	for _, kafka := range a.Kafka {
		if kafka.Type == KafkaTypeProducer {
			producers = append(producers, kafka)
		}
	}

	sort.Slice(producers, func(i, j int) bool {
		return strings.Compare(producers[i].Name, producers[j].Name) < 0
	})

	return producers
}

// HasKafkaProducers returns true if app has any kafka producers
func (a App) HasKafkaProducers() bool {
	for _, kafka := range a.Kafka {
		if kafka.Type == KafkaTypeProducer {
			return true
		}
	}

	return false
}

// GetTransportInfos returns transport info for Grafana dashboard generation.
func (a App) GetTransportInfos() []grafana.TransportInfo {
	infos := make([]grafana.TransportInfo, 0, len(a.Transports))

	for _, t := range a.Transports {
		infos = append(infos, grafana.TransportInfo{
			Name:          t.Name,
			GeneratorType: t.GeneratorType,
		})
	}

	sort.Slice(infos, func(i, j int) bool {
		return strings.Compare(infos[i].Name, infos[j].Name) < 0
	})

	return infos
}

func (app App) getTransport(t TransportType) []Transport {
	retTransports := []Transport{}

	for _, transport := range app.Transports {
		if transport.Type == t {
			// ToDo добавить проверку параметров генерации возможно при отличающихся параметрах надо делать разные ключи
			retTransports = append(retTransports, transport)
		}
	}

	sort.Slice(retTransports, func(i, j int) bool {
		return strings.Compare(retTransports[i].Name, retTransports[j].Name) < 0
	})

	return retTransports
}

func (a Apps) getTransport(t TransportType) []Transport {
	seen := make(map[string]struct{})
	listTransports := make([]Transport, 0)

	for _, app := range a {
		for _, transport := range app.getTransport(t) {
			if _, exists := seen[transport.Name]; !exists {
				seen[transport.Name] = struct{}{}
				listTransports = append(listTransports, transport)
			}
		}
	}

	sort.Slice(listTransports, func(i, j int) bool {
		return strings.Compare(listTransports[i].GeneratorType, listTransports[j].GeneratorType) < 0 &&
			strings.Compare(listTransports[i].Port, listTransports[j].Port) < 0
	})

	return listTransports
}

func (app App) GetRestTransport() []Transport {
	return app.getTransport(RestTransportType)
}

func (a Apps) GetRestTransport() []Transport {
	return a.getTransport(RestTransportType)
}

// func (a Apps) HasRestHandlers() bool {
// 	return len(a.getHandlers("rest")) > 0
// }

func (a Apps) GetGrpcTransport() []Transport {
	return a.getTransport(GrpcTransportType)
}

// func (a Apps) HasGrpcHandlers() bool {
// 	return len(a.getHandlers("grpc")) > 0
// }

// // ToDo kafka
func (a Apps) GetKafkaTransport() []Transport {
	return a.getTransport(KafkaTransportType)
}

// func (a Apps) HasKafkaHandlers() bool {
// 	return len(a.getHandlers("kafka")) > 0
// }

// func (a Apps) GetAllHandlers() []Handler {
// 	return a.getHandlers("")
// }

type Logger interface {
	ErrorMsg(string, string, string, ...string) string
	WarnMsg(string, string, ...string) string
	InfoMsg(string, string, ...string) string
	DebugMsg(string, string, ...string) string
	UpdateContext(...string) string
	Import() string
	FilesToGenerate() string
	DestDir() string
	InitLogger(ctx string, serviceName string) string
	// ReWrap generates code to rewrap logger from source context to destination context
	// sourceCtx - source context variable name
	// destCtx - destination context variable name
	// ocPrefix - onlineconf prefix
	// ocPath - onlineconf path
	ReWrap(sourceCtx, destCtx, ocPrefix, ocPath string) string
	// SetLoggerUpdater generates code to set the global logger updater for reqctx
	SetLoggerUpdater() string
}

func (t Transport) GetOgenConfigPath(targetDir string) string {
	switch t.GeneratorType {
	case "ogen":
		return filepath.Join(targetDir, "configs", "transport", string(t.Type), t.Name, t.ApiVersion, "ogen_server.yaml")
	case "ogen_client":
		return filepath.Join(targetDir, "configs", "transport", string(t.Type), t.Name, t.ApiVersion, "ogen_client.yaml")
	default:
		return ""
	}
}

func (t Transport) GetTargetSpecDir(targetDir string) string {
	if t.Type == GrpcTransportType {
		return filepath.Join(targetDir, "api", "grpc", t.Name)
	}

	return filepath.Join(targetDir, "api", "rest", t.Name, t.ApiVersion)
}

func (t Transport) GetTargetSpecFile(num int) string {
	_, file := filepath.Split(t.SpecPath[num])

	return file
}

func (t Transport) GetTargetGeneratePath(targetDir string) string {
	return filepath.Join(targetDir, "pkg", "rest", t.Name, t.ApiVersion)
}

type FilesDiff struct {
	NewFiles       map[string]struct{}
	IgnoreFiles    map[string]struct{}
	OtherFiles     map[string]struct{}
	NewDirectory   map[string]struct{}
	OtherDirectory map[string]struct{}
	UserContent    map[string][]byte
	RenameFiles    map[string]string
}

// GetTargetSpecDir returns the directory where schema files should be placed
func (j JSONSchema) GetTargetSpecDir(targetDir string) string {
	return filepath.Join(targetDir, apiDir, schemaDir, j.Name)
}

// GetTargetGeneratePath returns the directory for generated Go code
func (j JSONSchema) GetTargetGeneratePath(targetDir string) string {
	return filepath.Join(targetDir, pkgDir, schemaDir, j.Name)
}

// GetPackageName returns the package name for generated code
func (j JSONSchema) GetPackageName() string {
	if j.Package != "" {
		return j.Package
	}

	return j.Name
}

// GetSchemaFilenames returns the base filenames without extension for all schema paths
func (j JSONSchema) GetSchemaFilenames() []string {
	// Collect paths from both legacy Path[] and new Schemas[]
	var paths []string
	if len(j.Schemas) > 0 {
		for _, item := range j.Schemas {
			paths = append(paths, item.Path)
		}
	} else {
		paths = j.Path
	}

	filenames := make([]string, 0, len(paths))

	for _, path := range paths {
		base := filepath.Base(path)
		// Remove .json extension if present
		if ext := filepath.Ext(base); ext == ".json" {
			base = base[:len(base)-len(ext)]
		}

		filenames = append(filenames, base)
	}

	return filenames
}
