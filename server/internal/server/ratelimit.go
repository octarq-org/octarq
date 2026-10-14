package server

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Rate limiter (fixed-window, in-memory, IP-keyed)

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
			tierAuth:     defaultAuthRPM,
			tierAPI:      defaultAPIRPM,
			tierRedirect: defaultRedirectRPM,
		},
		counters:  make(map[string]*rlCounter),
		lastSweep: time.Now(),
	}
}

// setLimits replaces the per-tier budgets (from the runtime settings refresh).
func (rl *rateLimiter) setLimits(authRPM, apiRPM, redirectRPM int) {
	rl.mu.Lock()
	rl.limits = map[tier]int{tierAuth: authRPM, tierAPI: apiRPM, tierRedirect: redirectRPM}
	rl.mu.Unlock()
}

// allow reports whether a request from ip in the given tier may proceed. When
// denied it also returns the Retry-After duration until the window resets. A
// non-positive limit disables the tier (always allowed).
func (rl *rateLimiter) allow(t tier, ip string, now time.Time) (bool, time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	limit := rl.limits[t]
	if limit <= 0 {
		return true, 0
	}

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

// budget reports the current window's state for one ip/tier without consuming
// any of it: the tier's limit, how many requests remain, and when the window
// resets. Read-only on purpose — allow() stays the only mutator, so its
// signature (and its callers) are untouched by rate-limit headers existing.
//
// A limit of 0 means the tier is disabled; the caller then emits no headers,
// which is honest — "unlimited" has no number to report.
func (rl *rateLimiter) budget(t tier, ip string, now time.Time) (limit, remaining int, resetAt time.Time) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	limit = rl.limits[t]
	if limit <= 0 {
		return 0, 0, time.Time{}
	}
	c := rl.counters[strconv.Itoa(int(t))+"|"+ip]
	if c == nil || now.After(c.resetAt) {
		return limit, limit, now.Add(rl.window)
	}
	remaining = limit - c.count
	if remaining < 0 {
		remaining = 0
	}
	return limit, remaining, c.resetAt
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
	switch {
	case strings.HasPrefix(p, "/api/auth/"), strings.HasPrefix(p, "/api/webhook/"):
		return tierAuth
	case r.Method == http.MethodPost && p == "/abuse":
		return tierAuth
	case strings.HasPrefix(p, "/api/"):
		return tierAPI
	case p == "/admin" || strings.HasPrefix(p, "/admin/"),
		p == "/portal" || strings.HasPrefix(p, "/portal/"),
		p == "/instance" || strings.HasPrefix(p, "/instance/"),
		p == "/status" || p == "/status/",
		p == "/", p == "/metrics":
		// Dashboard/portal/root: treat as general API budget, not the loose
		// redirect budget (these are not the redirect hot path).
		return tierAPI
	default:
		// Root-namespace short-link redirects — the product hot path.
		return tierRedirect
	}
}

// Client IP

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

// setRateLimitHeaders publishes the caller's remaining budget on every response
// the limiter governs, allowed or refused. Enforcing a limit without telling
// anyone what it is makes every integrator discover it by tripping it: the
// headers are what let a client pace itself instead of backing off blindly.
//
// X-RateLimit-Reset is a Unix timestamp in seconds — the widespread convention
// (GitHub, Stripe). Retry-After, set only on a 429, stays delta-seconds per RFC 9110.
func (mw *middleware) setRateLimitHeaders(w http.ResponseWriter, t tier, ip string, now time.Time) {
	limit, remaining, resetAt := mw.limiter.budget(t, ip, now)
	if limit <= 0 {
		return
	}
	h := w.Header()
	h.Set("X-RateLimit-Limit", strconv.Itoa(limit))
	h.Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
	h.Set("X-RateLimit-Reset", strconv.FormatInt(resetAt.Unix(), 10))
}

// writeRateLimited emits the 429 in the same RFC 7807 shape huma gives every
// other API error, instead of the text/plain line http.Error produces. A client
// that parses error bodies should not need a special case for the one error
// this middleware raises before the API handlers are ever reached.
//
// The body is written here rather than borrowed from internal/api: server must
// not import api (api imports server), and duplicating four fields beats an
// import cycle.
func writeRateLimited(w http.ResponseWriter, retryAfterSecs int) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(http.StatusTooManyRequests)
	if err := json.NewEncoder(w).Encode(map[string]any{
		"title":  "Too Many Requests",
		"status": http.StatusTooManyRequests,
		"detail": "rate limit exceeded; retry in " + strconv.Itoa(retryAfterSecs) + "s",
	}); err != nil {
		slog.Warn("server: failed to encode rate-limit response", "err", err)
	}
}
