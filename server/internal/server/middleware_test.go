package server

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/octarq-org/octarq/pkg/telemetry"
	"github.com/octarq-org/octarq/webembed"
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

// TestOpenTelemetryTracingInMiddleware checks that incoming requests get traced
// and response carries the X-Trace-Id header.
func TestOpenTelemetryTracingInMiddleware(t *testing.T) {
	_, _ = telemetry.Init(context.Background(), telemetry.Config{
		Enabled:         true,
		ServiceName:     "test-server",
		TracesExporter:  "stdout",
		MetricsExporter: "prometheus",
	})
	mw := newMiddleware(RuntimeSettings{})
	req := httptest.NewRequest("GET", "/api/test", nil)
	rec := httptest.NewRecorder()
	mw.handle(rec, req, okHandler)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if traceID := rec.Header().Get("X-Trace-Id"); traceID == "" {
		t.Fatal("expected X-Trace-Id header in response")
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
	}
	for _, c := range cases {
		req := httptest.NewRequest(c.method, c.path, nil)
		if got := tierFor(req); got != c.want {
			t.Errorf("tierFor(%s %s) = %d, want %d", c.method, c.path, got, c.want)
		}
	}
}

// TestTierForV1AliasKeepsStrictTier is the security half of the /api/v1/ alias.
// The mux rewrites /api/v1/x to /api/x, but this middleware runs before the mux
// and sees the raw path, so tierFor has to normalize it itself. Without that,
// POST /api/v1/auth/login drops out of the strict auth tier into the generous
// API one — a rate-limit bypass for password brute force.
func TestTierForV1AliasKeepsStrictTier(t *testing.T) {
	cases := []struct{ method, v1Path, plainPath string }{
		{"POST", "/api/v1/auth/login", "/api/auth/login"},
		{"POST", "/api/v1/webhook/x", "/api/webhook/x"},
		{"GET", "/api/v1/links", "/api/links"},
	}
	for _, c := range cases {
		v1 := tierFor(httptest.NewRequest(c.method, c.v1Path, nil))
		plain := tierFor(httptest.NewRequest(c.method, c.plainPath, nil))
		if v1 != plain {
			t.Errorf("tierFor(%s %s) = %d, want %d (same tier as %s)",
				c.method, c.v1Path, v1, plain, c.plainPath)
		}
	}

	// Spelled out separately so the intent survives even if the tier constants
	// are renumbered: the login alias must not be classified as plain API.
	if got := tierFor(httptest.NewRequest("POST", "/api/v1/auth/login", nil)); got != tierAuth {
		t.Errorf("tierFor(POST /api/v1/auth/login) = %d, want tierAuth (%d)", got, tierAuth)
	}
}

// TestSecurityHeadersCSP pins the CSP contract that keeps XSS from becoming
// code execution: script-src must NOT carry 'unsafe-inline', while style-src
// MUST keep it (Tailwind injects its stylesheet at runtime and removing it
// blanks the dashboard).
//
// The allowed inline scripts are the two the dashboard genuinely ships in
// index.html (the synchronous theme toggle and the campaign-forwarding
// snippet). Their build-time SHA-256 hashes must be present in script-src,
// recomputed here from the actually-embedded dist — so if a frontend build
// ever refreshes those inline scripts without updating contentSecurityPolicy,
// this test fails instead of letting the browser silently block them.
func TestSecurityHeadersCSP(t *testing.T) {
	rec := httptest.NewRecorder()
	setSecurityHeaders(rec, httptest.NewRequest("GET", "/", nil))
	csp := rec.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Fatal("missing Content-Security-Policy header")
	}

	scriptSrc := cspDirective(csp, "script-src")
	if strings.Contains(scriptSrc, "'unsafe-inline'") {
		t.Errorf("script-src must not contain 'unsafe-inline', got %q", scriptSrc)
	}
	styleSrc := cspDirective(csp, "style-src")
	if !strings.Contains(styleSrc, "'unsafe-inline'") {
		t.Errorf("style-src must keep 'unsafe-inline' (Tailwind), got %q", styleSrc)
	}

	idx, err := webembed.FS()
	if err != nil {
		t.Fatalf("webembed.FS: %v", err)
	}
	raw, err := fs.ReadFile(idx, "index.html")
	if err != nil {
		t.Fatalf("read embedded index.html: %v", err)
	}
	hashes := inlineScriptHashes(string(raw))
	if len(hashes) == 0 {
		t.Fatal("no inline scripts found in embedded index.html; CSP hashes would be dead weight")
	}
	for _, h := range hashes {
		if !strings.Contains(scriptSrc, "'"+h+"'") {
			t.Errorf("script-src missing %s for an inline script in the served index.html; regenerate contentSecurityPolicy", h)
		}
	}
}

// cspDirective extracts a single directive (everything after "name ") from a
// semicolon-joined Content-Security-Policy value.
func cspDirective(csp, name string) string {
	for _, d := range strings.Split(csp, ";") {
		d = strings.TrimSpace(d)
		if strings.HasPrefix(d, name+" ") {
			return strings.TrimPrefix(d, name+" ")
		}
	}
	return ""
}

// inlineScriptHashes returns "sha256-<base64>" for every inline <script> block
// (script tags without attributes) in the given HTML. Vite preserves these
// verbatim in the build, so hashing the served file matches what the browser
// will actually check against the CSP.
func inlineScriptHashes(html string) []string {
	re := regexp.MustCompile(`(?s)<script>([\s\S]*?)</script>`)
	var out []string
	for _, m := range re.FindAllStringSubmatch(html, -1) {
		sum := sha256.Sum256([]byte(m[1]))
		out = append(out, "sha256-"+base64.StdEncoding.EncodeToString(sum[:]))
	}
	return out
}
