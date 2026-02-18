package templater

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"text/template"

	"github.com/Educentr/go-project-starter/internal/pkg/ds"
	"github.com/Educentr/go-project-starter/internal/pkg/grafana"
	"github.com/pkg/errors"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// MinRuntimeVersion is the minimum supported version of go-project-starter-runtime.
// This should be updated when new runtime version is released.
const MinRuntimeVersion = "v0.12.0"

type GeneratorParams struct {
	AppInfo             string
	Logger              ds.Logger
	Author              string
	Year                string
	ProjectName         string
	ProjectPath         string
	RegistryType        string
	UseActiveRecord     bool
	DevStand            bool
	Repo                string
	PrivateRepos        string
	DockerImagePrefix   string
	SkipServiceInit     bool
	GoLangVersion       string
	OgenVersion         string
	ArgenVersion        string
	GolangciVersion     string
	RuntimeVersion      string
	GoJSONSchemaVersion string
	GoatVersion         string
	GoatServicesVersion string
	Drivers             ds.Drivers
	Workers             ds.Workers
	JSONSchemas         ds.JSONSchemas
	Kafka               ds.KafkaConfigs
	// GRPCVersion   string
	// Transports ds.Transtorts
	// Models ???
	Applications  ds.Apps
	Grafana       grafana.Config
	Artifacts     ds.ArtifactsConfig
	Documentation ds.DocsConfig
}

// type GeneratorParamDriver struct {
// 	PackageName string
// 	ImportPath  string
// }

type GeneratorRepositoryParams struct {
	Name       string //unused
	TypeDB     string //unused
	DriverName string //unused
}

type GeneratorAppParams struct {
	GeneratorParams
	Deploy      ds.DeployType
	Application ds.App
}

type GeneratorHandlerParams struct {
	GeneratorParams
	Transport       ds.Transport
	TransportParams map[string]string
}

type GeneratorRunnerParams struct {
	GeneratorParams
	Worker       ds.Worker
	WorkerParams map[string]string
}

// GeneratorKafkaParams holds parameters for Kafka driver template generation
//
//nolint:decorder // follows existing pattern - types after consts
type GeneratorKafkaParams struct {
	GeneratorParams
	Kafka ds.KafkaConfig
}

type Template struct {
	Name string
	Tmpl string
}

type TemplateCache struct {
	sync.Mutex
	templates map[string]Template
}

// ToDo кажется эта константа бесполезная
const (
	TemplateName string = "ProjectStarterGeneratorTemplate"
)

var (
	tmplErrRx = regexp.MustCompile(`template: .*?:(\d+):(\d+:)? `)
	// importErrRx = regexp.MustCompile(`(\d+):(\d+): `)
)

var cache = TemplateCache{
	templates: make(map[string]Template),
}

func GetTemplate(filename string) (Template, error) {
	cache.Lock()
	defer cache.Unlock()

	if tmpl, ok := cache.templates[filename]; ok {
		return tmpl, nil
	}

	file, err := templates.ReadFile(filename)
	if err != nil {
		return Template{}, fmt.Errorf("failed to read %s: %w", filename, err)
	}

	tmpl := Template{
		Name: filename,
		Tmpl: string(file),
	}

	cache.templates[filename] = tmpl

	return tmpl, nil
}

func GenerateFilenameByTmpl(file *ds.Files, targetPath string, lastVer int) error {
	buf := new(strings.Builder)

	var funcs = template.FuncMap{
		"ToLower":     strings.ToLower,
		"ToUpper":     strings.ToUpper,
		"ReplaceDash": func(s string) string { return strings.ReplaceAll(s, "-", "_") },
		"Capitalize":  cases.Title(language.Und).String,
	}

	templatePackage, err := template.New(TemplateName).Funcs(funcs).Parse(file.DestName)
	if err != nil {
		return fmt.Errorf("error parse template name `%s`: %w", file.DestName, err)
	}

	if err = templatePackage.Execute(buf, file.ParamsTmpl); err != nil {
		return fmt.Errorf("error execute template name `%s`: %w", file.DestName, err)
	}

	destFileName := buf.String()

	if pos := strings.Index(destFileName, ".go"); pos > 0 && pos == len(destFileName)-3 {
		dir, fName := filepath.Split(destFileName)

		// Special handling for _test.go files to preserve Go test file naming convention
		// Go requires test files to end with _test.go, not _gen.go
		if strings.HasSuffix(fName, "_test.go") {
			// Convert main_test.go -> psg_main_test.go (not psg_main_test_gen.go)
			destFileName = filepath.Join(dir, "psg_"+fName)
		} else {
			destFileName = filepath.Join(dir, "psg_"+fName[:len(fName)-3]+"_gen.go")
		}
	}

	oldFileName := destFileName

	// ToDo сделать миграции
	switch {
	case lastVer < 2:
		oldFileName = buf.String()
	case lastVer < 3:
		if strings.Contains(destFileName, "pkg/ds") {
			oldFileName = strings.ReplaceAll(destFileName, "pkg/ds", "pkg/app/ds")
		}
	}

	file.DestName = filepath.Join(targetPath, destFileName)
	file.OldDestName = filepath.Join(targetPath, oldFileName)

	return nil
}

