package generator

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"gitlab.educentr.info/golang/service-starter/pkg/config"
	"gitlab.educentr.info/golang/service-starter/pkg/ds"
	"gitlab.educentr.info/golang/service-starter/pkg/templater"
	"gitlab.educentr.info/golang/service-starter/pkg/tools"
)

type Generator struct {
	AppInfo       string
	Logger        ds.Logger
	ProjectName   string
	ProjectPath   string
	GoLangVersion string
	OgenVersion   string
	TargetDir     string
	PostGenerate  []ExecCmd
	Transports    ds.Transports
	Applications  ds.Apps
}

type ExecCmd struct {
	Cmd string
	Arg []string
	Msg string
}

func New(AppInfo string, config config.Config) (*Generator, error) {
	g := Generator{
		Applications: make(ds.Apps, 0, len(config.Applications)),
		PostGenerate: make([]ExecCmd, 0, len(config.PostGenerate)),
		Transports:   make(ds.Transports),
	}

	if err := g.processConfig(config); err != nil {
		return nil, err
	}

	return &g, nil
}

func (g *Generator) processConfig(config config.Config) error {
	g.Logger = config.Main.LoggerObj
	g.ProjectName = config.Main.Name
	g.ProjectPath = config.Git.ModulePath
	g.GoLangVersion = config.Tools.GolangVersion
	g.OgenVersion = config.Tools.OgenVersion
	g.TargetDir = "./"

	if config.Main.TargetDir != "" {
		g.TargetDir = config.Main.TargetDir
	}

	for _, rest := range config.RestList {
		paths := make([]string, 0, len(rest.Path))

		for _, path := range rest.Path {
			paths = append(paths, filepath.Join(config.BasePath, path))
		}

		if rest.Name == "" {
			return errors.New("rest name is empty")
		}

		if rest.Port == 0 {
			return fmt.Errorf("rest port is empty for %s", rest.Name)
		}

		transport := ds.Transport{
			Import:            []string{fmt.Sprintf(`%s_%s "%s/internal/app/transport/rest/%s/%s"`, rest.Name, rest.Version, g.ProjectPath, rest.Name, rest.Version)},
			Init:              fmt.Sprintf(`rest.NewServer("%s_%s", &%s_%s.API{})`, rest.Name, rest.Version, rest.Name, rest.Version),
			Type:              ds.RestTransportType,
			GeneratorType:     rest.GeneratorType,
			GeneratorTemplate: rest.GeneratorTemplate,
			Handler:           ds.NewHandler(rest.Name, rest.Version, strconv.FormatUint(uint64(rest.Port), 10), paths),
		}

		if err := g.Transports.Add(rest.Name, transport); err != nil {
			return err
		}
	}

	for _, grpc := range config.GrpcList {
		panic("Not implemented " + grpc.Name) //ToDo
	}

	for _, app := range config.Applications {
		application := ds.App{
			Name:       app.Name,
			Transports: make(ds.Transports),
			Drivers:    []string{},
		}

		for _, transport := range app.TransportList {
			tr, ex := g.Transports[transport]
			if !ex {
				return fmt.Errorf("unknown transport: %s", transport)
			}

			application.Transports[transport] = tr
		}

		g.Applications = append(g.Applications, application)
	}

	for _, postGenerate := range config.PostGenerate {
		switch postGenerate {
		case "git_install":
			g.PostGenerate = append(g.PostGenerate, ExecCmd{Cmd: "git", Arg: []string{"init"}, Msg: "initialize git"})
		case "tools_install":
			g.PostGenerate = append(g.PostGenerate, ExecCmd{Cmd: "make", Arg: []string{"install-tools"}, Msg: "install tools"})
		case "clean_imports":
			g.PostGenerate = append(g.PostGenerate, ExecCmd{Cmd: "make", Arg: []string{"clean-import"}, Msg: "cleaning imports"})
		case "executable_scripts":
			g.PostGenerate = append(g.PostGenerate, ExecCmd{Cmd: "chmod", Arg: []string{"a+x", "scripts/goversioncheck.sh"}, Msg: "make scripts executable"})
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

func (g *Generator) GetTmplParams() templater.GeneratorParams {
	return templater.GeneratorParams{
		Logger:        g.Logger,
		ProjectName:   g.ProjectName,
		ProjectPath:   g.ProjectPath,
		GoLangVersion: g.GoLangVersion,
		OgenVersion:   g.OgenVersion,
		Applications:  g.Applications,
	}
}

func (g *Generator) GetTmplAppParams(app ds.App) templater.GeneratorAppParams {
	return templater.GeneratorAppParams{
		GeneratorParams: g.GetTmplParams(),
		Application:     app,
	}
}

func (g *Generator) GetTmplHandlerParams(transport ds.Transport) templater.GeneratorHandlerParams {
	return templater.GeneratorHandlerParams{
		GeneratorParams: g.GetTmplParams(),
		Transport:       transport,
	}
}

func (g *Generator) CopySpecs() error {
	for _, app := range g.Applications {
		for _, transport := range app.Transports {
			for _, spec := range transport.Handler.SpecPath {
				if _, err := os.Stat(spec); err != nil {
					return fmt.Errorf("spec file not found: %s", spec)
				}

				source := spec

				dest := filepath.Join(
					transport.Handler.GetTargetSpecDir(g.TargetDir),
					transport.Handler.GetTargetSpecFile(),
				)

				log.Printf("copy spec: `%s` to `%s`\n", source, dest)

				if err := tools.CopyFile(source, dest); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// ToDo Generate generates the content of a file and writes it to the specified destination path.
// It also applies custom code patches and saves a snapshot of the generated content.
// Добавить проверку, что хватает менста на диске
func (g *Generator) Generate() error {
	targetPath, err := filepath.Abs(g.TargetDir)
	if err != nil {
		return err
	}

	dirs, files, err := g.collectFiles(targetPath)
	if err != nil {
		return err
	}

	for _, dir := range dirs {
		fmt.Printf("Dir: %s -> %s\n", dir.SourceName, dir.DestName)
	}

	for _, file := range files {
		fmt.Printf("File: %s -> %s\n", file.SourceName, file.DestName)
	}

	existingCode, err := templater.GetUserCodeFromFiles(files)
	if err != nil {
		return err
	}

	for i := range files {
		tmpl, err := templater.GetTemplate(files[i].SourceName)
		if err != nil {
			return fmt.Errorf("failed to get template %s: %w", files[i].SourceName, err)
		}

		files[i].Code, err = templater.GenerateByTmpl(tmpl, files[i].ParamsTmpl, existingCode[files[i].DestName], files[i].DestName)
		if err != nil {
			return err
		}
	}

	if err = tools.MakeDirs(dirs); err != nil {
		return err
	}

	for _, file := range files {
		dstFile, err := os.Create(file.DestName)
		if err != nil {
			return err
		}
		defer dstFile.Close()

		file.Code.WriteTo(dstFile)
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

		if len(out) > 0 {
			log.Printf("result: %s\n", out)
		}
	}

	return nil
}

func (g *Generator) collectFiles(targetPath string) ([]ds.Files, []ds.Files, error) {
	dirs, files, err := templater.GetMainTemplates(g.GetTmplParams())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get main templates: %w", err)
	}

	types := g.Transports.GetUniqueTypes()

	for transportType, templateType := range types {
		dirsTr, filesTr, err := templater.GetTransportTemplates(transportType, g.GetTmplParams())
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get transport templates: %w", err)
		}

		dirs = append(dirs, dirsTr...)
		files = append(files, filesTr...)

		for tmplType, tr := range templateType {
			dirsTrT, filesTrT, err := templater.GetTransportGeneratorTemplates(transportType, tmplType, g.GetTmplParams())
			if err != nil {
				return nil, nil, fmt.Errorf("failed to get transport generator templates: %w", err)
			}

			dirs = append(dirs, dirsTrT...)
			files = append(files, filesTrT...)

			for _, transport := range tr {
				dirsH, filesH, err := templater.GetTransportHandlerTemplates(
					transport.Type,
					filepath.Join(transport.GeneratorType, transport.GeneratorTemplate),
					g.GetTmplHandlerParams(transport),
				)
				if err != nil {
					return nil, nil, errors.Wrapf(err, "failed to get transport handler templates: `%s`, `%s`, `%s`", transport.Type, transport.GeneratorType, transport.GeneratorTemplate)
				}

				dirs = append(dirs, dirsH...)
				files = append(files, filesH...)
			}
		}
	}

	dirsL, filesL, err := templater.GetLoggerTemplates(g.Logger.FilesToGenerate(), g.Logger.DestDir(), g.GetTmplParams())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get logger templates: %w", err)
	}

	dirs = append(dirs, dirsL...)
	files = append(files, filesL...)

	for _, app := range g.Applications {
		dirApp, filesApp, err := templater.GetAppTemplates(g.GetTmplAppParams(app))
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get app templates: %w", err)
		}

		dirs = append(dirs, dirApp...)
		files = append(files, filesApp...)
	}

	// ToDo check git status
	// ToDo make backup of targetPath
	// ToDo Dry run
	// ToDo validate checksum previous generated files

	for i := range dirs {
		destDirName, err := templater.GenerateFilenameByTmpl(dirs[i])
		if err != nil {
			return nil, nil, fmt.Errorf("failed to generate filename by template %s: %w", dirs[i].DestName, err)
		}

		dirs[i].DestName = filepath.Join(targetPath, destDirName)
	}

	for i := range files {
		destFileName, err := templater.GenerateFilenameByTmpl(files[i])
		if err != nil {
			return nil, nil, fmt.Errorf("failed to generate filename by template %s: %w", files[i].DestName, err)
		}

		files[i].DestName = filepath.Join(targetPath, destFileName)
	}

	return dirs, files, nil
}

// func (g *Generator) CollectTargetDir() ([]Files, error) {
// 	targetDirs, ex := dirToCreate[templateMainType]
// 	if !ex {
// 		return nil, fmt.Errorf("main directory not found")
// 	}

// 	for n := range targetDirs {
// 		// ToDo не надо передавать пустой объект App, либо разделить на разные типы либо передавать nil
// 		targetDirs[n].ParamsTmpl = g.GetTmplParams(App{})
// 	}

// 	for _, app := range g.Applications {
// 		if len(app.Handlers) > 0 {
// 			dirsToAdd, ex := dirToCreate[templateHandlerType]
// 			if !ex {
// 				return nil, fmt.Errorf("handler directory not found")
// 			}

// 			for n := range dirsToAdd {
// 				dirsToAdd[n].ParamsTmpl = g.GetTmplParams(app)
// 				targetDirs = append(targetDirs, dirsToAdd...)
// 			}

// 			dirsToAdd = g.Logger.DirsToGenerate()

// 			for n := range dirsToAdd {
// 				dirsToAdd[n].ParamsTmpl = g.GetTmplParams(app)
// 				targetDirs = append(targetDirs, dirsToAdd...)
// 			}

// 			for _, handler := range app.Handlers {
// 				tmplParams := g.GetTmplHandlerParams(handler)

// 				dirsToAdd, ex := dirToCreate[TemplateType(handler.Transport)]
// 				if !ex {
// 					return nil, fmt.Errorf("unknown transport: %s", handler.Transport)
// 				}

// 				for n := range dirsToAdd {
// 					dirsToAdd[n].ParamsTmpl = tmplParams
// 				}

// 				targetDirs = append(targetDirs, dirsToAdd...)
// 			}
// 		}
// 	}

// 	return targetDirs, nil
// }

// ToDo сделать проверку на дубли файлов
// Возможно надо вывернуть на изнанку мапку filesToGenerate в которой ключами файлы, а значениями будут транспорты/хендлеры/...
// func (g *Generator) CollectTargetFiles() ([]Files, error) {
// 	targetFiles, ex := filesToGenerate["main"]
// 	if !ex {
// 		return nil, fmt.Errorf("main files not found")
// 	}

// 	for n := range targetFiles {
// 		// ToDo не надо передавать пустой объект App, либо разделить на разные типы либо передавать nil
// 		targetFiles[n].ParamsTmpl = g.GetTmplParams(App{})
// 	}

// 	for _, app := range g.Applications {
// 		if len(app.Handlers) > 0 {
// 			filesToAdd, ex := filesToGenerate[templateHandlerType]
// 			if !ex {
// 				return nil, fmt.Errorf("handler files not found")
// 			}

// 			for n := range filesToAdd {
// 				filesToAdd[n].ParamsTmpl = g.GetTmplParams(app)
// 				targetFiles = append(targetFiles, filesToAdd...)
// 			}

// 			filesToAdd = g.Logger.FilesToGenerate()

// 			for n := range filesToAdd {
// 				filesToAdd[n].ParamsTmpl = g.GetTmplParams(app)
// 				targetFiles = append(targetFiles, filesToAdd...)
// 			}

// 			for _, handler := range app.Handlers {
// 				tmplParams := g.GetTmplHandlerParams(handler)

// 				filesToAdd, ex := filesToGenerate[TemplateType(handler.Transport)]
// 				if !ex {
// 					return nil, fmt.Errorf("unknown transport: %s", handler.Transport)
// 				}

// 				for n := range filesToAdd {
// 					filesToAdd[n].ParamsTmpl = tmplParams
// 				}

// 				targetFiles = append(targetFiles, filesToAdd...)
// 			}
// 		}
// 	}

// 	return targetFiles, nil
// }

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
