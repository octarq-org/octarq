package telemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCoverageLift_GlobalUninitialized(t *testing.T) {
	globalMu.Lock()
	orig := globalTelemetry
	globalTelemetry = nil
	globalMu.Unlock()

	t.Cleanup(func() {
		globalMu.Lock()
		globalTelemetry = orig
		globalMu.Unlock()
	})

	g := Global()
	if g == nil || g.Metrics == nil {
		t.Fatal("expected non-nil Telemetry and NoopMetrics when global is nil")
	}
}

func TestCoverageLift_TracingHelpers(t *testing.T) {
	ctx := context.Background()
	if id := TraceID(ctx); id != "" {
		t.Fatalf("expected empty trace id for empty context, got %q", id)
	}
	if id := SpanID(ctx); id != "" {
		t.Fatalf("expected empty span id for empty context, got %q", id)
	}

	// Should not panic on nil inputs
	RecordError(nil, nil)
	SetOK(nil)
}

func TestCoverageLift_HTTPMiddleware_DefaultsAnd500Status(t *testing.T) {
	// Empty tracerName triggers default tracer name branch
	mw := HTTPMiddleware("")
	handler500 := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/error-path", nil)
	handler500.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestCoverageLift_ResourceAttributesAndDefaultExporter(t *testing.T) {
	ctx := context.Background()
	cfg := Config{
		Enabled:        true,
		ServiceName:    "test-res-svc",
		TracesExporter: "none",
		// Unknown exporter falls back to default prometheus exporter
		MetricsExporter: "custom_unsupported_exporter",
		ResourceAttributes: map[string]string{
			"custom.environment": "test-env",
			"custom.zone":        "us-east-1",
		},
	}

	tel, err := Init(ctx, cfg)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	t.Cleanup(func() {
		_ = tel.Shutdown(ctx)
	})

	if tel.PrometheusExporter == nil {
		t.Fatal("expected default prometheus exporter to be initialized")
	}
}

func TestCoverageLift_OTLPDefaultEndpoints(t *testing.T) {
	ctx := context.Background()
	cfg := Config{
		Insecure: true,
	}

	// Trace exporter default endpoint (localhost:4317)
	tExp, err := createOTLPTraceExporter(ctx, cfg)
	if err != nil {
		t.Fatalf("createOTLPTraceExporter failed: %v", err)
	}
	if tExp == nil {
		t.Fatal("expected non-nil trace exporter")
	}

	// Metric exporter default endpoint (localhost:4317)
	mExp, err := createOTLPMetricExporter(ctx, cfg)
	if err != nil {
		t.Fatalf("createOTLPMetricExporter failed: %v", err)
	}
	if mExp == nil {
		t.Fatal("expected non-nil metric exporter")
	}
}

func TestCoverageLift_RegisterDBStatsMetrics_Nil(t *testing.T) {
	if err := RegisterDBStatsMetrics(nil, nil); err != nil {
		t.Fatalf("expected nil error for nil inputs, got %v", err)
	}
}
