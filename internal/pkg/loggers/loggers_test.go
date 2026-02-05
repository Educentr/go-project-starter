package loggers

import (
	"strings"
	"testing"

	"github.com/Educentr/go-project-starter/internal/pkg/ds"
)

// TestLoggerMapping verifies that LoggerMapping contains valid logger implementations.
func TestLoggerMapping(t *testing.T) {
	tests := []struct {
		name   string
		key    string
		wantOk bool
	}{
		{
			name:   "zerolog exists",
			key:    "zerolog",
			wantOk: true,
		},
		{
			name:   "logrus exists",
			key:    "logrus",
			wantOk: true,
		},
		{
			name:   "unknown logger does not exist",
			key:    "unknown",
			wantOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, ok := LoggerMapping[tt.key]

			if ok != tt.wantOk {
				t.Errorf("LoggerMapping[%q] exists = %v, want %v", tt.key, ok, tt.wantOk)
			}

			if tt.wantOk && logger == nil {
				t.Errorf("LoggerMapping[%q] = nil, want non-nil logger", tt.key)
			}
		})
	}
}

// TestLoggerInterface verifies that all loggers implement the ds.Logger interface.
func TestLoggerInterface(t *testing.T) {
	for name, logger := range LoggerMapping {
		t.Run(name, func(_ *testing.T) {
			var _ ds.Logger = logger // compile-time interface check
		})
	}
}

