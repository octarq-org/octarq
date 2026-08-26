package telemetry

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace"
)

func TestTelemetryInitAndShutdown(t *testing.T) {
	ctx := context.Background()

	cfg := Config{
		Enabled:         true,
		ServiceName:     "test-service",
		ServiceVersion:  "v1.0.0",
		Environment:     "test",
		TracesExporter:  "stdout",
		MetricsExporter: "prometheus",
	}

	tel, err := Init(ctx, cfg)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer tel.Shutdown(ctx)

	if tel.TracerProvider == nil {
		t.Fatal("expected TracerProvider to be non-nil")
		return
	}
	if tel.MeterProvider == nil {
		t.Fatal("expected MeterProvider to be non-nil")
		return
	}
	if tel.Metrics == nil {
		t.Fatal("expected Metrics to be non-nil")
		return
	}

	// Test StartSpan and TraceID
	tr := Tracer("test-tracer")
	spanCtx, span := tr.Start(ctx, "test-span")
	defer span.End()

	traceID := TraceID(spanCtx)
	if traceID == "" {
		t.Fatal("expected non-empty traceID")
	}

	spanID := SpanID(spanCtx)
	if spanID == "" {
		t.Fatal("expected non-empty spanID")
	}

	// Test Propagation
	req := httptest.NewRequest("GET", "/test", nil)
	InjectHTTP(spanCtx, req.Header)
	if req.Header.Get("traceparent") == "" {
		t.Fatal("expected traceparent header to be injected")
	}

	extractedCtx := ExtractHTTP(context.Background(), req.Header)
	extractedSpan := trace.SpanFromContext(extractedCtx)
	if extractedSpan.SpanContext().TraceID().String() != traceID {
		t.Fatalf("extracted traceID %q != %q", extractedSpan.SpanContext().TraceID().String(), traceID)
	}

	// Test Metrics recording
	tel.Metrics.RecordHTTPRequest(spanCtx, "GET", "/test", 200, 10*time.Millisecond, 1024)
	tel.Metrics.RecordCacheHit(spanCtx, "redis")
	tel.Metrics.RecordCacheMiss(spanCtx, "redis")
	tel.Metrics.RecordQueueTask(spanCtx, "email.send", "success", 50*time.Millisecond)
	tel.Metrics.RecordWebhookDelivery(spanCtx, "link.created", 200, 100*time.Millisecond)

	// Test Prometheus endpoint
	promHandler := tel.PrometheusHandler()
	rec := httptest.NewRecorder()
	promReq := httptest.NewRequest("GET", "/metrics", nil)
	promHandler.ServeHTTP(rec, promReq)

	if rec.Code != http.StatusOK {
		t.Fatalf("prometheus handler returned code %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if body == "" {
		t.Fatal("expected non-empty prometheus output")
	}
}

func TestHTTPMiddleware(t *testing.T) {
	ctx := context.Background()
	cfg := Config{
		Enabled:         true,
		ServiceName:     "test-http",
		TracesExporter:  "stdout",
		MetricsExporter: "prometheus",
	}

	tel, err := Init(ctx, cfg)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer tel.Shutdown(ctx)

	mw := HTTPMiddleware("test-http")
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := TraceID(r.Context())
		if traceID == "" {
			t.Error("expected traceID in handler context")
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest("GET", "/api/links", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("handler status = %d, want 200", rec.Code)
	}
	if rec.Header().Get("X-Trace-Id") == "" {
		t.Fatal("expected X-Trace-Id response header")
	}
}

func TestDisabledTelemetry(t *testing.T) {
	ctx := context.Background()
	cfg := Config{
		Enabled: false,
	}

	tel, err := Init(ctx, cfg)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer tel.Shutdown(ctx)

	// In disabled mode, no panic, tracing is noop
	ctx, span := StartSpan(ctx, "test", "noop-span")
	span.End()

	tel.Metrics.RecordHTTPRequest(ctx, "GET", "/test", 200, time.Millisecond, 100)
}

func TestConfigFromEnvDefaultDisabled(t *testing.T) {
	cfg := ConfigFromEnv("test-service", "v1.0.0")
	if cfg.Enabled {
		t.Error("expected telemetry to be disabled by default")
	}
}

func TestConfigParsers(t *testing.T) {
	t.Setenv("OCTARQ_OTEL_ENABLED", "true")
	t.Setenv("OTEL_SERVICE_NAME", "my-service")
	t.Setenv("OTEL_SERVICE_VERSION", "2.0.0")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318")
	t.Setenv("OTEL_EXPORTER_OTLP_HEADERS", "k1=v1,k2=v2")
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "env=prod,team=backend")
	t.Setenv("OTEL_TRACES_SAMPLER", "traceidratio")
	t.Setenv("OTEL_TRACES_SAMPLER_ARG", "0.5")

	cfg := ConfigFromEnv("default-service", "1.0.0")
	if !cfg.Enabled {
		t.Error("expected Enabled to be true when OCTARQ_OTEL_ENABLED=true")
	}
	if cfg.ServiceName != "my-service" {
		t.Errorf("got ServiceName %q, want my-service", cfg.ServiceName)
	}
	if cfg.ServiceVersion != "2.0.0" {
		t.Errorf("got ServiceVersion %q, want 2.0.0", cfg.ServiceVersion)
	}
	if cfg.Headers["k1"] != "v1" || cfg.Headers["k2"] != "v2" {
		t.Errorf("unexpected headers: %v", cfg.Headers)
	}
	if cfg.ResourceAttributes["env"] != "prod" || cfg.ResourceAttributes["team"] != "backend" {
		t.Errorf("unexpected resource attributes: %v", cfg.ResourceAttributes)
	}
	if cfg.SampleRatio != 0.5 {
		t.Errorf("got SampleRatio %f, want 0.5", cfg.SampleRatio)
	}

	// Test bool parser
	if !parseBool("true", false) || !parseBool("1", false) || !parseBool("yes", false) || !parseBool("on", false) {
		t.Error("parseBool truthy failed")
	}
	if parseBool("false", true) || parseBool("0", true) || parseBool("no", true) || parseBool("off", true) {
		t.Error("parseBool falsy failed")
	}
	if !parseBool("other", true) {
		t.Error("parseBool default failed")
	}

	// Test float parser
	if parseFloat("0.75", 1.0) != 0.75 {
		t.Errorf("got %f, want 0.75", parseFloat("0.75", 1.0))
	}
	if parseFloat("", 1.0) != 1.0 {
		t.Errorf("got %f, want 1.0", parseFloat("", 1.0))
	}
	if parseFloat("invalid", 1.0) != 1.0 {
		t.Errorf("got %f, want 1.0", parseFloat("invalid", 1.0))
	}
}

func TestSamplers(t *testing.T) {
	samplers := []string{
		"always_on",
		"always_off",
		"traceidratio",
		"parentbased_always_off",
		"parentbased_traceidratio",
		"parentbased_always_on",
		"unknown",
	}
	for _, s := range samplers {
		sampler := parseSampler(s, 0.5)
		if sampler == nil {
			t.Errorf("sampler %q returned nil", s)
		}
	}
}

func TestMapPropagation(t *testing.T) {
	ctx := context.Background()
	cfg := Config{
		Enabled:         true,
		ServiceName:     "test-map",
		TracesExporter:  "stdout",
		MetricsExporter: "prometheus",
	}
	tel, err := Init(ctx, cfg)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer tel.Shutdown(ctx)

	spanCtx, span := StartSpan(ctx, "test-map", "operation")
	defer span.End()

	carrier := make(map[string]string)
	InjectMap(spanCtx, carrier)
	if len(carrier) == 0 {
		t.Error("expected non-empty carrier after InjectMap")
	}

	extracted := ExtractMap(context.Background(), carrier)
	extSpan := SpanFromContext(extracted)
	if extSpan == nil {
		t.Error("expected span from extracted map context")
	}

	// Test tracing helpers
	RecordError(span, errors.New("test error"))
	SetOK(span)
	AddEvent(span, "custom_event")
	if NoopTracer() == nil {
		t.Error("expected non-nil NoopTracer")
	}
	if Global() == nil {
		t.Error("expected non-nil Global")
	}
}

func TestStdoutExporters(t *testing.T) {
	ctx := context.Background()
	cfg := Config{
		Enabled:         true,
		ServiceName:     "test-stdout",
		TracesExporter:  "stdout",
		MetricsExporter: "stdout",
	}
	tel, err := Init(ctx, cfg)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer tel.Shutdown(ctx)

	if tel.TracerProvider == nil || tel.MeterProvider == nil {
		t.Error("expected TracerProvider and MeterProvider to be non-nil")
	}
}

func TestTraceLogHandler(t *testing.T) {
	ctx := context.Background()
	cfg := Config{
		Enabled:         true,
		ServiceName:     "test-log",
		TracesExporter:  "stdout",
		MetricsExporter: "prometheus",
	}
	tel, err := Init(ctx, cfg)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer tel.Shutdown(ctx)

	spanCtx, span := StartSpan(ctx, "test-log", "log-operation")
	defer span.End()

	var buf bytes.Buffer
	baseHandler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{})
	logHandler := &TraceLogHandler{Handler: baseHandler}
	logger := slog.New(logHandler)

	logger.InfoContext(spanCtx, "test message with trace context")
	out := buf.String()
	if !strings.Contains(out, "trace_id") || !strings.Contains(out, "span_id") {
		t.Errorf("expected trace_id and span_id in log output, got %s", out)
	}

	// Test without trace context
	buf.Reset()
	logger.InfoContext(context.Background(), "message without trace context")
	outNoCtx := buf.String()
	if strings.Contains(outNoCtx, "trace_id") {
		t.Errorf("expected no trace_id without span, got %s", outNoCtx)
	}

	// Test WithAttrs & WithGroup
	subHandler := logHandler.WithAttrs([]slog.Attr{slog.String("component", "api")}).WithGroup("group1")
	if subHandler == nil {
		t.Error("expected non-nil subHandler")
	}
}

