package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func okHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// TestRateLimiterThresholdAndReset verifies a tier returns 429 once its
// per-window budget is exhausted, and recovers after the window rolls over.
func TestRateLimiterThresholdAndReset(t *testing.T) {
	rl := &rateLimiter{
		window:   time.Minute,
		limits:   map[tier]int{tierAuth: 3},
		counters: make(map[string]*rlCounter),
	}
	now := time.Now()

	for i := 0; i < 3; i++ {
		if ok, _ := rl.allow(tierAuth, "1.2.3.4", now); !ok {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	ok, retry := rl.allow(tierAuth, "1.2.3.4", now)
	if ok {
		t.Fatal("4th request should be rate limited")
	}
	if retry <= 0 {
		t.Fatalf("expected positive Retry-After, got %v", retry)
	}

	// A different IP has its own budget.
	if ok, _ := rl.allow(tierAuth, "5.6.7.8", now); !ok {
		t.Fatal("different IP should be allowed")
	}

	// After the window rolls over, the original IP recovers.
	later := now.Add(time.Minute + time.Second)
	if ok, _ := rl.allow(tierAuth, "1.2.3.4", later); !ok {
		t.Fatal("request after window reset should be allowed")
	}
}

// TestRateLimiterDisabledTier confirms a non-positive limit exempts the tier.
func TestRateLimiterDisabledTier(t *testing.T) {
	rl := &rateLimiter{
		window:   time.Minute,
		limits:   map[tier]int{tierRedirect: 0},
		counters: make(map[string]*rlCounter),
	}
	now := time.Now()
	for i := 0; i < 100; i++ {
		if ok, _ := rl.allow(tierRedirect, "9.9.9.9", now); !ok {
			t.Fatal("disabled tier should always allow")
		}
	}
}

// TestMiddleware429AfterThreshold drives the full handle() path and checks a
// 429 with Retry-After is returned once the auth tier budget is spent.
func TestMiddleware429AfterThreshold(t *testing.T) {
	mw := &middleware{
		limiter: &rateLimiter{
			window:   time.Minute,
			limits:   map[tier]int{tierAuth: 2},
			counters: make(map[string]*rlCounter),
		},
		metrics: newMetrics(),
	}

	do := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/api/auth/login", nil)
		req.RemoteAddr = "10.0.0.1:1234"
		rec := httptest.NewRecorder()
		mw.handle(rec, req, okHandler)
		return rec
	}

	if rec := do(); rec.Code != http.StatusOK {
		t.Fatalf("1st: want 200 got %d", rec.Code)
	}
	if rec := do(); rec.Code != http.StatusOK {
		t.Fatalf("2nd: want 200 got %d", rec.Code)
	}
	rec := do()
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("3rd: want 429 got %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header on 429")
	}
}

// TestRequestIDHeader ensures every response carries an X-Request-Id, and an
// inbound sane ID is echoed back.
func TestRequestIDHeader(t *testing.T) {
	mw := newMiddleware(RuntimeSettings{})

	req := httptest.NewRequest("GET", "/anything", nil)
	req.RemoteAddr = "10.0.0.2:1"
	rec := httptest.NewRecorder()
	mw.handle(rec, req, okHandler)
	if rec.Header().Get("X-Request-Id") == "" {
		t.Fatal("expected generated X-Request-Id header")
	}

	req2 := httptest.NewRequest("GET", "/anything", nil)
	req2.RemoteAddr = "10.0.0.3:1"
	req2.Header.Set("X-Request-Id", "abc-123")
	rec2 := httptest.NewRecorder()
	mw.handle(rec2, req2, okHandler)
	if got := rec2.Header().Get("X-Request-Id"); got != "abc-123" {
		t.Fatalf("expected inbound request id echoed, got %q", got)
	}
}

