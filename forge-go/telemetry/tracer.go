package telemetry

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

// InitTracerProvider initializes an OTLP exporter, and configures the corresponding trace and
// metric providers. If otlpEndpoint is empty, it falls back to stdout.
func InitTracerProvider(ctx context.Context, serviceName, otlpEndpoint string) (*sdktrace.TracerProvider, error) {
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	var exporter sdktrace.SpanExporter
	if otlpEndpoint != "" {
		exporter, err = otlptracegrpc.New(ctx, otlptracegrpc.WithEndpoint(otlpEndpoint), otlptracegrpc.WithInsecure())
		if err != nil {
			return nil, fmt.Errorf("failed to create OTLP trace exporter: %w", err)
		}
	} else {
		// Fallback to stdout if no OTLP endpoint is provided
		exporter, err = stdouttrace.New(stdouttrace.WithPrettyPrint())
		if err != nil {
			return nil, fmt.Errorf("failed to create stdout trace exporter: %w", err)
		}
	}

	bsp := sdktrace.NewBatchSpanProcessor(exporter, sdktrace.WithBatchTimeout(time.Second*5))
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithResource(res),
		sdktrace.WithSpanProcessor(bsp),
	)

	// Set the global TracerProvider and TextMapPropagator
	otel.SetTracerProvider(tracerProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	return tracerProvider, nil
}

func initProviders(ctx context.Context, cfg Config) (*sdktrace.TracerProvider, *sdkmetric.MeterProvider, error) {
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(cfg.ServiceName),
			semconv.ServiceVersionKey.String(cfg.ServiceVersion),
		),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create resource: %w", err)
	}

	traceExporter, err := newTraceExporter(ctx, cfg)
	if err != nil {
		return nil, nil, err
	}
	metricExporter, err := newMetricExporter(ctx, cfg)
	if err != nil {
		return nil, nil, err
	}

	meterProvider, promHandler, err := newMeterProviderWithOptions(
		[]sdkmetric.Option{sdkmetric.WithResource(res)},
		sdkmetric.NewPeriodicReader(metricExporter, sdkmetric.WithInterval(10*time.Second)),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create metric provider: %w", err)
	}

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithResource(res),
		sdktrace.WithSpanProcessor(sdktrace.NewBatchSpanProcessor(traceExporter, sdktrace.WithBatchTimeout(5*time.Second))),
	)

	otel.SetTracerProvider(tracerProvider)
	if err := installMeterProvider(meterProvider, promHandler); err != nil {
		return nil, nil, fmt.Errorf("failed to install metric provider: %w", err)
	}
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	return tracerProvider, meterProvider, nil
}

func newTraceExporter(ctx context.Context, cfg Config) (sdktrace.SpanExporter, error) {
	if cfg.EndpointURL == "" {
		return stdouttrace.New(stdouttrace.WithPrettyPrint())
	}
	endpoint, err := parseEndpointURL(cfg.EndpointURL)
	if err != nil {
		return nil, err
	}
	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(endpoint.Host),
	}
	if endpoint.Scheme == "http" {
		opts = append(opts, otlptracehttp.WithInsecure())
	}
	if path := endpoint.EscapedPath(); path != "" && path != "/" {
		opts = append(opts, otlptracehttp.WithURLPath(strings.TrimSuffix(path, "/")+"/v1/traces"))
	}
	return otlptracehttp.New(ctx, opts...)
}

func newMetricExporter(ctx context.Context, cfg Config) (sdkmetric.Exporter, error) {
	endpoint, err := parseEndpointURL(cfg.EndpointURL)
	if err != nil {
		return nil, err
	}
	opts := []otlpmetrichttp.Option{
		otlpmetrichttp.WithEndpoint(endpoint.Host),
	}
	if endpoint.Scheme == "http" {
		opts = append(opts, otlpmetrichttp.WithInsecure())
	}
	if path := endpoint.EscapedPath(); path != "" && path != "/" {
		opts = append(opts, otlpmetrichttp.WithURLPath(strings.TrimSuffix(path, "/")+"/v1/metrics"))
	}
	return otlpmetrichttp.New(ctx, opts...)
}