func TestOTLPExportersCreation(t *testing.T) {
	ctx := context.Background()

	// HTTP protocol
	cfgHTTP := Config{
		Endpoint: "http://localhost:4318",
		Protocol: "http/protobuf",
		Insecure: true,
		Headers:  map[string]string{"Authorization": "Bearer test"},
	}
	tExpHTTP, err := createOTLPTraceExporter(ctx, cfgHTTP)
	if err != nil {
		t.Errorf("createOTLPTraceExporter HTTP failed: %v", err)
	}
	if tExpHTTP != nil {
		_ = tExpHTTP.Shutdown(ctx)
	}

	mExpHTTP, err := createOTLPMetricExporter(ctx, cfgHTTP)
	if err != nil {
		t.Errorf("createOTLPMetricExporter HTTP failed: %v", err)
	}
	if mExpHTTP != nil {
		_ = mExpHTTP.Shutdown(ctx)
	}

	// gRPC protocol
	cfgGRPC := Config{
		Endpoint: "localhost:4317",
		Protocol: "grpc",
		Insecure: true,
	}
	tExpGRPC, err := createOTLPTraceExporter(ctx, cfgGRPC)
	if err != nil {
		t.Errorf("createOTLPTraceExporter gRPC failed: %v", err)
	}
	if tExpGRPC != nil {
		_ = tExpGRPC.Shutdown(ctx)
	}

	mExpGRPC, err := createOTLPMetricExporter(ctx, cfgGRPC)
	if err != nil {
		t.Errorf("createOTLPMetricExporter gRPC failed: %v", err)
	}
	if mExpGRPC != nil {
		_ = mExpGRPC.Shutdown(ctx)
	}
}
