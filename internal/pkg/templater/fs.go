package templater

import (
	"bytes"
	"embed"
	"io/fs"
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
	testsPath               = "tests"
	mocksPath               = "tests/mocks"
	packagingPath           = "packaging"

	// CI provider path prefixes for filtering
	ciGitHubPrefix   = ".github"
	ciGitLabFileName = ".gitlab-ci.yml"

	// Embedded template path prefixes (always use forward slashes for embed.FS)
	embedTransportPrefix = "embedded/templates/transport"
	embedWorkerPrefix    = "embedded/templates/worker"
	embedConfigSuffix    = "config"
	embedFilesSuffix     = "files"
)

//go:embed all:embedded
var templates embed.FS

//go:embed embedded/disclaimer.txt
var disclaimerTop string

//go:embed embedded/finish_disclaimer.txt
var disclaimerBottom string

// embedJoin joins path elements using forward slashes.
// embed.FS always uses "/" as separator, even on Windows.
// Using filepath.Join would produce backslash paths on Windows, breaking embed.FS lookups.
func embedJoin(elem ...string) string {
	return strings.Join(elem, "/")
}

func GetTemplates(templateFS embed.FS, prefix string, params any) (dirs []ds.Files, files []ds.Files, err error) {
	dirs = []ds.Files{}
	files = []ds.Files{}

	// embed.FS always uses forward slashes, even on Windows
	trimCnt := len(strings.Split(prefix, "/"))

	err = fs.WalkDir(templateFS, prefix, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// embed.FS paths always use forward slashes, even on Windows
		parts := strings.Split(path, "/")[trimCnt:]

		if d.IsDir() {
			dirs = append(dirs, ds.Files{
				SourceName: path,
				DestName:   filepath.Join(parts...),
				ParamsTmpl: params,
			})

			return nil
		}

		files = append(files, ds.Files{
			SourceName: path,
			DestName:   strings.TrimSuffix(filepath.Join(parts...), ".tmpl"),
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
		return
	}

	// Filter CI templates based on selected providers
	// Empty CI = generate both (backward compatibility)
	if len(params.CI) > 0 {
		ciSet := make(map[string]bool, len(params.CI))
		for _, ci := range params.CI {
			ciSet[ci] = true
		}

		filteredDirs := make([]ds.Files, 0, len(dirs))

		for _, d := range dirs {
			if !ciSet["github"] && strings.HasPrefix(d.DestName, ciGitHubPrefix) {
				continue
			}

			filteredDirs = append(filteredDirs, d)
		}

		dirs = filteredDirs

		filteredFiles := make([]ds.Files, 0, len(files))

		for _, f := range files {
			if !ciSet["github"] && strings.HasPrefix(f.DestName, ciGitHubPrefix) {
				continue
			}

			if !ciSet["gitlab"] && f.DestName == ciGitLabFileName {
				continue
			}

			filteredFiles = append(filteredFiles, f)
		}

		files = filteredFiles
	}

	// Filter out dev-stand specific templates if DevStand is false
	if !params.DevStand {
		filteredDirs := make([]ds.Files, 0, len(dirs))

		for _, d := range dirs {
			// Skip etc/onlineconf/ directory
			if strings.Contains(d.DestName, "etc/onlineconf") {
				continue
			}

			filteredDirs = append(filteredDirs, d)
		}

		dirs = filteredDirs

		filteredFiles := make([]ds.Files, 0, len(files))

		for _, f := range files {
			// Skip docker-compose-dev.yaml and etc/onlineconf/ files
			if strings.HasPrefix(f.DestName, "docker-compose-dev") ||
				strings.Contains(f.DestName, "etc/onlineconf") {
				continue
			}

			filteredFiles = append(filteredFiles, f)
		}

		files = filteredFiles
	}

	// Filter out LLMS.md if generate_llms_md is false; rename if generating CLAUDE.md
	if !params.GenerateLlmsMd {
		filteredFiles := make([]ds.Files, 0, len(files))

		for _, f := range files {
			if f.DestName == "LLMS.md" {
				continue
			}

			filteredFiles = append(filteredFiles, f)
		}

		files = filteredFiles
	} else if params.LlmsFileName == "CLAUDE.md" {
		for i := range files {
			if files[i].DestName == "LLMS.md" {
				files[i].DestName = "CLAUDE.md"
				break
			}
		}
	}

	return
}

// GetDocsTemplates returns documentation templates (mkdocs.yml, docs/index.md).
// Returns nil if documentation is not enabled.
func GetDocsTemplates(params GeneratorParams) (dirs []ds.Files, files []ds.Files, err error) {
	if !params.Documentation.IsEnabled() {
		return nil, nil, nil
	}

	dirs, files, err = GetTemplates(templates, "embedded/templates/docs", params)
	if err != nil {
		err = errors.Wrap(err, "error while get docs templates")
	}

	return
}

func GetLoggerTemplates(path string, dst string, params GeneratorParams) (dirs []ds.Files, files []ds.Files, err error) {
	dirs, files, err = GetTemplates(templates, embedJoin("embedded/templates/logger", path), params)
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
	dirs, files, err = GetTemplates(templates, embedJoin(embedWorkerPrefix, embedFilesSuffix), params)
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
	dirs, files, err = GetTemplates(templates, embedJoin(embedWorkerPrefix, generatorType, embedConfigSuffix), params)
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
	dirs, files, err = GetTemplates(templates, embedJoin(embedTransportPrefix, string(transportType), embedFilesSuffix), params)
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
	dirs, files, err = GetTemplates(templates, embedJoin(embedTransportPrefix, string(transportType), generatorType, embedConfigSuffix), params)
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
	cacheKey := embedJoin(embedWorkerPrefix, template, embedFilesSuffix)

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
	cacheKey := embedJoin(embedTransportPrefix, string(transport), template, embedFilesSuffix)

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

	// Filter out Docker-related files if app doesn't have docker artifacts
	if !params.Application.HasDocker() {
		filteredFiles := make([]ds.Files, 0, len(files))

		for _, f := range files {
			// Skip Dockerfile and docker-compose for non-docker apps
			if strings.HasPrefix(f.DestName, "Dockerfile") || strings.HasPrefix(f.DestName, "docker-compose") {
				continue
			}

			filteredFiles = append(filteredFiles, f)
		}

		files = filteredFiles
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

	templatePath := embedJoin(embedTransportPrefix, "cli", generatorType, embedFilesSuffix)

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

// GetTestTemplates returns GOAT integration test templates for an application
func GetTestTemplates(params GeneratorAppParams) (dirs []ds.Files, files []ds.Files, err error) {
	// Skip if test generation is not enabled
	if !params.Application.GoatTests {
		return nil, nil, nil
	}

	// Skip CLI applications (they don't have HTTP transports to test)
	if params.Application.IsCLI() {
		return nil, nil, nil
	}

	dirs, files, err = GetTemplates(templates, "embedded/templates/tests/files", params)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			err = nil

			return
		}

		err = errors.Wrap(err, "error while get test templates")

		return
	}

	// Set destination path for test files: tests/ (directly, without app name subdirectory)
	for i := range dirs {
		dirs[i].DestName = filepath.Join(testsPath, dirs[i].DestName)
	}

	for i := range files {
		files[i].DestName = filepath.Join(testsPath, files[i].DestName)
	}

	return
}

// GetKafkaDriverTemplates returns Kafka driver templates for auto-generated producers/consumers
// kafkaType should be "producer" or "consumer"
func GetKafkaDriverTemplates(kafka ds.KafkaConfig, params GeneratorParams) ([]ds.Files, []ds.Files, error) {
	// Only generate templates for segmentio driver (not custom)
	if kafka.IsCustomDriver() {
		return nil, nil, nil
	}

	kafkaParams := GeneratorKafkaParams{
		GeneratorParams: params,
		Kafka:           kafka,
	}

	templatePath := embedJoin("embedded/templates/driver/kafka", kafka.Type, embedFilesSuffix)

	dirs, files, err := GetTemplates(templates, templatePath, kafkaParams)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil, nil
		}

		return nil, nil, errors.Wrapf(err, "error while get kafka driver templates for %s", kafka.Name)
	}

	// Set destination path: pkg/drivers/kafka/{name}
	kafkaPrefix := "pkg/drivers/kafka/{{ .Kafka.Name | ToLower }}"

	for i := range dirs {
		dirs[i].DestName = filepath.Join(kafkaPrefix, dirs[i].DestName)
	}

	for i := range files {
		files[i].DestName = filepath.Join(kafkaPrefix, files[i].DestName)
	}

	return dirs, files, nil
}

