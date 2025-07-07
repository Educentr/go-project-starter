package config

import (
	"path/filepath"

	"github.com/pkg/errors"
	"gitlab.educentr.info/golang/service-starter/internal/pkg/ds"
	"gitlab.educentr.info/golang/service-starter/internal/pkg/loggers"
	"gitlab.educentr.info/golang/service-starter/internal/pkg/tools"
)

type (
	Main struct {
		Name            string `mapstructure:"name"`
		RegistryType    string `mapstructure:"registry_type"`
		Logger          string `mapstructure:"logger"`
		Author          string `mapstructure:"author"`
		SkipServiceInit bool   `mapstructure:"skip_service_init"`
		UseActiveRecord bool   `mapstructure:"use_active_record"`
		LoggerObj       ds.Logger
		TargetDir       string
		ConfigDir       string
	}

	Scheduler struct {
		Enabled bool `mapstructure:"enabled"`
	}

	Git struct {
		Repo       string `mapstructure:"repo"`
		ModulePath string `mapstructure:"module_path"`
		// ProjectID  uint   `mapstructure:"project_id"` // Todo
	}

	Tools struct {
		ProtobufVersion string `mapstructure:"protobuf_version"`
		GolangVersion   string `mapstructure:"golang_version"`
		OgenVersion     string `mapstructure:"ogen_version"`
		ArgenVersion    string `mapstructure:"argen_version"`
		GolangciVersion string `mapstructure:"golangci_version"`
	}

	Rest struct {
		Name              string            `mapstructure:"name"`
		Path              []string          `mapstructure:"path"`
		APIPrefix         string            `mapstructure:"api_prefix"`
		Port              uint              `mapstructure:"port"`
		Version           string            `mapstructure:"version"`
		PublicService     bool              `mapstructure:"public_service"`
		GeneratorType     string            `mapstructure:"generator_type"`
		HealthCheckPath   string            `mapstructure:"health_check_path"`
		GeneratorTemplate string            `mapstructure:"generator_template"`
		GeneratorParams   map[string]string `mapstructure:"generator_params"`
	}

	Worker struct {
		Name              string            `mapstructure:"name"`
		Path              []string          `mapstructure:"path"`
		Version           string            `mapstructure:"version"`
		GeneratorType     string            `mapstructure:"generator_type"`
		GeneratorTemplate string            `mapstructure:"generator_template"`
		GeneratorParams   map[string]string `mapstructure:"generator_params"`
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
		Name             string `mapstructure:"name"`
		Import           string `mapstructure:"import"`
		Package          string `mapstructure:"package"`
		ObjName          string `mapstructure:"obj_name"`
		ServiceInjection string `mapstructure:"service_injection"`
	}

	RestList       []Rest
	GrpcList       []Grpc
	WsList         []Ws
	RepositoryList []Repository
	WorkerList     []Worker
	ConsumerList   []Consumer
	DriverList     []Driver

	AppDriver struct {
		Name   string   `mapstructure:"name"`
		Params []string `mapstructure:"params"`
	}

	DeployVolume struct {
		Path  string `mapstructure:"path"`
		Mount string `mapstructure:"mount"`
	}

	AppDeploy struct {
		Volumes []DeployVolume `mapstructure:"volumes"`
	}

	Application struct {
		Name          string      `mapstructure:"name"`
		TransportList []string    `mapstructure:"transport"`
		DriverList    []AppDriver `mapstructure:"driver"`
		WorkerList    []string    `mapstructure:"worker"`
		Deploy        AppDeploy   `mapstructure:"deploy"`
	}

	Docker struct {
		ImagePrefix string `mapstructure:"image_prefix"`
	}

	LogCollector struct {
		Type       string            `mapstructure:"type"`
		Parameters map[string]string `mapstructure:"parameters"`
	}

	Deploy struct {
		LogCollector LogCollector `mapstructure:"log_collector"`
	}

	Config struct {
		BasePath       string
		Main           Main           `mapstructure:"main"`
		Deploy         Deploy         `mapstructure:"deploy"`
		PostGenerate   []string       `mapstructure:"post_generate"`
		Git            Git            `mapstructure:"git"`
		Tools          Tools          `mapstructure:"tools"`
		RepositoryList RepositoryList `mapstructure:"repository"`
		Scheduler      Scheduler      `mapstructure:"scheduler"`
		RestList       RestList       `mapstructure:"rest"`
		WorkerList     WorkerList     `mapstructure:"worker"`
		GrpcList       GrpcList       `mapstructure:"grpc"`
		WsList         WsList         `mapstructure:"ws"`
		ConsumerList   ConsumerList   `mapstructure:"consumer"`
		DriverList     DriverList     `mapstructure:"driver"`
		Applications   []Application  `mapstructure:"applications"`
		Docker         Docker         `mapstructure:"docker"`

		RestMap   map[string]Rest
		GrpcMap   map[string]Grpc
		DriverMap map[string]Driver
		WorkerMap map[string]Worker
	}
)

type (
	BufType  string
	TypeDB   string
	DriverDB string
)

const (
	defaultGolangVersion   = "1.20"
	defaultProtobufVersion = "1.7.0"
	defaultGolangciVersion = "1.55.2"
	defaultOgenVersion     = "v0.78.0"
	defaultArgenVersion    = "v1.0.0"
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

	if len(g.Repo) == 0 {
		return false, "Empty repo"
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
		if len(r.GeneratorParams) != 0 {
			for k := range r.GeneratorParams {
				switch k {
				case "auth_handler":
				default:
					return false, "Invalid generator params"
				}
			}
		}
	case "template":
		if len(r.GeneratorTemplate) == 0 {
			return false, "Empty generator template"
		}
		if len(r.GeneratorParams) != 0 {
			return false, "Generator params not supported"
		}
	case "ogen_client":
		if len(r.GeneratorTemplate) != 0 {
			return false, "Generator template not supported"
		}
		if len(r.GeneratorParams) != 0 {
			for k := range r.GeneratorParams {
				switch k {
				case "auth_type":
					if r.GeneratorParams[k] != "apikey" {
						return false, "Invalid auth type in generator params. Only 'apikey' is supported for ogen_client"
					}
				default:
					return false, "Invalid generator params"
				}
			}
		}
	default:
		return false, "Invalid generator type"
	}

	return true, ""
}

func (w Worker) IsValid(baseConfigDir string) (bool, string) {
	if len(w.Name) == 0 {
		return false, "Empty name"
	}

	if len(w.Path) == 0 {
		return false, "Empty path"
	}

	for _, p := range w.Path {
		absPath := filepath.Join(baseConfigDir, p)

		if tools.FileExists(absPath) != tools.ErrExist {
			return false, "Invalid path: " + p
		}
	}

	switch w.GeneratorType {
	case "template":
		if len(w.GeneratorTemplate) == 0 {
			return false, "Empty generator template"
		}
		if len(w.GeneratorParams) != 0 {
			return false, "Generator params not supported"
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

func (d Driver) IsValid() (bool, string) {
	if len(d.Name) == 0 {
		return false, "Empty name"
	}

	if len(d.Import) == 0 {
		return false, "Empty import"
	}

	if len(d.Package) == 0 {
		return false, "Empty package"
	}

	if len(d.ObjName) == 0 {
		return false, "Empty object name"
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
