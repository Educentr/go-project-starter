package templater

import (
	"fmt"
	"path/filepath"
	"strings"
)

const (
	extYaml    = ".yaml"
	extYml     = ".yml"
	extGo      = ".go"
	extMod     = ".mod"
	extSum     = ".sum"
	extSQL     = ".sql"
	extMD      = ".md"
	extTxt     = ".txt"
	extSh      = ".sh"
	extDot     = "."
	extJSON    = ".json"
	extService = ".service"
)

func isFileIgnored(fName string) bool {
	switch fName {
	case ".keep":
		fallthrough
	case "LICENSE.txt":
		return true
	}

	switch filepath.Ext(fName) {
	case extMod:
		return true
	case extJSON:
		// JSON doesn't support comments, skip disclaimer
		return true
	}

	return false
}

func makeStartDisclaimer(fName string) (string, error) {
	_, fname := filepath.Split(fName)

	if isFileIgnored(fname) {
		return "", nil
	}

	prefix := ""

	switch filepath.Ext(fname) {
	case extMD:
		// ToDo надо доставать заголовок первого уровня из файла и вставлять его перед disclaimer
		prefix = "# " + fname + "\n\n"
	}

	text, err := MakeComment(fname, DisclaimerTop)
	if err != nil {
		return "", fmt.Errorf("error while make comment: %w", err)
	}

	return prefix + text, nil

}

func makeFinishDisclaimer(fName string) (string, error) {
	_, fname := filepath.Split(fName)

	if isFileIgnored(fname) {
		return "", nil
	}

	return MakeComment(fname, DisclaimerBottom)
}

func MakeComment(fName, text string) (string, error) {
	var (
		res         string
		commPrefix  string
		commPostfix string = ""
	)

	switch filepath.Ext(fName) {
	case extSQL:
		commPrefix = "--"
	case extGo:
		commPrefix = "//"
	case ".example":
		if !strings.HasPrefix(fName, ".env-") {
			return "", fmt.Errorf("unknown dot file: %s", fName)
		}

		fallthrough
	case extSh:
		fallthrough
	case extService:
		fallthrough
	case ".gitignore":
		fallthrough
	case extYml:
		fallthrough
	case extYaml:
		commPrefix = "#"
	case extTxt:
		commPrefix = ""
	case extMD:
		commPrefix = "<!--"
		commPostfix = "-->"
	case extMod:
		fallthrough
	case extSum:
		fallthrough
	case "":
		if strings.HasPrefix(fName, "Dockerfile") {
			fName = "Dockerfile"
		}

		switch fName {
		case "pre-commit":
			fallthrough
		case "Dockerfile":
			fallthrough
		case "Makefile":
			commPrefix = "#"
		default:
			return "", fmt.Errorf("unknown file: %s", fName)
		}
	default:
		return "", fmt.Errorf("unknown ext: %s", fName)
	}

	for _, ln := range strings.Split(text, "\n") {
		res += commPrefix + " " + ln + " " + commPostfix + "\n"
	}

	return res, nil
}
