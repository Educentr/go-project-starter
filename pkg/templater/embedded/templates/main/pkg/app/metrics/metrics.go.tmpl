package metrics

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"

	"go.opentelemetry.io/otel"
	exporter "go.opentelemetry.io/otel/exporters/prometheus"
	provider "go.opentelemetry.io/otel/sdk/metric"
)

func InitMetrics(_ context.Context) (*prometheus.Registry, error) {
	registry := prometheus.NewRegistry()

	exp, err := exporter.New(exporter.WithRegisterer(registry))
	if err != nil {
		panic(err)
	}

	meterProvider := provider.NewMeterProvider(provider.WithReader(exp.Reader))
	otel.SetMeterProvider(meterProvider)

	return registry, nil
}
