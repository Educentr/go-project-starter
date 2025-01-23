package ds

import (
	"bytes"
	"fmt"
	"path/filepath"
)

type TransportType string

const (
	RestTransportType  TransportType = "rest"
	GrpcTransportType  TransportType = "grpc"
	KafkaTransportType TransportType = "kafka"

// ServiceName  = "service_name"
// ConsumerName = "consumer_name"

// templateMainType    TemplateType = "main"
// templateRestType    TemplateType = "rest"
// templateSysType     TemplateType = "sys"
// templateGrpcType    TemplateType = "grpc"
// templateHandlerType TemplateType = "handlers"
)

type IDriver interface {
	Import() string
	DataType() string
}

type IHandler interface {
	Name() string
	Version() string
}

type ITransports interface {
	Import() []string
	Init() []string
}

type Files struct {
	SourceName string
	DestName   string
	ParamsTmpl any
	Code       *bytes.Buffer
}

type App struct {
	Name       string     // unused
	Transports Transports // unused
	Drivers    []string   // unused
}

type Transports map[string]Transport

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

type Transport struct {
	Import            []string
	Init              string
	Handler           Handler
	Type              TransportType
	GeneratorType     string
	GeneratorTemplate string
}

type Apps []App

func (a App) TransportImports() []string {
	imports := make([]string, 0)

	for _, transport := range a.Transports {
		imports = append(imports, transport.Import...)
	}

	return imports
}

func (a Apps) getTransport(t TransportType) []Transport {
	retTransports := make([]Transport, 0)

	for _, app := range a {
		for _, transport := range app.Transports {
			if transport.Type == t {
				retTransports = append(retTransports, transport)
			}
		}
	}

	return retTransports
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
	UpdateContext(...string) string
	Import() string
	FilesToGenerate() string
	DestDir() string
	InitLogger(ctx string, serviceName string) string
}

// ToDo кажется, что тип Handler не нужен и надо объединить его с Transport
type Handler struct {
	Name       string
	ApiVersion string
	Port       string   //unused
	SpecPath   []string //unused
}

func NewHandler(name, apiVersion, port string, specPath []string) Handler {
	return Handler{
		Name:       name,
		ApiVersion: apiVersion,
		Port:       port,
		SpecPath:   specPath,
	}
}

func (h Handler) GetTargetSpecDir(targetDir string) string {
	return filepath.Join(targetDir, "api", "rest", h.Name, h.ApiVersion)
}

func (h Handler) GetTargetSpecFile() string {
	_, file := filepath.Split(h.SpecPath[0])

	return file
}

func (h Handler) GetTargetGeneratePath(targetDir string) string {
	return filepath.Join(targetDir, "pkg", "rest", h.Name, h.ApiVersion)
}

type FilesDiff struct {
	NewFiles     map[string]struct{}
	IgnoreFiles  map[string]struct{}
	OldFiles     map[string]struct{}
	NewDirectory map[string]struct{}
	OldDirectory map[string]struct{}
	UserContent  map[string][]byte
}
