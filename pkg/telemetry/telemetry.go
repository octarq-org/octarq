package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	promExporter "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	metricSdk "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	traceSdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	noopTrace "go.opentelemetry.io/otel/trace/noop"
)

var (
	globalMu        sync.RWMutex
	globalTelemetry *Telemetry
)

// Telemetry manages OpenTelemetry tracing and metrics lifecycle.
type Telemetry struct {
	Config             Config
	TracerProvider     *traceSdk.TracerProvider
	MeterProvider      *metricSdk.MeterProvider
	PrometheusExporter *promExporter.Exporter
	Metrics            *Metrics
	Resource           *resource.Resource
	shutdownFuncs      []func(context.Context) error
}

// Global returns the active global Telemetry instance, or a no-op instance if uninitialized.
func Global() *Telemetry {
	globalMu.RLock()
	defer globalMu.RUnlock()
	if globalTelemetry != nil {
		return globalTelemetry
	}
	return &Telemetry{
		Metrics: NewNoopMetrics(),
	}
}

// Init initializes OpenTelemetry SDK with the supplied configuration.
func Init(ctx context.Context, cfg Config) (*Telemetry, error) {
	globalMu.Lock()
	defer globalMu.Unlock()

	// If disabled, setup no-op providers and propagators
	if !cfg.Enabled {
		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
		t := &Telemetry{
			Config:  cfg,
			Metrics: NewNoopMetrics(),
		}
		globalTelemetry = t
		return t, nil
	}

	res, err := buildResource(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("telemetry: build resource: %w", err)
	}

	t := &Telemetry{
		Config:   cfg,
		Resource: res,
	}

	// 1. TextMapPropagator setup (W3C Trace Context + W3C Baggage)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// 2. TracerProvider setup
	tp, tpShutdown, err := initTracerProvider(ctx, cfg, res)
	if err != nil {
		return nil, fmt.Errorf("telemetry: init tracer provider: %w", err)
	}
	if tp != nil {
		t.TracerProvider = tp
		otel.SetTracerProvider(tp)
		if tpShutdown != nil {
			t.shutdownFuncs = append(t.shutdownFuncs, tpShutdown)
		}
	}

	// 3. MeterProvider setup
	mp, promExp, mpShutdown, err := initMeterProvider(ctx, cfg, res)
	if err != nil {
		return nil, fmt.Errorf("telemetry: init meter provider: %w", err)
	}
	if mp != nil {
		t.MeterProvider = mp
		t.PrometheusExporter = promExp
		otel.SetMeterProvider(mp)
		if mpShutdown != nil {
			t.shutdownFuncs = append(t.shutdownFuncs, mpShutdown)
		}
	}

	// 4. Initialize pre-defined metric instruments
	meter := otel.GetMeterProvider().Meter("github.com/octarq-org/octarq")
	metrics, err := NewMetrics(meter)
	if err != nil {
		slog.Warn("telemetry: failed to create metrics instruments", "err", err)
		metrics = NewNoopMetrics()
	}
	t.Metrics = metrics

	globalTelemetry = t
	slog.Info("telemetry: initialized",
		"service", cfg.ServiceName,
		"version", cfg.ServiceVersion,
		"traces_exporter", cfg.TracesExporter,
		"metrics_exporter", cfg.MetricsExporter,
	)

	return t, nil
}

func buildResource(ctx context.Context, cfg Config) (*resource.Resource, error) {
	attrs := []attribute.KeyValue{
		semconv.ServiceNameKey.String(cfg.ServiceName),
		semconv.ServiceVersionKey.String(cfg.ServiceVersion),
		semconv.DeploymentEnvironmentKey.String(cfg.Environment),
	}

	for k, v := range cfg.ResourceAttributes {
		attrs = append(attrs, attribute.String(k, v))
	}

	customRes := resource.NewSchemaless(attrs...)
	defaultRes := resource.Default()
	return resource.Merge(defaultRes, customRes)
}

func initTracerProvider(ctx context.Context, cfg Config, res *resource.Resource) (*traceSdk.TracerProvider, func(context.Context) error, error) {
	if cfg.TracesExporter == "none" || (!cfg.Enabled && cfg.Endpoint == "") {
		return nil, nil, nil
	}

	var exporter traceSdk.SpanExporter
	var err error

	switch cfg.TracesExporter {
	case "stdout":
		exporter, err = stdouttrace.New(stdouttrace.WithPrettyPrint())
	case "otlp":
		fallthrough
	default:
		if cfg.Endpoint == "" && cfg.TracesEndpoint == "" && cfg.TracesExporter != "otlp" {
			return nil, nil, nil
		}
		exporter, err = createOTLPTraceExporter(ctx, cfg)
	}

	if err != nil {
		return nil, nil, err
	}

	sampler := parseSampler(cfg.Sampler, cfg.SampleRatio)

	tp := traceSdk.NewTracerProvider(
		traceSdk.WithBatcher(exporter,
			traceSdk.WithBatchTimeout(5*time.Second),
			traceSdk.WithMaxExportBatchSize(512),
		),
		traceSdk.WithResource(res),
		traceSdk.WithSampler(sampler),
	)

	return tp, tp.Shutdown, nil
}

