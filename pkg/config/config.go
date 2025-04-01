package config

import (
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"gitlab.educentr.info/golang/service-starter/pkg/loggers"
)

func GetConfig(baseDir, configPath string) (Config, error) {
	var config Config

	realConfigPath := configPath

	if !strings.Contains(configPath, "/") {
		realConfigPath = filepath.Join(baseDir, configPath)
	}

	viper.SetConfigFile(realConfigPath)
	if err := viper.ReadInConfig(); err != nil {
		return config, err
	}

	viper.SetDefault("docker.image_prefix", "educentr")

	viper.SetDefault("post_generate.git_install", true)
	viper.SetDefault("post_generate.tools_install", true)
	viper.SetDefault("post_generate.clean_imports", true)
	viper.SetDefault("post_generate.executable_scripts", true)
	viper.SetDefault("post_generate.call_generate", true)
	viper.SetDefault("post_generate.go_mod_tidy", true)

	viper.SetDefault("tools.protobuf_version", defaultProtobufVersion)
	viper.SetDefault("tools.golang_version", defaultGolangVersion)
	viper.SetDefault("tools.ogen_version", defaultOgenVersion)
	viper.SetDefault("tools.argen_version", defaultArgenVersion)
	viper.SetDefault("tools.golangci_version", defaultGolangciVersion)

	viper.SetDefault("main.author", "Unknown author")

	if err := viper.Unmarshal(&config); err != nil {
		return config, err
	}

	if ok, msg := config.Main.IsValid(); !ok {
		return config, errors.WithMessage(ErrInvalidConfig, "invalid config main section: "+msg)
	}

	config.Main.LoggerObj = loggers.LoggerMapping[config.Main.Logger]

	config.RestMap = make(map[string]Rest)
	config.GrpcMap = make(map[string]Grpc)
	config.DriverMap = make(map[string]Driver)
	config.WorkerMap = make(map[string]Worker)

	for i, rest := range config.RestList {
		if ok, msg := rest.IsValid(baseDir); !ok {
			return config, errors.WithMessage(ErrInvalidConfig, "invalid config rest section: "+msg)
		}

		if _, ex := config.RestMap[rest.Name]; ex {
			return config, errors.WithMessage(ErrInvalidConfig, "duplicate rest name: "+rest.Name)
		}

		if rest.Version == "" {
			config.RestList[i].Version = "v1"
		}

		config.RestMap[rest.Name] = rest
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
			if _, ex := config.DriverMap[driver]; !ex {
				return config, errors.WithMessage(ErrInvalidConfig, "unknown driver: "+driver+" in application: "+app.Name)
			}
		}
	}

	// ToDo ws, ...

	config.BasePath = baseDir

	return config, nil
}
