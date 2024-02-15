package generator

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"text/template"

	"github.com/pkg/errors"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var (
	tmplErrRx   = regexp.MustCompile(TemplateName + `:(\d+):`)
	importErrRx = regexp.MustCompile(`(\d+):(\d+): `)
)

type Template struct {
	Name string
	Tmpl string
}

type TemplateCache struct {
	sync.Mutex
	templates map[string]Template
}

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

func GenerateFilenameByTmpl(file Files, params any) (string, error) {
	buf := new(strings.Builder)

	var funcs = template.FuncMap{
		"ToLower":    strings.ToLower,
		"ToUpper":    strings.ToUpper,
		"Capitalize": cases.Title(language.Und).String,
	}

	templatePackage, err := template.New(TemplateName).Funcs(funcs).Parse(file.DestTmplName)
	if err != nil {
		return "", fmt.Errorf("error parse template name `%s` at line %s: %w", file.DestTmplName, err)
	}

	if err = templatePackage.Execute(buf, params); err != nil {
		return "", fmt.Errorf("error execute template name `%s` at line %s: %w", file.DestTmplName, err)
	}

	return buf.String(), nil
}

func GenerateByTmpl(tmpl Template, params any, destPath string) error {
	dstFile, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	startDisclaimer, err := makeComment(destPath, disclaimer)
	if err != nil {
		return err
	}

	finishDisclaimer := makeFinishDisclaimer()
	if err != nil {
		return err
	}

	var funcs = template.FuncMap{
		"ToLower":    strings.ToLower,
		"ToUpper":    strings.ToUpper,
		"Capitalize": cases.Title(language.Und).String,
	}

	templatePackage, err := template.New(TemplateName).Funcs(funcs).Parse(startDisclaimer + tmpl.Tmpl + "\n" + finishDisclaimer)
	if err != nil {
		tmplLines, errGetLine := getTmplErrorLine(strings.SplitAfter(startDisclaimer+tmpl.Tmpl+"\n"+finishDisclaimer, "\n"), err.Error())
		if errGetLine != nil {
			tmplLines = errGetLine.Error()
		}

		return errors.New("error parse template `" + tmpl.Name + "` at line " + tmplLines + ": " + err.Error())
	}

	if err = templatePackage.Execute(dstFile, params); err != nil {
		tmplLines, errGetLine := getTmplErrorLine(strings.SplitAfter(startDisclaimer+tmpl.Tmpl+"\n"+finishDisclaimer, "\n"), err.Error())
		if errGetLine != nil {
			tmplLines = errGetLine.Error()
		}

		return errors.New("error execute template `" + tmpl.Name + "` at line " + tmplLines + ": " + err.Error())
	}

	return nil
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

func fetchFileName(path, def string) string {
	if parts := strings.Split(path, "/"); len(parts) > 0 {
		return parts[len(parts)-1]
	}

	return def
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
