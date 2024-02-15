package generator

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gitlab.educentr.info/golang/service-starter/pkg/tools"
)

type Logger interface {
	ErrorMsg(string, string, string, ...string) string
	WarnMsg(string, string, ...string) string
	InfoMsg(string, string, ...string) string
	Import() string
}

type Handler struct {
	Transport  string
	Name       string
	ApiVersion string
	Port       string
	SpecPath   string // ToDo []strings for spec with refs
}

type GeneratorParamsBase struct {
	Logger      Logger
	ProjName    string
	ProjectPath string
}

type GeneratorParams struct {
	GeneratorParamsBase
	Handlers []Handler
}

type GeneratorHandlerParams struct {
	GeneratorParamsBase
	Handler Handler
}

type App struct {
	Name     string
	Handlers []Handler
	// RestList     []string
	// GrpcList     []string `mapstructure:"grpc"`
	// WsList       []string `mapstructure:"ws"`
	// ConsumerList []string `mapstructure:"consumer"`
}

type ExecCmd struct {
	Cmd string
	Arg []string
	Msg string
}

type Generator struct {
	Logger       Logger
	ProjectName  string
	ProjectPath  string
	TargetDir    string
	PostGenerate []ExecCmd
	Applications []App
}

func New(config Config) (*Generator, error) {
	g := Generator{
		Applications: make([]App, 0, len(config.Applications)),
	}

	if err := g.processConfig(config); err != nil {
		return nil, err
	}

	return &g, nil
}

func (g *Generator) GetTmplParams() GeneratorParams {
	return GeneratorParams{
		GeneratorParamsBase: GeneratorParamsBase{
			Logger:      g.Logger,
			ProjName:    g.ProjectName,
			ProjectPath: g.ProjectPath,
		},
	}
}

func (g *Generator) GetTmplAppParams(app App) GeneratorParams {
	return GeneratorParams{
		GeneratorParamsBase: GeneratorParamsBase{
			Logger:      g.Logger,
			ProjName:    g.ProjectName,
			ProjectPath: g.ProjectPath,
		},
		Handlers: app.Handlers,
	}
}

func (g *Generator) GetTmplHandlerParams(handler Handler) GeneratorHandlerParams {
	return GeneratorHandlerParams{
		GeneratorParamsBase: GeneratorParamsBase{
			Logger:      g.Logger,
			ProjName:    g.ProjectName,
			ProjectPath: g.ProjectPath,
		},
		Handler: handler,
	}
}