// ToDo сделать проверку, что эта строка есть в файле дисклеймера
// для случаев, когда дисклеймер меняется, надо поддержать массив строк для поиска
// CI должен проверять, что массив строк "никогда" не уменьшается, что бы не нарушать обратную совместимость
const (
	disclaimer = "If you need you can add your code after this message"
)

var (
	ignoreExistingPath = []string{
		".git/",
		"docs/",
	}
	ignoreIfExistsFiles = map[string]struct{}{
		".gitignore":           {},
		"go.mod":               {},
		"go.sum":               {},
		"LICENSE.txt":          {},
		"README.md":            {},
		"etc/onlineconf/.keep": {},
		"public/.keep":         {}, // ToDo ignore all keep files
	}
)

func isFileIgnore(path string) bool {
	if _, ok := ignoreIfExistsFiles[path]; ok {
		return true
	}

	for _, ignorePath := range ignoreExistingPath {
		if strings.HasPrefix(path, ignorePath) {
			return true
		}
	}

	return false
}

func GetUserCodeFromFiles(targetDir string, files []ds.Files) (ds.FilesDiff, error) {
	filesDiff := ds.FilesDiff{
		NewFiles:       make(map[string]struct{}),
		IgnoreFiles:    make(map[string]struct{}),
		NewDirectory:   make(map[string]struct{}),
		OtherFiles:     make(map[string]struct{}),
		OtherDirectory: make(map[string]struct{}),
		UserContent:    make(map[string][]byte),
		RenameFiles:    make(map[string]string),
	}

	for _, file := range files {
		filesDiff.NewFiles[file.DestName] = struct{}{}
		dir := file.DestName

		for {
			dir, _ = filepath.Split(dir)
			if _, ex := filesDiff.NewDirectory[dir]; ex || len(dir) < len(targetDir) {
				break
			}

			filesDiff.NewDirectory[dir] = struct{}{}
			dir = dir[:len(dir)-1]
		}

		if file.OldDestName != file.DestName {
			filesDiff.RenameFiles[file.OldDestName] = file.DestName
		}
	}

	foundDirs := make(map[string]struct{})

	err := fs.WalkDir(os.DirFS(targetDir), ".", func(relPath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		path := filepath.Join(targetDir, relPath)

		if isFileIgnore(relPath) {
			delete(filesDiff.NewFiles, path)
			filesDiff.IgnoreFiles[path] = struct{}{}

			return nil
		}

		newFile := path
		if renameFile, ex := filesDiff.RenameFiles[path]; ex {
			newFile = renameFile
		}

		if d.IsDir() {
			newFile = newFile + "/"
			if _, ok := filesDiff.NewDirectory[newFile]; ok {
				foundDirs[newFile] = struct{}{}
			} else {
				filesDiff.OtherDirectory[newFile] = struct{}{}
			}

			return nil
		}

		if _, ex := filesDiff.NewFiles[newFile]; ex {
			delete(filesDiff.NewFiles, newFile)

			// Files without disclaimer support (e.g., JSON) are fully overwritten
			// No user code to preserve, just continue
			_, fname := filepath.Split(path)
			if isFileIgnored(fname) {
				return nil
			}

			fileContent, err := os.ReadFile(path)
			if err != nil {
				return err
			}

			_, userData, err := splitDisclaimer(string(fileContent))
			if err != nil {
				// ToDo сделать force режим
				return errors.Wrap(err, "error split disclaimer in file "+path)
			}

			if len(userData) > 0 {
				filesDiff.UserContent[newFile] = []byte(userData)
			}
		} else {
			// Files without disclaimer support (e.g., JSON) - just mark as other files
			_, fname := filepath.Split(path)
			if isFileIgnored(fname) {
				filesDiff.OtherFiles[path] = struct{}{}

				return nil
			}

			fileContent, err := os.ReadFile(path)
			if err != nil {
				return err
			}

			// ToDo сделать удаление старых файлов с дисклеймером которые больше не будут генерироваться
			_, userData, err := splitDisclaimer(string(fileContent))
			if err == nil && len(userData) > 0 {
				// ToDo сделать миграции с возможностью указать перемещение файлов из одного места в другое, что бы пользовательский код переносить
				return errors.New("found user code in stale gen file " + targetDir + " / " + path)
			}

			filesDiff.OtherFiles[path] = struct{}{}
		}

		return nil
	})
	if err != nil {
		return ds.FilesDiff{}, err
	}

	for delDir := range foundDirs {
		delete(filesDiff.NewDirectory, delDir)
	}

	return filesDiff, nil
}

