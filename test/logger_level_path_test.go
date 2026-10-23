package test

import (
	"strings"
	"testing"
)

// Composition of the OnlineConf path for the log level, locked down per TOOLS-65
// and re-stated per TOOLS-43, which moved every OnlineConf read behind the
// generated occonf accessors.
//
// The invariant the generated code must hold is unchanged: the prefix handed to
// the logger creator is already the absolute path of the subtree that owns the
// logger, so it already carries the service name. Nothing may prepend the
// service name again — doing so moved every read one segment deeper than the
// tree and made the log_level override dead config.
//
// What changed is where the invariant lives. The logger no longer composes a
// path at all: it calls the one shared accessor, and the accessor is the thing
// that must not prepend the tree. The logger's own share is normalizing the
// prefix it is handed — an empty one is the process logger, whose level lives at
// the service root — because the accessor joins what it is given and an empty
// component would yield a path containing "//".
//
// The checks are intentionally written against the generated sources: the
// composition lives in the templates and in the accessor generator, so a
// generated-code assertion is what catches a regression here.

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

// TestGenerateLoggerLevelPathComposition checks that both generated loggers read
// the level through the shared accessor under the prefix they are given, without
// re-prepending the service name and without composing an empty path component.
func TestGenerateLoggerLevelPathComposition(t *testing.T) {
	for _, variant := range loggerVariants {
		t.Run(variant.name, func(t *testing.T) {
			tmpDir := generateFromConfig(t, variant.configName, "Generate project for "+variant.name+" level path")
			src := readGenerated(t, tmpDir, variant.genFile...)

			// The logger reads no key itself: the accessor package is the only
			// caller of the library, which is what `make onlineconf-validate`
			// enforces in the generated project.
			if strings.Contains(src, "onlineconf.") {
				t.Error("logger must read OnlineConf through the occonf accessors, not through the library")
			}

			// Both reads go through the one shared accessor, under the
			// normalized prefix.
			wantCalls := []string{
				"occonf.LogLevel(ctx, prefix, levelDefaultKey)",
				"occonf.LogLevel(ctx, prefix, ocPath)",
			}
			for _, call := range wantCalls {
				if !strings.Contains(src, call) {
					t.Errorf("generated logger should read the level via %s", call)
				}
			}

			// Prefix contract: the prefix is passed through as given — the
			// service name is NOT prepended to it — and an empty prefix falls
			// back to the service root, so no composed path is empty.
			wantHelper := []string{
				"func levelPrefix(serviceName, ocPrefix string) string {",
				`if strings.Trim(ocPrefix, "/") == "" {`,
				"return serviceName",
				"return ocPrefix",
				"prefix := levelPrefix(serviceName, onlineconfPrefix)",
			}
			for _, line := range wantHelper {
				if !strings.Contains(src, line) {
					t.Errorf("generated logger should contain prefix normalization line %q", line)
				}
			}

			// The per-endpoint read is skipped entirely for an empty ocPath:
			// there is no override key to read, and composing one would give "//".
			if !strings.Contains(src, `if strings.Trim(ocPath, "/") != "" {`) {
				t.Error("per-endpoint level read should be guarded by a non-empty ocPath")
			}

			// The key the level that applies to a whole prefix is read at.
			if !strings.Contains(src, `const levelDefaultKey = "default"`) {
				t.Error("baseline level key should stay default")
			}

			// Baseline read keeps its code default.
			if !strings.Contains(src, variant.levelDefault) {
				t.Errorf("baseline read should keep %s as the code default", variant.levelDefault)
			}
		})
	}
}

