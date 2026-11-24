package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/octarq-org/octarq/server/pkg/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (sr *statusRecorder) WriteHeader(code int) {
	if !sr.wrote {
		sr.status = code
		sr.wrote = true
	}
	sr.ResponseWriter.WriteHeader(code)
}

func (sr *statusRecorder) Write(b []byte) (int, error) {
	if !sr.wrote {
		sr.status = http.StatusOK
		sr.wrote = true
	}
	return sr.ResponseWriter.Write(b)
}

func (sr *statusRecorder) Flush() {
	if f, ok := sr.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

const (
	defaultAuthRPM     = 60
	defaultAPIRPM      = 600
	defaultRedirectRPM = 6000
)

const settingsRefreshInterval = 30 * time.Second

type RuntimeSettings struct {
	MetricsToken func() string
	RateLimits   func() (authRPM, apiRPM int)
	CORSOrigins  func() []string
	PublicGET    func(path string) bool
}

type middleware struct {
	limiter  *rateLimiter
	metrics  *metrics
	settings RuntimeSettings

	confMu       sync.Mutex
	confAt       time.Time
	metricsToken string
	corsOrigins  []string
}

func newMiddleware(rs RuntimeSettings) *middleware {
	mw := &middleware{
		limiter:  newRateLimiter(),
		metrics:  newMetrics(),
		settings: rs,
	}
	if rs.MetricsToken != nil {
		mw.metricsToken = strings.TrimSpace(rs.MetricsToken())
	}
	if mw.metricsToken == "" {
		mw.metricsToken = strings.TrimSpace(os.Getenv("OCTARQ_METRICS_TOKEN"))
	}
	return mw
}

func (mw *middleware) refreshConfig(now time.Time) {
	mw.confMu.Lock()
	defer mw.confMu.Unlock()
	if now.Sub(mw.confAt) < settingsRefreshInterval && !mw.confAt.IsZero() {
		return
	}
	mw.confAt = now
	if mw.settings.MetricsToken != nil {
		mw.metricsToken = strings.TrimSpace(mw.settings.MetricsToken())
	}
	if mw.metricsToken == "" {
		mw.metricsToken = strings.TrimSpace(os.Getenv("OCTARQ_METRICS_TOKEN"))
	}
	if mw.settings.RateLimits != nil {
		authRPM, apiRPM := mw.settings.RateLimits()
		mw.limiter.setLimits(authRPM, apiRPM)
	}
	if mw.settings.CORSOrigins != nil {
		mw.corsOrigins = mw.settings.CORSOrigins()
	}
}

func (mw *middleware) currentMetricsToken() string {
	mw.confMu.Lock()
	defer mw.confMu.Unlock()
	return mw.metricsToken
}

func (mw *middleware) currentCORSOrigins() []string {
	mw.confMu.Lock()
	defer mw.confMu.Unlock()
	return mw.corsOrigins
}

func (mw *middleware) handle(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	start := time.Now()
	ip := clientIP(r)
	mw.refreshConfig(start)
	setSecurityHeaders(w, r)

	ctx := telemetry.ExtractHTTP(r.Context(), r.Header)
	spanName := fmt.Sprintf("HTTP %s %s", r.Method, r.URL.Path)
	ctx, span := telemetry.StartSpan(ctx, "github.com/octarq-org/octarq/server/http", spanName,
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(
			attribute.String("http.method", r.Method),
			attribute.String("http.path", r.URL.Path),
			attribute.String("http.route", r.URL.Path),
			attribute.String("http.host", r.Host),
			attribute.String("client.ip", ip),
			attribute.String("user_agent", r.UserAgent()),
		),
	)
	defer span.End()

	if sc := span.SpanContext(); sc.IsValid() {
		w.Header().Set("X-Trace-Id", sc.TraceID().String())
	}

	rid := sanitizeRequestID(r.Header.Get("X-Request-Id"))
	if rid == "" {
		rid = newRequestID()
	}
	w.Header().Set("X-Request-Id", rid)
	span.SetAttributes(attribute.String("octarq.request_id", rid))
	r = r.WithContext(context.WithValue(ctx, RequestIDKey, rid))

	if mw.applyCORS(w, r) {
		telemetry.SetOK(span)
		mw.finish(r, ip, rid, http.StatusNoContent, start)
		return
	}

	if r.Method == http.MethodGet && r.URL.Path == "/metrics" {
		if !mw.metricsEnabled() {
			http.NotFound(w, r)
			return
		}
		mw.serveMetrics(w, r)
		telemetry.SetOK(span)
		mw.finish(r, ip, rid, http.StatusOK, start)
		return
	}

	t := tierFor(r)
	ok, retry := mw.limiter.allow(t, ip, start)
	mw.setRateLimitHeaders(w, t, ip, start)
	if !ok {
		secs := int(retry.Seconds())
		if secs < 1 {
			secs = 1
		}
		w.Header().Set("Retry-After", strconv.Itoa(secs))
		writeRateLimited(w, secs)
		telemetry.RecordError(span, fmt.Errorf("rate limit exceeded"))
		span.SetAttributes(attribute.Int("http.status_code", http.StatusTooManyRequests))
		mw.finish(r, ip, rid, http.StatusTooManyRequests, start)
		return
	}

	mw.metrics.inFlight.Add(1)
	tel := telemetry.Global()
	if tel.Metrics != nil && tel.Metrics.HTTPRequestsInFlight != nil {
		tel.Metrics.HTTPRequestsInFlight.Add(ctx, 1)
		defer tel.Metrics.HTTPRequestsInFlight.Add(ctx, -1)
	}

	sr := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
	next(sr, r)
	mw.metrics.inFlight.Add(-1)

	span.SetAttributes(attribute.Int("http.status_code", sr.status))
	if sr.status >= 500 {
		span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", sr.status))
	} else {
		span.SetStatus(codes.Ok, "")
	}

	mw.metrics.record(sr.status)
	telemetry.RecordHTTPRequest(r.Method, r.URL.Path, sr.status, time.Since(start))
	if tel.Metrics != nil {
		tel.Metrics.RecordHTTPRequest(ctx, r.Method, r.URL.Path, sr.status, time.Since(start), 0)
	}
	mw.finish(r, ip, rid, sr.status, start)
}

func redactPathSecrets(path string) string {
	if !strings.HasPrefix(path, "/api/webhook/") {
		return path
	}
	i := strings.LastIndex(path, "/")
	if i < 0 || i == len(path)-1 {
		return path
	}
	return path[:i+1] + "[redacted]"
}

func (mw *middleware) finish(r *http.Request, ip, rid string, status int, start time.Time) {
	attrs := []any{
		"method", r.Method,
		"path", redactPathSecrets(r.URL.Path),
		"status", status,
		"duration_ms", time.Since(start).Milliseconds(),
		"request_id", rid,
		"client_ip", ip,
	}
	if traceID := telemetry.TraceID(r.Context()); traceID != "" {
		attrs = append(attrs, "trace_id", traceID, "span_id", telemetry.SpanID(r.Context()))
	}
	slog.InfoContext(r.Context(), "request", attrs...)
}

func (mw *middleware) serveMetrics(w http.ResponseWriter, r *http.Request) {
	if mw.currentMetricsToken() == "" {
		http.NotFound(w, r)
		return
	}
	if !mw.metricsAllowed(r) {
		http.Error(w, "forbidden: send the OCTARQ_METRICS_TOKEN bearer token", http.StatusForbidden)
		return
	}

	mw.metrics.record(http.StatusOK)
	telemetry.RecordHTTPRequest(r.Method, r.URL.Path, http.StatusOK, 0)

	if r.URL.Query().Get("format") == "json" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if err := json.NewEncoder(w).Encode(mw.metrics.snapshot()); err != nil {
			slog.Warn("server: failed to encode metrics snapshot", "err", err)
		}
		return
	}

	var promHandler http.Handler
	tel := telemetry.Global()
	if tel != nil && tel.PrometheusExporter != nil {
		promHandler = tel.PrometheusHandler()
	} else {
		promHandler = telemetry.DefaultPrometheusHandler()
	}
	promHandler.ServeHTTP(w, r)
}

func (mw *middleware) metricsAllowed(r *http.Request) bool {
	token := mw.currentMetricsToken()
	if token == "" {
		return false
	}
	got := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	return got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1
}

func (mw *middleware) metricsEnabled() bool {
	if v := os.Getenv("OCTARQ_METRICS_ENABLED"); v != "" {
		v = strings.ToLower(strings.TrimSpace(v))
		return v != "false" && v != "0" && v != "no" && v != "off"
	}
	return true
}
