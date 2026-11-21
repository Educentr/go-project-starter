package config

import (
	"strings"

	"github.com/Educentr/go-project-starter/internal/pkg/ds"
	"github.com/pkg/errors"
)

// AppMetrics is the metrics section of an application — where the metrics the
// application collects go.
//
// The registry itself is not configurable: every application initializes one,
// because the code that writes metrics (the REST middleware, the generated
// collectors, ActiveRecord) is handed it on initialization. What this section
// declares is the export channel, and "no channel" is one of its values: an
// application that nobody scrapes collects its metrics into a registry nothing
// reads, and starts.
//
// YAML example:
//
//	applications:
//	  - name: api
//	    transport: [api, sys]
//	    metrics:
//	      export: pull   # /metrics on the sys transport
//
//	  - name: localUI
//	    transport: [ui]
//	    metrics:
//	      export: none   # runs on a user's machine; no scraper can reach it
//
// See docs/configuration/applications.md for full documentation.
type AppMetrics struct {
	// Export is the export channel: "pull", "push" or "none". Empty means the
	// generator derives it from the application's transports — read via
	// Config.ResolveAppMetricsExport, never directly.
	Export string `mapstructure:"export"`
}

// appCtx names the application an error belongs to: every error of this
// section is about one application, and the message is useless without knowing
// which.
const appCtx = "application '%s'"

// Sentinel errors of the application metrics section. Use errors.Is to
// distinguish.
var (
	ErrAppMetricsExportUnknown = errors.New(
		`metrics.export: unknown channel, expected "pull", "push" or "none"`,
	)
	ErrAppMetricsExportPushUnsupported = errors.New(
		"metrics.export: \"push\" is reserved and not implemented yet — " +
			"declare \"none\" to collect metrics without handing them out",
	)
	ErrAppMetricsExportPullNoSys = errors.New(
		`metrics.export: "pull" needs a sys transport in the application — ` +
			"it is the server that exposes /metrics",
	)
	ErrAppMetricsExportNoneWithSys = errors.New(
		`metrics.export: "none" contradicts the sys transport of the application, ` +
			"which serves /metrics — declare \"pull\" or drop the sys transport",
	)
)

// appHasSysTransport reports whether the application runs a sys transport, the
// one that serves /metrics. The check needs the transport definitions, which is
// why the metrics export channel is resolved on Config and not on Application.
//
// The test is the template the transport comes from, not its name — the same
// test ds.App.HasSysTransport makes on the template side, through the same
// constants.
func (c Config) appHasSysTransport(app Application) bool {
	for _, transport := range app.TransportList {
		rest, ok := c.RestMap[transport.Name]
		if !ok {
			continue
		}

		if rest.GeneratorType == ds.GeneratorTypeTemplate && rest.GeneratorTemplate == ds.GeneratorTemplateSys {
			return true
		}
	}

	return false
}

// ResolveAppMetricsExport returns the export channel of the application.
//
// An application that declares nothing keeps the behavior its transports
// already imply: a sys transport serves /metrics, so it is pull; without one
// there is nothing to serve it on, so it is none. That is the whole migration
// path — no existing project.yaml has to change, and none of them changes
// meaning.
//
// A declared value is checked against the same transports, because a channel
// the application cannot provide is a misconfiguration and not a preference:
// "pull" without a sys transport asks for an endpoint that is never served,
// and "none" alongside a sys transport asks for metrics to stay inside a
// process that is already serving them.
func (c Config) ResolveAppMetricsExport(app Application) (ds.MetricsExport, error) {
	hasSys := c.appHasSysTransport(app)

	declared := strings.TrimSpace(app.Metrics.Export)
	if declared == "" {
		if hasSys {
			return ds.MetricsExportPull, nil
		}

		return ds.MetricsExportNone, nil
	}

	switch ds.MetricsExport(declared) {
	case ds.MetricsExportPull:
		if !hasSys {
			return "", errors.Wrapf(ErrAppMetricsExportPullNoSys, appCtx, app.Name)
		}

		return ds.MetricsExportPull, nil

	case ds.MetricsExportPush:
		return "", errors.Wrapf(ErrAppMetricsExportPushUnsupported, appCtx, app.Name)

	case ds.MetricsExportNone:
		if hasSys {
			return "", errors.Wrapf(ErrAppMetricsExportNoneWithSys, appCtx, app.Name)
		}

		return ds.MetricsExportNone, nil

	default:
		return "", errors.Wrapf(ErrAppMetricsExportUnknown, appCtx+": %q", app.Name, declared)
	}
}
