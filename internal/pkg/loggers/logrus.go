package loggers

import (
	"fmt"
	"strings"
)

type LogrusLogger struct{}

func (ll *LogrusLogger) convertParam(param string) string {
	parts := strings.SplitN(param, "::", 3)

	if len(parts) == 2 && parts[0] == "err" {
		return fmt.Sprintf("WithError(%s)", parts[1])
	}

	if len(parts) != 3 {
		return param // fallback
	}

	key := formatKey(parts[1])
	value := parts[2]

	// logrus uses WithField for all types
	return fmt.Sprintf("WithField(%s, %s)", key, value)
}

func (ll *LogrusLogger) getAddParams(params ...string) string {
	if len(params) == 0 {
		return ""
	}

	converted := make([]string, 0, len(params))
	for _, p := range params {
		converted = append(converted, ll.convertParam(p))
	}

	return "." + strings.Join(converted, ".")
}

func (ll *LogrusLogger) ErrorMsg(ctx, err, msg string, params ...string) string {
	if err == "nil" {
		return fmt.Sprintf("logger.LogrusFromContext(%s)%s.Error(\"%s\")", ctx, ll.getAddParams(params...), msg)
	}

	return fmt.Sprintf("logger.LogrusFromContext(%s)%s.WithError(%s).Error(\"%s\")", ctx, ll.getAddParams(params...), err, msg)
}

func (ll *LogrusLogger) WarnMsg(ctx, msg string, params ...string) string {
	return fmt.Sprintf("logger.LogrusFromContext(%s)%s.Warn(\"%s\")", ctx, ll.getAddParams(params...), msg)
}

func (ll *LogrusLogger) InfoMsg(ctx, msg string, params ...string) string {
	return fmt.Sprintf("logger.LogrusFromContext(%s)%s.Info(\"%s\")", ctx, ll.getAddParams(params...), msg)
}

func (ll *LogrusLogger) DebugMsg(ctx, msg string, params ...string) string {
	return fmt.Sprintf("logger.LogrusFromContext(%s)%s.Debug(\"%s\")", ctx, ll.getAddParams(params...), msg)
}

func (ll *LogrusLogger) ErrorMsgCaller(ctx, err, msg string, callerSkip int, params ...string) string {
	// logrus doesn't have built-in caller skip like zerolog.
	// Use runtime.Caller to get the caller info and add it as a field.
	callerCode := fmt.Sprintf("func() string { _, file, line, _ := runtime.Caller(%d); return file + \":\" + strconv.Itoa(line) }()", callerSkip)

	if err == "nil" {
		return fmt.Sprintf("logger.LogrusFromContext(%s)%s.WithField(\"caller\", %s).Error(\"%s\")",
			ctx, ll.getAddParams(params...), callerCode, msg)
	}

	return fmt.Sprintf("logger.LogrusFromContext(%s)%s.WithField(\"caller\", %s).WithError(%s).Error(\"%s\")",
		ctx, ll.getAddParams(params...), callerCode, err, msg)
}

func (ll *LogrusLogger) UpdateContext(params ...string) string {
	if len(params) < 2 {
		return ""
	}

	ctxVar := params[0]
	converted := make([]string, 0, len(params)-1)

	for _, p := range params[1:] {
		converted = append(converted, ll.convertParam(p))
	}

	return fmt.Sprintf(`%s = logger.LogrusToContext(%s, logger.LogrusFromContext(%s).%s)`,
		ctxVar, ctxVar, ctxVar, strings.Join(converted, "."))
}

func (ll *LogrusLogger) SubContext(ctxVar string, params ...string) string {
	converted := make([]string, 0, len(params))
	for _, p := range params {
		converted = append(converted, ll.convertParam(p))
	}

	return fmt.Sprintf("%s = logger.LogrusToContext(%s, logger.LogrusFromContext(%s).%s)",
		ctxVar, ctxVar, ctxVar, strings.Join(converted, "."))
}

func (ll *LogrusLogger) Import() string {
	return `"github.com/sirupsen/logrus"`
}

func (ll *LogrusLogger) FilesToGenerate() string {
	return "logrus"
}

func (ll *LogrusLogger) DestDir() string {
	return "pkg/app/logger"
}

func (ll *LogrusLogger) InitLogger(ctx string, serviceName string) string {
	return fmt.Sprintf("logger.InitLogrus(%s, %s)", ctx, serviceName)
}

func (ll *LogrusLogger) ReWrap(sourceCtx, destCtx, ocPrefix, ocPath string) string {
	return fmt.Sprintf("%s = logger.ReWrap(%s, %s, %s, %s)", destCtx, sourceCtx, destCtx, ocPrefix, ocPath)
}

func (ll *LogrusLogger) SetLoggerUpdater() string {
	return "reqctx.SetLoggerUpdater(runtimelogger.NewLogrusUpdater())"
}

func (ll *LogrusLogger) SetupTestLogger(ctxVar string) string {
	return fmt.Sprintf(`testLogger := logrus.New()
	testLogger.SetOutput(os.Stdout)
	testLogger.SetLevel(logrus.DebugLevel)
	testEntry := testLogger.WithField("service", "test")
	%s = logger.LogrusToContext(%s, testEntry)`, ctxVar, ctxVar)
}