// GetMockTemplates returns mock templates for an application with ogen_clients.
// It generates:
// - tests/mocks/mocks.go - MockServers struct and MocksSetup
// - tests/mocks/{transport_name}/doc.go - for each ogen_client transport
func GetMockTemplates(params GeneratorAppParams) ([]ds.Files, []ds.Files, error) {
	// Skip if no ogen clients
	if !params.Application.HasOgenClients() {
		return nil, nil, nil
	}

	dirs := []ds.Files{}
	files := []ds.Files{}

	// Generate mocks.go from mocks.go.tmpl (in tests/ directory, same package as base_suite)
	mocksTemplate := "embedded/templates/mocks/mocks.go.tmpl"
	files = append(files, ds.Files{
		SourceName: mocksTemplate,
		DestName:   filepath.Join(testsPath, "mocks.go"),
		ParamsTmpl: params,
	})

	// For each ogen_client transport, generate doc.go
	docTemplate := "embedded/templates/mocks/files/doc.go.tmpl"
	for _, transport := range params.Application.GetOgenClients() {
		// Create handler params for this transport
		handlerParams := GeneratorHandlerParams{
			GeneratorParams: params.GeneratorParams,
			Transport:       transport,
			TransportParams: transport.GeneratorParams,
		}

		// Add directory for this transport's mocks
		transportMocksPath := filepath.Join(mocksPath, transport.Name)
		dirs = append(dirs, ds.Files{
			SourceName: docTemplate,
			DestName:   transportMocksPath,
			ParamsTmpl: handlerParams,
		})

		// Add doc.go file
		files = append(files, ds.Files{
			SourceName: docTemplate,
			DestName:   filepath.Join(transportMocksPath, "doc.go"),
			ParamsTmpl: handlerParams,
		})
	}

	return dirs, files, nil
}

