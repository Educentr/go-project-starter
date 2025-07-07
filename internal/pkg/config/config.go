package config

import (
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"gitlab.educentr.info/golang/service-starter/internal/pkg/loggers"
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

	viper.SetDefault("post_generate.git_install", true)        // устанавливаем значения по умолчанию для "post_generate.git_install"
	viper.SetDefault("post_generate.tools_install", true)      // устанавливаем значения по умолчанию для "post_generate.tools_install"
	viper.SetDefault("post_generate.clean_imports", true)      // устанавливаем значения по умолчанию для "post_generate.clean_imports"
	viper.SetDefault("post_generate.executable_scripts", true) // устанавливаем значения по умолчанию для "post_generate.executable_scripts"
	viper.SetDefault("post_generate.call_generate", true)      // устанавливаем значения по умолчанию для "post_generate.call_generate"
	viper.SetDefault("post_generate.go_mod_tidy", true)        // устанавливаем значения по умолчанию для "post_generate.go_mod_tidy"

	viper.SetDefault("tools.protobuf_version", defaultProtobufVersion) // устанавливаем значения по умолчанию для "tools.protobuf_version"
	viper.SetDefault("tools.golang_version", defaultGolangVersion)     // устанавливаем значения по умолчанию для "tools.golang_version"
	viper.SetDefault("tools.ogen_version", defaultOgenVersion)         // устанавливаем значения по умолчанию для "tools.ogen_version"
	viper.SetDefault("tools.argen_version", defaultArgenVersion)       // устанавливаем значения по умолчанию для "tools.argen_version"
	viper.SetDefault("tools.golangci_version", defaultGolangciVersion) // устанавливаем значения по умолчанию для "tools.golangci_version"

	viper.SetDefault("main.author", "Unknown author") // устанавливаем значения по умолчанию для "main.author"

	viper.SetDefault("m.RegistryType", "github") // устанавливаем значения по умолчанию для "github"

	if err := viper.Unmarshal(&config); err != nil { // если при преобразовании данных из Viper в структуру config получили ошибку, то
		return config, err // останавливаем программу и отдаем структуру «config типа Config» и саму ошибку
	}

	if ok, msg := config.Main.IsValid(); !ok { // проверяем валидность конфигурации
		return config, errors.WithMessage(ErrInvalidConfig, "invalid config main section: "+msg)
	}

	config.Main.LoggerObj = loggers.LoggerMapping[config.Main.Logger]

	// создаем мапки
	config.RestMap = make(map[string]Rest)
	config.GrpcMap = make(map[string]Grpc)
	config.DriverMap = make(map[string]Driver)
	config.WorkerMap = make(map[string]Worker)

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

	for _, app := range config.Applications {
		if ok, msg := app.IsValid(); !ok {
			return config, errors.WithMessage(ErrInvalidConfig, "invalid config application section: "+msg)
		}

		for _, transport := range app.TransportList {
			_, exRest := config.RestMap[transport]
			_, exGrpc := config.GrpcMap[transport]

			if !exRest && !exGrpc {
				return config, errors.WithMessage(ErrInvalidConfig, "unknown transport: "+transport+" in application: "+app.Name)
			}
		}

		for _, driver := range app.DriverList {
			if _, ex := config.DriverMap[driver.Name]; !ex {
				return config, errors.WithMessage(ErrInvalidConfig, "unknown driver: "+driver.Name+" in application: "+app.Name)
			}
		}
	}

	// ToDo ws, ...

	config.BasePath = baseDir

	return config, nil
}
