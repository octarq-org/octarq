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

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

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

type dummyFlusher struct {
	http.ResponseWriter
	flushed bool
}

func (d *dummyFlusher) Flush() {
	d.flushed = true
}

func TestResponseWriter_Flush_And_ImplicitHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	df := &dummyFlusher{ResponseWriter: rec}
	rw := &responseWriter{ResponseWriter: df}

	// Write without WriteHeader
	n, err := rw.Write([]byte("hello world"))
	if err != nil || n != 11 {
		t.Errorf("Write error: %v, n=%d", err, n)
	}
	if rw.status != http.StatusOK || rw.bytes != 11 {
		t.Errorf("status=%d, bytes=%d", rw.status, rw.bytes)
	}

	rw.Flush()
	if !df.flushed {
		t.Errorf("expected dummyFlusher.Flush to be called")
	}
}

func TestRegisterDBStatsMetrics(t *testing.T) {
	// Nil checks
	if err := RegisterDBStatsMetrics(nil, nil); err != nil {
		t.Errorf("expected nil error for nil args, got %v", err)
	}

	ctx := context.Background()
	cfg := Config{
		Enabled:         true,
		ServiceName:     "test-db",
		MetricsExporter: "prometheus",
	}
	tel, err := Init(ctx, cfg)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer tel.Shutdown(ctx)

	meter := tel.MeterProvider.Meter("test-db-meter")
	if err := RegisterDBStatsMetrics(meter, nil); err != nil {
		t.Errorf("expected nil error for nil db, got %v", err)
	}

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to get sql.DB: %v", err)
	}
	defer sqlDB.Close()

	if err := RegisterDBStatsMetrics(meter, sqlDB); err != nil {
		t.Fatalf("RegisterDBStatsMetrics failed: %v", err)
	}

	// Trigger metrics scrape so callbacks are called
	promHandler := tel.PrometheusHandler()
	rec := httptest.NewRecorder()
	promReq := httptest.NewRequest("GET", "/metrics", nil)
	promHandler.ServeHTTP(rec, promReq)
	if rec.Code != http.StatusOK {
		t.Errorf("metrics scrape failed: %d", rec.Code)
	}
}

func TestNoopMetrics(t *testing.T) {
	nm := NewNoopMetrics()
	ctx := context.Background()
	nm.RecordHTTPRequest(ctx, "GET", "/test", 200, time.Second, 100)
	nm.RecordCacheHit(ctx, "redis")
	nm.RecordCacheMiss(ctx, "redis")
	nm.RecordQueueTask(ctx, "task", "ok", time.Second)
	nm.RecordWebhookDelivery(ctx, "wh", 200, time.Second)
}