func (g *Generator) processConfig(config Config) error {
	l, ex := LoggerMapping[config.Main.Logger]
	if !ex {
		log.Fatalln("invalid logger", config.Main.Logger)
	}

	g.Logger = l
	g.ProjectName = config.Main.Name
	g.ProjectPath = config.Git.ModulePath
	g.TargetDir = config.Main.TargetDir

	if g.TargetDir == "" {
		g.TargetDir = "./"
	}

	for _, app := range config.Applications {
		application := App{}

		for _, restHandler := range app.RestList {
			application.Handlers = append(application.Handlers, Handler{Transport: "rest", Name: restHandler, SpecPath: config.restMap[restHandler].Path})
		}

		for _, grpcHandler := range app.GrpcList {
			application.Handlers = append(application.Handlers, Handler{Transport: "grpc", Name: grpcHandler, SpecPath: config.grpcMap[grpcHandler].Path})
		}

		g.Applications = append(g.Applications, application)
	}

	// ToDo move to config (map[string]ExecCmd)
	// ToDo add priority for post generate steps
	for _, postGenerate := range config.PostGenerate {
		switch postGenerate {
		case "git_install":
			g.PostGenerate = append(g.PostGenerate, ExecCmd{Cmd: "git", Arg: []string{"init"}, Msg: "initialize git"})
		case "tools_install":
			g.PostGenerate = append(g.PostGenerate, ExecCmd{Cmd: "make", Arg: []string{"install-tools"}, Msg: "install tools"})
		case "clean_imports":
			g.PostGenerate = append(g.PostGenerate, ExecCmd{Cmd: "make", Arg: []string{"clean-import"}, Msg: "cleaning imports"})
		case "executable_scripts":
			g.PostGenerate = append(g.PostGenerate, ExecCmd{Cmd: "chmod", Arg: []string{"a+x", "scripts/goversioncheck.sh"}, Msg: "executable scripts"})
		case "call_generate":
			g.PostGenerate = append(g.PostGenerate, ExecCmd{Cmd: "make", Arg: []string{"generate"}, Msg: "generate"})
		case "go_mod_tidy":
			g.PostGenerate = append(g.PostGenerate, ExecCmd{Cmd: "make", Arg: []string{"tidy"}, Msg: "go mod tidy"})
		case "go_get_u":
			g.PostGenerate = append(g.PostGenerate, ExecCmd{Cmd: "make", Arg: []string{"go-get-u"}, Msg: "updating dependencies"})
		default:
			cmd := strings.Split(postGenerate, " ")
			g.PostGenerate = append(g.PostGenerate, ExecCmd{Cmd: cmd[0], Arg: cmd[1:], Msg: "custom command"})
		}
	}

	// for _, e := range config.Applications {

	// for i, e := range config.RestList {
	// 	if !e.IsValid() {
	// 		log.Fatalln("invalid rest config with", i, "index")
	// 	}

	// 	if err := g.AddOpenApi(e.Path, e.Name, e.APIPrefix); err != nil {
	// 		return err
	// 	}
	// }

	// for i, e := range g.config.GrpcList {
	// 	if !e.IsValid() {
	// 		log.Fatalln("invalid grpc config with", i, "index")
	// 	}

	// 	if err := g.AddGrpcApi(e.Path, e.Name, e.Short); err != nil {
	// 		return err
	// 	}
	// }

	// for i, e := range g.config.WsList {
	// 	if !e.IsValid() {
	// 		log.Fatalln("invalid ws config with", i, "index")
	// 	}

	// 	if err := g.AddWSApi(e.Path, e.Name); err != nil {
	// 		return err
	// 	}
	// }

	// for i, e := range g.config.ConsumerList {
	// 	if !e.IsValid() {
	// 		log.Fatalln("invalid consumer config with", i, "index")
	// 	}

	// 	if err := g.AddConsumer(e.Name, e.Path, e.Backend, e.Group, e.Topic); err != nil {
	// 		return err
	// 	}
	// }

	// for i, e := range g.config.RepositoryList {
	// 	if !e.IsValid() {
	// 		log.Fatalln("invalid repository config with", i, "index")
	// 	}

	// 	if err := g.AddRepository(e.Name, e.TypeDB, e.DriverDB); err != nil {
	// 		return err
	// 	}
	// }

	return nil
}

func (g *Generator) CopySpecs() error {
	for _, app := range g.Applications {
		for _, handler := range app.Handlers {
			tools.CopyFile(
				handler.SpecPath,
				filepath.Join(g.TargetDir, "api", handler.Name, "v"+handler.ApiVersion, handler.Name+".swagger.yml"),
			)
		}
	}

	return nil
}

// ToDo Generate generates the content of a file and writes it to the specified destination path.
// It also applies custom code patches and saves a snapshot of the generated content.
func (g *Generator) Generate() error {
	targetPath, err := filepath.Abs(g.TargetDir)
	if err != nil {
		return err
	}

	// ToDo check git status
	// ToDo make backup of targetPath

	dirs, err := g.CollectTargetDir()
	if err != nil {
		return fmt.Errorf("failed to collect target directories: %w", err)
	}

	files, err := g.CollectTargetFiles()
	if err != nil {
		return fmt.Errorf("failed to collect target files: %w", err)
	}

	// ToDo Dry run
	// ToDo validate checksum previous generated files

	for _, dir := range dirs {
		newDir := filepath.Join(targetPath, dir.DestTmplName)
		if err := os.MkdirAll(newDir, 0700); err != nil && err != os.ErrExist {
			return fmt.Errorf("failed to create directory %s: %w", newDir, err)
		}
	}

	for _, file := range files {
		destFileName, err := GenerateFilenameByTmpl(file, file.ParamsTmpl)
		if err != nil {
			return fmt.Errorf("failed to generate filename by template %s: %w", file.DestTmplName, err)
		}

		// ToDo rename destPath -> destFile
		destPath := filepath.Join(targetPath, destFileName)

		tmpl, err := GetTemplate(file.SourceName)
		if err != nil {
			return fmt.Errorf("failed to get template %s: %w", file.SourceName, err)
		}

		if err = GenerateByTmpl(tmpl, file.ParamsTmpl, destPath); err != nil {
			return err
		}
	}

	if err = g.CopySpecs(); err != nil {
		return err
	}

	for _, procData := range g.PostGenerate {
		cmd := exec.Command(procData.Cmd, procData.Arg...)
		cmd.Dir = targetPath

		log.Printf("run: %s\n", procData.Msg)

		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("%w: %s", err, out)
		}

		log.Printf("result: %s\n", cmd.Stdout)
	}

	return nil
}

