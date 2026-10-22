package test

import (
	"strings"
	"testing"
)

// Composition of the OnlineConf path for the log level, locked down per TOOLS-65.
//
// The invariant the generated code must hold: the prefix handed to the logger
// creator is already the absolute path of the subtree that owns the logger, so
// it already carries the service name. The creator must therefore NOT prepend
// the service name again — doing so moved every read one segment deeper than
// the tree and made the log_level override dead config.
//
// The checks are intentionally written against the generated sources: the
// composition lives in the templates, so a template-level assertion is what
// catches a regression here.

// loggerVariant describes one logger implementation and the path of its
// generated file inside a project.
type loggerVariant struct {
	name       string
	configName string
	genFile    []string
	// levelDefault is the parsed level used as the fallback for the baseline read.
	levelDefault string
}

var loggerVariants = []loggerVariant{
	{
		name:         "zlog",
		configName:   "rest-only",
		genFile:      []string{"pkg", "app", "logger", "psg_zlog_gen.go"},
		levelDefault: "zlog.LevelInfoValue",
	},
	{
		name:         "logrus",
		configName:   "rest-logrus",
		genFile:      []string{"pkg", "app", "logger", "psg_logrus_gen.go"},
		levelDefault: "logrus.InfoLevel.String()",
	},
}

// TestGenerateLoggerLevelPathComposition checks that both generated loggers
// compose the log level path from the prefix they are given, without
// re-prepending the service name and without emitting empty path components.
func TestGenerateLoggerLevelPathComposition(t *testing.T) {
	for _, variant := range loggerVariants {
		t.Run(variant.name, func(t *testing.T) {
			tmpDir := generateFromConfig(t, variant.configName, "Generate project for "+variant.name+" level path")
			src := readGenerated(t, tmpDir, variant.genFile...)

			// The defect: service name prepended to a prefix that already has it.
			if strings.Contains(src, "onlineconf.MakePath(serviceName, onlineconfPrefix") {
				t.Error("logger must not prepend serviceName to onlineconfPrefix — the prefix is already absolute")
			}

			// Both reads go through the single composition helper.
			wantCalls := []string{
				"logLevelPath(serviceName, onlineconfPrefix, onlineconfLevelDefaultNode)",
				"logLevelPath(serviceName, onlineconfPrefix, ocPath)",
			}
			for _, call := range wantCalls {
				if !strings.Contains(src, call) {
					t.Errorf("generated logger should read the level via %s", call)
				}
			}

			// Helper contract: empty prefix falls back to the service root,
			// empty components are dropped so no path contains "//".
			wantHelper := []string{
				"func logLevelPath(serviceName, ocPrefix, node string) string {",
				`if strings.Trim(ocPrefix, "/") == "" {`,
				"ocPrefix = serviceName",
				`for _, part := range []string{ocPrefix, node, onlineconfLevelPath} {`,
				`if strings.Trim(part, "/") != "" {`,
				"return onlineconf.MakePath(parts...)",
			}
			for _, line := range wantHelper {
				if !strings.Contains(src, line) {
					t.Errorf("generated logger should contain composition helper line %q", line)
				}
			}

			// The per-endpoint read is skipped entirely for an empty ocPath:
			// there is no override node to read, and composing one would give "//".
			if !strings.Contains(src, `if strings.Trim(ocPath, "/") != "" {`) {
				t.Error("per-endpoint level read should be guarded by a non-empty ocPath")
			}

			// The node names the paths are built from.
			if !strings.Contains(src, `onlineconfLevelPath = "log_level"`) {
				t.Error("level node should stay log_level")
			}

			if !strings.Contains(src, `onlineconfLevelDefaultNode = "default"`) {
				t.Error("baseline level node should stay default")
			}

			// Baseline read keeps its code default.
			if !strings.Contains(src, variant.levelDefault) {
				t.Errorf("baseline read should keep %s as the code default", variant.levelDefault)
			}
		})
	}
}

// TestGenerateLoggerPrefixIsAbsolute checks the other half of the invariant:
// every caller hands the logger a prefix that starts at the service root.
// If a caller ever starts passing a service-relative prefix, the level reads
// silently move to a non-existent subtree again.
func TestGenerateLoggerPrefixIsAbsolute(t *testing.T) {
	t.Run("rest middleware", func(t *testing.T) {
		tmpDir := generateFromConfig(t, "rest-only", "Generate project for REST logger prefix")
		src := readGenerated(t, tmpDir, "pkg", "app", "restconfig", "psg_config_oc_gen.go")

		const wantPrefix = `ocPrefix := onlineconf.MakePath(c.serviceName, "transport/rest", c.transportName, c.appName, "mw/metrics")`
		if !strings.Contains(src, wantPrefix) {
			t.Errorf("REST logger prefix should be absolute: %s", wantPrefix)
		}

		if !strings.Contains(src, "logger.ReWrap(requestCtx, mwCtx, ocPrefix, urlPath)") {
			t.Error("REST middleware should rewrap the logger with the absolute prefix and the url path")
		}
	})

	t.Run("daemon worker", func(t *testing.T) {
		tmpDir := generateFromConfig(t, "worker-daemon", "Generate project for worker logger prefix")
		src := readGenerated(t, tmpDir, "internal", "app", "worker", "daemon", "psg_daemon_gen.go")

		if !strings.Contains(src, `components := []string{constant.ServiceName, "worker", WorkerName}`) {
			t.Error("worker config path should be absolute (built from constant.ServiceName)")
		}

		if !strings.Contains(src, `logger.ReWrap(ctx, ctxJob, ocPrefix, "job")`) {
			t.Error("worker should rewrap the job logger with the absolute prefix and the job node")
		}
	})
}

// TestGenerateEnvExampleLogLevel checks that the env example seeds the level at
// the path the generated logger actually reads. Before TOOLS-65 it seeded
// /{svc}/log/level, which no code has ever read.
func TestGenerateEnvExampleLogLevel(t *testing.T) {
	tmpDir := generateFromConfig(t, "rest-only", "Generate project for env example log level")
	src := readGenerated(t, tmpDir, ".env-api.example")

	if !strings.Contains(src, "OC_resttest__default__log_level=info") {
		t.Error("env example should seed the level at the path the process logger reads: /{svc}/default/log_level")
	}

	if strings.Contains(src, "OC_resttest__log__level=") {
		t.Error("env example should not seed /{svc}/log/level — nothing reads it")
	}
}
