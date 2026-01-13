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

	"github.com/Educentr/go-project-starter/internal/pkg/config"
	"github.com/Educentr/go-project-starter/internal/pkg/ds"
	"github.com/Educentr/go-project-starter/internal/pkg/grafana"
	"github.com/Educentr/go-project-starter/internal/pkg/meta"
	"github.com/Educentr/go-project-starter/internal/pkg/templater"
	"github.com/Educentr/go-project-starter/internal/pkg/tools"
	"github.com/pkg/errors"
)

type Generator struct {
	AppInfo             string
	DryRun              bool
	Meta                meta.Meta
	Logger              ds.Logger
	ProjectName         string
	Deploy              ds.DeployType
	RegistryType        string
	Author              string
	ProjectPath         string
	UseActiveRecord     bool
	DevStand            bool
	Repo                string
	PrivateRepos        string
	GoLangVersion       string
	OgenVersion         string
	ArgenVersion        string
	GolangciVersion     string
	RuntimeVersion      string
	GoJSONSchemaVersion string
	TargetDir           string
	ConfigPath          string // Source config file path for copying to target
	DockerImagePrefix   string
	SkipInitService     bool
	PostGenerate        []ExecCmd
	Transports          ds.Transports
	Workers             ds.Workers
	Drivers             ds.Drivers
	JSONSchemas         ds.JSONSchemas
	Kafka               ds.KafkaConfigs
	Applications        ds.Apps
	Grafana             grafana.Config
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
		JSONSchemas:  make(ds.JSONSchemas),
		Kafka:        make(ds.KafkaConfigs),
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
	g.DevStand = config.Main.DevStand
	g.Repo = config.Git.Repo
	g.PrivateRepos = config.Git.PrivateRepos
	g.GoLangVersion = config.Tools.GolangVersion
	g.OgenVersion = config.Tools.OgenVersion
	g.ArgenVersion = config.Tools.ArgenVersion
	g.GolangciVersion = config.Tools.GolangciVersion
	g.GoJSONSchemaVersion = config.Tools.GoJSONSchemaVersion

	// Set RuntimeVersion: use config value if provided, otherwise use MinRuntimeVersion
	if config.Tools.RuntimeVersion != "" {
		// Validate that config version >= MinRuntimeVersion
		if config.Tools.RuntimeVersion < templater.MinRuntimeVersion {
			return fmt.Errorf("runtime_version %s is lower than minimum required version %s", config.Tools.RuntimeVersion, templater.MinRuntimeVersion)
		}
		g.RuntimeVersion = config.Tools.RuntimeVersion
	} else {
		g.RuntimeVersion = templater.MinRuntimeVersion
	}

	g.TargetDir = "./"
	g.ConfigPath = config.ConfigFilePath

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
			AuthParams: ds.AuthParams{
				Transport: rest.AuthParams.Transport,
				Type:      rest.AuthParams.Type,
			},
			PublicService:        rest.PublicService,
			EmptyConfigAvailable: rest.EmptyConfigAvailable,
		}

		if rest.GeneratorType == "ogen_client" {
			transport.Import = []string{
				fmt.Sprintf(`%s_%s "%s/pkg/rest/%s/%s"`, rest.Name, rest.Version, g.ProjectPath, rest.Name, rest.Version),
				fmt.Sprintf(`"%s/internal/app/transport/rest/%s/%s"`, g.ProjectPath, rest.Name, rest.Version),
			}
			transport.Name = rest.Name
			transport.ApiVersion = rest.Version
			transport.Port = strconv.FormatUint(uint64(rest.Port), 10)
			transport.SpecPath = paths
		} else {
			transport.Import = []string{fmt.Sprintf(`%s_%s "%s/internal/app/transport/rest/%s/%s"`, rest.Name, rest.Version, g.ProjectPath, rest.Name, rest.Version)} // ToDo точно ли нужен срез?
			transport.Init = fmt.Sprintf(`rest.NewServer("%s_%s", &%s_%s.API{})`, rest.Name, rest.Version, rest.Name, rest.Version)
			transport.Name = rest.Name
			transport.ApiVersion = rest.Version
			transport.Port = strconv.FormatUint(uint64(rest.Port), 10)
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
			Import:            []string{fmt.Sprintf(`"%s/internal/app/worker/%s"`, g.ProjectPath, w.Name)},
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
		if grpc.Name == "" {
			return errors.New("grpc name is empty")
		}

		paths := []string{filepath.Join(config.BasePath, grpc.Path)}

		transport := ds.Transport{
			Name:                 grpc.Name,
			PkgName:              grpc.Name,
			Type:                 ds.GrpcTransportType,
			GeneratorType:        grpc.GeneratorType,
			Port:                 strconv.FormatUint(uint64(grpc.Port), 10),
			SpecPath:             paths,
			EmptyConfigAvailable: grpc.EmptyConfigAvailable,
			BufLocalPlugins:      grpc.BufLocalPlugins,
		}

		if grpc.GeneratorType == "buf_client" {
			transport.Import = []string{
				fmt.Sprintf(`"%s/internal/app/transport/grpc/%s"`, g.ProjectPath, grpc.Name),
			}
		}

		if err := g.Transports.Add(grpc.Name, transport); err != nil {
			return err
		}
	}

	// Process Grafana datasources
	for _, cfgDs := range config.Grafana.Datasources {
		g.Grafana.Datasources = append(g.Grafana.Datasources, grafana.Datasource{
			Name:      cfgDs.Name,
			Type:      cfgDs.Type,
			Access:    cfgDs.Access,
			URL:       cfgDs.URL,
			IsDefault: cfgDs.IsDefault,
			Editable:  cfgDs.Editable,
			UID:       grafana.GenerateDatasourceUID(cfgDs.Name),
		})
	}

	// Process JSON Schema configurations
	for _, js := range config.JSONSchemaList {
		if js.Name == "" {
			return errors.New("jsonschema name is empty")
		}

		schema := ds.JSONSchema{
			Name:    js.Name,
			Package: js.Package,
		}

		// Support both legacy path[] and new schemas[] format
		if len(js.Schemas) > 0 {
			schema.Schemas = make([]ds.JSONSchemaItem, 0, len(js.Schemas))
			for _, s := range js.Schemas {
				schemaType := s.Type
				if schemaType == "" {
					// Auto-calculate type from filename: abonent.user.schema.json → AbonentUserSchemaJson
					schemaType = filenameToTypeName(s.Path)
				}
				schema.Schemas = append(schema.Schemas, ds.JSONSchemaItem{
					ID:   s.ID,
					Path: filepath.Join(config.BasePath, s.Path),
					Type: schemaType,
				})
			}
		} else {
			// Legacy format: plain path list
			schema.Path = make([]string, 0, len(js.Path))
			for _, path := range js.Path {
				schema.Path = append(schema.Path, filepath.Join(config.BasePath, path))
			}
		}

		g.JSONSchemas[js.Name] = schema
	}

	// Process Kafka configurations
	for _, kafka := range config.KafkaList {
		if kafka.Name == "" {
			return errors.New("kafka name is empty")
		}

		driver := kafka.Driver
		if driver == "" {
			driver = "segmentio" // default driver
		}

		topics := make([]ds.KafkaTopic, 0, len(kafka.Topics))

		for _, t := range kafka.Topics {
			topic := ds.KafkaTopic{
				ID:     t.ID,
				Name:   t.Name,
				Schema: t.Schema,
			}

			// Compute GoType and GoImport from Schema field (format: "jsonschema_name.schema_id")
			// If Schema is empty, topic will use raw []byte
			if t.Schema != "" {
				parts := strings.SplitN(t.Schema, ".", 2)
				if len(parts) == 2 {
					schemaSetName := parts[0]
					schemaID := parts[1]
					// Lookup jsonschema by name
					if schemaSet, exists := g.JSONSchemas[schemaSetName]; exists {
						// Find schema item by ID
						for _, item := range schemaSet.Schemas {
							if item.ID == schemaID {
								topic.GoImport = g.ProjectPath + "/pkg/schema/" + schemaSet.Name
								topic.GoType = schemaSet.GetPackageName() + "." + item.Type
								break
							}
						}
					}
				}
			}

			topics = append(topics, topic)
		}

		kafkaConfig := ds.KafkaConfig{
			Name:          kafka.Name,
			Type:          kafka.Type,
			Driver:        driver,
			DriverImport:  kafka.DriverImport,
			DriverPackage: kafka.DriverPackage,
			DriverObj:     kafka.DriverObj,
			ClientName:    kafka.Client,
			Group:         kafka.Group,
			Topics:        topics,
		}

		g.Kafka[kafka.Name] = kafkaConfig
	}

	for _, app := range config.Applications {
		// Вычисляем use_active_record для приложения
		// Default из main, override может быть только false
		useActiveRecord := config.Main.UseActiveRecord
		if app.UseActiveRecord != nil {
			useActiveRecord = *app.UseActiveRecord // будет только false (валидация проверила)
		}

		// Вычисляем use_envs для приложения
		// Default false, может быть установлено в true
		useEnvs := false
		if app.UseEnvs != nil && *app.UseEnvs {
			useEnvs = true
		}

		// Вычисляем goat_tests для приложения
		// Default false, может быть установлено в true
		goatTests := false
		if app.GoatTests != nil && *app.GoatTests {
			goatTests = true
		}

		// Process GoatTestsConfig if provided
		var goatTestsConfig *ds.GoatTestsConfig
		if app.GoatTestsConfig != nil && app.GoatTestsConfig.Enabled {
			goatTests = true // Enable goatTests if config is provided
			binaryPath := app.GoatTestsConfig.BinaryPath
			if binaryPath == "" {
				binaryPath = fmt.Sprintf("/tmp/%s", app.Name)
			}
			goatTestsConfig = &ds.GoatTestsConfig{
				Enabled:    app.GoatTestsConfig.Enabled,
				BinaryPath: binaryPath,
			}
		}

		application := ds.App{
			Name:                  app.Name,
			Transports:            make(ds.Transports),
			Workers:               make(ds.Workers),
			Drivers:               make(ds.Drivers),
			Kafka:                 make(ds.KafkaConfigs),
			UseActiveRecord:       useActiveRecord,
			DependsOnDockerImages: app.DependsOnDockerImages,
			UseEnvs:               useEnvs,
			GoatTests:             goatTests,
			GoatTestsConfig:       goatTestsConfig,
		}

		// Resolve Grafana datasources for this app
		for _, dsName := range app.Grafana.Datasources {
			for _, globalDs := range g.Grafana.Datasources {
				if globalDs.Name == dsName {
					application.Grafana.Datasources = append(application.Grafana.Datasources, globalDs)
					break
				}
			}
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

		// Set CLI if this is a CLI application
		if app.CLI != "" {
			cli, ex := config.CLIMap[app.CLI]
			if !ex {
				return fmt.Errorf("unknown cli: %s", app.CLI)
			}

			application.CLI = &ds.CLIApp{
				Name:              cli.Name,
				Import:            fmt.Sprintf(`"%s/internal/app/transport/cli/%s"`, g.ProjectPath, cli.Name),
				Init:              fmt.Sprintf(`cli%s.NewHandler(srv)`, strings.Title(cli.Name)),
				GeneratorType:     cli.GeneratorType,
				GeneratorTemplate: cli.GeneratorTemplate,
				GeneratorParams:   cli.GeneratorParams,
			}
		}

		// Add Kafka producers/consumers to this application
		for _, kafkaName := range app.KafkaList {
			kafka, ex := g.Kafka[kafkaName]
			if !ex {
				return errors.Errorf("unknown kafka: %s in application: %s", kafkaName, app.Name)
			}

			application.Kafka[kafkaName] = kafka
		}

		g.Applications = append(g.Applications, application)
	}

	for _, postGenerate := range config.PostGenerate {
		switch postGenerate {
		case "git_install":
			g.PostGenerate = append(g.PostGenerate, ExecCmd{Cmd: "make", Arg: []string{"git-init"}, Msg: "initialize git"})
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
		case "git_initial_commit":
			g.PostGenerate = append(g.PostGenerate, ExecCmd{Cmd: "git", Arg: []string{"add", "."}, Msg: "git add ."})
			g.PostGenerate = append(g.PostGenerate, ExecCmd{Cmd: "git", Arg: []string{"commit", "-m", "Initial commit"}, Msg: "git initial commit"})
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
		Logger:              g.Logger,
		ProjectName:         g.ProjectName,
		RegistryType:        g.RegistryType,
		Author:              g.Author,
		Year:                time.Now().Format("2006"),
		ProjectPath:         g.ProjectPath,
		UseActiveRecord:     g.UseActiveRecord,
		DevStand:            g.DevStand,
		Repo:                g.Repo,
		PrivateRepos:        g.PrivateRepos,
		DockerImagePrefix:   g.DockerImagePrefix,
		SkipServiceInit:     g.SkipInitService,
		GoLangVersion:       g.GoLangVersion,
		OgenVersion:         g.OgenVersion,
		ArgenVersion:        g.ArgenVersion,
		GolangciVersion:     g.GolangciVersion,
		RuntimeVersion:      g.RuntimeVersion,
		GoJSONSchemaVersion: g.GoJSONSchemaVersion,
		Applications:        g.Applications,
		Drivers:             g.Drivers,
		Workers:             g.Workers,
		JSONSchemas:         g.JSONSchemas,
		Kafka:               g.Kafka,
		Grafana:             g.Grafana,
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
			for specNum, spec := range transport.SpecPath {
				if _, err := os.Stat(spec); err != nil {
					return fmt.Errorf("spec file not found: %s", spec)
				}

				source := spec

				dest := filepath.Join(
					transport.GetTargetSpecDir(g.TargetDir),
					transport.GetTargetSpecFile(specNum),
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

func (g *Generator) CopySchemas() error {
	for _, schema := range g.JSONSchemas {
		targetDir := schema.GetTargetSpecDir(g.TargetDir)

		// Ensure target directory exists
		if err := os.MkdirAll(targetDir, tools.DefaultDirPerm); err != nil {
			return fmt.Errorf("failed to create schema directory %s: %w", targetDir, err)
		}

		// Collect paths from both legacy Path[] and new Schemas[]
		var paths []string
		if len(schema.Schemas) > 0 {
			for _, item := range schema.Schemas {
				paths = append(paths, item.Path)
			}
		} else {
			paths = schema.Path
		}

		for _, schemaPath := range paths {
			if _, err := os.Stat(schemaPath); err != nil {
				return errors.Wrapf(err, "schema file not found: %s", schemaPath)
			}

			_, fileName := filepath.Split(schemaPath)
			dest := filepath.Join(targetDir, fileName)

			log.Printf("copy schema: `%s` to `%s`\n", schemaPath, dest)

			if err := tools.CopyFile(schemaPath, dest); err != nil {
				return err
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

	if err = g.CopySchemas(); err != nil {
		return errors.Wrap(err, "Error copy schemas")
	}

	// Create .project-config directory in target for meta.yaml and config
	projectConfigDir := filepath.Join(targetPath, ".project-config")
	if err = os.MkdirAll(projectConfigDir, tools.DefaultDirPerm); err != nil {
		return fmt.Errorf("error creating .project-config directory: %w", err)
	}

	// Copy config file to target's .project-config for regeneration support
	if g.ConfigPath != "" {
		targetConfigPath := filepath.Join(projectConfigDir, "project.yaml")
		// Only copy if source is different from target
		if g.ConfigPath != targetConfigPath {
			if err = tools.CopyFile(g.ConfigPath, targetConfigPath); err != nil {
				return fmt.Errorf("error copying config to target: %w", err)
			}

			log.Printf("copy config: `%s` to `%s`", g.ConfigPath, targetConfigPath)
		}
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

	// Add onlineconf submodule when dev_stand is true (skip if already exists)
	if g.DevStand {
		submodulePath := filepath.Join(targetPath, "etc/repo-oc")

		if _, err := os.Stat(submodulePath); os.IsNotExist(err) {
			// Add submodule
			cmd := exec.Command("git", "submodule", "add", "--depth", "1",
				"https://github.com/onlineconf/onlineconf", "etc/repo-oc")
			cmd.Dir = targetPath

			log.Println("run: add onlineconf submodule")

			out, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("error adding submodule: %w (output: %s)", err, out)
			}

			// Note: Using default branch (main) which contains the node:18 fix
			// Tag v3.5.0 has a bug with FROM node (uses latest which is v25, incompatible with postcss)
		} else {
			log.Println("skip: onlineconf submodule already exists")
		}

		// Create initial commit so that git HEAD works for docker builds
		// Check if HEAD exists (i.e., there are commits)
		checkCmd := exec.Command("git", "rev-parse", "HEAD")
		checkCmd.Dir = targetPath

		if err := checkCmd.Run(); err != nil {
			// No commits yet, create initial commit
			cmd := exec.Command("git", "add", ".")
			cmd.Dir = targetPath

			log.Println("run: git add .")

			out, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("error git add: %w (output: %s)", err, out)
			}

			// Use -c to set author/committer for this commit only (works without global git config)
			cmd = exec.Command("git",
				"-c", "user.name=go-project-starter",
				"-c", "user.email=go-project-starter@localhost",
				"commit", "-m", "Initial commit (auto-generated by go-project-starter)")
			cmd.Dir = targetPath

			log.Println("run: git commit -m 'Initial commit'")

			out, err = cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("error git commit: %w (output: %s)", err, out)
			}
		} else {
			log.Println("skip: git repository already has commits")
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
			for _, transport := range tr {
				dirsTrT, filesTrT, err := templater.GetTransportGeneratorTemplates(transportType, tmplType, g.GetTmplHandlerParams(transport))
				if err != nil {
					return nil, nil, fmt.Errorf("failed to get transport generator templates: %w", err)
				}

				dirs = append(dirs, dirsTrT...)
				files = append(files, filesTrT...)

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

	// Generate Kafka driver templates for segmentio drivers
	for _, kafka := range g.Kafka {
		dirsK, filesK, err := templater.GetKafkaDriverTemplates(kafka, g.GetTmplParams())
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get kafka driver templates for %s: %w", kafka.Name, err)
		}

		dirs = append(dirs, dirsK...)
		files = append(files, filesK...)
	}

	for _, app := range g.Applications {
		dirApp, filesApp, err := templater.GetAppTemplates(g.GetTmplAppParams(app))
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get app templates: %w", err)
		}

		dirs = append(dirs, dirApp...)
		files = append(files, filesApp...)

		// Generate CLI handler templates for CLI apps
		if app.IsCLI() {
			dirsCLI, filesCLI, err := templater.GetCLIHandlerTemplates(app.CLI, g.GetTmplParams())
			if err != nil {
				return nil, nil, fmt.Errorf("failed to get CLI handler templates: %w", err)
			}

			dirs = append(dirs, dirsCLI...)
			files = append(files, filesCLI...)
		}

		// Generate GOAT test templates for applications with goat_tests enabled
		if app.GoatTests && !app.IsCLI() {
			dirsTest, filesTest, err := templater.GetTestTemplates(g.GetTmplAppParams(app))
			if err != nil {
				return nil, nil, fmt.Errorf("failed to get test templates for %s: %w", app.Name, err)
			}

			dirs = append(dirs, dirsTest...)
			files = append(files, filesTest...)

			// Generate mock templates for applications with ogen_clients
			if app.HasOgenClients() {
				dirsMock, filesMock, err := templater.GetMockTemplates(g.GetTmplAppParams(app))
				if err != nil {
					return nil, nil, fmt.Errorf("failed to get mock templates for %s: %w", app.Name, err)
				}

				dirs = append(dirs, dirsMock...)
				files = append(files, filesMock...)
			}
		}
	}

	// Generate Grafana templates if any datasources are configured
	if g.Grafana.HasDatasources() {
		// Generate global provisioning templates (datasources and dashboard provider config)
		dirsGrafana, filesGrafana, err := templater.GetGrafanaProvisioningTemplates(g.GetTmplParams())
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get grafana provisioning templates: %w", err)
		}

		dirs = append(dirs, dirsGrafana...)
		files = append(files, filesGrafana...)

		// Generate dashboard for each application that has Grafana datasources
		for _, app := range g.Applications {
			if app.Grafana.HasDatasources() {
				dirsAppGrafana, filesAppGrafana, err := templater.GetGrafanaDashboardTemplates(g.GetTmplAppParams(app))
				if err != nil {
					return nil, nil, fmt.Errorf("failed to get grafana dashboard templates for %s: %w", app.Name, err)
				}

				dirs = append(dirs, dirsAppGrafana...)
				files = append(files, filesAppGrafana...)
			}
		}

		// Generate Prometheus config for dev environment if prometheus datasource exists
		if g.Grafana.HasPrometheus() {
			dirsProm, filesProm, err := templater.GetPrometheusTemplates(g.GetTmplParams())
			if err != nil {
				return nil, nil, fmt.Errorf("failed to get prometheus templates: %w", err)
			}

			dirs = append(dirs, dirsProm...)
			files = append(files, filesProm...)
		}

		// Generate Loki config for dev environment if loki datasource exists
		if g.Grafana.HasLoki() {
			dirsLoki, filesLoki, err := templater.GetLokiTemplates(g.GetTmplParams())
			if err != nil {
				return nil, nil, fmt.Errorf("failed to get loki templates: %w", err)
			}

			dirs = append(dirs, dirsLoki...)
			files = append(files, filesLoki...)
		}
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

// filenameToTypeName converts a schema filename to a Go type name
// Example: abonent.user.schema.json → AbonentUserSchemaJson
func filenameToTypeName(path string) string {
	// Get base filename (keep .json extension for type name)
	base := filepath.Base(path)

	// Split by dots and dashes, convert each part to title case
	var result strings.Builder
	parts := strings.FieldsFunc(base, func(r rune) bool {
		return r == '.' || r == '-' || r == '_'
	})

	for _, part := range parts {
		if len(part) > 0 {
			result.WriteString(strings.ToUpper(string(part[0])))
			result.WriteString(strings.ToLower(part[1:]))
		}
	}

	return result.String()
}