func (g *Generator) CollectTargetDir() ([]Files, error) {
	targetDirs, ex := dirToCreate["main"]
	if !ex {
		return nil, fmt.Errorf("main directory not found")
	}

	for n := range targetDirs {
		targetDirs[n].ParamsTmpl = g.GetTmplParams()
	}

	for _, app := range g.Applications {
		for _, handler := range app.Handlers {
			tmplParams := g.GetTmplHandlerParams(handler)

			dirsToAdd, ex := dirToCreate[handler.Transport]
			if !ex {
				return nil, fmt.Errorf("unknown transport: %s", handler.Transport)
			}

			for n := range dirsToAdd {
				dirsToAdd[n].ParamsTmpl = tmplParams
			}

			targetDirs = append(targetDirs, dirsToAdd...)
		}
	}

	return targetDirs, nil
}

func (g *Generator) CollectTargetFiles() ([]Files, error) {
	targetFiles, ex := filesToGenerate["main"]
	if !ex {
		return nil, fmt.Errorf("main files not found")
	}

	for n := range targetFiles {
		targetFiles[n].ParamsTmpl = g.GetTmplParams()
	}

	for _, app := range g.Applications {
		for _, handler := range app.Handlers {
			tmplParams := g.GetTmplHandlerParams(handler)

			filesToAdd, ex := filesToGenerate[handler.Transport]
			if !ex {
				return nil, fmt.Errorf("unknown transport: %s", handler.Transport)
			}

			for n := range filesToAdd {
				filesToAdd[n].ParamsTmpl = tmplParams
			}

			targetFiles = append(targetFiles, filesToAdd...)
		}
	}

	return targetFiles, nil
}