func createOTLPTraceExporter(ctx context.Context, cfg Config) (traceSdk.SpanExporter, error) {
	endpoint := cfg.TracesEndpoint
	if endpoint == "" {
		endpoint = cfg.Endpoint
	}
	if endpoint == "" {
		endpoint = "localhost:4317"
	}

	isHTTP := strings.HasPrefix(cfg.Protocol, "http") || strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://")

	if isHTTP {
		cleanEndpoint := strings.TrimPrefix(endpoint, "http://")
		cleanEndpoint = strings.TrimPrefix(cleanEndpoint, "https://")
		opts := []otlptracehttp.Option{
			otlptracehttp.WithEndpoint(cleanEndpoint),
			otlptracehttp.WithHeaders(cfg.Headers),
		}
		if cfg.Insecure || strings.HasPrefix(endpoint, "http://") {
			opts = append(opts, otlptracehttp.WithInsecure())
		}
		return otlptrace.New(ctx, otlptracehttp.NewClient(opts...))
	}

	// gRPC
	cleanEndpoint := strings.TrimPrefix(endpoint, "grpc://")
	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(cleanEndpoint),
		otlptracegrpc.WithHeaders(cfg.Headers),
	}
	if cfg.Insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}
	return otlptrace.New(ctx, otlptracegrpc.NewClient(opts...))
}

func initMeterProvider(ctx context.Context, cfg Config, res *resource.Resource) (*metricSdk.MeterProvider, *promExporter.Exporter, func(context.Context) error, error) {
	var readers []metricSdk.Reader
	var promExp *promExporter.Exporter
	var shutdownFuncs []func(context.Context) error

	switch cfg.MetricsExporter {
	case "none":
		return nil, nil, nil, nil
	case "stdout":
		exp, err := stdoutmetric.New(stdoutmetric.WithPrettyPrint())
		if err != nil {
			return nil, nil, nil, err
		}
		readers = append(readers, metricSdk.NewPeriodicReader(exp, metricSdk.WithInterval(15*time.Second)))
	case "prometheus":
		exp, err := promExporter.New()
		if err != nil {
			return nil, nil, nil, err
		}
		promExp = exp
		readers = append(readers, exp)
	case "otlp":
		exp, err := createOTLPMetricExporter(ctx, cfg)
		if err != nil {
			return nil, nil, nil, err
		}
		readers = append(readers, metricSdk.NewPeriodicReader(exp, metricSdk.WithInterval(15*time.Second)))
	default:
		// Default to prometheus exporter if no other exporter is chosen
		exp, err := promExporter.New()
		if err != nil {
			return nil, nil, nil, err
		}
		promExp = exp
		readers = append(readers, exp)
	}

	opts := []metricSdk.Option{
		metricSdk.WithResource(res),
	}
	for _, r := range readers {
		opts = append(opts, metricSdk.WithReader(r))
	}

	mp := metricSdk.NewMeterProvider(opts...)
	shutdownFuncs = append(shutdownFuncs, mp.Shutdown)

	combinedShutdown := func(ctx context.Context) error {
		var errs []error
		for _, fn := range shutdownFuncs {
			if err := fn(ctx); err != nil {
				errs = append(errs, err)
			}
		}
		return errors.Join(errs...)
	}

	return mp, promExp, combinedShutdown, nil
}

func createOTLPMetricExporter(ctx context.Context, cfg Config) (metricSdk.Exporter, error) {
	endpoint := cfg.MetricsEndpoint
	if endpoint == "" {
		endpoint = cfg.Endpoint
	}
	if endpoint == "" {
		endpoint = "localhost:4317"
	}

	isHTTP := strings.HasPrefix(cfg.Protocol, "http") || strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://")

	if isHTTP {
		cleanEndpoint := strings.TrimPrefix(endpoint, "http://")
		cleanEndpoint = strings.TrimPrefix(cleanEndpoint, "https://")
		opts := []otlpmetrichttp.Option{
			otlpmetrichttp.WithEndpoint(cleanEndpoint),
			otlpmetrichttp.WithHeaders(cfg.Headers),
		}
		if cfg.Insecure || strings.HasPrefix(endpoint, "http://") {
			opts = append(opts, otlpmetrichttp.WithInsecure())
		}
		return otlpmetrichttp.New(ctx, opts...)
	}

	cleanEndpoint := strings.TrimPrefix(endpoint, "grpc://")
	opts := []otlpmetricgrpc.Option{
		otlpmetricgrpc.WithEndpoint(cleanEndpoint),
		otlpmetricgrpc.WithHeaders(cfg.Headers),
	}
	if cfg.Insecure {
		opts = append(opts, otlpmetricgrpc.WithInsecure())
	}
	return otlpmetricgrpc.New(ctx, opts...)
}

func parseSampler(samplerName string, ratio float64) traceSdk.Sampler {
	switch strings.ToLower(samplerName) {
	case "always_on":
		return traceSdk.AlwaysSample()
	case "always_off":
		return traceSdk.NeverSample()
	case "traceidratio":
		return traceSdk.TraceIDRatioBased(ratio)
	case "parentbased_always_off":
		return traceSdk.ParentBased(traceSdk.NeverSample())
	case "parentbased_traceidratio":
		return traceSdk.ParentBased(traceSdk.TraceIDRatioBased(ratio))
	case "parentbased_always_on":
		fallthrough
	default:
		return traceSdk.ParentBased(traceSdk.AlwaysSample())
	}
}

// PrometheusHandler returns an http.Handler that serves Prometheus metrics.
func (t *Telemetry) PrometheusHandler() http.Handler {
	if t.PrometheusExporter != nil {
		return promhttp.Handler()
	}
	return http.NotFoundHandler()
}

// Shutdown flushes and closes all configured providers.
func (t *Telemetry) Shutdown(ctx context.Context) error {
	var errs []error
	for _, fn := range t.shutdownFuncs {
		if err := fn(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Tracer returns a named tracer.
func Tracer(name string) trace.Tracer {
	if name == "" {
		name = "github.com/octarq-org/octarq"
	}
	return otel.GetTracerProvider().Tracer(name)
}

// NoopTracer returns a no-op tracer.
func NoopTracer() trace.Tracer {
	return noopTrace.NewTracerProvider().Tracer("noop")
}
