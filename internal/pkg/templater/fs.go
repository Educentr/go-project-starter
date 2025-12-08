package templater

import (
	"bytes"
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Educentr/go-project-starter/internal/pkg/ds"
	"github.com/pkg/errors"
)

const (
	grafanaProvisioningPath = "configs/grafana/provisioning"
	grafanaDashboardsPath   = "configs/grafana/dashboards"
	devPrometheusPath       = "configs/dev/prometheus"
	devLokiPath             = "configs/dev/loki"
)

//go:embed all:embedded
var templates embed.FS

//go:embed embedded/disclaimer.txt
var disclaimerTop string

//go:embed embedded/finish_disclaimer.txt
var disclaimerBottom string

func GetTemplates(templateFS embed.FS, prefix string, params any) (dirs []ds.Files, files []ds.Files, err error) {
	dirs = []ds.Files{}
	files = []ds.Files{}

	trimCnt := len(strings.Split(prefix, string(os.PathSeparator)))

	err = fs.WalkDir(templateFS, prefix, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			dirs = append(dirs, ds.Files{
				SourceName: path,
				DestName:   filepath.Join(strings.Split(path, string(os.PathSeparator))[trimCnt:]...),
				ParamsTmpl: params,
			})

			return nil
		}

		files = append(files, ds.Files{
			SourceName: path,
			DestName:   strings.TrimSuffix(filepath.Join(strings.Split(path, string(os.PathSeparator))[trimCnt:]...), ".tmpl"),
			ParamsTmpl: params,
			Code:       &bytes.Buffer{},
		})

		return nil
	})

	if err != nil {
		err = errors.Wrapf(err, "error while walk dir `%s`", prefix)
	}

	return
}

func GetMainTemplates(params GeneratorParams) (dirs []ds.Files, files []ds.Files, err error) {
	dirs, files, err = GetTemplates(templates, "embedded/templates/main", params)
	if err != nil {
		err = errors.Wrap(err, "error while get main templates")
	}

	return
}

func GetLoggerTemplates(path string, dst string, params GeneratorParams) (dirs []ds.Files, files []ds.Files, err error) {
	dirs, files, err = GetTemplates(templates, filepath.Join("embedded/templates/logger", path), params)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// Logger templates moved to runtime, return empty
			err = nil

			return
		}

		err = errors.Wrap(err, "error while get logger templates")
	}

	for i := range dirs {
		dirs[i].DestName = filepath.Join(dst, dirs[i].DestName)
	}

	for i := range files {
		files[i].DestName = filepath.Join(dst, files[i].DestName)
	}

	return
}

func GetWorkerTemplates(params GeneratorParams) (dirs []ds.Files, files []ds.Files, err error) {
	dirs, files, err = GetTemplates(templates, filepath.Join("embedded/templates/worker/files"), params)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			err = nil

			return
		}

		err = errors.Wrap(err, "error while get worker templates")

		return
	}

	return
}

func GetWorkerGeneratorTemplates(generatorType string, params GeneratorParams) (dirs []ds.Files, files []ds.Files, err error) {
	dirs, files, err = GetTemplates(templates, filepath.Join("embedded/templates/worker", generatorType, "config"), params)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			err = nil

			return
		}

		err = errors.Wrap(err, "error while get worker templates")

		return
	}

	return
}

func GetTransportTemplates(transportType ds.TransportType, params GeneratorParams) (dirs []ds.Files, files []ds.Files, err error) {
	dirs, files, err = GetTemplates(templates, filepath.Join("embedded/templates/transport", string(transportType), "files"), params)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			err = nil

			return
		}

		err = errors.Wrap(err, "error while get transport templates")

		return
	}

	return
}

func GetTransportGeneratorTemplates(transportType ds.TransportType, generatorType string, params GeneratorHandlerParams) (dirs []ds.Files, files []ds.Files, err error) {
	dirs, files, err = GetTemplates(templates, filepath.Join("embedded/templates/transport", string(transportType), generatorType, "config"), params)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			err = nil

			return
		}

		err = errors.Wrap(err, "error while get transport templates")

		return
	}

	for i := range dirs {
		dirs[i].DestName = filepath.Join(prefixDirs["transportConfig"], dirs[i].DestName)
	}

	for i := range files {
		files[i].DestName = filepath.Join(prefixDirs["transportConfig"], files[i].DestName)
	}

	return
}

