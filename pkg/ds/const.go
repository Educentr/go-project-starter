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

// Сделать Возможность не указывать DestTmplName, а брать его из SourceName
// var filesToGenerate = map[TemplateType][]Files{
// 	templateMainType: {
// 		{SourceName: "go.mod.tmpl", DestTmplName: "go.mod"},
// 		{SourceName: "Makefile.tmpl", DestTmplName: "Makefile"},
// 		{SourceName: "README.md.tmpl", DestTmplName: "README.md"},
// 		{SourceName: "LICENSE.txt.tmpl", DestTmplName: "LICENSE.txt"},
// 		{SourceName: "docker-compose.yaml.tmpl", DestTmplName: "docker-compose.yaml"},
// 		{SourceName: "configs/golangci-lint.yml.tmpl", DestTmplName: "configs/golangci-lint.yml"},
// 		{SourceName: "scripts/goversioncheck.sh.tmpl", DestTmplName: "scripts/goversioncheck.sh"},
// 		{SourceName: "scripts/githooks/pre-commit.tmpl", DestTmplName: "scripts/githooks/pre-commit"},
// 		{SourceName: "internal/pkg/constant/constant.go.tmpl", DestTmplName: "internal/pkg/constant/constant.go"},
// 		{SourceName: "internal/pkg/ds/app.go.tmpl", DestTmplName: "internal/pkg/ds/app.go"},
// 		{SourceName: "internal/pkg/metrics/build_collector.go.tmpl", DestTmplName: "internal/pkg/metrics/build_collector.go"},
// 		{SourceName: "internal/pkg/metrics/metrics.go.tmpl", DestTmplName: "internal/pkg/metrics/metrics.go"},
// 		{SourceName: "internal/pkg/service/service.go.tmpl", DestTmplName: "internal/pkg/service/service.go"},
// 		{SourceName: "internal/pkg/service/README.md.tmpl", DestTmplName: "internal/pkg/service/README.md"},
// 		// ToDo этих файлов должно быть несколько для каждого application свой
// 		{SourceName: "internal/app/app.go.tmpl", DestTmplName: "internal/app/app.go"},
// 		{SourceName: "internal/app/closer.go.tmpl", DestTmplName: "internal/app/closer.go"},
// 		{SourceName: "docker.tmpl", DestTmplName: "Dockerfile"},
// 	},
// 	templateHandlerType: {
// 		{SourceName: "cmd/api/main.go.tmpl", DestTmplName: "cmd/api/main.go"},
// 		{SourceName: "cmd/api/main_test.go.tmpl", DestTmplName: "cmd/api/main_test.go"},
// 		{SourceName: "internal/pkg/auth/auth.go.tmpl", DestTmplName: "internal/pkg/auth/auth.go"},
// 		{SourceName: "pkg/model/actor/actor.go.tmpl", DestTmplName: "pkg/model/actor/actor.go"},
// 		{SourceName: "pkg/req_ctx/context.go.tmpl", DestTmplName: "pkg/req_ctx/context.go"},
// 		{SourceName: "pkg/req_ctx/cumulative_metric.go.tmpl", DestTmplName: "pkg/req_ctx/cumulative_metric.go"},
// 	},
// 	templateRestType: {
// 		{SourceName: "internal/transport/rest/router.go.tmpl", DestTmplName: "internal/transport/rest/router.go"},
// 		{SourceName: "internal/transport/rest/mw/common.go.tmpl", DestTmplName: "internal/transport/rest/mw/common.go"},
// 		{SourceName: "internal/transport/rest/mw/csrf.go.tmpl", DestTmplName: "internal/transport/rest/mw/csrf.go"},
// 		{SourceName: "internal/transport/rest/mw/metrics.go.tmpl", DestTmplName: "internal/transport/rest/mw/metrics.go"},
// 		{SourceName: "internal/transport/rest/mw/tracing.go.tmpl", DestTmplName: "internal/transport/rest/mw/tracing.go"},
// 		{SourceName: "internal/transport/rest/mw/error_responses.go.tmpl", DestTmplName: "internal/transport/rest/mw/error_responses.go"},
// 		{SourceName: "internal/transport/rest/server.go.tmpl", DestTmplName: "internal/transport/rest/server.go"},
// 		{SourceName: "internal/transport/rest/closer.go.tmpl", DestTmplName: "internal/transport/rest/closer.go"},
// 		{SourceName: "internal/transport/rest/ogen_handler/router.go.tmpl", DestTmplName: "internal/transport/rest/{{ .Handler.Name | ToLower }}/{{ .Handler.ApiVersion | ToLower }}/router.go"},
// 		{SourceName: "internal/transport/rest/ogen_handler/middleware.go.tmpl", DestTmplName: "internal/transport/rest/{{ .Handler.Name | ToLower }}/{{ .Handler.ApiVersion | ToLower }}/middleware.go"},
// 		{SourceName: "internal/transport/rest/ogen_handler/error_response.go.tmpl", DestTmplName: "internal/transport/rest/{{ .Handler.Name | ToLower }}/{{ .Handler.ApiVersion | ToLower }}/error_response.go"},
// 		{SourceName: "internal/transport/rest/ogen_handler/handler/handler.go.tmpl", DestTmplName: "internal/transport/rest/{{ .Handler.Name | ToLower }}/{{ .Handler.ApiVersion | ToLower }}/handler/handler.go"},
// 		{SourceName: "internal/transport/rest/ogen_handler/handler/README.md.tmpl", DestTmplName: "internal/transport/rest/{{ .Handler.Name | ToLower }}/{{ .Handler.ApiVersion | ToLower }}/handler/README.md"},
// 	},
// 	templateSysType: {
// 		{SourceName: "internal/transport/rest/template/sys/server.go.tmpl", DestTmplName: "internal/transport/rest/sys/server.go"},
// 		{SourceName: "internal/transport/rest/template/sys/closer.go.tmpl", DestTmplName: "internal/transport/rest/sys/closer.go"},
// 		{SourceName: "internal/transport/rest/template/sys/handler/handler.go.tmpl", DestTmplName: "internal/transport/rest/sys/handler/handler.go"},
// 		{SourceName: "internal/transport/rest/template/sys/handler/sys.go.tmpl", DestTmplName: "internal/transport/rest/sys/handler/sys.go"},
// 	},
// }