func splitDisclaimer(fileContent string) (string, string, error) {
	disclamerFind := strings.Index(fileContent, disclaimer)
	if disclamerFind == -1 {
		return fileContent, "", errors.New("end disclaimer not found in file")
	}

	newLine := strings.Index(fileContent[disclamerFind+len(disclaimer):], "\n")
	if newLine == -1 {
		return fileContent[:disclamerFind], "", nil
	}

	userData := fileContent[disclamerFind+len(disclaimer)+newLine+1:]

	return fileContent[:disclamerFind+len(disclaimer)+newLine+1], userData, nil
}

const bufferSizeStep = 1024

func GenerateByTmpl(tmpl Template, params any, userCode []byte, destPath string) (*bytes.Buffer, error) {
	startDisclaimer, err := makeStartDisclaimer(destPath)
	if err != nil {
		return nil, err
	}

	finishDisclaimer, err := makeFinishDisclaimer(destPath)
	if err != nil {
		return nil, err
	}

	var funcs = template.FuncMap{
		"ToLower":     strings.ToLower,
		"ToUpper":     strings.ToUpper,
		"ReplaceDash": func(s string) string { return strings.ReplaceAll(s, "-", "_") },
		"Capitalize":  cases.Title(language.Und).String,
		"CapitalizeFirst": func(s string) string {
			if s == "" {
				return s
			}
			// Split by both "-" and "_" to produce PascalCase
			parts := strings.FieldsFunc(s, func(r rune) bool {
				return r == '-' || r == '_'
			})
			for i, p := range parts {
				if p == "" {
					continue
				}
				parts[i] = strings.ToUpper(p[:1]) + p[1:]
			}
			return strings.Join(parts, "")
		},
		"errorf": func(format string, args ...any) error { return fmt.Errorf(format, args...) },
		"add":    func(a, b int) int { return a + b },
		"escapeJSON": func(s string) string {
			// Escape special characters for JSON string
			s = strings.ReplaceAll(s, `\`, `\\`)
			s = strings.ReplaceAll(s, `"`, `\"`)
			return s
		},
		"ImageNameToPullerService": func(image string) string {
			// Extract image name from path like ghcr.io/org/name:tag -> name
			lastSlash := strings.LastIndex(image, "/")
			name := image
			if lastSlash >= 0 {
				name = image[lastSlash+1:]
			}
			// Remove tag
			if colonIndex := strings.Index(name, ":"); colonIndex >= 0 {
				name = name[:colonIndex]
			}
			return name + "-image-puller"
		},
	}

	buf := &bytes.Buffer{}

	buf.Grow(len(userCode) + bufferSizeStep*((len(startDisclaimer)+len(tmpl.Tmpl)+len(finishDisclaimer))/bufferSizeStep) + 1)

	templatePackage, err := template.New(destPath).Funcs(funcs).Parse(startDisclaimer + tmpl.Tmpl + "\n" + finishDisclaimer)
	if err != nil {
		tmplLines, errGetLine := getTmplErrorLine(strings.SplitAfter(startDisclaimer+tmpl.Tmpl+"\n"+finishDisclaimer, "\n"), err.Error())
		if errGetLine != nil {
			tmplLines = errGetLine.Error()
		}

		return nil, errors.New("error parse template `" + tmpl.Name + "` at line " + tmplLines + ": " + err.Error())
	}

	if err = templatePackage.Execute(buf, params); err != nil {
		tmplLines, errGetLine := getTmplErrorLine(strings.SplitAfter(startDisclaimer+tmpl.Tmpl+"\n"+finishDisclaimer, "\n"), err.Error())
		if errGetLine != nil {
			tmplLines = errGetLine.Error()
		}

		return nil, errors.New("error execute template `" + tmpl.Name + "` at line " + tmplLines + ": " + err.Error())
	}

	if len(userCode) > 0 {
		buf.Write(userCode)
	}

	return buf, nil
}

func getTmplErrorLine(lines []string, tmplErr string) (string, error) {
	lineTmpl := tmplErrRx.FindStringSubmatch(tmplErr)
	if len(lineTmpl) > 1 {
		lineNum, errParse := strconv.ParseInt(lineTmpl[1], 10, 64)
		if errParse != nil {
			return "", errors.New("error get line from error")
		} else if len(lines) == 0 {
			return "", errors.New("empty lines in template")
		} else {
			cntLine := 3
			startLine := int(lineNum) - cntLine - 1
			if startLine < 0 {
				startLine = 0
			}

			stopLine := int(lineNum) + cntLine
			if stopLine > int(lineNum) {
				stopLine = int(lineNum)
			}

			errorLines := lines[startLine:stopLine]
			for num := range errorLines {
				if num == cntLine {
					errorLines[num] = "-->> " + errorLines[num]
				} else {
					errorLines[num] = "     " + errorLines[num]
				}
			}

			return "\n" + strings.Join(errorLines, ""), nil
		}
	} else {
		return "", errors.New("can't get line from error")
	}
}
