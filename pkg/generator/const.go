package generator

import "github.com/pkg/errors"

const (
	ServiceName  = "service_name"
	ConsumerName = "consumer_name"

	extYaml = ".yaml"
	extYml  = ".yml"
	extGo   = ".go"
	extMod  = ".mod"
	extSum  = ".sum"
	extSQL  = ".sql"
	extMD   = ".md"
	extSh   = ".sh"
	extDot  = "."
)

const (
	defaultGolangVersion   = "1.20"
	defaultProtobufVersion = "1.7.0"
	defaultGolangciVersion = "1.55.2"
	defaultOgenVersion     = "v0.78.0"

	defaultRestPort = 8080
	defaultGrpcPort = 8082
	defaultSysPort  = 8084
)

var (
	ErrInvalidConfig = errors.New("invalid config")
)

type Files struct {
	SourceName   string
	DestTmplName string
	ParamsTmpl   any
}

var filesToGenerate = map[string][]Files{
	"main": []Files{
		{SourceName: "Makefile.tmpl", DestTmplName: "Makefile"},
		{SourceName: "README.md.tmpl", DestTmplName: "README.md"},
		{SourceName: "LICENSE.txt.tmpl", DestTmplName: "LICENSE.txt"},
		{SourceName: "Dockefile.tmpl", DestTmplName: "Dockerfile"},
		{SourceName: "docker-compose.yml.tmpl", DestTmplName: "docker-compose.yml"},
		{SourceName: "configs/golangci-lint.yml.tmpl", DestTmplName: "configs/golangci-lint.yml"},
		{SourceName: "internal/pkg/constant/constant.go.tmpl", DestTmplName: "internal/pkg/constant/constant.go"},
		{SourceName: "scripts/goversioncheck.sh.tmpl", DestTmplName: "scripts/goversioncheck.sh"},
		{SourceName: "scripts/githooks/pre-commit.tmpl", DestTmplName: "scripts/githooks/pre-commit"},
		{SourceName: "internal/pkg/service/service.go.tmpl", DestTmplName: "internal/pkg/service/service.go"},
		{SourceName: "internal/pkg/service/README.md.tmpl", DestTmplName: "internal/pkg/service/README.md"},
	},
}

var dirToCreate = map[string][]Files{
	"main": []Files{
		{SourceName: "api", DestTmplName: "api"},
		{SourceName: "cmd", DestTmplName: "cmd"},
		{SourceName: "docs", DestTmplName: "docs"},
		{SourceName: "configs", DestTmplName: "configs"},
		{SourceName: "internal", DestTmplName: "internal"},
		{SourceName: "internal/transport", DestTmplName: "internal/transport"},
		{SourceName: "internal/service", DestTmplName: "internal/service"},
		{SourceName: "internal/pkg", DestTmplName: "internal/pkg"},
		{SourceName: "internal/pkg/constant", DestTmplName: "internal/pkg/constant"},
		{SourceName: "internal/pkg/interface", DestTmplName: "internal/pkg/interface"},
		{SourceName: "scripts", DestTmplName: "scripts"},
	},
}
