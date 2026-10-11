package telemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
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

func TestTelemetry_ExtraBranches(t *testing.T) {
	ctx := context.Background()

	// Tracer with empty name
	tr := Tracer("")
	if tr == nil {
		t.Error("expected non-nil default tracer")
	}

	// Telemetry without prometheus exporter -> PrometheusHandler returns 404
	telNone := &Telemetry{}
	h := telNone.PrometheusHandler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 from PrometheusHandler when exporter is nil, got %d", rec.Code)
	}

	// Init with MetricsExporter="none" and TracesExporter="none"
	cfgNone := Config{
		Enabled:         true,
		ServiceName:     "test-none",
		TracesExporter:  "none",
		MetricsExporter: "none",
	}
	telN, err := Init(ctx, cfgNone)
	if err != nil {
		t.Fatalf("Init with none exporters failed: %v", err)
	}
	_ = telN.Shutdown(ctx)

	// Init with MetricsExporter="otlp" and TracesExporter="otlp"
	cfgOTLP := Config{
		Enabled:         true,
		ServiceName:     "test-otlp",
		Endpoint:        "http://localhost:4318",
		Protocol:        "http/protobuf",
		TracesExporter:  "otlp",
		MetricsExporter: "otlp",
		Insecure:        true,
	}
	telO, err := Init(ctx, cfgOTLP)
	if err != nil {
		t.Fatalf("Init with otlp exporters failed: %v", err)
	}
	_ = telO.Shutdown(ctx)
}

func TestConfigFromEnv_EmptyDefaultsAndDisabled(t *testing.T) {
	t.Setenv("OTEL_SDK_DISABLED", "true")
	cfg := ConfigFromEnv("", "")
	if cfg.Enabled {
		t.Error("expected disabled when OTEL_SDK_DISABLED=true")
	}
	if cfg.ServiceName != "octarq" || cfg.ServiceVersion != "dev" {
		t.Errorf("expected default octarq/dev, got %s/%s", cfg.ServiceName, cfg.ServiceVersion)
	}
}
