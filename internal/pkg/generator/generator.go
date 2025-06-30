package generator

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"gitlab.educentr.info/golang/service-starter/internal/pkg/config"
	"gitlab.educentr.info/golang/service-starter/internal/pkg/ds"
	"gitlab.educentr.info/golang/service-starter/internal/pkg/meta"
	"gitlab.educentr.info/golang/service-starter/internal/pkg/templater"
	"gitlab.educentr.info/golang/service-starter/internal/pkg/tools"
)

type Generator struct {
	AppInfo           string
	DryRun            bool
	Meta              meta.Meta
	Logger            ds.Logger
	ProjectName       string
	Deploy            ds.DeployType
	RegistryType      string
	Author            string
	ProjectPath       string
	UseActiveRecord   bool
	Repo              string
	GoLangVersion     string
	OgenVersion       string
	ArgenVersion      string
	GolangciVersion   string
	TargetDir         string
	DockerImagePrefix string
	SkipInitService   bool
	PostGenerate      []ExecCmd
	Transports        ds.Transports
	Workers           ds.Workers
	Drivers           ds.Drivers
	Applications      ds.Apps
}

type ExecCmd struct {
	Cmd string
	Arg []string
	Msg string
}

func New(AppInfo string, config config.Config, genMeta meta.Meta, dryrun bool) (*Generator, error) {
	g := Generator{
		Applications: make(ds.Apps, 0, len(config.Applications)),
		PostGenerate: make([]ExecCmd, 0, len(config.PostGenerate)),
		Transports:   make(ds.Transports),
		Workers:      make(ds.Workers),
		Drivers:      make(ds.Drivers),
		DryRun:       dryrun,
		Meta:         genMeta,
	}

	if err := g.processConfig(config); err != nil {
		return nil, err
	}

	return &g, nil
}

