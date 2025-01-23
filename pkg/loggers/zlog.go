package loggers

import (
	"fmt"
	"strings"
)

type ZlogLogger struct{}

func (zl *ZlogLogger) getAddParams(params ...string) string {
	addParams := strings.Join(params, ".")
	if len(addParams) > 100 {
		addParams = strings.Join(params, ".\n")
	}

	if addParams != "" {
		if len(addParams) > 100 {
			addParams = fmt.Sprintf(".\n%s.\n", addParams)
		} else {
			addParams = fmt.Sprintf(".%s.", addParams)
		}
	} else {
		addParams = "."
	}

	return addParams
}

func (zl *ZlogLogger) ErrorMsg(ctx, err, msg string, params ...string) string {
	return fmt.Sprintf("zlog.Ctx(%s).Error()%sErr(%s).Msg(\"%s\")", ctx, zl.getAddParams(params...), err, msg)
}

func (zl *ZlogLogger) WarnMsg(ctx, msg string, params ...string) string {
	return fmt.Sprintf("zlog.Ctx(%s).Warn()%sMsg(\"%s\")", ctx, zl.getAddParams(params...), msg)
}

func (zl *ZlogLogger) InfoMsg(ctx, msg string, params ...string) string {
	return fmt.Sprintf("zlog.Ctx(%s).Info()%sMsg(\"%s\")", ctx, zl.getAddParams(params...), msg)
}

func (zl *ZlogLogger) UpdateContext(params ...string) string {
	return fmt.Sprintf(`zlog.Ctx(%s).UpdateContext(func(c zlog.Context) zlog.Context {
		return c.%s
	})`, params[0], strings.Join(params[1:], "."))
}

// ToDo сделать проверку на то, что логгер не используется в шаблонах напрямую и положить в CI
func (zl *ZlogLogger) Import() string {
	return `zlog "github.com/rs/zerolog"`
}

func (zl *ZlogLogger) FilesToGenerate() string {
	return "zlog"
}

func (zl *ZlogLogger) DestDir() string {
	return "pkg/logger"
}

func (zl *ZlogLogger) InitLogger(ctx string, serviceName string) string {
	return fmt.Sprintf("logger.InitZlog(%s, %s)", ctx, serviceName)
}
