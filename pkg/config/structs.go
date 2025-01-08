package config

import (
	"path/filepath"

	"github.com/pkg/errors"
	"gitlab.educentr.info/golang/service-starter/pkg/ds"
	"gitlab.educentr.info/golang/service-starter/pkg/loggers"
	"gitlab.educentr.info/golang/service-starter/pkg/tools"
)

type (
	Main struct {
		Name      string `mapstructure:"name"`
		Logger    string `mapstructure:"logger"`
		LoggerObj ds.Logger
		TargetDir string
		ConfigDir string
	}

	Scheduler struct {
		Enabled bool `mapstructure:"enabled"`
	}

	Git struct {
		ModulePath string `mapstructure:"module_path"`
		// ProjectID  uint   `mapstructure:"project_id"` // Todo
	}

	Tools struct {
		ProtobufVersion string `mapstructure:"protobuf_version"`
		GolangVersion   string `mapstructure:"golang_version"`
		OgenVersion     string `mapstructure:"ogen_version"`
		GolangciVersion string `mapstructure:"golangci_version"`
	}

	Rest struct {
		Name              string   `mapstructure:"name"`
		Path              []string `mapstructure:"path"`
		APIPrefix         string   `mapstructure:"api_prefix"`
		Port              uint     `mapstructure:"port"`
		Version           string   `mapstructure:"version"`
		GeneratorType     string   `mapstructure:"generator_type"`
		GeneratorTemplate string   `mapstructure:"generator_template"`
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

	Driver struct {
		Name string `mapstructure:"name"`
	}

	RestList       []Rest
	GrpcList       []Grpc
	WsList         []Ws
	RepositoryList []Repository
	ConsumerList   []Consumer
	DriverList     []Driver

	Application struct {
		Name          string   `mapstructure:"name"`
		TransportList []string `mapstructure:"transport"`
		DriverList    []string `mapstructure:"driver"`
	}

	Config struct {
		BasePath        string
		Main            Main           `mapstructure:"main"`
		PostGenerate    []string       `mapstructure:"post_generate"`
		Git             Git            `mapstructure:"git"`
		Tools           Tools          `mapstructure:"tools"`
		RepositoryList  RepositoryList `mapstructure:"repository"`
		Scheduler       Scheduler      `mapstructure:"scheduler"`
		RestList        RestList       `mapstructure:"rest"`
		GrpcList        GrpcList       `mapstructure:"grpc"`
		WsList          WsList         `mapstructure:"ws"`
		ConsumerList    ConsumerList   `mapstructure:"consumer"`
		DriverList      DriverList     `mapstructure:"driver"`
		Applications    []Application  `mapstructure:"applications"`
		SkipServiceInit bool           `mapstructure:"skip_service_init"`

		RestMap map[string]Rest
		GrpcMap map[string]Grpc
		//driverMap map[string]Driver
	}
)

type (
	BufType  string
	TypeDB   string
	DriverDB string

	// consumer struct {
	// 	Name    string
	// 	Path    string
	// 	Backend BufType
	// 	Group   string
	// 	Topic   string
	// }

	// repository struct {
	// 	Name string
	// 	// Alias    string
	// 	TypeDB   TypeDB
	// 	DriverDB DriverDB
	// }

	// Contract struct {
	// 	Path       string
	// 	ServerName string
	// 	Short      string
	// 	APIPrefix  string
	// }

	// Ports struct {
	// 	Grpc string
	// 	Rest string
	// 	Sys  string
	// }

	// PkgDataRepo struct {
	// 	Name             string
	// 	TypeDB           string
	// 	DriverDB         string
	// 	MigrationFileExt string
	// 	Alias            string
	// }

	// PkgDataSchedulerWorker struct {
	// 	PkgName string
	// }

	// PkgDataConsumer struct {
	// 	Name        string
	// 	PackageName string
	// 	Topic       string
	// 	Group       string
	// }

	// PkgDataGrpc struct {
	// 	Name        string
	// 	PackageName string
	// }

	// PkgDataRest struct {
	// 	Name        string
	// 	PackageName string
	// 	APIPrefix   string
	// }

	// PkgData struct {
	// 	ProjectName     string
	// 	GoLangVersion   string
	// 	OgenVersion     string
	// 	ProtobufVersion string
	// 	GolangciVersion string
	// 	AppInfo         string

	// 	GitlabModulePath string
	// 	GitlabProjectID  uint

	// 	Ports Ports

	// 	Kafka    bool
	// 	Repo     bool
	// 	PG       bool
	// 	Redis    bool
	// 	Repos    []*PkgDataRepo
	// 	Daemons  []*PkgDataSchedulerWorker
	// 	GrpcData []*PkgDataGrpc
	// 	RestData []*PkgDataRest
	// 	GRPC     bool
	// 	REST     bool
	// 	Clean    bool

	// 	Scheduler bool
	// 	// Scheduler *PkgDataSchedulerWorker
	// 	RepoData *PkgDataRepo
	// 	Grpc     *PkgDataGrpc
	// 	Consumer *PkgDataConsumer
	// }

	// process struct {
	// 	pr   string
	// 	argv []string
	// 	msg  string
	// }
)

// const (
// 	Kafka BufType = "kafka"

// 	Psql  TypeDB = "psql"
// 	Mongo TypeDB = "mongodb"
// 	Redis TypeDB = "redis"

// 	SQLC        DriverDB = "sqlc"
// 	DriverRedis DriverDB = "redis"
// )

const (
	defaultGolangVersion   = "1.20"
	defaultProtobufVersion = "1.7.0"
	defaultGolangciVersion = "1.55.2"
	defaultOgenVersion     = "v0.78.0"

	// defaultRestPort = 8080
	// defaultGrpcPort = 8082
	// defaultSysPort  = 8084
)

var (
	ErrInvalidConfig = errors.New("invalid config")
)

func (c *Config) SetTargetDir(dir string)     { c.Main.TargetDir = dir }
func (c *Config) SetBaseConfigDir(dir string) { c.Main.ConfigDir = dir }

func (m Main) IsValid() (bool, string) {
	if len(m.Name) == 0 {
		return false, "Empty name"
	}

	_, ex := loggers.LoggerMapping[m.Logger]
	if !ex {
		return false, "invalid logger"
	}

	return true, ""
}

func (g Git) IsValid() (bool, string) {
	if len(g.ModulePath) == 0 {
		return false, "Empty module path"
	}

	return true, ""
}

func (r Rest) IsValid(baseConfigDir string) (bool, string) {
	if len(r.Name) == 0 {
		return false, "Empty name"
	}

	if r.Name == "sys" {
		return true, ""
	}

	if len(r.Path) == 0 {
		return false, "Empty path"
	}

	for _, p := range r.Path {
		absPath := filepath.Join(baseConfigDir, p)

		if tools.FileExists(absPath) != tools.ErrExist {
			return false, "Invalid path: " + p
		}
	}

	switch r.GeneratorType {
	case "ogen":
		if len(r.GeneratorTemplate) != 0 {
			return false, "Invalid generator template for type ogen"
		}
	case "template":
		if len(r.GeneratorTemplate) == 0 {
			return false, "Empty generator template"
		}
	default:
		return false, "Invalid generator type"
	}

	return true, ""
}

// ToDo GRPC
func (g Grpc) IsValid(baseConfigDir string) (bool, string) {
	if len(g.Name) == 0 {
		return false, "Empty name"
	}

	if len(g.Path) == 0 {
		return false, "Empty path"
	}

	absPath := filepath.Join(baseConfigDir, g.Path)

	if tools.FileExists(absPath) != tools.ErrExist {
		return false, "Invalid path: " + g.Path
	}

	return true, ""
}

// ToDo WS
func (w Ws) IsValid(baseConfigDir string) (bool, string) {
	if len(w.Name) == 0 {
		return false, "Empty name"
	}

	if len(w.Path) == 0 {
		return false, "Empty path"
	}

	if tools.FileExists(filepath.Join(baseConfigDir, w.Path)) != tools.ErrExist {
		return false, "Invalid path: " + w.Path
	}

	return true, ""
}

func (a Application) IsValid() (bool, string) {
	if len(a.Name) == 0 {
		return false, "Empty name"
	}

	if len(a.TransportList) == 0 {
		return false, "Empty transport list"
	}

	return true, ""
}

// ToDo Repository
func (r Repository) IsValid() (bool, string) {
	if len(r.Name) == 0 {
		return false, "Empty name"
	}

	if len(r.TypeDB) == 0 || len(r.DriverDB) == 0 {
		return false, "Empty type or driver"
	}

	return true, ""
}

// ToDo Consumer
func (c Consumer) IsValid() (bool, string) {
	if len(c.Name) == 0 {
		return false, "Empty name"
	}

	if len(c.Path) == 0 || len(c.Backend) == 0 || len(c.Group) == 0 || len(c.Topic) == 0 {
		return false, "Empty path, backend, group or topic"
	}

	return true, ""
}