// TestGenerateLogLevelAccessorKeepsPrefixAbsolute checks the half of the
// invariant that moved into the accessor package: the shared accessor composes
// its path from the prefix it is handed and nothing else. Every other builder in
// that package starts from serviceTree; this one must not, because its caller
// already did.
func TestGenerateLogLevelAccessorKeepsPrefixAbsolute(t *testing.T) {
	tmpDir := generateFromConfig(t, "rest-only", "Generate project for the log level accessor")
	src := readGenerated(t, tmpDir, "internal", "pkg", "occonf", "psg_occonf_gen.go")

	const wantBuilder = `func LogLevelPath(prefix, levelKey string) string {
	return onlineconf.MakePath(prefix, levelKey, "log_level")
}`

	if !strings.Contains(src, wantBuilder) {
		t.Errorf("log level path should be composed from the caller's prefix alone:\n%s", wantBuilder)
	}

	// The defect this test exists for: the service tree prepended to a prefix
	// that already carries it.
	if strings.Contains(src, `onlineconf.MakePath(serviceTree, prefix`) {
		t.Error("log level accessor must not prepend serviceTree to the prefix — the prefix is already absolute")
	}
}

// TestGenerateLoggerPrefixIsAbsolute checks the other half of the invariant:
// every caller hands the logger a prefix that starts at the service root. If a
// caller ever starts passing a service-relative prefix, the level reads silently
// move to a non-existent subtree again. The prefixes are now built by the
// accessor package's path builders, which are rooted in serviceTree.
func TestGenerateLoggerPrefixIsAbsolute(t *testing.T) {
	t.Run("rest middleware", func(t *testing.T) {
		tmpDir := generateFromConfig(t, "rest-only", "Generate project for REST logger prefix")
		src := readGenerated(t, tmpDir, "pkg", "app", "restconfig", "psg_config_oc_gen.go")

		const wantPrefix = `ocPrefix := occonf.RESTAppMetricsPath(c.transportName, c.appName)`
		if !strings.Contains(src, wantPrefix) {
			t.Errorf("REST logger prefix should come from the accessor package: %s", wantPrefix)
		}

		if !strings.Contains(src, "logger.ReWrap(requestCtx, mwCtx, ocPrefix, urlPath)") {
			t.Error("REST middleware should rewrap the logger with the absolute prefix and the url path")
		}

		occonfSrc := readGenerated(t, tmpDir, "internal", "pkg", "occonf", "psg_occonf_gen.go")

		const wantBuilder = `serviceTree, "transport", "rest", transport, app, "mw", "metrics"`
		if !strings.Contains(occonfSrc, wantBuilder) {
			t.Errorf("the REST prefix builder should start at the service tree: MakePath(%s)", wantBuilder)
		}
	})

	t.Run("daemon worker", func(t *testing.T) {
		tmpDir := generateFromConfig(t, "worker-daemon", "Generate project for worker logger prefix")
		src := readGenerated(t, tmpDir, "internal", "app", "worker", "daemon", "psg_daemon_gen.go")

		if !strings.Contains(src, "return occonf.WorkerPath(WorkerName)") {
			t.Error("worker config path should come from the accessor package")
		}

		if !strings.Contains(src, `logger.ReWrap(ctx, ctxJob, ocPrefix, "job")`) {
			t.Error("worker should rewrap the job logger with the absolute prefix and the job node")
		}

		occonfSrc := readGenerated(t, tmpDir, "internal", "pkg", "occonf", "psg_occonf_gen.go")

		const wantBuilder = `serviceTree, "worker", worker`
		if !strings.Contains(occonfSrc, wantBuilder) {
			t.Errorf("the worker prefix builder should start at the service tree: MakePath(%s)", wantBuilder)
		}
	})
}

// TestGenerateProcessLoggerLevelIsDeclared pins that the path the process logger
// reads is part of the key map, not an undeclared read.
//
// It is the one level path with no owner segment between the tree and `default`:
// the process logger is handed an empty prefix, which normalizes to the service
// root. Nothing else in the surface would produce that path, so without its own
// declaration it would be a read the registry does not list and the service map
// could claim.
func TestGenerateProcessLoggerLevelIsDeclared(t *testing.T) {
	tmpDir := generateFromConfig(t, "rest-only", "Generate project for the process logger declaration")
	registry := readGenerated(t, tmpDir, "configs", "onlineconf-tree.yaml")

	const wantRow = "- path: /resttest/default/log_level"
	if !strings.Contains(registry, wantRow) {
		t.Errorf("the registry should list the process logger level: %s", wantRow)
	}
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