var (
	prefixDirs = map[string]string{
		"transportConfig": "configs/transport/{{ .Transport.Type }}/{{ .Transport.Name }}/{{ .Transport.ApiVersion }}",
		"transport":       "internal/app/transport/{{ .Transport.Type }}/{{ .Transport.Name }}/{{ .Transport.ApiVersion }}",
		"worker":          "internal/app/worker/{{ .Worker.Name }}",

		"app": "cmd/{{ .Application.Name }}",
	}
)

// ToDo добавить кеширование шаблонов
// type MapTemplateFileName struct {
// 	dirs  []ds.Files
// 	files []ds.Files
// }

// type TemplateFileNameCache struct {
// 	sync.Mutex
// 	cache map[string]MapTemplateFileName
// }

// var (
// 	templateFileNameCache = TemplateFileNameCache{
// 		cache: make(map[string]MapTemplateFileName),
// 	}
// )

func GetWorkerRunnerTemplates(template string, params GeneratorRunnerParams) (dirs, files []ds.Files, err error) {
	cacheKey := filepath.Join("embedded/templates/worker", template, "files")

	// ToDo если включить кеширование то кешируются и параметры, а не только файлы
	// templateFileNameCache.Lock()
	// defer templateFileNameCache.Unlock()

	// if v, ok := templateFileNameCache.cache[cacheKey]; ok {
	// 	return v.dirs, v.files, nil
	// }

	dirs, files, err = GetTemplates(templates, cacheKey, params)
	if err != nil {
		err = errors.Wrapf(err, "error while get worker runner templates `%s`", cacheKey)

		return
	}

	for i := range dirs {
		dirs[i].DestName = filepath.Join(prefixDirs["worker"], dirs[i].DestName)
	}

	for i := range files {
		files[i].DestName = filepath.Join(prefixDirs["worker"], files[i].DestName)
	}

	// templateFileNameCache.cache[cacheKey] = MapTemplateFileName{dirs, files}

	return dirs, files, nil

}

func GetTransportHandlerTemplates(transport ds.TransportType, template string, params GeneratorHandlerParams) (dirs, files []ds.Files, err error) {
	cacheKey := filepath.Join("embedded/templates/transport", string(transport), template, "files")

	// ToDo если включить кеширование то кешируются и параметры, а не только файлы
	// templateFileNameCache.Lock()
	// defer templateFileNameCache.Unlock()

	// if v, ok := templateFileNameCache.cache[cacheKey]; ok {
	// 	return v.dirs, v.files, nil
	// }

	dirs, files, err = GetTemplates(templates, cacheKey, params)
	if err != nil {
		err = errors.Wrapf(err, "error while get transport handler templates `%s`", cacheKey)

		return
	}

	for i := range dirs {
		dirs[i].DestName = filepath.Join(prefixDirs["transport"], dirs[i].DestName)
	}

	for i := range files {
		files[i].DestName = filepath.Join(prefixDirs["transport"], files[i].DestName)
	}

	// templateFileNameCache.cache[cacheKey] = MapTemplateFileName{dirs, files}

	return dirs, files, nil

}

func GetAppTemplates(params GeneratorAppParams) (dirs []ds.Files, files []ds.Files, err error) {
	dirs, files, err = GetTemplates(templates, "embedded/templates/app/files", params)
	if err != nil {
		err = errors.Wrap(err, "error while get app templates")

		return
	}

	for i := range files {
		ext := filepath.Ext(files[i].DestName)
		fname := strings.TrimSuffix(files[i].DestName, ext)

		files[i].DestName = fname + "-" + params.Application.Name + ext
	}

	// Select cmd template based on app type (CLI vs regular)
	cmdTemplateDir := "embedded/templates/app/cmd"
	if params.Application.IsCLI() {
		cmdTemplateDir = "embedded/templates/app/cmd_cli"
	}

	dirsC, filesC, err := GetTemplates(templates, cmdTemplateDir, params)
	if err != nil {
		err = errors.Wrap(err, "error while get app templates")

		return
	}

	for i := range dirsC {
		dirsC[i].DestName = filepath.Join(prefixDirs["app"], dirsC[i].DestName)
	}

	for i := range filesC {
		filesC[i].DestName = filepath.Join(prefixDirs["app"], filesC[i].DestName)
	}

	dirs = append(dirs, dirsC...)
	files = append(files, filesC...)

	return
}

