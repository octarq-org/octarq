package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"expvar"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ctxKey is an unexported type for context keys defined in this package, so
// they can never collide with keys defined elsewhere.
type ctxKey int

// RequestIDKey is the context key under which the per-request ID is stored.
// Future handlers can read it with r.Context().Value(server.RequestIDKey).
const RequestIDKey ctxKey = iota

// RequestID returns the request ID carried on the context, or "" if absent.
func RequestID(ctx context.Context) string {
	if v, ok := ctx.Value(RequestIDKey).(string); ok {
		return v
	}
	return ""
}

// -----------------------------------------------------------------------------
// Metrics (stdlib expvar)
// -----------------------------------------------------------------------------

// metrics holds the operational counters exposed at /metrics. They are backed
// by expvar so they are safe for concurrent updates without extra locking.
type metrics struct {
	startedAt time.Time
	requests  *expvar.Int
	status2xx *expvar.Int
	status3xx *expvar.Int
	status4xx *expvar.Int
	status5xx *expvar.Int
	inFlight  *expvar.Int
}

// newMetrics builds the counters. expvar's global registry panics on duplicate
// names, so we look up existing vars first (handy across test re-runs) and fall
// back to publishing fresh ones.
func newMetrics() *metrics {
	return &metrics{
		startedAt: time.Now(),
		requests:  getOrNewInt("led_requests_total"),
		status2xx: getOrNewInt("led_responses_2xx"),
		status3xx: getOrNewInt("led_responses_3xx"),
		status4xx: getOrNewInt("led_responses_4xx"),
		status5xx: getOrNewInt("led_responses_5xx"),
		inFlight:  getOrNewInt("led_requests_in_flight"),
	}
}

func getOrNewInt(name string) *expvar.Int {
	if v := expvar.Get(name); v != nil {
		if iv, ok := v.(*expvar.Int); ok {
			return iv
		}
	}
	return expvar.NewInt(name)
}

// record updates the counters for one completed request.
func (m *metrics) record(status int) {
	m.requests.Add(1)
	switch {
	case status >= 500:
		m.status5xx.Add(1)
	case status >= 400:
		m.status4xx.Add(1)
	case status >= 300:
		m.status3xx.Add(1)
	default:
		m.status2xx.Add(1)
	}
}

// snapshot renders the current metrics as a JSON-serialisable map.
func (m *metrics) snapshot() map[string]any {
	return map[string]any{
		"uptime_seconds":     int64(time.Since(m.startedAt).Seconds()),
		"requests_total":     m.requests.Value(),
		"responses_2xx":      m.status2xx.Value(),
		"responses_3xx":      m.status3xx.Value(),
		"responses_4xx":      m.status4xx.Value(),
		"responses_5xx":      m.status5xx.Value(),
		"requests_in_flight": m.inFlight.Value(),
	}
}

// -----------------------------------------------------------------------------
// Rate limiter (fixed-window, in-memory, IP-keyed)
// -----------------------------------------------------------------------------

// tier identifies which threshold applies to a request.
type tier int

const (
	tierAuth     tier = iota // auth-sensitive: strict
	tierAPI                  // general API: generous
	tierRedirect             // short-link hot path: very loose
)

// rlCounter is one IP's request count within the current fixed window.
type rlCounter struct {
	count   int
	resetAt time.Time
}

// rateLimiter is a fixed-window per-IP limiter shared across tiers. The map key
// is "tier|ip" so the same IP gets an independent budget per tier.
type rateLimiter struct {
	window    time.Duration
	limits    map[tier]int
	mu        sync.Mutex
	counters  map[string]*rlCounter
	lastSweep time.Time
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{
		window: time.Minute,
		limits: map[tier]int{
			tierAuth:     envInt("LED_RATELIMIT_AUTH_RPM", 60),
			tierAPI:      envInt("LED_RATELIMIT_API_RPM", 600),
			tierRedirect: envInt("LED_RATELIMIT_REDIRECT_RPM", 6000),
		},
		counters:  make(map[string]*rlCounter),
		lastSweep: time.Now(),
	}
}

