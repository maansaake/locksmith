package otel

import (
	"context"
	"errors"

	"github.com/trebent/zerologr"
	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.30.0"
)

// Instrument bootstraps the OpenTelemetry pipeline.
// If it does not return an error, make sure to call shutdown for proper cleanup.
func Instrument(
	ctx context.Context,
	serviceName,
	serviceVersion string,
	runtimeMetrics bool,
) (func(context.Context) error, error) {
	var shutdownFuncs []func(context.Context) error

	shutdown := func(ctx context.Context) error {
		zerologr.Info("Shutting down OpenTelemetry SDK")

		var err error
		for _, fn := range shutdownFuncs {
			err = errors.Join(err, fn(ctx))
		}
		shutdownFuncs = nil
		return err
	}

	var err error

	// handleErr calls shutdown for cleanup and makes sure that all errors are returned.
	handleErr := func(inErr error) {
		err = errors.Join(inErr, shutdown(ctx))
	}

	// Set up propagator.
	prop := newPropagator()
	otel.SetTextMapPropagator(prop)

	// Create telemetry resource.
	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(serviceName),
		semconv.ServiceVersion(serviceVersion),
	)

	// Set up trace provider.
	tracerProvider, err := newTracerProvider(ctx, res)
	if err != nil {
		handleErr(err)
		return shutdown, err
	}
	shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
	otel.SetTracerProvider(tracerProvider)

	// Set up meter provider.
	meterProvider, err := newMeterProvider(ctx, res)
	if err != nil {
		handleErr(err)
		return shutdown, err
	}
	shutdownFuncs = append(shutdownFuncs, meterProvider.Shutdown)
	otel.SetMeterProvider(meterProvider)

	/*
		go.memory.used          By            Memory used by the Go runtime.
		go.memory.limit         By            Go runtime memory limit configured by the user, if a limit exists.
		go.memory.allocated     By            Memory allocated to the heap by the application.
		go.memory.allocations   {allocation}  Count of allocations to the heap by the application.
		go.memory.gc.goal       By            Heap size target for the end of the GC cycle.
		go.goroutine.count      {goroutine}   Count of live goroutines.
		go.processor.limit      {thread}      The number of OS threads that can execute user-level Go code simultaneously.
		go.config.gogc          %             Heap size target percentage configured by the user, otherwise 100.
	*/
	if runtimeMetrics {
		if err = runtime.Start(); err != nil {
			handleErr(err)
			return shutdown, err
		}
	}

	return shutdown, err
}

func newPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

func newTracerProvider(ctx context.Context, res *resource.Resource) (*trace.TracerProvider, error) {
	traceExporter, err := autoexport.NewSpanExporter(ctx)
	if err != nil {
		return nil, err
	}

	// NOTE: make sure to configure a sampler in production. The default is AlwaysSample unless otherwise stated by the parent span.
	// This is appropriate for development and testing, but can lead to high load in production settings.
	// OTEL_TRACES_SAMPLER=parentbased_traceidratio
	// OTEL_TRACES_SAMPLER_ARG=0.1
	return trace.NewTracerProvider(
		trace.WithBatcher(traceExporter),
		trace.WithResource(res),
	), nil
}

func newMeterProvider(ctx context.Context, res *resource.Resource) (*metric.MeterProvider, error) {
	reader, err := autoexport.NewMetricReader(ctx)
	if err != nil {
		return nil, err
	}

	return metric.NewMeterProvider(
		metric.WithReader(reader),
		metric.WithResource(res),
	), nil
}