// GeneratorCLIParams holds parameters for CLI handler template generation
type GeneratorCLIParams struct {
	GeneratorParams
	CLI *ds.CLIApp
}

// GetCLIHandlerTemplates returns templates for CLI handler
func GetCLIHandlerTemplates(cli *ds.CLIApp, params GeneratorParams) (dirs []ds.Files, files []ds.Files, err error) {
	cliParams := GeneratorCLIParams{
		GeneratorParams: params,
		CLI:             cli,
	}

	// Use template generator type, default to "template"
	generatorType := cli.GeneratorType
	if generatorType == "" {
		generatorType = "template"
	}

	templatePath := filepath.Join("embedded/templates/transport/cli", generatorType, "files")

	dirs, files, err = GetTemplates(templates, templatePath, cliParams)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			err = nil
			return
		}
		err = errors.Wrap(err, "error while get CLI handler templates")
		return
	}

	// Set destination path for CLI handler files
	cliPrefix := "internal/app/transport/cli/" + cli.Name

	for i := range dirs {
		dirs[i].DestName = filepath.Join(cliPrefix, dirs[i].DestName)
	}

	for i := range files {
		files[i].DestName = filepath.Join(cliPrefix, files[i].DestName)
	}

	return
}

// GetGrafanaProvisioningTemplates returns Grafana provisioning templates (datasources and dashboard provider config)
func GetGrafanaProvisioningTemplates(params GeneratorParams) (dirs []ds.Files, files []ds.Files, err error) {
	dirs, files, err = GetTemplates(templates, "embedded/templates/grafana/provisioning", params)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			err = nil

			return
		}

		err = errors.Wrap(err, "error while get grafana provisioning templates")

		return
	}

	// Set destination path
	for i := range dirs {
		dirs[i].DestName = filepath.Join(grafanaProvisioningPath, dirs[i].DestName)
	}

	for i := range files {
		files[i].DestName = filepath.Join(grafanaProvisioningPath, files[i].DestName)
	}

	return
}

// GetGrafanaDashboardTemplates returns Grafana dashboard templates for an application
func GetGrafanaDashboardTemplates(params GeneratorAppParams) (dirs []ds.Files, files []ds.Files, err error) {
	dirs, files, err = GetTemplates(templates, "embedded/templates/grafana/dashboards", params)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			err = nil

			return
		}

		err = errors.Wrap(err, "error while get grafana dashboard templates")

		return
	}

	// Set destination path and rename dashboard.json to {app_name}.json
	for i := range dirs {
		dirs[i].DestName = filepath.Join(grafanaDashboardsPath, dirs[i].DestName)
	}

	for i := range files {
		destName := files[i].DestName
		// Rename dashboard.json to {app_name}.json
		if destName == "dashboard.json" {
			destName = params.Application.Name + ".json"
		}

		files[i].DestName = filepath.Join(grafanaDashboardsPath, destName)
	}

	return
}

// GetPrometheusTemplates returns Prometheus configuration templates for dev environment
func GetPrometheusTemplates(params GeneratorParams) (dirs []ds.Files, files []ds.Files, err error) {
	dirs, files, err = GetTemplates(templates, "embedded/templates/dev-infra/prometheus", params)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			err = nil

			return
		}

		err = errors.Wrap(err, "error while get prometheus templates")

		return
	}

	// Set destination path
	for i := range dirs {
		dirs[i].DestName = filepath.Join(devPrometheusPath, dirs[i].DestName)
	}

	for i := range files {
		files[i].DestName = filepath.Join(devPrometheusPath, files[i].DestName)
	}

	return
}

// GetLokiTemplates returns Loki configuration templates for dev environment
func GetLokiTemplates(params GeneratorParams) (dirs []ds.Files, files []ds.Files, err error) {
	dirs, files, err = GetTemplates(templates, "embedded/templates/dev-infra/loki", params)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			err = nil

			return
		}

		err = errors.Wrap(err, "error while get loki templates")

		return
	}

	// Set destination path
	for i := range dirs {
		dirs[i].DestName = filepath.Join(devLokiPath, dirs[i].DestName)
	}

	for i := range files {
		files[i].DestName = filepath.Join(devLokiPath, files[i].DestName)
	}

	return
}