// allow reports whether a request from ip in the given tier may proceed. When
// denied it also returns the Retry-After duration until the window resets. A
// non-positive limit disables the tier (always allowed).
func (rl *rateLimiter) allow(t tier, ip string, now time.Time) (bool, time.Duration) {
	limit := rl.limits[t]
	if limit <= 0 {
		return true, 0
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.sweepLocked(now)

	key := strconv.Itoa(int(t)) + "|" + ip
	c := rl.counters[key]
	if c == nil || now.After(c.resetAt) {
		rl.counters[key] = &rlCounter{count: 1, resetAt: now.Add(rl.window)}
		return true, 0
	}
	if c.count >= limit {
		return false, time.Until(c.resetAt)
	}
	c.count++
	return true, 0
}

// sweepLocked drops expired counters so the map can't grow without bound. It is
// throttled to run at most once per window. Caller must hold rl.mu.
func (rl *rateLimiter) sweepLocked(now time.Time) {
	if now.Sub(rl.lastSweep) < rl.window {
		return
	}
	rl.lastSweep = now
	for k, c := range rl.counters {
		if now.After(c.resetAt) {
			delete(rl.counters, k)
		}
	}
}

// tierFor classifies a request into a rate-limit tier by path/method.
func tierFor(r *http.Request) tier {
	p := r.URL.Path
	// Version prefixes are aliases (the mux rewrites /api/v1/x → /api/x), so
	// normalize before classifying — otherwise /api/v1/auth/* would slip into the
	// generous API tier instead of the strict auth tier.
	if strings.HasPrefix(p, "/api/v1/") {
		p = "/api/" + strings.TrimPrefix(p, "/api/v1/")
	}
	switch {
	case strings.HasPrefix(p, "/api/auth/"), strings.HasPrefix(p, "/api/webhook/"):
		return tierAuth
	case r.Method == http.MethodPost && (p == "/abuse" || p == "/api/abuse"):
		return tierAuth
	case strings.HasPrefix(p, "/api/"):
		return tierAPI
	case p == "/admin" || strings.HasPrefix(p, "/admin/"),
		p == "/portal" || strings.HasPrefix(p, "/portal/"),
		p == "/", p == "/metrics":
		// Dashboard/portal/root: treat as general API budget, not the loose
		// redirect budget (these are not the redirect hot path).
		return tierAPI
	default:
		// Root-namespace short-link redirects — the product hot path.
		return tierRedirect
	}
}

// -----------------------------------------------------------------------------
// Client IP
// -----------------------------------------------------------------------------

// trustProxy gates whether proxy-supplied client-IP headers are honoured. Set
// once from config at server construction; when false, X-Forwarded-For /
// X-Real-IP are ignored so clients can't spoof their IP to evade rate limits.
var trustProxy bool

// clientIP returns the best-guess client IP. When trustProxy is enabled it
// honours the first hop of X-Forwarded-For then X-Real-IP, falling back to
// RemoteAddr; otherwise it always uses RemoteAddr.
func clientIP(r *http.Request) string {
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if first := strings.TrimSpace(strings.Split(xff, ",")[0]); first != "" {
				return first
			}
		}
		if rip := strings.TrimSpace(r.Header.Get("X-Real-IP")); rip != "" {
			return rip
		}
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// isLoopback reports whether the direct peer (RemoteAddr, ignoring proxy
// headers) is a loopback address. Used to bind /metrics to localhost when no
// token is configured.
func isLoopback(r *http.Request) bool {
	host := r.RemoteAddr
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if host == "" {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return host == "localhost"
}

// -----------------------------------------------------------------------------
// Request ID
// -----------------------------------------------------------------------------

// newRequestID returns a random 16-hex-char (8-byte) request ID.
func newRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b[:])
}

// sanitizeRequestID accepts an inbound X-Request-Id only if it's short and made
// of safe characters, otherwise returns "" so we generate a fresh one.
func sanitizeRequestID(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || len(v) > 128 {
		return ""
	}
	for _, c := range v {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.') {
			return ""
		}
	}
	return v
}

// -----------------------------------------------------------------------------
// ResponseWriter wrapper (capture status + byte count)
// -----------------------------------------------------------------------------

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

// -----------------------------------------------------------------------------
// Middleware
// -----------------------------------------------------------------------------

// middleware bundles the edge concerns (request IDs, rate limiting, metrics,
// access logging) wrapped around the router.
type middleware struct {
	limiter      *rateLimiter
	metrics      *metrics
	metricsToken string
}

func newMiddleware() *middleware {
	return &middleware{
		limiter:      newRateLimiter(),
		metrics:      newMetrics(),
		metricsToken: strings.TrimSpace(os.Getenv("LED_METRICS_TOKEN")),
	}
}

// handle applies the edge middleware and then dispatches to next.
func (mw *middleware) handle(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	start := time.Now()
	ip := clientIP(r)

	// 1. Request ID: reuse a sane inbound one, else generate.
	rid := sanitizeRequestID(r.Header.Get("X-Request-Id"))
	if rid == "" {
		rid = newRequestID()
	}
	w.Header().Set("X-Request-Id", rid)
	r = r.WithContext(context.WithValue(r.Context(), RequestIDKey, rid))

	// 2. Gated metrics endpoint.
	if r.Method == http.MethodGet && r.URL.Path == "/metrics" {
		mw.serveMetrics(w, r)
		mw.finish(r, ip, rid, http.StatusOK, start)
		return
	}

	// 3. Rate limit by tier.
	t := tierFor(r)
	if ok, retry := mw.limiter.allow(t, ip, start); !ok {
		secs := int(retry.Seconds())
		if secs < 1 {
			secs = 1
		}
		w.Header().Set("Retry-After", strconv.Itoa(secs))
		http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
		mw.finish(r, ip, rid, http.StatusTooManyRequests, start)
		return
	}

	// 4. Dispatch with status capture + in-flight gauge.
	mw.metrics.inFlight.Add(1)
	sr := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
	next(sr, r)
	mw.metrics.inFlight.Add(-1)

	mw.metrics.record(sr.status)
	mw.finish(r, ip, rid, sr.status, start)
}

// finish emits the edge access-log line.
func (mw *middleware) finish(r *http.Request, ip, rid string, status int, start time.Time) {
	slog.Info("request",
		"method", r.Method,
		"path", r.URL.Path,
		"status", status,
		"duration_ms", time.Since(start).Milliseconds(),
		"request_id", rid,
		"client_ip", ip,
	)
}

// serveMetrics gates and serves the expvar snapshot as JSON. It is closed by
// default: allowed only when the caller presents the LED_METRICS_TOKEN bearer,
// or (when no token is configured) originates from loopback.
func (mw *middleware) serveMetrics(w http.ResponseWriter, r *http.Request) {
	if !mw.metricsAllowed(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(mw.metrics.snapshot())
}

func (mw *middleware) metricsAllowed(r *http.Request) bool {
	if mw.metricsToken != "" {
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		got = strings.TrimSpace(got)
		if got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(mw.metricsToken)) == 1 {
			return true
		}
		return false
	}
	// No token configured: bind to loopback only.
	return isLoopback(r)
}

// envInt reads an integer env var, falling back to def on absence/parse error.
func envInt(key string, def int) int {
	if v, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return def
}
