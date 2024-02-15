package generator

import (
	"os"

	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

type (
	Main struct {
		Name      string `mapstructure:"name"`
		Logger    string `mapstructure:"logger"`
		TargetDir string
	}

	Scheduler struct {
		Enabled bool `mapstructure:"enabled"`
	}

	Git struct {
		ModulePath string `mapstructure:"module_path"`
		// ProjectID  uint   `mapstructure:"project_id"` Todo
	}

	Tools struct {
		ProtobufVersion string `mapstructure:"protobuf_version"`
		GolangVersion   string `mapstructure:"golang_version"`
		OgenVersion     string `mapstructure:"ogen_version"`
		GolangciVersion string `mapstructure:"golangci_version"`
	}

	ConfigPorts struct {
		Grpc uint `mapstructure:"grpc"`
		Rest uint `mapstructure:"rest"`
		Sys  uint `mapstructure:"sys"`
	}

	Rest struct {
		Name      string `mapstructure:"name"`
		Path      string `mapstructure:"path"`
		APIPrefix string `mapstructure:"api_prefix"`
		Port      uint   `mapstructure:"port"`
	}

	Grpc struct {
		Name  string `mapstructure:"name"`
		Path  string `mapstructure:"path"`
		Short string `mapstructure:"short"`
		Port  uint   `mapstructure:"port"`
	}

	Ws struct {
		Name string `mapstructure:"name"`
		Path string `mapstructure:"path"`
		Port uint   `mapstructure:"port"`
	}

	Repository struct {
		Name     string   `mapstructure:"name"`
		TypeDB   TypeDB   `mapstructure:"type_db"`
		DriverDB DriverDB `mapstructure:"driver_db"`
	}

	Consumer struct {
		Name    string  `mapstructure:"name"`
		Path    string  `mapstructure:"path"`
		Backend BufType `mapstructure:"backend"`
		Group   string  `mapstructure:"group"`
		Topic   string  `mapstructure:"topic"`
	}

	RestList       []Rest
	GrpcList       []Grpc
	WsList         []Ws
	RepositoryList []Repository
	ConsumerList   []Consumer

	Application struct {
		Name         string   `mapstructure:"name"`
		RestList     []string `mapstructure:"rest"`
		GrpcList     []string `mapstructure:"grpc"`
		WsList       []string `mapstructure:"ws"`
		ConsumerList []string `mapstructure:"consumer"`
	}

	Config struct {
		Main           Main           `mapstructure:"main"`
		PostGenerate   []string       `mapstructure:"post_generate"`
		Git            Git            `mapstructure:"git"`
		Tools          Tools          `mapstructure:"tools"`
		RepositoryList RepositoryList `mapstructure:"repository"`
		Scheduler      Scheduler      `mapstructure:"scheduler"`
		RestList       RestList       `mapstructure:"rest"`
		GrpcList       GrpcList       `mapstructure:"grpc"`
		WsList         WsList         `mapstructure:"ws"`
		ConsumerList   ConsumerList   `mapstructure:"consumer"`
		Applications   []Application  `mapstructure:"applications"`

		restMap map[string]Rest
		grpcMap map[string]Grpc
	}
)

func (c Config) SetTargetDir(dir string) { c.Main.TargetDir = dir }

func (m Main) IsValid() bool        { return len(m.Name) > 0 && (m.Logger == "zerolog") }
func (g Git) IsValid() bool         { return len(g.ModulePath) > 0 }
func (p ConfigPorts) IsValid() bool { return p.Grpc > 0 && p.Rest > 0 && p.Sys > 0 }
func (r Rest) IsValid() bool {
	return len(r.Name) > 0 && ((len(r.Path) > 0 && fileExists(r.Path) == retExist) || r.Name == "sys")
}
func (g Grpc) IsValid() bool {
	return len(g.Name) > 0 && len(g.Path) > 0 && fileExists(g.Path) == retExist
}
func (w Ws) IsValid() bool { return len(w.Name) > 0 && len(w.Path) > 0 }
func (a Application) IsValid() bool {
	return len(a.Name) > 0 && len(a.RestList)+len(a.GrpcList)+len(a.WsList)+len(a.ConsumerList) > 0
}

type retFileExist error

var retExist retFileExist = errors.New("file exist")
var retErr retFileExist = errors.New("invalid file")

func fileExists(filename string) retFileExist {
	info, err := os.Stat(filename)
	if err != nil {
		return err
	}

	if info.Mode().IsRegular() {
		return retExist
	}

	return retErr
}

func (r Repository) IsValid() bool {
	return len(r.Name) > 0 && len(r.TypeDB) > 0 && len(r.DriverDB) > 0
}

func (c Consumer) IsValid() bool {
	return len(c.Name) > 0 && len(c.Path) > 0 && len(c.Backend) > 0 && len(c.Group) > 0 && len(c.Topic) > 0
}

func GetConfig(configPath string) (Config, error) {
	var config Config

	viper.SetConfigFile(configPath)
	if err := viper.ReadInConfig(); err != nil {
		return config, err
	}

	viper.SetDefault("ports.grpc", defaultGrpcPort)
	viper.SetDefault("ports.rest", defaultRestPort)
	viper.SetDefault("ports.sys", defaultSysPort)

	viper.SetDefault("post_generate.git_install", true)
	viper.SetDefault("post_generate.tools_install", true)
	viper.SetDefault("post_generate.clean_imports", true)
	viper.SetDefault("post_generate.executable_scripts", true)
	viper.SetDefault("post_generate.call_generate", true)
	viper.SetDefault("post_generate.go_mod_tidy", true)
	viper.SetDefault("post_generate.get_last_linter_config", true)

	viper.SetDefault("tools.protobuf_version", defaultProtobufVersion)
	viper.SetDefault("tools.golang_version", defaultGolangVersion)
	viper.SetDefault("tools.ogen_version", defaultOgenVersion)
	viper.SetDefault("tools.golangci_version", defaultGolangciVersion)

	if err := viper.Unmarshal(&config); err != nil {
		return config, err
	}

	if !config.Main.IsValid() {
		return config, errors.WithMessage(ErrInvalidConfig, "invalid config main section")
	}

	config.restMap = make(map[string]Rest)
	config.grpcMap = make(map[string]Grpc)

	for _, rest := range config.RestList {
		if !rest.IsValid() {
			return config, errors.WithMessage(ErrInvalidConfig, "invalid config rest section")
		}

		if _, ex := config.restMap[rest.Name]; ex {
			return config, errors.WithMessage(ErrInvalidConfig, "duplicate rest name: "+rest.Name)
		}

		config.restMap[rest.Name] = rest
	}

	for _, grpc := range config.GrpcList {
		if !grpc.IsValid() {
			return config, errors.WithMessage(ErrInvalidConfig, "invalid config grpc section")
		}

		if _, ex := config.grpcMap[grpc.Name]; ex {
			return config, errors.WithMessage(ErrInvalidConfig, "duplicate grpc name: "+grpc.Name)
		}

		config.grpcMap[grpc.Name] = grpc
	}

	for _, app := range config.Applications {
		if !app.IsValid() {
			return config, errors.WithMessage(ErrInvalidConfig, "invalid config application section")
		}

		for _, rest := range app.RestList {
			if _, ex := config.restMap[rest]; !ex {
				return config, errors.WithMessage(ErrInvalidConfig, "unknown rest name: "+rest)
			}
		}

		for _, grpc := range app.GrpcList {
			if _, ex := config.grpcMap[grpc]; !ex {
				return config, errors.WithMessage(ErrInvalidConfig, "unknown grpc name: "+grpc)
			}
		}
	}

	return config, nil
}
