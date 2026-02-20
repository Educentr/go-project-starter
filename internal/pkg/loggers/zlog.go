package loggers

import (
	"fmt"
	"strings"
)

type ZlogLogger struct{}

// formatKey formats a key parameter. If the key starts with "$", it's treated as a variable reference.
// Otherwise, it's treated as a string literal and quoted.
func formatKey(key string) string {
	if strings.HasPrefix(key, "$") {
		return key[1:]
	}

	return fmt.Sprintf(`"%s"`, key)
}

// convertParam converts a universal format param (type::key::value) to zerolog method call.
// Supported formats:
//   - str::key::value  → Str("key", value)
//   - int::key::value  → Int("key", value)
//   - int64::key::value → Int64("key", value)
//   - any::key::value  → Interface("key", value)
//   - bool::key::value → Bool("key", value)
//   - err::value       → Err(value)
//
// Key prefixed with "$" is treated as a variable reference (no quotes).
func (zl *ZlogLogger) convertParam(param string) string {
	parts := strings.SplitN(param, "::", paramParts)

	if len(parts) == 2 && parts[0] == errParam {
		return fmt.Sprintf("Err(%s)", parts[1])
	}

	if len(parts) != 3 {
		return param // fallback: pass through as-is for backward compatibility
	}

	key := formatKey(parts[1])
	value := parts[2]

	switch parts[0] {
	case "str":
		return fmt.Sprintf("Str(%s, %s)", key, value)
	case "int":
		return fmt.Sprintf("Int(%s, %s)", key, value)
	case "int64":
		return fmt.Sprintf("Int64(%s, %s)", key, value)
	case "any":
		return fmt.Sprintf("Interface(%s, %s)", key, value)
	case "bool":
		return fmt.Sprintf("Bool(%s, %s)", key, value)
	default:
		return param // fallback
	}
}

func (zl *ZlogLogger) getAddParams(params ...string) string {
	converted := make([]string, 0, len(params))
	for _, p := range params {
		converted = append(converted, zl.convertParam(p))
	}

	addParams := strings.Join(converted, ".")
	if len(addParams) > 100 {
		addParams = strings.Join(converted, ".\n")
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
	if err == nilErr {
		return fmt.Sprintf("zlog.Ctx(%s).Error()%sMsg(\"%s\")", ctx, zl.getAddParams(params...), msg)
	}

	return fmt.Sprintf("zlog.Ctx(%s).Error()%sErr(%s).Msg(\"%s\")", ctx, zl.getAddParams(params...), err, msg)
}

func (zl *ZlogLogger) WarnMsg(ctx, msg string, params ...string) string {
	return fmt.Sprintf("zlog.Ctx(%s).Warn()%sMsg(\"%s\")", ctx, zl.getAddParams(params...), msg)
}

func (zl *ZlogLogger) InfoMsg(ctx, msg string, params ...string) string {
	return fmt.Sprintf("zlog.Ctx(%s).Info()%sMsg(\"%s\")", ctx, zl.getAddParams(params...), msg)
}

func (zl *ZlogLogger) DebugMsg(ctx, msg string, params ...string) string {
	return fmt.Sprintf("zlog.Ctx(%s).Debug()%sMsg(\"%s\")", ctx, zl.getAddParams(params...), msg)
}

func (zl *ZlogLogger) ErrorMsgCaller(ctx, err, msg string, callerSkip int, params ...string) string {
	if err == nilErr {
		return fmt.Sprintf("zlog.Ctx(%s).Error().Caller(%d)%sMsg(\"%s\")", ctx, callerSkip, zl.getAddParams(params...), msg)
	}

	return fmt.Sprintf("zlog.Ctx(%s).Error().Caller(%d)%sErr(%s).Msg(\"%s\")", ctx, callerSkip, zl.getAddParams(params...), err, msg)
}

func (zl *ZlogLogger) UpdateContext(params ...string) string {
	converted := make([]string, 0, len(params)-1)
	for _, p := range params[1:] {
		converted = append(converted, zl.convertParam(p))
	}

	return fmt.Sprintf(`zlog.Ctx(%s).UpdateContext(func(c zlog.Context) zlog.Context {
		return c.%s
	})`, params[0], strings.Join(converted, "."))
}

// ToDo сделать проверку на то, что логгер не используется в шаблонах напрямую и положить в CI
func (zl *ZlogLogger) Import() string {
	return `zlog "github.com/rs/zerolog"`
}

func (zl *ZlogLogger) FilesToGenerate() string {
	return "zlog"
}

func (zl *ZlogLogger) DestDir() string {
	return destDir
}

func (zl *ZlogLogger) InitLogger(ctx string, serviceName string) string {
	return fmt.Sprintf("logger.InitZlog(%s, %s)", ctx, serviceName)
}

// ReWrap generates code to rewrap logger from source context to destination context
func (zl *ZlogLogger) ReWrap(sourceCtx, destCtx, ocPrefix, ocPath string) string {
	return fmt.Sprintf("%s = logger.ReWrap(%s, %s, %s, %s)", destCtx, sourceCtx, destCtx, ocPrefix, ocPath)
}

// SetLoggerUpdater generates code to set the global logger updater for reqctx
func (zl *ZlogLogger) SetLoggerUpdater() string {
	return "reqctx.SetLoggerUpdater(runtimelogger.NewZerologUpdater())"
}

// SetEventLogger generates code to set the global event logger for runtime
func (zl *ZlogLogger) SetEventLogger() string {
	return "runtimelogger.SetEventLogger(runtimelogger.NewZerologEventLogger())"
}

// SubContext generates code to create a new context with a derived logger.
// Unlike UpdateContext which mutates the logger in-place, this creates a new
// sub-logger with additional fields and reassigns the context variable.
func (zl *ZlogLogger) SubContext(ctxVar string, params ...string) string {
	converted := make([]string, 0, len(params))
	for _, p := range params {
		converted = append(converted, zl.convertParam(p))
	}

	return fmt.Sprintf("%s = zlog.Ctx(%s).With().%s.Logger().WithContext(%s)",
		ctxVar, ctxVar, strings.Join(converted, "."), ctxVar)
}

// SetupTestLogger generates code to create a test zerolog logger and attach it to context
func (zl *ZlogLogger) SetupTestLogger(ctxVar string) string {
	return fmt.Sprintf(`testLogger := zlog.New(os.Stdout).With().
		Timestamp().
		Str("service", "test").
		Logger()

	%s = testLogger.WithContext(%s)`, ctxVar, ctxVar)
}