func (g *Generator) processConfig(config config.Config) error {
	g.Logger = config.Main.LoggerObj
	g.ProjectName = config.Main.Name
	g.RegistryType = config.Main.RegistryType
	g.Author = config.Main.Author
	g.DockerImagePrefix = config.Docker.ImagePrefix
	g.SkipInitService = config.Main.SkipServiceInit
	g.ProjectPath = config.Git.ModulePath
	g.UseActiveRecord = config.Main.UseActiveRecord
	g.Repo = config.Git.Repo
	g.GoLangVersion = config.Tools.GolangVersion
	g.OgenVersion = config.Tools.OgenVersion
	g.ArgenVersion = config.Tools.ArgenVersion
	g.GolangciVersion = config.Tools.GolangciVersion
	g.TargetDir = "./"

	if config.Deploy.LogCollector.Type != "" {
		g.Deploy.LogCollector.Type = config.Deploy.LogCollector.Type
		g.Deploy.LogCollector.Enabled = true
		g.Deploy.LogCollector.Parameters = config.Deploy.LogCollector.Parameters
	}

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

		if rest.GeneratorType != "ogen_client" && rest.Port == 0 {
			return fmt.Errorf("rest port is empty for %s", rest.Name)
		}

		transport := ds.Transport{
			Name:              rest.Name,
			PkgName:           fmt.Sprintf("%s_%s", rest.Name, rest.Version),
			Type:              ds.RestTransportType,
			GeneratorType:     rest.GeneratorType,
			HealthCheckPath:   rest.HealthCheckPath,
			GeneratorTemplate: rest.GeneratorTemplate,
			GeneratorParams:   rest.GeneratorParams,
			PublicService:     rest.PublicService,
		}

		if rest.GeneratorType == "ogen_client" {
			transport.Import = []string{fmt.Sprintf(`%s_%s "%s/pkg/rest/%s/%s"`, rest.Name, rest.Version, g.ProjectPath, rest.Name, rest.Version)} // ToDo точно ли нужен срез?
			// transport.Init = fmt.Sprintf(`rest.NewServer("%s_%s", &%s_%s.API{})`, rest.Name, rest.Version, rest.Name, rest.Version)
			transport.Handler = ds.NewHandler(rest.Name, rest.Version, strconv.FormatUint(uint64(rest.Port), 10))
			transport.SpecPath = paths
		} else {
			transport.Import = []string{fmt.Sprintf(`%s_%s "%s/internal/app/transport/rest/%s/%s"`, rest.Name, rest.Version, g.ProjectPath, rest.Name, rest.Version)} // ToDo точно ли нужен срез?
			transport.Init = fmt.Sprintf(`rest.NewServer("%s_%s", &%s_%s.API{})`, rest.Name, rest.Version, rest.Name, rest.Version)
			transport.Handler = ds.NewHandler(rest.Name, rest.Version, strconv.FormatUint(uint64(rest.Port), 10))
			transport.SpecPath = paths
		}

		if err := g.Transports.Add(rest.Name, transport); err != nil {
			return err
		}
	}

	for _, w := range config.WorkerList {
		if w.Name == "" {
			return errors.New("worker name is empty")
		}

		worker := ds.Worker{
			Import:            fmt.Sprintf(`"%s/internal/app/worker/%s"`, g.ProjectPath, w.Name),
			Name:              w.Name,
			GeneratorType:     w.GeneratorType,
			GeneratorTemplate: w.GeneratorTemplate,
			GeneratorParams:   w.GeneratorParams,
		}

		if err := g.Workers.Add(w.Name, worker); err != nil {
			return err
		}
	}

	for _, driver := range config.DriverList {
		if _, ex := g.Drivers[driver.Name]; ex {
			return fmt.Errorf("duplicate driver name: %s", driver.Name)
		}

		g.Drivers[driver.Name] = ds.Driver{
			Name:             driver.Name,
			Import:           driver.Import,
			Package:          driver.Package,
			ObjName:          driver.ObjName,
			ServiceInjection: driver.ServiceInjection,
		}
	}

	for _, grpc := range config.GrpcList {
		panic("Not implemented " + grpc.Name) //ToDo
	}

	for _, app := range config.Applications {
		application := ds.App{
			Name:       app.Name,
			Transports: make(ds.Transports),
			Workers:    make(ds.Workers),
			Drivers:    make(ds.Drivers),
		}

		if len(app.Deploy.Volumes) > 0 {
			application.Deploy.Volumes = make([]ds.DeployVolume, 0, len(app.Deploy.Volumes))
			for _, vol := range app.Deploy.Volumes {
				application.Deploy.Volumes = append(application.Deploy.Volumes, ds.DeployVolume{
					Path:  vol.Path,
					Mount: vol.Mount,
				})
			}
		}

		for _, transport := range app.TransportList {
			tr, ex := g.Transports[transport]
			if !ex {
				return fmt.Errorf("unknown transport: %s", transport)
			}

			application.Transports[transport] = tr
		}

		for _, worker := range app.WorkerList {
			w, ex := g.Workers[worker]
			if !ex {
				return fmt.Errorf("unknown worker: %s", worker)
			}

			application.Workers[worker] = w
		}

		for _, driver := range app.DriverList {
			dr, ex := g.Drivers[driver.Name]
			if !ex {
				return fmt.Errorf("unknown driver: %s", driver)
			}

			application.Drivers[driver.Name] = ds.Driver{
				Name:             dr.Name,
				Import:           dr.Import,
				Package:          dr.Package,
				ObjName:          dr.ObjName,
				ServiceInjection: dr.ServiceInjection,
				CreateParams:     driver.Params,
			}
		}

		g.Applications = append(g.Applications, application)
	}

	for _, postGenerate := range config.PostGenerate {
		switch postGenerate {
		case "git_install":
			g.PostGenerate = append(g.PostGenerate, ExecCmd{Cmd: "make", Arg: []string{"git-repo"}, Msg: "initialize git"})
		case "tools_install":
			g.PostGenerate = append(g.PostGenerate, ExecCmd{Cmd: "make", Arg: []string{"install-tools"}, Msg: "install tools"})
		case "clean_imports":
			g.PostGenerate = append(g.PostGenerate, ExecCmd{Cmd: "make", Arg: []string{"clean-import"}, Msg: "cleaning imports"})
		case "executable_scripts":
			g.PostGenerate = append(g.PostGenerate, ExecCmd{Cmd: "chmod", Arg: []string{"a+x", "scripts/goversioncheck.sh"}, Msg: "make scripts executable"})
		case "call_generate_mock":
			g.PostGenerate = append(g.PostGenerate, ExecCmd{Cmd: "make", Arg: []string{"mock"}, Msg: "generate"})
		case "go_mod_tidy":
			g.PostGenerate = append(g.PostGenerate, ExecCmd{Cmd: "make", Arg: []string{"tidy"}, Msg: "go mod tidy"})
		case "call_generate":
			g.PostGenerate = append(g.PostGenerate, ExecCmd{Cmd: "make", Arg: []string{"generate"}, Msg: "generate"})
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
		Logger:            g.Logger,
		ProjectName:       g.ProjectName,
		RegistryType:      g.RegistryType,
		Author:            g.Author,
		Year:              time.Now().Format("2006"),
		ProjectPath:       g.ProjectPath,
		UseActiveRecord:   g.UseActiveRecord,
		Repo:              g.Repo,
		DockerImagePrefix: g.DockerImagePrefix,
		SkipServiceInit:   g.SkipInitService,
		GoLangVersion:     g.GoLangVersion,
		OgenVersion:       g.OgenVersion,
		ArgenVersion:      g.ArgenVersion,
		GolangciVersion:   g.GolangciVersion,
		Applications:      g.Applications,
		Drivers:           g.Drivers,
		Workers:           g.Workers,
	}
}

