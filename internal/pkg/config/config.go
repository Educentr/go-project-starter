package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Educentr/go-project-starter/internal/pkg/loggers"
	"github.com/Educentr/go-project-starter/internal/pkg/migrate"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

func GetConfig(baseDir, configPath string) (Config, error) { // конструктор, принимает две строки конфигурации и отдает структуру Config и ошибку
	var config Config // объявлена переменная типа Config

	realConfigPath := configPath

	if !strings.Contains(configPath, "/") { // если строка с конфигурацией "configPath" не содержит "/", то
		realConfigPath = filepath.Join(baseDir, configPath) // объединяем пути "baseDir" и "configPath" в один "realConfigPath" в правильном формате для текущей операционной системы
	}

	viper.SetConfigFile(realConfigPath)          // Указываем путь к файлу конфигурации, который будет использоваться для загрузки настроек приложения
	if err := viper.ReadInConfig(); err != nil { // если получили ошибку при чтении конфигурации, то
		return config, err // останавливаем программу и отдаем структуру «config типа Config» и саму ошибку
	}

	viper.SetDefault("docker.image_prefix", "educentr") // устанавливаем значения по умолчанию для "docker.image_prefix"

	// post_generate defaults removed - now uses []string format, users must explicitly specify steps

	viper.SetDefault("tools.protobuf_version", defaultProtobufVersion)          // устанавливаем значения по умолчанию для "tools.protobuf_version"
	viper.SetDefault("tools.golang_version", defaultGolangVersion)              // устанавливаем значения по умолчанию для "tools.golang_version"
	viper.SetDefault("tools.ogen_version", defaultOgenVersion)                  // устанавливаем значения по умолчанию для "tools.ogen_version"
	viper.SetDefault("tools.argen_version", defaultArgenVersion)                // устанавливаем значения по умолчанию для "tools.argen_version"
	viper.SetDefault("tools.golangci_version", defaultGolangciVersion)          // устанавливаем значения по умолчанию для "tools.golangci_version"
	viper.SetDefault("tools.go_jsonschema_version", defaultGoJSONSchemaVersion) // устанавливаем значения по умолчанию для "tools.go_jsonschema_version"

	viper.SetDefault("main.author", "Unknown author") // устанавливаем значения по умолчанию для "main.author"

	viper.SetDefault("m.RegistryType", "github") // устанавливаем значения по умолчанию для "github"

	if err := viper.Unmarshal(&config); err != nil { // если при преобразовании данных из Viper в структуру config получили ошибку, то
		return config, err // останавливаем программу и отдаем структуру «config типа Config» и саму ошибку
	}

	if ok, msg := config.Main.IsValid(); !ok { // проверяем валидность конфигурации
		return config, errors.WithMessage(ErrInvalidConfig, "invalid config main section: "+msg)
	}

	// Валидация ArgenVersion когда use_active_record включен
	if config.Main.UseActiveRecord && len(config.Tools.ArgenVersion) == 0 {
		return config, errors.WithMessage(ErrInvalidConfig, "ArgenVersion required when use_active_record is true")
	}

	config.Main.LoggerObj = loggers.LoggerMapping[config.Main.Logger]

	// создаем мапки
	config.RestMap = make(map[string]Rest)
	config.GrpcMap = make(map[string]Grpc)
	config.DriverMap = make(map[string]Driver)
	config.WorkerMap = make(map[string]Worker)
	config.CLIMap = make(map[string]CLI)
	config.JSONSchemaMap = make(map[string]JSONSchema)
	config.KafkaMap = make(map[string]Kafka)
	config.GrafanaDatasourceMap = make(map[string]GrafanaDatasource)

	for i, rest := range config.RestList { // "rest" названия полей
		if ok, msg := rest.IsValid(baseDir); !ok { // проверяем валидность конфигурации
			return config, errors.WithMessage(ErrInvalidConfig, "invalid config rest section: "+msg)
		}

		if _, ex := config.RestMap[rest.Name]; ex { // проверка, есть ли уже такое имя
			return config, errors.WithMessage(ErrInvalidConfig, "duplicate rest name: "+rest.Name)
		}

		if rest.Version == "" { // если в переменной "rest" типа Rest поле "Version" типа string не задано (пустая строка)
			config.RestList[i].Version = "v1" // в переменную "config" типа Config в срез RestList по ключу [i] полю "Version" типа string присваиваем значение "v1"
		}

		config.RestMap[rest.Name] = rest // в переменной "config" типа Config в мапку "RestMap" по ключу [rest.Name] ложим переменную "rest" типа Rest
	}

	for _, grpc := range config.GrpcList {
		if ok, msg := grpc.IsValid(baseDir); !ok {
			return config, errors.WithMessage(ErrInvalidConfig, "invalid config grpc section: "+msg)
		}

		if _, ex := config.GrpcMap[grpc.Name]; ex {
			return config, errors.WithMessage(ErrInvalidConfig, "duplicate grpc name: "+grpc.Name)
		}

		config.GrpcMap[grpc.Name] = grpc
	}

	for _, driver := range config.DriverList {
		if ok, msg := driver.IsValid(); !ok {
			return config, errors.WithMessage(ErrInvalidConfig, "invalid config driver section: "+msg)
		}

		if _, ex := config.DriverMap[driver.Name]; ex {
			return config, errors.WithMessage(ErrInvalidConfig, "duplicate driver name: "+driver.Name)
		}

		config.DriverMap[driver.Name] = driver
	}

	for _, worker := range config.WorkerList {
		if ok, msg := worker.IsValid(baseDir); !ok {
			return config, errors.WithMessage(ErrInvalidConfig, "invalid config worker section: "+msg)
		}

		if _, ex := config.WorkerMap[worker.Name]; ex {
			return config, errors.WithMessage(ErrInvalidConfig, "duplicate worker name: "+worker.Name)
		}

		config.WorkerMap[worker.Name] = worker
	}

	for _, cli := range config.CLIList {
		if ok, msg := cli.IsValid(); !ok {
			return config, errors.WithMessage(ErrInvalidConfig, "invalid config cli section: "+msg)
		}

		if _, ex := config.CLIMap[cli.Name]; ex {
			return config, errors.WithMessage(ErrInvalidConfig, "duplicate cli name: "+cli.Name)
		}

		config.CLIMap[cli.Name] = cli
	}

	for _, js := range config.JSONSchemaList {
		if ok, msg := js.IsValid(baseDir); !ok {
			return config, errors.WithMessage(ErrInvalidConfig, "invalid config jsonschema section: "+msg)
		}

		if _, ex := config.JSONSchemaMap[js.Name]; ex {
			return config, errors.WithMessage(ErrInvalidConfig, "duplicate jsonschema name: "+js.Name)
		}

		config.JSONSchemaMap[js.Name] = js
	}

	for _, kafka := range config.KafkaList {
		if ok, msg := kafka.IsValid(config.JSONSchemaMap); !ok {
			return config, errors.WithMessage(ErrInvalidConfig, "invalid config kafka section: "+msg)
		}

		if _, ex := config.KafkaMap[kafka.Name]; ex {
			return config, errors.WithMessage(ErrInvalidConfig, "duplicate kafka name: "+kafka.Name)
		}

		config.KafkaMap[kafka.Name] = kafka
	}

	// Validate Grafana configuration
	if ok, msg := config.Grafana.IsValid(); !ok {
		return config, errors.WithMessage(ErrInvalidConfig, "invalid config grafana section: "+msg)
	}

	for _, ds := range config.Grafana.Datasources {
		config.GrafanaDatasourceMap[ds.Name] = ds
	}

	for i := range config.Applications {
		// Normalize transport list (supports both old string[] and new object[] format)
		if err := config.Applications[i].NormalizeTransports(); err != nil {
			appName := config.Applications[i].Name

			return config, errors.WithMessage(ErrInvalidConfig,
				fmt.Sprintf("application[%d] '%s': %s", i, appName, err.Error()))
		}

		app := config.Applications[i]

		if ok, msg := app.IsValid(); !ok {
			return config, errors.WithMessage(ErrInvalidConfig, "invalid config application section: "+msg)
		}

		// Валидация use_active_record: может быть только false (для отключения AR)
		if app.UseActiveRecord != nil && *app.UseActiveRecord == true {
			return config, errors.WithMessage(ErrInvalidConfig,
				"application '"+app.Name+"': use_active_record can only be set to false (to disable AR for specific app)")
		}

		// Валидация use_envs: может быть только true или nil, false запрещен
		if app.UseEnvs != nil && !*app.UseEnvs {
			return config, errors.WithMessage(ErrInvalidConfig,
				fmt.Sprintf("application[%d] '%s': use_envs can only be true or omitted, false is not allowed", i, app.Name))
		}

		for _, transport := range app.TransportList {
			_, exRest := config.RestMap[transport.Name]
			_, exGrpc := config.GrpcMap[transport.Name]

			if !exRest && !exGrpc {
				return config, errors.WithMessage(ErrInvalidConfig,
					"unknown transport: "+transport.Name+" in application: "+app.Name)
			}

			// Validate instantiation is only allowed for ogen_client
			if transport.Config.Instantiation != "" {
				rest, isRest := config.RestMap[transport.Name]
				if !isRest || rest.GeneratorType != GeneratorTypeOgenClient {
					return config, errors.WithMessage(ErrInvalidConfig,
						fmt.Sprintf("transport '%s' in application '%s': %s",
							transport.Name, app.Name, errInstantiationOnlyOgenClient))
				}
			}
		}

		for _, driver := range app.DriverList {
			if _, ex := config.DriverMap[driver.Name]; !ex {
				return config, errors.WithMessage(ErrInvalidConfig, "unknown driver: "+driver.Name+" in application: "+app.Name)
			}
		}

		for _, worker := range app.WorkerList {
			if _, ex := config.WorkerMap[worker]; !ex {
				return config, errors.WithMessage(ErrInvalidConfig, "unknown worker: "+worker+" in application: "+app.Name)
			}
		}

		// Validate CLI reference
		if app.CLI != "" {
			if _, ex := config.CLIMap[app.CLI]; !ex {
				return config, errors.WithMessage(ErrInvalidConfig, "unknown cli: "+app.CLI+" in application: "+app.Name)
			}
		}

		// Validate Kafka references
		for _, kafkaName := range app.KafkaList {
			if _, ex := config.KafkaMap[kafkaName]; !ex {
				return config, errors.WithMessage(ErrInvalidConfig, "unknown kafka: "+kafkaName+" in application: "+app.Name)
			}
		}

		// Validate Grafana datasource references
		for _, dsName := range app.Grafana.Datasources {
			if _, ex := config.GrafanaDatasourceMap[dsName]; !ex {
				return config, errors.WithMessage(ErrInvalidConfig, "unknown grafana datasource: "+dsName+" in application: "+app.Name)
			}
		}
	}

	// ToDo ws, ...

	// Validate dev_stand requires git_install in post_generate
	if config.Main.DevStand {
		hasGitInstall := false

		for _, pg := range config.PostGenerate {
			if pg == "git_install" {
				hasGitInstall = true

				break
			}
		}

		if !hasGitInstall {
			return config, errors.WithMessage(ErrInvalidConfig, "dev_stand requires 'git_install' in post_generate section")
		}
	}

	// Validate that all defined entities are used in at least one application
	if err := validateEntityUsage(&config); err != nil {
		return config, err
	}

	// Print deprecation warnings
	printDeprecationWarnings(&config)

	config.BasePath = baseDir
	config.ConfigFilePath = realConfigPath

	return config, nil
}

