package server

// The inbound-mail webhook carries the org's secret in the path, on purpose:
// SendGrid Inbound Parse and Mailgun routes let you configure a URL and nothing
// else, so there is no header to put it in (plugins/mail/webhook.go argues the
// case). What was NOT on purpose is that the edge access log wrote that path
// verbatim, so every tenant's inbound-mail credential sat in plaintext in the
// logs and in whatever aggregator collects them.

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRedactPathSecrets(t *testing.T) {
	cases := []struct{ in, want string }{
		// The three real inbound-mail routes: token is always the last segment.
		{"/api/webhook/acme/email/inbound/s3cr3t-token", "/api/webhook/acme/email/inbound/[redacted]"},
		{"/api/webhook/acme/email/inbound/raw/s3cr3t-token", "/api/webhook/acme/email/inbound/raw/[redacted]"},
		{"/api/webhook/acme/email/bounce/s3cr3t-token", "/api/webhook/acme/email/bounce/[redacted]"},
		// The /api/v1 alias reaches this middleware unrewritten.
		{"/api/v1/webhook/acme/email/inbound/s3cr3t-token", "/api/v1/webhook/acme/email/inbound/[redacted]"},
		// Everything else is logged as-is: over-redacting would blind the log.
		{"/api/auth/login", "/api/auth/login"},
		{"/api/links/abc123", "/api/links/abc123"},
		{"/", "/"},
		// Degenerate webhook paths must not panic or mangle.
		{"/api/webhook/", "/api/webhook/"},
	}
	for _, c := range cases {
		if got := redactPathSecrets(c.in); got != c.want {
			t.Errorf("redactPathSecrets(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestAccessLogNeverCarriesTheInboundToken drives the real handle() path and
// reads what actually landed in the log.
func TestAccessLogNeverCarriesTheInboundToken(t *testing.T) {
	const token = "b6f1c0de-secret-inbound-token"

	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	mw := newMiddleware(RuntimeSettings{})
	req := httptest.NewRequest("POST", "/api/webhook/acme/email/inbound/"+token, nil)
	req.RemoteAddr = "10.0.0.9:5555"
	mw.handle(httptest.NewRecorder(), req, okHandler)

	if logged := buf.String(); strings.Contains(logged, token) {
		t.Fatalf("access log carries the inbound-mail secret in plaintext:\n%s", logged)
	}
	if logged := buf.String(); !strings.Contains(logged, "/api/webhook/acme/email/inbound/") {
		t.Fatalf("redaction ate the whole path, leaving nothing to debug with:\n%s", logged)
	}
}

// TestAccessLogStillRecordsOrdinaryPaths keeps the redaction honest: blanking
// every path would pass the test above and destroy the access log.
func TestAccessLogStillRecordsOrdinaryPaths(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	mw := newMiddleware(RuntimeSettings{})
	req := httptest.NewRequest("GET", "/api/links/abc123", nil)
	req.RemoteAddr = "10.0.0.9:5555"
	mw.handle(httptest.NewRecorder(), req, okHandler)

	if logged := buf.String(); !strings.Contains(logged, "/api/links/abc123") {
		t.Fatalf("ordinary path missing from the access log:\n%s", logged)
	}
}

// --- rate-limit communication ---

// TestRateLimitHeadersOnAllowedRequest: a limit nobody can see is a limit every
// integrator discovers by tripping it.
func TestRateLimitHeadersOnAllowedRequest(t *testing.T) {
	mw := &middleware{
		limiter: &rateLimiter{
			window:   time.Minute,
			limits:   map[tier]int{tierAuth: 5},
			counters: make(map[string]*rlCounter),
		},
		metrics: newMetrics(),
	}

	fire := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/api/auth/login", nil)
		req.RemoteAddr = "10.0.0.2:1234"
		rec := httptest.NewRecorder()
		mw.handle(rec, req, okHandler)
		return rec
	}

	rec := fire()
	if got := rec.Header().Get("X-RateLimit-Limit"); got != "5" {
		t.Fatalf("X-RateLimit-Limit = %q, want \"5\"", got)
	}
	if got := rec.Header().Get("X-RateLimit-Remaining"); got != "4" {
		t.Fatalf("X-RateLimit-Remaining after 1 of 5 = %q, want \"4\"", got)
	}
	reset, err := strconv.ParseInt(rec.Header().Get("X-RateLimit-Reset"), 10, 64)
	if err != nil {
		t.Fatalf("X-RateLimit-Reset = %q, want a unix timestamp: %v", rec.Header().Get("X-RateLimit-Reset"), err)
	}
	if d := time.Until(time.Unix(reset, 0)); d <= 0 || d > 2*time.Minute {
		t.Fatalf("X-RateLimit-Reset is %v away, want within the current window", d)
	}

	// The counter must actually move — a hardcoded header would be worse than none.
	rec2 := fire()
	if got := rec2.Header().Get("X-RateLimit-Remaining"); got != "3" {
		t.Fatalf("X-RateLimit-Remaining after 2 of 5 = %q, want \"3\"", got)
	}
}

// TestRateLimit429IsJSONWithHeaders pins the refusal shape: same RFC 7807 body
// every other API error uses, plus the budget headers and Retry-After.
func TestRateLimit429IsJSONWithHeaders(t *testing.T) {
	mw := &middleware{
		limiter: &rateLimiter{
			window:   time.Minute,
			limits:   map[tier]int{tierAuth: 1},
			counters: make(map[string]*rlCounter),
		},
		metrics: newMetrics(),
	}

	fire := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/api/auth/login", nil)
		req.RemoteAddr = "10.0.0.3:1234"
		rec := httptest.NewRecorder()
		mw.handle(rec, req, okHandler)
		return rec
	}

	fire()
	rec := fire()
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("2nd request: got %d, want 429", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "json") {
		t.Fatalf("429 Content-Type = %q, want a JSON media type", ct)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("429 body is not JSON (%v): %s", err, rec.Body.String())
	}
	if status, _ := body["status"].(float64); int(status) != http.StatusTooManyRequests {
		t.Fatalf("429 body status = %v, want 429: %s", body["status"], rec.Body.String())
	}
	if title, _ := body["title"].(string); title == "" {
		t.Fatalf("429 body has no title: %s", rec.Body.String())
	}
	if got := rec.Header().Get("Retry-After"); got == "" {
		t.Fatal("429 without Retry-After")
	}
	if got := rec.Header().Get("X-RateLimit-Remaining"); got != "0" {
		t.Fatalf("X-RateLimit-Remaining on a 429 = %q, want \"0\"", got)
	}
	if got := rec.Header().Get("X-RateLimit-Limit"); got != "1" {
		t.Fatalf("X-RateLimit-Limit on a 429 = %q, want \"1\"", got)
	}
}

// TestNoRateLimitHeadersWhenTierDisabled: a disabled tier has no number to
// report, so it must report none rather than "0", which reads as "exhausted".
func TestNoRateLimitHeadersWhenTierDisabled(t *testing.T) {
	mw := &middleware{
		limiter: &rateLimiter{
			window:   time.Minute,
			limits:   map[tier]int{tierAuth: 0},
			counters: make(map[string]*rlCounter),
		},
		metrics: newMetrics(),
	}
	req := httptest.NewRequest("POST", "/api/auth/login", nil)
	req.RemoteAddr = "10.0.0.4:1234"
	rec := httptest.NewRecorder()
	mw.handle(rec, req, okHandler)

	if rec.Code != http.StatusOK {
		t.Fatalf("disabled tier: got %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("X-RateLimit-Limit"); got != "" {
		t.Fatalf("disabled tier still advertised X-RateLimit-Limit = %q", got)
	}
}
