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
	}
	if tel.MeterProvider == nil {
		t.Fatal("expected MeterProvider to be non-nil")
	}
	if tel.Metrics == nil {
		t.Fatal("expected Metrics to be non-nil")
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