// TestFormatKey tests the formatKey helper function.
func TestFormatKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want string
	}{
		{
			name: "literal key gets quoted",
			key:  "user_id",
			want: `"user_id"`,
		},
		{
			name: "variable reference strips $ prefix",
			key:  "$nameFieldLogger",
			want: "nameFieldLogger",
		},
		{
			name: "empty key gets quoted",
			key:  "",
			want: `""`,
		},
		{
			name: "key with special chars gets quoted",
			key:  "user-id",
			want: `"user-id"`,
		},
		{
			name: "dollar sign only becomes empty string",
			key:  "$",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatKey(tt.key)
			if got != tt.want {
				t.Errorf("formatKey(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

// ----- ZlogLogger Tests -----

func TestZlogLogger_convertParam(t *testing.T) {
	zl := &ZlogLogger{}

	tests := []struct {
		name  string
		param string
		want  string
	}{
		{
			name:  "str type with literal key",
			param: "str::user_id::userID",
			want:  `Str("user_id", userID)`,
		},
		{
			name:  "str type with variable key",
			param: "str::$keyVar::value",
			want:  "Str(keyVar, value)",
		},
		{
			name:  "int type",
			param: "int::count::10",
			want:  `Int("count", 10)`,
		},
		{
			name:  "int64 type",
			param: "int64::timestamp::ts",
			want:  `Int64("timestamp", ts)`,
		},
		{
			name:  "any type",
			param: "any::data::payload",
			want:  `Interface("data", payload)`,
		},
		{
			name:  "bool type",
			param: "bool::active::isActive",
			want:  `Bool("active", isActive)`,
		},
		{
			name:  "err type (2 parts)",
			param: "err::errVar",
			want:  "Err(errVar)",
		},
		{
			name:  "unknown type falls back to passthrough",
			param: "unknown::key::value",
			want:  "unknown::key::value",
		},
		{
			name:  "invalid format (1 part) falls back",
			param: "invalid",
			want:  "invalid",
		},
		{
			name:  "invalid format (2 parts non-err) falls back",
			param: "str::key",
			want:  "str::key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := zl.convertParam(tt.param)
			if got != tt.want {
				t.Errorf("ZlogLogger.convertParam(%q) = %q, want %q", tt.param, got, tt.want)
			}
		})
	}
}

func TestZlogLogger_getAddParams(t *testing.T) {
	zl := &ZlogLogger{}

	tests := []struct {
		name   string
		params []string
		want   string
	}{
		{
			name:   "empty params returns dot",
			params: nil,
			want:   ".",
		},
		{
			name:   "single param",
			params: []string{"str::key::val"},
			want:   `.Str("key", val).`,
		},
		{
			name:   "multiple params joined with dot",
			params: []string{"str::a::aVal", "int::b::bVal"},
			want:   `.Str("a", aVal).Int("b", bVal).`,
		},
		{
			name: "long params get newlines",
			params: []string{
				"str::very_long_key_name_that_exceeds_normal_length::veryLongVariableNameThatIsAlsoVeryLong",
				"str::another_extremely_long_key_name::anotherExtremelyLongVariableNameHere",
			},
			want: ".\nStr(\"very_long_key_name_that_exceeds_normal_length\", veryLongVariableNameThatIsAlsoVeryLong).\nStr(\"another_extremely_long_key_name\", anotherExtremelyLongVariableNameHere).\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := zl.getAddParams(tt.params...)
			if got != tt.want {
				t.Errorf("ZlogLogger.getAddParams(%v) = %q, want %q", tt.params, got, tt.want)
			}
		})
	}
}

func TestZlogLogger_ErrorMsg(t *testing.T) {
	zl := &ZlogLogger{}

	tests := []struct {
		name   string
		ctx    string
		err    string
		msg    string
		params []string
		want   string
	}{
		{
			name: "with error",
			ctx:  "ctx",
			err:  "err",
			msg:  "operation failed",
			want: `zlog.Ctx(ctx).Error().Err(err).Msg("operation failed")`,
		},
		{
			name: "with nil error",
			ctx:  "ctx",
			err:  "nil",
			msg:  "operation completed",
			want: `zlog.Ctx(ctx).Error().Msg("operation completed")`,
		},
		{
			name:   "with params and error",
			ctx:    "ctx",
			err:    "err",
			msg:    "failed",
			params: []string{"str::user::u"},
			want:   `zlog.Ctx(ctx).Error().Str("user", u).Err(err).Msg("failed")`,
		},
		{
			name:   "with params and nil error",
			ctx:    "ctx",
			err:    "nil",
			msg:    "done",
			params: []string{"int::code::200"},
			want:   `zlog.Ctx(ctx).Error().Int("code", 200).Msg("done")`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := zl.ErrorMsg(tt.ctx, tt.err, tt.msg, tt.params...)
			if got != tt.want {
				t.Errorf("ZlogLogger.ErrorMsg() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestZlogLogger_WarnMsg(t *testing.T) {
	zl := &ZlogLogger{}

	tests := []struct {
		name   string
		ctx    string
		msg    string
		params []string
		want   string
	}{
		{
			name: "basic warn",
			ctx:  "ctx",
			msg:  "warning message",
			want: `zlog.Ctx(ctx).Warn().Msg("warning message")`,
		},
		{
			name:   "warn with params",
			ctx:    "ctx",
			msg:    "rate limit",
			params: []string{"int::limit::100"},
			want:   `zlog.Ctx(ctx).Warn().Int("limit", 100).Msg("rate limit")`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := zl.WarnMsg(tt.ctx, tt.msg, tt.params...)
			if got != tt.want {
				t.Errorf("ZlogLogger.WarnMsg() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestZlogLogger_InfoMsg(t *testing.T) {
	zl := &ZlogLogger{}

	tests := []struct {
		name   string
		ctx    string
		msg    string
		params []string
		want   string
	}{
		{
			name: "basic info",
			ctx:  "ctx",
			msg:  "server started",
			want: `zlog.Ctx(ctx).Info().Msg("server started")`,
		},
		{
			name:   "info with params",
			ctx:    "ctx",
			msg:    "connected",
			params: []string{"str::host::h", "int::port::p"},
			want:   `zlog.Ctx(ctx).Info().Str("host", h).Int("port", p).Msg("connected")`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := zl.InfoMsg(tt.ctx, tt.msg, tt.params...)
			if got != tt.want {
				t.Errorf("ZlogLogger.InfoMsg() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestZlogLogger_DebugMsg(t *testing.T) {
	zl := &ZlogLogger{}

	tests := []struct {
		name   string
		ctx    string
		msg    string
		params []string
		want   string
	}{
		{
			name: "basic debug",
			ctx:  "ctx",
			msg:  "debugging",
			want: `zlog.Ctx(ctx).Debug().Msg("debugging")`,
		},
		{
			name:   "debug with params",
			ctx:    "ctx",
			msg:    "variable dump",
			params: []string{"any::data::obj"},
			want:   `zlog.Ctx(ctx).Debug().Interface("data", obj).Msg("variable dump")`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := zl.DebugMsg(tt.ctx, tt.msg, tt.params...)
			if got != tt.want {
				t.Errorf("ZlogLogger.DebugMsg() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestZlogLogger_ErrorMsgCaller(t *testing.T) {
	zl := &ZlogLogger{}

	tests := []struct {
		name       string
		ctx        string
		err        string
		msg        string
		callerSkip int
		params     []string
		want       string
	}{
		{
			name:       "with error and caller",
			ctx:        "ctx",
			err:        "err",
			msg:        "error",
			callerSkip: 2,
			want:       `zlog.Ctx(ctx).Error().Caller(2).Err(err).Msg("error")`,
		},
		{
			name:       "with nil error and caller",
			ctx:        "ctx",
			err:        "nil",
			msg:        "error",
			callerSkip: 1,
			want:       `zlog.Ctx(ctx).Error().Caller(1).Msg("error")`,
		},
		{
			name:       "with params",
			ctx:        "ctx",
			err:        "err",
			msg:        "failed",
			callerSkip: 3,
			params:     []string{"str::op::operation"},
			want:       `zlog.Ctx(ctx).Error().Caller(3).Str("op", operation).Err(err).Msg("failed")`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := zl.ErrorMsgCaller(tt.ctx, tt.err, tt.msg, tt.callerSkip, tt.params...)
			if got != tt.want {
				t.Errorf("ZlogLogger.ErrorMsgCaller() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestZlogLogger_UpdateContext(t *testing.T) {
	zl := &ZlogLogger{}

	tests := []struct {
		name   string
		params []string
		want   string
	}{
		{
			name:   "single field",
			params: []string{"ctx", "str::request_id::reqID"},
			want: `zlog.Ctx(ctx).UpdateContext(func(c zlog.Context) zlog.Context {
		return c.Str("request_id", reqID)
	})`,
		},
		{
			name:   "multiple fields",
			params: []string{"ctx", "str::user::u", "int::count::c"},
			want: `zlog.Ctx(ctx).UpdateContext(func(c zlog.Context) zlog.Context {
		return c.Str("user", u).Int("count", c)
	})`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := zl.UpdateContext(tt.params...)
			if got != tt.want {
				t.Errorf("ZlogLogger.UpdateContext() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestZlogLogger_SubContext(t *testing.T) {
	zl := &ZlogLogger{}

	tests := []struct {
		name   string
		ctxVar string
		params []string
		want   string
	}{
		{
			name:   "single field",
			ctxVar: "ctx",
			params: []string{"str::worker::WorkerName"},
			want:   `ctx = zlog.Ctx(ctx).With().Str("worker", WorkerName).Logger().WithContext(ctx)`,
		},
		{
			name:   "multiple fields",
			ctxVar: "ctx",
			params: []string{"str::app::appName", "str::transport::transportName"},
			want:   `ctx = zlog.Ctx(ctx).With().Str("app", appName).Str("transport", transportName).Logger().WithContext(ctx)`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := zl.SubContext(tt.ctxVar, tt.params...)
			if got != tt.want {
				t.Errorf("ZlogLogger.SubContext() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestZlogLogger_Metadata(t *testing.T) {
	zl := &ZlogLogger{}

	t.Run("Import", func(t *testing.T) {
		want := `zlog "github.com/rs/zerolog"`
		if got := zl.Import(); got != want {
			t.Errorf("ZlogLogger.Import() = %q, want %q", got, want)
		}
	})

	t.Run("FilesToGenerate", func(t *testing.T) {
		want := "zlog"
		if got := zl.FilesToGenerate(); got != want {
			t.Errorf("ZlogLogger.FilesToGenerate() = %q, want %q", got, want)
		}
	})

	t.Run("DestDir", func(t *testing.T) {
		want := "pkg/app/logger"
		if got := zl.DestDir(); got != want {
			t.Errorf("ZlogLogger.DestDir() = %q, want %q", got, want)
		}
	})
}

func TestZlogLogger_InitLogger(t *testing.T) {
	zl := &ZlogLogger{}
	want := "logger.InitZlog(ctx, serviceName)"
	got := zl.InitLogger("ctx", "serviceName")

	if got != want {
		t.Errorf("ZlogLogger.InitLogger() = %q, want %q", got, want)
	}
}

func TestZlogLogger_ReWrap(t *testing.T) {
	zl := &ZlogLogger{}
	want := "destCtx = logger.ReWrap(sourceCtx, destCtx, prefix, path)"
	got := zl.ReWrap("sourceCtx", "destCtx", "prefix", "path")

	if got != want {
		t.Errorf("ZlogLogger.ReWrap() = %q, want %q", got, want)
	}
}

func TestZlogLogger_SetLoggerUpdater(t *testing.T) {
	zl := &ZlogLogger{}
	want := "reqctx.SetLoggerUpdater(runtimelogger.NewZerologUpdater())"
	got := zl.SetLoggerUpdater()

	if got != want {
		t.Errorf("ZlogLogger.SetLoggerUpdater() = %q, want %q", got, want)
	}
}

func TestZlogLogger_SetupTestLogger(t *testing.T) {
	zl := &ZlogLogger{}

	got := zl.SetupTestLogger("ctx")

	// Check key parts of the generated code
	if !strings.Contains(got, "testLogger := zlog.New(os.Stdout)") {
		t.Errorf("SetupTestLogger should create testLogger, got: %s", got)
	}

	if !strings.Contains(got, `Str("service", "test")`) {
		t.Errorf("SetupTestLogger should set service field, got: %s", got)
	}

	if !strings.Contains(got, "ctx = testLogger.WithContext(ctx)") {
		t.Errorf("SetupTestLogger should attach logger to context, got: %s", got)
	}
}

// ----- LogrusLogger Tests -----

func TestLogrusLogger_convertParam(t *testing.T) {
	ll := &LogrusLogger{}

	tests := []struct {
		name  string
		param string
		want  string
	}{
		{
			name:  "str type with literal key",
			param: "str::user_id::userID",
			want:  `WithField("user_id", userID)`,
		},
		{
			name:  "str type with variable key",
			param: "str::$keyVar::value",
			want:  "WithField(keyVar, value)",
		},
		{
			name:  "int type uses WithField",
			param: "int::count::10",
			want:  `WithField("count", 10)`,
		},
		{
			name:  "int64 type uses WithField",
			param: "int64::timestamp::ts",
			want:  `WithField("timestamp", ts)`,
		},
		{
			name:  "any type uses WithField",
			param: "any::data::payload",
			want:  `WithField("data", payload)`,
		},
		{
			name:  "bool type uses WithField",
			param: "bool::active::isActive",
			want:  `WithField("active", isActive)`,
		},
		{
			name:  "err type (2 parts)",
			param: "err::errVar",
			want:  "WithError(errVar)",
		},
		{
			name:  "unknown type falls back",
			param: "unknown::key::value",
			want:  `WithField("key", value)`,
		},
		{
			name:  "invalid format (1 part) falls back",
			param: "invalid",
			want:  "invalid",
		},
		{
			name:  "invalid format (2 parts non-err) falls back",
			param: "str::key",
			want:  "str::key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ll.convertParam(tt.param)
			if got != tt.want {
				t.Errorf("LogrusLogger.convertParam(%q) = %q, want %q", tt.param, got, tt.want)
			}
		})
	}
}

func TestLogrusLogger_getAddParams(t *testing.T) {
	ll := &LogrusLogger{}

	tests := []struct {
		name   string
		params []string
		want   string
	}{
		{
			name:   "empty params returns empty string",
			params: nil,
			want:   "",
		},
		{
			name:   "single param",
			params: []string{"str::key::val"},
			want:   `.WithField("key", val)`,
		},
		{
			name:   "multiple params joined with dot",
			params: []string{"str::a::aVal", "int::b::bVal"},
			want:   `.WithField("a", aVal).WithField("b", bVal)`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ll.getAddParams(tt.params...)
			if got != tt.want {
				t.Errorf("LogrusLogger.getAddParams(%v) = %q, want %q", tt.params, got, tt.want)
			}
		})
	}
}

func TestLogrusLogger_ErrorMsg(t *testing.T) {
	ll := &LogrusLogger{}

	tests := []struct {
		name   string
		ctx    string
		err    string
		msg    string
		params []string
		want   string
	}{
		{
			name: "with error",
			ctx:  "ctx",
			err:  "err",
			msg:  "operation failed",
			want: `rlog.LogrusFromContext(ctx).WithError(err).Error("operation failed")`,
		},
		{
			name: "with nil error",
			ctx:  "ctx",
			err:  "nil",
			msg:  "operation completed",
			want: `rlog.LogrusFromContext(ctx).Error("operation completed")`,
		},
		{
			name:   "with params and error",
			ctx:    "ctx",
			err:    "err",
			msg:    "failed",
			params: []string{"str::user::u"},
			want:   `rlog.LogrusFromContext(ctx).WithField("user", u).WithError(err).Error("failed")`,
		},
		{
			name:   "with params and nil error",
			ctx:    "ctx",
			err:    "nil",
			msg:    "done",
			params: []string{"int::code::200"},
			want:   `rlog.LogrusFromContext(ctx).WithField("code", 200).Error("done")`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ll.ErrorMsg(tt.ctx, tt.err, tt.msg, tt.params...)
			if got != tt.want {
				t.Errorf("LogrusLogger.ErrorMsg() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLogrusLogger_WarnMsg(t *testing.T) {
	ll := &LogrusLogger{}

	tests := []struct {
		name   string
		ctx    string
		msg    string
		params []string
		want   string
	}{
		{
			name: "basic warn",
			ctx:  "ctx",
			msg:  "warning message",
			want: `rlog.LogrusFromContext(ctx).Warn("warning message")`,
		},
		{
			name:   "warn with params",
			ctx:    "ctx",
			msg:    "rate limit",
			params: []string{"int::limit::100"},
			want:   `rlog.LogrusFromContext(ctx).WithField("limit", 100).Warn("rate limit")`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ll.WarnMsg(tt.ctx, tt.msg, tt.params...)
			if got != tt.want {
				t.Errorf("LogrusLogger.WarnMsg() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLogrusLogger_InfoMsg(t *testing.T) {
	ll := &LogrusLogger{}

	tests := []struct {
		name   string
		ctx    string
		msg    string
		params []string
		want   string
	}{
		{
			name: "basic info",
			ctx:  "ctx",
			msg:  "server started",
			want: `rlog.LogrusFromContext(ctx).Info("server started")`,
		},
		{
			name:   "info with params",
			ctx:    "ctx",
			msg:    "connected",
			params: []string{"str::host::h", "int::port::p"},
			want:   `rlog.LogrusFromContext(ctx).WithField("host", h).WithField("port", p).Info("connected")`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ll.InfoMsg(tt.ctx, tt.msg, tt.params...)
			if got != tt.want {
				t.Errorf("LogrusLogger.InfoMsg() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLogrusLogger_DebugMsg(t *testing.T) {
	ll := &LogrusLogger{}

	tests := []struct {
		name   string
		ctx    string
		msg    string
		params []string
		want   string
	}{
		{
			name: "basic debug",
			ctx:  "ctx",
			msg:  "debugging",
			want: `rlog.LogrusFromContext(ctx).Debug("debugging")`,
		},
		{
			name:   "debug with params",
			ctx:    "ctx",
			msg:    "variable dump",
			params: []string{"any::data::obj"},
			want:   `rlog.LogrusFromContext(ctx).WithField("data", obj).Debug("variable dump")`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ll.DebugMsg(tt.ctx, tt.msg, tt.params...)
			if got != tt.want {
				t.Errorf("LogrusLogger.DebugMsg() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLogrusLogger_ErrorMsgCaller(t *testing.T) {
	ll := &LogrusLogger{}

	tests := []struct {
		name       string
		ctx        string
		err        string
		msg        string
		callerSkip int
		params     []string
	}{
		{
			name:       "with error and caller",
			ctx:        "ctx",
			err:        "err",
			msg:        "error",
			callerSkip: 2,
		},
		{
			name:       "with nil error and caller",
			ctx:        "ctx",
			err:        "nil",
			msg:        "error",
			callerSkip: 1,
		},
		{
			name:       "with params",
			ctx:        "ctx",
			err:        "err",
			msg:        "failed",
			callerSkip: 3,
			params:     []string{"str::op::operation"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ll.ErrorMsgCaller(tt.ctx, tt.err, tt.msg, tt.callerSkip, tt.params...)

			// Verify key parts since logrus uses runtime.Caller
			if !strings.Contains(got, "rlog.LogrusFromContext(ctx)") {
				t.Errorf("should use LogrusFromContext, got: %s", got)
			}

			if !strings.Contains(got, "runtime.Caller") {
				t.Errorf("should use runtime.Caller, got: %s", got)
			}

			if !strings.Contains(got, `WithField("caller"`) {
				t.Errorf("should add caller field, got: %s", got)
			}

			if !strings.Contains(got, `.Error("`) {
				t.Errorf("should call Error, got: %s", got)
			}

			if tt.err != "nil" && !strings.Contains(got, "WithError") {
				t.Errorf("should use WithError when err != nil, got: %s", got)
			}
		})
	}
}

func TestLogrusLogger_UpdateContext(t *testing.T) {
	ll := &LogrusLogger{}

	tests := []struct {
		name   string
		params []string
		want   string
	}{
		{
			name:   "empty params",
			params: nil,
			want:   "",
		},
		{
			name:   "single param (ctx only)",
			params: []string{"ctx"},
			want:   "",
		},
		{
			name:   "single field",
			params: []string{"ctx", "str::request_id::reqID"},
			want:   `ctx = rlog.LogrusToContext(ctx, rlog.LogrusFromContext(ctx).WithField("request_id", reqID))`,
		},
		{
			name:   "multiple fields",
			params: []string{"ctx", "str::user::u", "int::count::c"},
			want:   `ctx = rlog.LogrusToContext(ctx, rlog.LogrusFromContext(ctx).WithField("user", u).WithField("count", c))`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ll.UpdateContext(tt.params...)
			if got != tt.want {
				t.Errorf("LogrusLogger.UpdateContext() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLogrusLogger_SubContext(t *testing.T) {
	ll := &LogrusLogger{}

	tests := []struct {
		name   string
		ctxVar string
		params []string
		want   string
	}{
		{
			name:   "single field",
			ctxVar: "ctx",
			params: []string{"str::worker::WorkerName"},
			want:   `ctx = rlog.LogrusToContext(ctx, rlog.LogrusFromContext(ctx).WithField("worker", WorkerName))`,
		},
		{
			name:   "multiple fields",
			ctxVar: "ctx",
			params: []string{"str::app::appName", "str::transport::transportName"},
			want:   `ctx = rlog.LogrusToContext(ctx, rlog.LogrusFromContext(ctx).WithField("app", appName).WithField("transport", transportName))`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ll.SubContext(tt.ctxVar, tt.params...)
			if got != tt.want {
				t.Errorf("LogrusLogger.SubContext() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLogrusLogger_Metadata(t *testing.T) {
	ll := &LogrusLogger{}

	t.Run("Import", func(t *testing.T) {
		want := `rlog "github.com/Educentr/go-project-starter-runtime/pkg/logger"`
		if got := ll.Import(); got != want {
			t.Errorf("LogrusLogger.Import() = %q, want %q", got, want)
		}
	})

	t.Run("FilesToGenerate", func(t *testing.T) {
		want := "logrus"
		if got := ll.FilesToGenerate(); got != want {
			t.Errorf("LogrusLogger.FilesToGenerate() = %q, want %q", got, want)
		}
	})

	t.Run("DestDir", func(t *testing.T) {
		want := "pkg/app/logger"
		if got := ll.DestDir(); got != want {
			t.Errorf("LogrusLogger.DestDir() = %q, want %q", got, want)
		}
	})
}

func TestLogrusLogger_InitLogger(t *testing.T) {
	ll := &LogrusLogger{}
	want := "logger.InitLogrus(ctx, serviceName)"
	got := ll.InitLogger("ctx", "serviceName")

	if got != want {
		t.Errorf("LogrusLogger.InitLogger() = %q, want %q", got, want)
	}
}

func TestLogrusLogger_ReWrap(t *testing.T) {
	ll := &LogrusLogger{}
	want := "destCtx = rlog.ReWrap(sourceCtx, destCtx, prefix, path)"
	got := ll.ReWrap("sourceCtx", "destCtx", "prefix", "path")

	if got != want {
		t.Errorf("LogrusLogger.ReWrap() = %q, want %q", got, want)
	}
}

func TestLogrusLogger_SetLoggerUpdater(t *testing.T) {
	ll := &LogrusLogger{}
	want := "reqctx.SetLoggerUpdater(rlog.NewLogrusUpdater())"
	got := ll.SetLoggerUpdater()

	if got != want {
		t.Errorf("LogrusLogger.SetLoggerUpdater() = %q, want %q", got, want)
	}
}

func TestLogrusLogger_SetupTestLogger(t *testing.T) {
	ll := &LogrusLogger{}

	got := ll.SetupTestLogger("ctx")

	// Check key parts of the generated code
	if !strings.Contains(got, "testLogger := logrus.New()") {
		t.Errorf("SetupTestLogger should create testLogger, got: %s", got)
	}

	if !strings.Contains(got, "testLogger.SetOutput(os.Stdout)") {
		t.Errorf("SetupTestLogger should set output, got: %s", got)
	}

	if !strings.Contains(got, "testLogger.SetLevel(logrus.DebugLevel)") {
		t.Errorf("SetupTestLogger should set level, got: %s", got)
	}

	if !strings.Contains(got, `WithField("service", "test")`) {
		t.Errorf("SetupTestLogger should set service field, got: %s", got)
	}

	if !strings.Contains(got, "ctx = rlog.LogrusToContext(ctx, testEntry)") {
		t.Errorf("SetupTestLogger should attach logger to context, got: %s", got)
	}
}

// ----- Cross-Logger Consistency Tests -----

func TestLoggerConsistency(t *testing.T) {
	// Ensure both loggers produce valid code structure for the same inputs
	zl := &ZlogLogger{}
	ll := &LogrusLogger{}

	t.Run("ErrorMsg structure", func(t *testing.T) {
		zlogOut := zl.ErrorMsg("ctx", "err", "test message")
		logrusOut := ll.ErrorMsg("ctx", "err", "test message")

		// Both should contain the context variable
		if !strings.Contains(zlogOut, "ctx") {
			t.Error("zerolog ErrorMsg should use ctx")
		}

		if !strings.Contains(logrusOut, "ctx") {
			t.Error("logrus ErrorMsg should use ctx")
		}

		// Both should contain the error variable
		if !strings.Contains(zlogOut, "err") {
			t.Error("zerolog ErrorMsg should use err")
		}

		if !strings.Contains(logrusOut, "err") {
			t.Error("logrus ErrorMsg should use err")
		}

		// Both should contain the message
		if !strings.Contains(zlogOut, "test message") {
			t.Error("zerolog ErrorMsg should contain message")
		}

		if !strings.Contains(logrusOut, "test message") {
			t.Error("logrus ErrorMsg should contain message")
		}
	})

	t.Run("DestDir matches", func(t *testing.T) {
		// Both loggers should generate to the same directory
		if zl.DestDir() != ll.DestDir() {
			t.Errorf("DestDir mismatch: zerolog=%q, logrus=%q", zl.DestDir(), ll.DestDir())
		}
	})
}
