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

	"github.com/octarq-org/octarq/server/pkg/telemetry"
	"github.com/octarq-org/octarq/server/webembed"
)

func okHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

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

	if ok, _ := rl.allow(tierAuth, "5.6.7.8", now); !ok {
		t.Fatal("different IP should be allowed")
	}

	later := now.Add(time.Minute + time.Second)
	if ok, _ := rl.allow(tierAuth, "1.2.3.4", later); !ok {
		t.Fatal("request after window reset should be allowed")
	}
}

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

func TestMetricsGating(t *testing.T) {
	mwNoToken := &middleware{
		limiter: newRateLimiter(),
		metrics: newMetrics(),
	}
	for _, remote := range []string{"8.8.8.8:9999", "127.0.0.1:5555"} {
		req := httptest.NewRequest("GET", "/metrics", nil)
		req.RemoteAddr = remote
		rec := httptest.NewRecorder()
		mwNoToken.handle(rec, req, okHandler)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("/metrics from %s without a configured token: want 404 got %d", remote, rec.Code)
		}
	}

	mwToken := newMiddleware(RuntimeSettings{MetricsToken: func() string { return "s3cret" }})
	for _, remote := range []string{"8.8.8.8:1", "127.0.0.1:1"} {
		reqBad := httptest.NewRequest("GET", "/metrics", nil)
		reqBad.RemoteAddr = remote
		recBad := httptest.NewRecorder()
		mwToken.handle(recBad, reqBad, okHandler)
		if recBad.Code != http.StatusForbidden {
			t.Fatalf("/metrics from %s without the token: want 403 got %d", remote, recBad.Code)
		}
	}

	reqGood := httptest.NewRequest("GET", "/metrics", nil)
	reqGood.RemoteAddr = "8.8.8.8:1"
	reqGood.Header.Set("Authorization", "Bearer s3cret")
	recGood := httptest.NewRecorder()
	mwToken.handle(recGood, reqGood, okHandler)
	if recGood.Code != http.StatusOK {
		t.Fatalf("correct token /metrics: want 200 got %d", recGood.Code)
	}
	if ct := recGood.Header().Get("Content-Type"); ct == "" {
		t.Fatal("expected a content type on metrics")
	}
}

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

func TestClientIP(t *testing.T) {
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

	trustProxy = false
	req4 := httptest.NewRequest("GET", "/", nil)
	req4.RemoteAddr = "192.0.2.5:5555"
	req4.Header.Set("X-Forwarded-For", "203.0.113.7")
	if got := clientIP(req4); got != "192.0.2.5" {
		t.Fatalf("untrusted XFF should be ignored: got %q", got)
	}
}

func TestTierFor(t *testing.T) {
	cases := []struct {
		method, path string
		want         tier
	}{
		{"POST", "/api/auth/login", tierAuth},
		{"POST", "/api/webhook/x", tierAuth},
		{"GET", "/api/links", tierAPI},
		{"GET", "/v1/models", tierAPI},
		{"GET", "/.well-known/opencode", tierAPI},
		{"POST", "/v1/chat/completions", tierAPI},
		{"POST", "/v1/responses", tierAPI},
		{"GET", "/v1beta/models", tierAPI},
		{"POST", "/v1beta/models/gemini-2.5-pro:streamGenerateContent", tierAPI},
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

func TestTierForAuthRoutesKeepsStrictTier(t *testing.T) {
	cases := []struct {
		method, path string
		want         tier
	}{
		{"POST", "/api/auth/login", tierAuth},
		{"POST", "/api/webhook/x", tierAuth},
		{"GET", "/api/links", tierAPI},
	}
	for _, c := range cases {
		got := tierFor(httptest.NewRequest(c.method, c.path, nil))
		if got != c.want {
			t.Errorf("tierFor(%s %s) = %d, want %d", c.method, c.path, got, c.want)
		}
	}
}

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
	if !strings.Contains(scriptSrc, "'wasm-unsafe-eval'") {
		t.Errorf("script-src must keep 'wasm-unsafe-eval' for WebAssembly apps, got %q", scriptSrc)
	}
	if strings.Contains(scriptSrc, "'unsafe-eval'") {
		t.Errorf("script-src must not contain 'unsafe-eval', got %q", scriptSrc)
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

func cspDirective(csp, name string) string {
	for _, d := range strings.Split(csp, ";") {
		d = strings.TrimSpace(d)
		if strings.HasPrefix(d, name+" ") {
			return strings.TrimPrefix(d, name+" ")
		}
	}
	return ""
}

func inlineScriptHashes(html string) []string {
	re := regexp.MustCompile(`(?s)<script>([\s\S]*?)</script>`)
	var out []string
	for _, m := range re.FindAllStringSubmatch(html, -1) {
		sum := sha256.Sum256([]byte(m[1]))
		out = append(out, "sha256-"+base64.StdEncoding.EncodeToString(sum[:]))
	}
	return out
}
