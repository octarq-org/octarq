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

type tier int

const (
	tierAuth tier = iota
	tierAPI
	tierRedirect
)

type rlCounter struct {
	count   int
	resetAt time.Time
}

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

func (rl *rateLimiter) setLimits(authRPM, apiRPM int) {
	rl.mu.Lock()
	rl.limits[tierAuth] = authRPM
	rl.limits[tierAPI] = apiRPM
	rl.mu.Unlock()
}

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

func tierFor(r *http.Request) tier {
	p := r.URL.Path
	switch {
	case strings.HasPrefix(p, "/api/auth/"), strings.HasPrefix(p, "/api/webhook/"):
		return tierAuth
	case r.Method == http.MethodPost && p == "/abuse":
		return tierAuth
	case strings.HasPrefix(p, "/api/"), strings.HasPrefix(p, "/v1/"), strings.HasPrefix(p, "/v1beta/"),
		strings.HasPrefix(p, "/.well-known/"):
		return tierAPI
	case p == "/admin" || strings.HasPrefix(p, "/admin/"),
		p == "/portal" || strings.HasPrefix(p, "/portal/"),
		p == "/instance" || strings.HasPrefix(p, "/instance/"),
		p == "/status" || p == "/status/",
		p == "/", p == "/metrics":
		return tierAPI
	default:
		return tierRedirect
	}
}

var trustProxy bool

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