func (g *Generator) GetTmplAppParams(app ds.App) templater.GeneratorAppParams {
	return templater.GeneratorAppParams{
		GeneratorParams: g.GetTmplParams(),
		Application:     app,
		Deploy:          g.Deploy,
	}
}

func (g *Generator) GetTmplHandlerParams(transport ds.Transport) templater.GeneratorHandlerParams {
	return templater.GeneratorHandlerParams{
		GeneratorParams: g.GetTmplParams(),
		Transport:       transport,
		TransportParams: transport.GeneratorParams,
	}
}

func (g *Generator) GetTmplRunnerParams(worker ds.Worker) templater.GeneratorRunnerParams {
	return templater.GeneratorRunnerParams{
		GeneratorParams: g.GetTmplParams(),
		Worker:          worker,
		WorkerParams:    worker.GeneratorParams,
	}
}

func (g *Generator) CopySpecs() error {
	for _, app := range g.Applications {
		for _, transport := range app.Transports {
			for _, spec := range transport.SpecPath {
				if _, err := os.Stat(spec); err != nil {
					return fmt.Errorf("spec file not found: %s", spec)
				}

				source := spec

				dest := filepath.Join(
					transport.Handler.GetTargetSpecDir(g.TargetDir),
					transport.GetTargetSpecFile(),
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
		return errors.Wrap(err, "Error target path")
	}

	dirs, files, err := g.collectFiles(targetPath)
	if err != nil {
		return errors.Wrap(err, "Error collect files")
	}

	filesDiff, err := templater.GetUserCodeFromFiles(g.TargetDir, files)
	if err != nil {
		return errors.Wrap(err, "Error get user code")
	}

	for i := range files {
		tmpl, err := templater.GetTemplate(files[i].SourceName)
		if err != nil {
			return fmt.Errorf("failed to get template %s: %w", files[i].SourceName, err)
		}

		files[i].Code, err = templater.GenerateByTmpl(tmpl, files[i].ParamsTmpl, filesDiff.UserContent[files[i].DestName], files[i].DestName)
		if err != nil {
			return errors.Wrap(err, "Error generate")
		}
	}

	if g.DryRun {
		for file := range filesDiff.IgnoreFiles {
			fmt.Printf("Ignore file: %s\n", file)
		}

		for file := range filesDiff.NewDirectory {
			fmt.Printf("Created new directory: %s\n", file)
		}

		for oldFile, newFile := range filesDiff.RenameFiles {
			fmt.Printf("Rename file: %s -> %s\n", oldFile, newFile)
		}

		for file := range filesDiff.NewFiles {
			fmt.Printf("Created new file: %s\n", file)
		}

		for file := range filesDiff.OtherDirectory {
			fmt.Printf("User dir: %s\n", file)
		}

		for file := range filesDiff.OtherFiles {
			fmt.Printf("User file: %s\n", file)
		}

		for file, content := range filesDiff.UserContent {
			fmt.Printf("Store user content in file: %s (len: %d)\n", file, len(content))
		}

		return nil
	}

	if err = tools.MakeDirs(dirs); err != nil {
		return errors.Wrap(err, "Error make dir")
	}

	for oldFile, newFile := range filesDiff.RenameFiles {
		st, err := os.Stat(oldFile)
		if err != nil {
			if _, ok := err.(*fs.PathError); ok {
				continue
			}

			return fmt.Errorf("error stat file %s: %w", oldFile, err)
		}

		log.Printf("stat: %+v -> %T\n", st, err)
		if st, err := os.Stat(newFile); err == nil && st.Name() == filepath.Base(newFile) {
			return errors.New("Want to rename but new file exists: " + newFile)
		}

		if err = os.Rename(oldFile, newFile); err != nil {
			return errors.Wrap(err, "Error rename old file")
		}
	}

	for _, file := range files {
		if _, ex := filesDiff.IgnoreFiles[file.DestName]; ex {
			continue
		}

		dstFile, err := os.Create(file.DestName)
		if err != nil {
			return errors.Wrap(err, "Error create file")
		}
		defer dstFile.Close()

		file.Code.WriteTo(dstFile)
	}

	if err = g.CopySpecs(); err != nil {
		return errors.Wrap(err, "Error copy spec")
	}

	if err = g.Meta.Save(); err != nil {
		return fmt.Errorf("error save meta: %w", err)
	}

	for _, procData := range g.PostGenerate {
		cmd := exec.Command(procData.Cmd, procData.Arg...)
		cmd.Dir = targetPath

		log.Printf("run: %s\n", procData.Msg)

		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("error run %s %s: %w (with output: %s)", procData.Cmd, strings.Join(procData.Arg, ", "), err, out)
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
				if transport.GeneratorType != "ogen_client" {
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
	}

	workerTypes := g.Workers.GetUniqueTypes()

	for tmplType, w := range workerTypes {
		dirsTr, filesTr, err := templater.GetWorkerTemplates(g.GetTmplParams())
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get worker templates: %w", err)
		}

		dirs = append(dirs, dirsTr...)
		files = append(files, filesTr...)

		dirsTrT, filesTrT, err := templater.GetWorkerGeneratorTemplates(tmplType, g.GetTmplParams())
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get worker generator templates: %w", err)
		}

		dirs = append(dirs, dirsTrT...)
		files = append(files, filesTrT...)

		for _, work := range w {
			dirsH, filesH, err := templater.GetWorkerRunnerTemplates(
				filepath.Join(work.GeneratorType, work.GeneratorTemplate),
				g.GetTmplRunnerParams(work),
			)
			if err != nil {
				return nil, nil, errors.Wrapf(err, "failed to get worker runner templates: `%s`, `%s`", work.GeneratorType, work.GeneratorTemplate)
			}

			dirs = append(dirs, dirsH...)
			files = append(files, filesH...)
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
	// ToDo validate checksum previous generated files

	for i := range dirs {
		if err := templater.GenerateFilenameByTmpl(&dirs[i], targetPath, g.Meta.Version); err != nil {
			return nil, nil, fmt.Errorf("failed to generate filename by template %s: %w", dirs[i].DestName, err)
		}
	}

	for i := range files {
		if err := templater.GenerateFilenameByTmpl(&files[i], targetPath, g.Meta.Version); err != nil {
			return nil, nil, fmt.Errorf("failed to generate filename by template %s: %w", files[i].DestName, err)
		}
	}

	return dirs, files, nil
}