// TestMetricsGating checks /metrics is closed by default, open to loopback when
// no token is set, and open to the correct bearer token.
func TestMetricsGating(t *testing.T) {
	// No token configured: remote (non-loopback) is refused.
	mwNoToken := &middleware{
		limiter: newRateLimiter(),
		metrics: newMetrics(),
	}
	reqRemote := httptest.NewRequest("GET", "/metrics", nil)
	reqRemote.RemoteAddr = "8.8.8.8:9999"
	recRemote := httptest.NewRecorder()
	mwNoToken.handle(recRemote, reqRemote, okHandler)
	if recRemote.Code != http.StatusForbidden {
		t.Fatalf("remote /metrics without token: want 403 got %d", recRemote.Code)
	}

	// No token configured: loopback is allowed.
	reqLocal := httptest.NewRequest("GET", "/metrics", nil)
	reqLocal.RemoteAddr = "127.0.0.1:5555"
	recLocal := httptest.NewRecorder()
	mwNoToken.handle(recLocal, reqLocal, okHandler)
	if recLocal.Code != http.StatusOK {
		t.Fatalf("loopback /metrics: want 200 got %d", recLocal.Code)
	}
	if ct := recLocal.Header().Get("Content-Type"); ct == "" {
		t.Fatal("expected JSON content type on metrics")
	}

	// Token configured: wrong/absent token refused, correct token served even
	// from a remote address.
	// Token supplied through the DB-backed settings seam, picked up by the
	// first refreshConfig.
	mwToken := newMiddleware(RuntimeSettings{MetricsToken: func() string { return "s3cret" }})
	reqBad := httptest.NewRequest("GET", "/metrics", nil)
	reqBad.RemoteAddr = "8.8.8.8:1"
	recBad := httptest.NewRecorder()
	mwToken.handle(recBad, reqBad, okHandler)
	if recBad.Code != http.StatusForbidden {
		t.Fatalf("wrong token /metrics: want 403 got %d", recBad.Code)
	}

	reqGood := httptest.NewRequest("GET", "/metrics", nil)
	reqGood.RemoteAddr = "8.8.8.8:1"
	reqGood.Header.Set("Authorization", "Bearer s3cret")
	recGood := httptest.NewRecorder()
	mwToken.handle(recGood, reqGood, okHandler)
	if recGood.Code != http.StatusOK {
		t.Fatalf("correct token /metrics: want 200 got %d", recGood.Code)
	}
}

// TestClientIP checks the XFF/X-Real-IP/RemoteAddr precedence, and that proxy
// headers are honoured only when trustProxy is enabled.
func TestClientIP(t *testing.T) {
	// With trustProxy on, proxy headers take precedence.
	trustProxy = true
	defer func() { trustProxy = false }()

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.9:1"
	req.Header.Set("X-Forwarded-For", "203.0.113.7, 10.0.0.1")
	if got := clientIP(req); got != "203.0.113.7" {
		t.Fatalf("XFF first hop: got %q", got)
	}

	req2 := httptest.NewRequest("GET", "/", nil)
	req2.RemoteAddr = "10.0.0.9:1"
	req2.Header.Set("X-Real-IP", "198.51.100.4")
	if got := clientIP(req2); got != "198.51.100.4" {
		t.Fatalf("X-Real-IP: got %q", got)
	}

	req3 := httptest.NewRequest("GET", "/", nil)
	req3.RemoteAddr = "192.0.2.5:5555"
	if got := clientIP(req3); got != "192.0.2.5" {
		t.Fatalf("RemoteAddr fallback: got %q", got)
	}

	// With trustProxy off, a spoofed XFF must be ignored in favour of RemoteAddr.
	trustProxy = false
	req4 := httptest.NewRequest("GET", "/", nil)
	req4.RemoteAddr = "192.0.2.5:5555"
	req4.Header.Set("X-Forwarded-For", "203.0.113.7")
	if got := clientIP(req4); got != "192.0.2.5" {
		t.Fatalf("untrusted XFF should be ignored: got %q", got)
	}
}

// TestTierFor confirms path classification, especially that short-link
// redirects land in the loose redirect tier.
func TestTierFor(t *testing.T) {
	cases := []struct {
		method, path string
		want         tier
	}{
		{"POST", "/api/auth/login", tierAuth},
		{"POST", "/api/webhook/x", tierAuth},
		{"POST", "/abuse", tierAuth},
		{"GET", "/api/links", tierAPI},
		{"GET", "/admin/", tierAPI},
		{"GET", "/portal/", tierAPI},
		{"GET", "/", tierAPI},
		{"GET", "/mySlug", tierRedirect},
		// Version-prefixed aliases normalize before classifying, so /api/v1/auth
		// still lands in the strict auth tier (not the generous API tier).
		{"POST", "/api/v1/auth/login", tierAuth},
		{"GET", "/api/v1/links", tierAPI},
	}
	for _, c := range cases {
		req := httptest.NewRequest(c.method, c.path, nil)
		if got := tierFor(req); got != c.want {
			t.Errorf("tierFor(%s %s) = %d, want %d", c.method, c.path, got, c.want)
		}
	}
}