// collectDeprecationWarnings collects all deprecation warnings from config
func collectDeprecationWarnings(config *Config) []migrate.DeprecationWarning {
	var warnings []migrate.DeprecationWarning

	for _, app := range config.Applications {
		if app.HasDeprecatedFormat {
			warnings = append(warnings, migrate.DeprecationWarning{
				Feature:       "transport string array format",
				Description:   fmt.Sprintf("Application '%s' uses deprecated string array format for transports", app.Name),
				RemovalVer:    migrate.RemovalVersionTransportStringArray,
				MigrationHint: "Run 'go-project-starter migrate' to auto-migrate",
			})
		}
	}

	return warnings
}

// printDeprecationWarnings prints warnings for deprecated config formats
func printDeprecationWarnings(config *Config) {
	warnings := collectDeprecationWarnings(config)

	if len(warnings) == 0 {
		return
	}

	// Group warnings by removal version
	byVersion := make(map[string][]migrate.DeprecationWarning)
	for _, w := range warnings {
		byVersion[w.RemovalVer] = append(byVersion[w.RemovalVer], w)
	}

	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "⚠️  DEPRECATION WARNINGS:")
	fmt.Fprintln(os.Stderr, "========================")

	for version, versionWarnings := range byVersion {
		fmt.Fprintf(os.Stderr, "\nWill be REMOVED in version %s:\n", version)

		for _, w := range versionWarnings {
			fmt.Fprintf(os.Stderr, "  - %s\n", w.Description)
			fmt.Fprintf(os.Stderr, "    Migration: %s\n", w.MigrationHint)
		}
	}

	fmt.Fprintln(os.Stderr, "")
}