// GetPackagingTemplates returns packaging templates for an application (nfpm, systemd, scripts).
// It generates files per application when packaging artifacts (deb/rpm/apk) are enabled.
func GetPackagingTemplates(params GeneratorAppParams) ([]ds.Files, []ds.Files, error) {
	// Skip if packaging is not enabled
	if !params.Artifacts.HasPackaging() {
		return nil, nil, nil
	}

	// Skip CLI applications (they don't have systemd services)
	if params.Application.IsCLI() {
		return nil, nil, nil
	}

	dirs := []ds.Files{}
	files := []ds.Files{}

	appPackagingPath := filepath.Join(packagingPath, params.Application.Name)
	systemdPath := filepath.Join(appPackagingPath, "systemd")
	scriptsPath := filepath.Join(appPackagingPath, "scripts")

	// Create directories
	dirs = append(dirs, ds.Files{
		DestName:   appPackagingPath,
		ParamsTmpl: params,
	})
	dirs = append(dirs, ds.Files{
		DestName:   systemdPath,
		ParamsTmpl: params,
	})
	dirs = append(dirs, ds.Files{
		DestName:   scriptsPath,
		ParamsTmpl: params,
	})

	// nfpm.yaml configuration
	nfpmTemplate := "embedded/templates/packaging/nfpm.yaml.tmpl"
	files = append(files, ds.Files{
		SourceName: nfpmTemplate,
		DestName:   filepath.Join(appPackagingPath, "nfpm.yaml"),
		ParamsTmpl: params,
	})

	// Systemd service file
	serviceTemplate := "embedded/templates/packaging/systemd/service.tmpl"
	serviceName := params.ProjectName + "-" + params.Application.Name + ".service"
	files = append(files, ds.Files{
		SourceName: serviceTemplate,
		DestName:   filepath.Join(systemdPath, serviceName),
		ParamsTmpl: params,
	})

	// Postinstall script
	postinstallTemplate := "embedded/templates/packaging/scripts/postinstall.sh.tmpl"
	files = append(files, ds.Files{
		SourceName: postinstallTemplate,
		DestName:   filepath.Join(scriptsPath, "postinstall.sh"),
		ParamsTmpl: params,
	})

	// Preremove script
	preremoveTemplate := "embedded/templates/packaging/scripts/preremove.sh.tmpl"
	files = append(files, ds.Files{
		SourceName: preremoveTemplate,
		DestName:   filepath.Join(scriptsPath, "preremove.sh"),
		ParamsTmpl: params,
	})

	return dirs, files, nil
}