//==============================================================
/*
type Generator1 struct {
	apps             map[string]App
	openApiContracts []*Contract
	protoContracts   []*Contract

	ports Ports

	grpc  bool
	rest  bool
	ws    bool
	Kafka bool

	consumers    []*consumer
	repositories []*repository
}

var (
	ErrInvalidProtobufVersion = errors.New("invalid protobuf version for grpc generator")
	ErrInvalidOgenVersion     = errors.New("invalid ogen version for rest generator")
)

func New(config Config) (*Generator, error) {
	g := Generator{}

	if err := g.processConfig(config); err != nil {
		return nil, err
	}

	return &g, nil
}

func (g *Generator) processConfig(config Config) error {
	for i, e := range config.RestList {
		if !e.IsValid() {
			log.Fatalln("invalid rest config with", i, "index")
		}

		if err := g.AddOpenApi(e.Path, e.Name, e.APIPrefix); err != nil {
			return err
		}
	}

	for i, e := range g.config.GrpcList {
		if !e.IsValid() {
			log.Fatalln("invalid grpc config with", i, "index")
		}

		if err := g.AddGrpcApi(e.Path, e.Name, e.Short); err != nil {
			return err
		}
	}

	for i, e := range g.config.WsList {
		if !e.IsValid() {
			log.Fatalln("invalid ws config with", i, "index")
		}

		if err := g.AddWSApi(e.Path, e.Name); err != nil {
			return err
		}
	}

	for i, e := range g.config.ConsumerList {
		if !e.IsValid() {
			log.Fatalln("invalid consumer config with", i, "index")
		}

		if err := g.AddConsumer(e.Name, e.Path, e.Backend, e.Group, e.Topic); err != nil {
			return err
		}
	}

	for i, e := range g.config.RepositoryList {
		if !e.IsValid() {
			log.Fatalln("invalid repository config with", i, "index")
		}

		if err := g.AddRepository(e.Name, e.TypeDB, e.DriverDB); err != nil {
			return err
		}
	}

	return nil
}

func (g *Generator) CopySpecs() error {
	//Todo check
	// sourceFileStat, err := os.Stat(src)
	// if err != nil {
	// 	return 0, err
	// }

	// if !sourceFileStat.Mode().IsRegular() {
	// 	return 0, fmt.Errorf("%s is not a regular file", src)
	// }

	for _, openapi := range g.openApiContracts {
		name := fetchFileName(openapi.Path, openapi.ServerName+".swagger.yml")
		err := CopyFile(openapi.Path, "../"+g.config.Main.Name+"/api/rest/"+openapi.ServerName+"/v1/"+name)
		if err != nil {
			return err
		}
	}

	for _, proto := range g.protoContracts {
		name := fetchFileName(proto.Path, proto.ServerName+".proto")
		err := CopyFile(proto.Path, "../"+g.config.Main.Name+"/api/grpc/"+proto.ServerName+"/v1/"+name)
		if err != nil {
			return err
		}
	}

	for _, cons := range g.consumers {
		name := fetchFileName(cons.Path, cons.Name+".proto")
		err := CopyFile(cons.Path, "../"+g.config.Main.Name+"/api/kafka/"+cons.Name+"/v1/"+name)
		if err != nil {
			return err
		}
	}

	return nil
}

func (g *Generator) isGitInitialized() bool {
	_, err := git.PlainOpenWithOptions("../"+g.config.Main.Name, &git.PlainOpenOptions{DetectDotGit: true})
	return err == nil // if err == nil, then git repository already initialized
}


func (g *Generator) Generate() error {
	targetPath, err := filepath.Abs("../" + g.config.Main.Name)
	if err != nil {
		return err
	}

	// ToDo
	// if err = tools.CheckGitStatus(targetPath); err != nil {
	// 	return err
	// }

	if err = g.walkAndGenerateCodeUsingTemplates(targetPath); err != nil {
		return err
	}

	if err = g.CopySpecs(); err != nil {
		return err
	}

	for _, procData := range g.postGenerateSteps() {
		cmd := exec.Command(procData.pr, procData.argv...)
		cmd.Dir = "../" + g.config.Main.Name

		log.Printf("run: %s\n", procData.msg)

		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("%w: %s", err, out)
		}

		log.Printf("result: %s\n", cmd.Stdout)
	}

	return nil
}

func (g *Generator) GetParams() PkgData {
	params := PkgData{
		ProjectName:     g.config.Main.Name,
		GoLangVersion:   g.config.Tools.GolangVersion,
		GolangciVersion: g.config.Tools.GolangciVersion,
		OgenVersion:     g.config.Tools.OgenVersion,
		ProtobufVersion: g.config.Tools.ProtobufVersion,
		GrpcData:        []*PkgDataGrpc{},
		AppInfo:         "beta",
		Ports:           g.ports,
		Kafka:           g.Kafka,
		Repo:            len(g.repositories) > 0,
		Repos:           []*PkgDataRepo{},
		Scheduler:       g.config.Scheduler.Enabled,
		REST:            g.rest,
		GRPC:            g.grpc,
	}

	for _, proto := range g.protoContracts {
		params.GrpcData = append(params.GrpcData, &PkgDataGrpc{Name: proto.ServerName, PackageName: proto.Short})
	}

	for _, e := range g.openApiContracts {
		params.RestData = append(params.RestData, &PkgDataRest{
			Name:        e.ServerName,
			PackageName: e.Short,
			APIPrefix:   e.APIPrefix,
		})
	}

	for _, repo := range g.repositories {
		switch repo.TypeDB {
		case Psql:
			params.PG = true
		case Redis:
			params.Redis = true
		}

		params.Repos = append(params.Repos, &PkgDataRepo{
			Name:     repo.Name,
			TypeDB:   string(repo.TypeDB),
			DriverDB: string(repo.DriverDB),
		})
	}

	return params
}

func (g *Generator) AddOpenApi(path string, name string, apiPrefix string) error {
	if g.config.Tools.OgenVersion == "" {
		return ErrInvalidOgenVersion
	}

	g.openApiContracts = append(g.openApiContracts, &Contract{
		Path:       path,
		ServerName: name,
		APIPrefix:  apiPrefix,
	})

	g.rest = true

	return nil
}

func (g *Generator) AddGrpcApi(path, serverName, name string) error {
	if g.config.Tools.ProtobufVersion == "" {
		return ErrInvalidProtobufVersion
	}

	g.protoContracts = append(g.protoContracts, &Contract{Path: path, ServerName: serverName, Short: name})
	g.grpc = true

	return nil
}

func (g *Generator) AddWSApi(path string, name string) error {
	// ToDo ws
	g.ws = true

	return nil
}

func (g *Generator) AddConsumer(name, path string, backend BufType, group, topic string) error {
	g.consumers = append(g.consumers, &consumer{Name: name, Path: path, Backend: backend, Group: group, Topic: topic})

	if backend == Kafka {
		g.Kafka = true
	}

	return nil
}

func (g *Generator) AddRepository(name string, typeDB TypeDB, driverDB DriverDB) error {
	g.repositories = append(g.repositories, &repository{
		Name:     name,
		TypeDB:   typeDB,
		DriverDB: driverDB,
	})

	return nil
}
*/
