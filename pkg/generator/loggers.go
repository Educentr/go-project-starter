package generator

import (
	"fmt"
	"strings"
)

var LoggerMapping = map[string]Logger{
	"zerolog": &ZlogLogger{},
}

type ZlogLogger struct{}

func (zl *ZlogLogger) getAddParams(params ...string) string {
	addParams := strings.Join(params, ".")
	if addParams != "" {
		addParams = fmt.Sprintf(`.%s`, addParams)
	}

	return addParams
}

func (zl *ZlogLogger) ErrorMsg(ctx, err, msg string, params ...string) string {
	return fmt.Sprintf("zlog.Ctx(%s).Error()%s.Err(%s).Msg(%s)", ctx, zl.getAddParams(), err, msg)
}

func (zl *ZlogLogger) WarnMsg(ctx, msg string, params ...string) string {
	return fmt.Sprintf("zlog.Ctx(%s).Warn()%s.Msg(%s)", ctx, zl.getAddParams(), msg)
}

func (zl *ZlogLogger) InfoMsg(ctx, msg string, params ...string) string {
	return fmt.Sprintf("zlog.Ctx(%s).Info()%s.Msg(%s)", ctx, zl.getAddParams(), msg)
}

func (zl *ZlogLogger) Import() string {
	return `zlog "github.com/rs/zerolog"`
}
