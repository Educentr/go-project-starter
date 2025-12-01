package ds

import (
	"bytes"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
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

const (
	RestTransportType  TransportType = "rest"
	GrpcTransportType  TransportType = "grpc"
	KafkaTransportType TransportType = "kafka"
	CLITransportType   TransportType = "cli"

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

type App struct {
	Name                  string
	Transports            Transports
	Drivers               Drivers
	Workers               Workers
	CLI                   *CLIApp // CLI app config (exclusive with Transports/Workers)
	Deploy                DeployParams
	UseActiveRecord       bool
	DependsOnDockerImages []string
	UseEnvs               bool
}

// CLIApp represents a CLI transport configuration
type CLIApp struct {
	Name              string
	Import            string            // Import path for the CLI handler
	Init              string            // Initialization code
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
	Name                 string
	PkgName              string
	Import               []string // ToDo точно ли нужен срез?
	PublicService        bool
	Init                 string
	HealthCheckPath      string
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
}

type Worker struct {
	Import            string
	Name              string
	GeneratorType     string
	GeneratorTemplate string
	GeneratorParams   map[string]string
}

type Apps []App

func (a App) TransportImports() []string {
	imports := make([]string, 0)

	for _, transport := range a.Transports {
		if transport.GeneratorType != "ogen_client" {
			imports = append(imports, transport.Import...)
		}
	}

	sort.Slice(imports, func(i, j int) bool {
		return strings.Compare(imports[i], imports[j]) < 0
	})

	return imports
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
	// retTransports := map[string]Transport{}

	listTransports := make([]Transport, 0)

	for _, app := range a {
		listTransports = append(listTransports, app.getTransport(t)...)
	}

	// for _, transport := range retTransports {
	// 	listTransports = append(listTransports, transport)
	// }

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
