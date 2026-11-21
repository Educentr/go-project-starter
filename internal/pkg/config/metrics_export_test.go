package config

import (
	"errors"
	"testing"

	"github.com/Educentr/go-project-starter/internal/pkg/ds"
)

// configWithSys returns a config whose "sys" transport is the template sys
// server and whose "api" transport is an ordinary ogen one.
func configWithSys() Config {
	return Config{
		RestMap: map[string]Rest{
			"api": {Name: "api", GeneratorType: "ogen"},
			"sys": {Name: "sys", GeneratorType: "template", GeneratorTemplate: "sys"},
		},
	}
}

func app(name string, export string, transports ...string) Application {
	out := Application{Name: name, Metrics: AppMetrics{Export: export}}

	for _, t := range transports {
		out.TransportList = append(out.TransportList, AppTransport{Name: t})
	}

	return out
}

func TestConfig_ResolveAppMetricsExport(t *testing.T) {
	config := configWithSys()

	tests := []struct {
		name    string
		app     Application
		want    ds.MetricsExport
		wantErr error
	}{
		{
			// The whole migration path: no project.yaml has to change, and
			// none of them changes meaning.
			name: "not declared, sys transport present",
			app:  app("api", "", "api", "sys"),
			want: ds.MetricsExportPull,
		},
		{
			// The shape that used to panic on startup: the registry was never
			// initialized, and the REST middleware registered into nil.
			name: "not declared, no sys transport",
			app:  app("localUI", "", "api"),
			want: ds.MetricsExportNone,
		},
		{
			name: "not declared, no transports at all",
			app:  app("cli", ""),
			want: ds.MetricsExportNone,
		},
		{
			name: "pull declared with a sys transport",
			app:  app("api", "pull", "api", "sys"),
			want: ds.MetricsExportPull,
		},
		{
			name: "none declared without a sys transport",
			app:  app("localUI", "none", "api"),
			want: ds.MetricsExportNone,
		},
		{
			// Asks for an endpoint nothing serves.
			name:    "pull declared without a sys transport",
			app:     app("localUI", "pull", "api"),
			wantErr: ErrAppMetricsExportPullNoSys,
		},
		{
			// Asks for metrics to stay inside a process that already serves
			// them on /metrics.
			name:    "none declared with a sys transport",
			app:     app("api", "none", "api", "sys"),
			wantErr: ErrAppMetricsExportNoneWithSys,
		},
		{
			// Reserved by the grammar, rejected until it works: silently
			// ignoring a declared channel would be worse than failing.
			name:    "push is reserved",
			app:     app("api", "push", "api", "sys"),
			wantErr: ErrAppMetricsExportPushUnsupported,
		},
		{
			name:    "unknown channel",
			app:     app("api", "otlp", "api", "sys"),
			wantErr: ErrAppMetricsExportUnknown,
		},
		{
			name: "surrounding whitespace is not a channel of its own",
			app:  app("api", "  pull  ", "api", "sys"),
			want: ds.MetricsExportPull,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := config.ResolveAppMetricsExport(tt.app)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("expected error %v, got %v", tt.wantErr, err)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tt.want {
				t.Errorf("expected channel %q, got %q", tt.want, got)
			}
		})
	}
}

// TestConfig_appHasSysTransport pins what makes a transport the sys one: the
// template it is generated from, not its name. A transport named "sys" that is
// an ordinary REST server serves no /metrics.
func TestConfig_appHasSysTransport(t *testing.T) {
	config := Config{
		RestMap: map[string]Rest{
			"sys":     {Name: "sys", GeneratorType: "ogen"},
			"metrics": {Name: "metrics", GeneratorType: "template", GeneratorTemplate: "sys"},
			"static":  {Name: "static", GeneratorType: "template", GeneratorTemplate: "static"},
		},
	}

	if config.appHasSysTransport(app("a", "", "sys")) {
		t.Error("an ogen transport named sys is not the sys server")
	}

	if config.appHasSysTransport(app("a", "", "static")) {
		t.Error("another template transport is not the sys server")
	}

	if !config.appHasSysTransport(app("a", "", "static", "metrics")) {
		t.Error("the sys template is the sys server, whatever the transport is named")
	}

	// A gRPC transport is not in RestMap at all and must not be mistaken for
	// one that is missing.
	if config.appHasSysTransport(app("a", "", "grpc")) {
		t.Error("an unknown transport name must not count as a sys server")
	}
}
