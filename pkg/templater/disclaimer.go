package templater

import (
	"fmt"
	"path/filepath"
	"strings"
)

const (
	extYaml = ".yaml"
	extYml  = ".yml"
	extGo   = ".go"
	extMod  = ".mod"
	extSum  = ".sum"
	extSQL  = ".sql"
	extMD   = ".md"
	extTxt  = ".txt"
	extSh   = ".sh"
	extDot  = "."
)

func makeStartDisclaimer(fName string) (string, error) {
	_, fname := filepath.Split(fName)
	prefix := ""
	switch filepath.Ext(fname) {
	case extMD:
		prefix = "# " + fname + "\n\n"
	case extMod:
		return "", nil
	}

	text, err := makeComment(fname, disclaimerTop)
	if err != nil {
		return "", fmt.Errorf("error while make comment: %w", err)
	}

	return prefix + text, nil

}

func makeFinishDisclaimer(fName string) (string, error) {
	_, fname := filepath.Split(fName)
	switch filepath.Ext(fname) {
	case extMod:
		return "", nil
	}

	return makeComment(fname, disclaimerBottom)
}

func makeComment(fName, text string) (string, error) {
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
	case extSh:
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

	return res + "\n", nil
}