// validateEntityUsage checks that all defined entities (rest, grpc, kafka, drivers, workers)
// are referenced in at least one application
func validateEntityUsage(config *Config) error {
	usedTransports := make(map[string]bool)
	usedKafka := make(map[string]bool)
	usedDrivers := make(map[string]bool)
	usedWorkers := make(map[string]bool)
	usedCLI := make(map[string]bool)

	for _, app := range config.Applications {
		for _, t := range app.TransportList {
			usedTransports[t.Name] = true
		}

		for _, k := range app.KafkaList {
			usedKafka[k] = true
		}

		for _, d := range app.DriverList {
			usedDrivers[d.Name] = true
		}

		for _, w := range app.WorkerList {
			usedWorkers[w] = true
		}

		if app.CLI != "" {
			usedCLI[app.CLI] = true
		}
	}

	// Check rest
	for name := range config.RestMap {
		if !usedTransports[name] {
			return errors.WithMessage(ErrInvalidConfig, fmt.Sprintf("rest '%s' is not used in any application", name))
		}
	}

	// Check grpc
	for name := range config.GrpcMap {
		if !usedTransports[name] {
			return errors.WithMessage(ErrInvalidConfig, fmt.Sprintf("grpc '%s' is not used in any application", name))
		}
	}

	// Check kafka
	for name := range config.KafkaMap {
		if !usedKafka[name] {
			return errors.WithMessage(ErrInvalidConfig, fmt.Sprintf("kafka '%s' is not used in any application", name))
		}
	}

	// Check drivers
	for name := range config.DriverMap {
		if !usedDrivers[name] {
			return errors.WithMessage(ErrInvalidConfig, fmt.Sprintf("driver '%s' is not used in any application", name))
		}
	}

	// Check workers
	for name := range config.WorkerMap {
		if !usedWorkers[name] {
			return errors.WithMessage(ErrInvalidConfig, fmt.Sprintf("worker '%s' is not used in any application", name))
		}
	}

	// Check CLI
	for name := range config.CLIMap {
		if !usedCLI[name] {
			return errors.WithMessage(ErrInvalidConfig, fmt.Sprintf("cli '%s' is not used in any application", name))
		}
	}

	return nil
}
