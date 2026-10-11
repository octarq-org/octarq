package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/octarq-org/octarq/server/pkg/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// ResponseWriter wrapper (capture status + byte count)

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

// Flush lets the wrapper stay transparent to streaming handlers.
func (sr *statusRecorder) Flush() {
	if f, ok := sr.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Middleware

// Rate-limit defaults (requests per minute per IP), used until the first
// settings refresh and when a setting is absent. Kept in sync with the API
// layer's defaults for the same settings keys.
const (
	defaultAuthRPM     = 60
	defaultAPIRPM      = 600
	defaultRedirectRPM = 6000
)

// settingsRefreshInterval bounds how often the edge middleware re-reads its
// DB-backed runtime settings (rate limits, metrics token). The redirect hot
// path must never query the settings table per request.
const settingsRefreshInterval = 30 * time.Second

// RuntimeSettings supplies the DB-backed runtime configuration the edge
// middleware needs. All funcs are optional (nil = built-in defaults); they are
// polled at most once per settingsRefreshInterval.
type RuntimeSettings struct {
	// MetricsToken returns the /metrics bearer token; empty = loopback-only.
	MetricsToken func() string
	// RateLimits returns the per-IP RPM budgets for the auth/api/redirect tiers.
	RateLimits func() (authRPM, apiRPM, redirectRPM int)
	// CORSOrigins returns the exact origins allowed to read public GET API
	// endpoints cross-origin. nil/empty = CORS disabled (no headers ever sent).
	CORSOrigins func() []string
	// PublicGET reports whether a path hosts a public GET endpoint — the only
	// routes CORS is ever granted to. nil disables CORS.
	PublicGET func(path string) bool
}

// middleware bundles the edge concerns (request IDs, rate limiting, metrics,
// access logging) wrapped around the router.
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
	return &middleware{
		limiter:  newRateLimiter(),
		metrics:  newMetrics(),
		settings: rs,
	}
}

// refreshConfig re-reads the DB-backed runtime settings at most once per
// settingsRefreshInterval, so setting changes apply without a restart while the
// hot path stays off the database.
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
	if mw.settings.RateLimits != nil {
		mw.limiter.setLimits(mw.settings.RateLimits())
	}
	if mw.settings.CORSOrigins != nil {
		mw.corsOrigins = mw.settings.CORSOrigins()
	}
}

// currentMetricsToken returns the cached metrics token.
func (mw *middleware) currentMetricsToken() string {
	mw.confMu.Lock()
	defer mw.confMu.Unlock()
	return mw.metricsToken
}

// currentCORSOrigins returns the cached cross-origin allowlist.
func (mw *middleware) currentCORSOrigins() []string {
	mw.confMu.Lock()
	defer mw.confMu.Unlock()
	return mw.corsOrigins
}

// handle applies the edge middleware and then dispatches to next.
func (mw *middleware) handle(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	start := time.Now()
	ip := clientIP(r)
	mw.refreshConfig(start)
	setSecurityHeaders(w, r)

	// 0. OpenTelemetry incoming trace context extraction & server span
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

	// 1. Request ID: reuse a sane inbound one, else generate.
	rid := sanitizeRequestID(r.Header.Get("X-Request-Id"))
	if rid == "" {
		rid = newRequestID()
	}
	w.Header().Set("X-Request-Id", rid)
	span.SetAttributes(attribute.String("octarq.request_id", rid))
	r = r.WithContext(context.WithValue(ctx, RequestIDKey, rid))

	// 1.5. Cross-origin reads of public GET endpoints. An approved preflight
	// short-circuits here (before rate limiting — preflights are cheap and
	// browsers batch them); everything else falls through to normal routing.
	if mw.applyCORS(w, r) {
		telemetry.SetOK(span)
		mw.finish(r, ip, rid, http.StatusNoContent, start)
		return
	}

	// 2. Gated metrics endpoint.
	if r.Method == http.MethodGet && r.URL.Path == "/metrics" {
		mw.serveMetrics(w, r)
		telemetry.SetOK(span)
		mw.finish(r, ip, rid, http.StatusOK, start)
		return
	}

	// 3. Rate limit by tier.
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

	// 4. Dispatch with status capture + in-flight gauge.
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
	if tel.Metrics != nil {
		tel.Metrics.RecordHTTPRequest(ctx, r.Method, r.URL.Path, sr.status, time.Since(start), 0)
	}
	mw.finish(r, ip, rid, sr.status, start)
}

// redactPathSecrets returns a path safe to write to the access log.
//
// The inbound-mail webhook carries the org's secret as the LAST path segment —
// POST /api/webhook/{orgSlug}/email/inbound/{token} and its siblings. Putting
// the secret in the URL is deliberate and argued at plugins/mail/webhook.go:
// SendGrid Inbound Parse and Mailgun routes let you configure a URL and nothing
// else, so a custom auth header is not on the table, and the token rotates from
// Mail settings. Logging it verbatim was not deliberate: it left every tenant's
// inbound-mail credential in plaintext in the access log, readable by anyone
// with log access and shipped onward to whatever aggregator collects them.
//
// OPERATOR CAVEAT: this only redacts octarq's own log line. Any reverse proxy,
// CDN, or load balancer in front of octarq logs the full request URI and will
// still record the token. Suppress the path (or the whole webhook route) in the
// proxy's access-log config too, and rotate the token if those logs leaked.
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

// finish emits the edge access-log line.
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

// serveMetrics gates and serves metrics. When OpenTelemetry Prometheus exporter
// is active and the client does not explicitly ask for ?format=json, it serves
// standard Prometheus metrics; otherwise it returns the JSON expvar snapshot.
func (mw *middleware) serveMetrics(w http.ResponseWriter, r *http.Request) {
	if !mw.metricsAllowed(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	tel := telemetry.Global()
	if tel != nil && tel.PrometheusExporter != nil && r.URL.Query().Get("format") != "json" {
		tel.PrometheusHandler().ServeHTTP(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(mw.metrics.snapshot())
}

func (mw *middleware) metricsAllowed(r *http.Request) bool {
	if token := mw.currentMetricsToken(); token != "" {
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		got = strings.TrimSpace(got)
		return got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1
	}
	// No token configured: bind to loopback only.
	return isLoopback(r)
}