// var dirToCreate = map[TemplateType][]Files{
// 	templateMainType: {
// 		{DestTmplName: "api"},
// 		{DestTmplName: "cmd"},
// 		{DestTmplName: "docs"},
// 		{DestTmplName: "configs"},
// 		{DestTmplName: "internal"},
// 		{DestTmplName: "internal/transport"},
// 		{DestTmplName: "internal/app"},
// 		{DestTmplName: "internal/pkg"},
// 		{DestTmplName: "internal/pkg/constant"},
// 		{DestTmplName: "internal/pkg/ds"},
// 		{DestTmplName: "internal/pkg/interface"},
// 		{DestTmplName: "internal/pkg/metrics"},
// 		{DestTmplName: "internal/pkg/service"},
// 		{DestTmplName: "scripts"},
// 		{DestTmplName: "scripts/githooks"},
// 	},
// 	templateHandlerType: {
// 		{DestTmplName: "cmd/api"},
// 		{DestTmplName: "internal/pkg/auth"},
// 		{DestTmplName: "pkg/model"},
// 		{DestTmplName: "pkg/req_ctx"},
// 		{DestTmplName: "pkg/model/actor"},
// 	},
// 	templateRestType: {
// 		{DestTmplName: "internal/transport/rest"},
// 		{DestTmplName: "internal/transport/rest/mw"},
// 		{DestTmplName: "internal/transport/rest/{{ .Handler.Name | ToLower }}"},
// 		{DestTmplName: "internal/transport/rest/{{ .Handler.Name | ToLower }}/{{ .Handler.ApiVersion | ToLower }}"},
// 		{DestTmplName: "internal/transport/rest/{{ .Handler.Name | ToLower }}/{{ .Handler.ApiVersion | ToLower }}/handler"},
// 	},
// 	templateSysType: {
// 		{DestTmplName: "internal/transport/rest"},
// 		{DestTmplName: "internal/transport/rest/sys"},
// 	},
// }
