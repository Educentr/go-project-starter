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

	"github.com/pkg/errors"
	"gitlab.educentr.info/golang/service-starter/pkg/ds"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

type GeneratorParams struct {
	AppInfo           string
	Logger            ds.Logger
	Author            string
	Year              string
	ProjectName       string
	ProjectPath       string
	DockerImagePrefix string
	SkipServiceInit   bool
	GoLangVersion     string
	OgenVersion       string
	// GRPCVersion   string
	// Transports ds.Transtorts
	Drivers []ds.IDriver
	// Models ???
	Applications ds.Apps
}

type GeneratorRepositoryParams struct {
	Name       string //unused
	TypeDB     string //unused
	DriverName string //unused
}

type GeneratorAppParams struct {
	GeneratorParams
	Application ds.App
}

type GeneratorHandlerParams struct {
	GeneratorParams              //unused
	Transport       ds.Transport //unused
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
	tmplErrRx   = regexp.MustCompile(`template: .*?:(\d+):(\d+:)? `)
	importErrRx = regexp.MustCompile(`(\d+):(\d+): `)
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

func GenerateFilenameByTmpl(file ds.Files) (string, error) {
	buf := new(strings.Builder)

	var funcs = template.FuncMap{
		"ToLower":    strings.ToLower,
		"ToUpper":    strings.ToUpper,
		"Capitalize": cases.Title(language.Und).String,
	}

	templatePackage, err := template.New(TemplateName).Funcs(funcs).Parse(file.DestName)
	if err != nil {
		return "", fmt.Errorf("error parse template name `%s`: %w", file.DestName, err)
	}

	if err = templatePackage.Execute(buf, file.ParamsTmpl); err != nil {
		return "", fmt.Errorf("error execute template name `%s`: %w", file.DestName, err)
	}

	return buf.String(), nil
}

// ToDo сделать проверку, что эта строка есть в файле дисклеймера
// для случаев, когда дисклеймер меняется, надо поддержать массив строк для поиска
// CI должен проверять, что массив строк "никогда" не уменьшается, что бы не нарушать обратную совместимость
const (
	disclaimer = "If you need you can add your code after this message"
)

var (
	ignoreIfExistsPath = map[string]struct{}{
		".git/hooks/pre-commit": {},
		"go.mod":                {},
		"go.sum":                {},
		"LICENSE.txt":           {},
		"README.md":             {},
		"etc/onlineconf/.keep":  {},
	}
)

func GetUserCodeFromFiles(targetDir string, files []ds.Files) (ds.FilesDiff, error) {
	filesDiff := ds.FilesDiff{
		NewFiles:     make(map[string]struct{}),
		IgnoreFiles:  make(map[string]struct{}),
		NewDirectory: make(map[string]struct{}),
		OldFiles:     make(map[string]struct{}),
		OldDirectory: make(map[string]struct{}),
		UserContent:  make(map[string][]byte),
	}

	for _, file := range files {
		filesDiff.NewFiles[file.DestName] = struct{}{}
		dir, _ := filepath.Split(file.DestName)
		filesDiff.NewDirectory[dir] = struct{}{}
	}

	err := fs.WalkDir(os.DirFS(targetDir), ".", func(relPath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		path := filepath.Join(targetDir, relPath)

		if _, ok := ignoreIfExistsPath[relPath]; ok {
			delete(filesDiff.NewFiles, path)
			filesDiff.IgnoreFiles[path] = struct{}{}

			return nil
		}

		if d.IsDir() {
			path = path + "/"
			if _, ok := filesDiff.NewDirectory[path]; ok {
				delete(filesDiff.NewDirectory, path)
			} else {
				filesDiff.OldDirectory[path] = struct{}{}
			}

			return nil
		}

		if _, ok := filesDiff.NewFiles[path]; ok {
			delete(filesDiff.NewFiles, path)
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
				filesDiff.UserContent[path] = []byte(userData)
			}
		} else {
			fileContent, err := os.ReadFile(path)
			if err != nil {
				return err
			}

			// ToDo сделать удаление старых файлов с дисклеймером которые больше не будут генерироваться
			_, userData, err := splitDisclaimer(string(fileContent))
			if err == nil && len(userData) > 0 {
				// ToDo сделать миграции с возможностью указать перемещение файлов из одного места в другое, что бы пользовательский код переносить
				return errors.New("end disclaimer found in file " + targetDir + " / " + path + " buf file won't be regenerated")
			}

			filesDiff.OldFiles[path] = struct{}{}
		}

		return nil
	})
	if err != nil {
		return ds.FilesDiff{}, err
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
		"ToLower":    strings.ToLower,
		"ToUpper":    strings.ToUpper,
		"Capitalize": cases.Title(language.Und).String,
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

func getImportErrorLine(lines []string, tmplerror string) (string, error) {
	lineTmpl := importErrRx.FindStringSubmatch(tmplerror)
	if len(lineTmpl) > 1 {
		lineNum, errParse := strconv.ParseInt(lineTmpl[1], 10, 64)
		if errParse != nil {
			return "", errors.New("error get line from error")
		} else if len(lines) == 0 {
			return "", errors.New("empty lines in template")
		} else {
			cntline := 3
			startLine := int(lineNum) - cntline - 1
			if startLine < 0 {
				startLine = 0
			}

			stopLine := int(lineNum) + cntline
			if stopLine > int(lineNum) {
				stopLine = int(lineNum)
			}

			errorLines := lines[startLine:stopLine]
			for num := range errorLines {
				if num == cntline {
					errorLines[num] = "-->> " + errorLines[num]
				} else {
					errorLines[num] = "     " + errorLines[num]
				}
			}

			return "\n" + strings.Join(errorLines, ""), nil
		}
	} else {
		return "", errors.New("can't get line from error `" + tmplerror + "`")
	}
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
