package generator

import (
	"fmt"
	"strings"
)

func makeComment(fName, text string) (string, error) {
	var (
		res  string
		comm string
	)

	switch {
	case strings.HasSuffix(fName, extSQL):
		comm = "--"
	case strings.HasSuffix(fName, extGo):
		comm = "//"
	case strings.HasSuffix(fName, extSh):
		fallthrough
	case strings.HasSuffix(fName, extYml):
		fallthrough
	case strings.HasSuffix(fName, extYaml):
		comm = "#"
	case strings.HasSuffix(fName, extMod):
		fallthrough
	case strings.HasSuffix(fName, extSum):
		fallthrough
	case strings.HasSuffix(fName, extMD):
		fallthrough
	case !strings.Contains(fName, extDot):
		return "", nil
	default:
		return "", fmt.Errorf("unknown ext: %s", fName)
	}

	for _, ln := range strings.Split(text, "\n") {
		res += comm + " " + ln + "\n"
	}

	return res + "\n", nil
}

func makeFinishDisclaimer() string {
	return "ToDo make end disclaimer"
}
