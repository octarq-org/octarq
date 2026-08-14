package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// corsTestMW returns a middleware whose PublicGET recognizes the two public GET
// paths the CORS tests exercise, with origins supplied per test.
func corsTestMW(origins []string) *middleware {
	return newMiddleware(RuntimeSettings{
		PublicGET: func(path string) bool {
			return path == "/api/storefront" || path == "/api/auth/config"
		},
		CORSOrigins: func() []string { return origins },
	})
}

func corsGET(mw *middleware, path, origin string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = "10.0.0.1:1234"
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	rec := httptest.NewRecorder()
	mw.handle(rec, req, okHandler)
	return rec
}

// TestCORSWhitelistedOriginGetsHeaders: the marketing site's origin, once
// configured, gets Access-Control-Allow-Origin on public GET responses — and
// never Access-Control-Allow-Credentials.
func TestCORSWhitelistedOriginGetsHeaders(t *testing.T) {
	mw := corsTestMW([]string{"https://octarq.org"})
	rec := corsGET(mw, "/api/storefront", "https://octarq.org")
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://octarq.org" {
		t.Fatalf("whitelisted origin: want ACAO %q, got %q", "https://octarq.org", got)
	}
	if rec.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Fatal("Access-Control-Allow-Credentials must never be set on a granted response")
	}
	if v := rec.Header().Get("Vary"); !strings.Contains(v, "Origin") {
		t.Fatalf("granted response must Vary: Origin, got %q", v)
	}
}

// TestCORSDisallowedOriginGetsNothing: an origin outside the whitelist must not
// receive any CORS header, even though the endpoint is public GET.
func TestCORSDisallowedOriginGetsNothing(t *testing.T) {
	mw := corsTestMW([]string{"https://octarq.org"})
	for _, evil := range []string{
		"https://evil.example",
		"https://octarq.org.evil.com",            // contains the host but is a different origin
		"https://evil.com/?x=https://octarq.org", // origin header carrying a forged value
		"http://octarq.org",                      // scheme mismatch
		"https://octarq.org:8443",                // port mismatch
	} {
		rec := corsGET(mw, "/api/storefront", evil)
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("origin %q must not match whitelist entry https://octarq.org; got ACAO %q", evil, got)
		}
	}
}

// TestCORSEmptyWhitelistSendsNothing: with no configured origins the server
// behaves exactly as before this feature — no CORS headers at all.
func TestCORSEmptyWhitelistSendsNothing(t *testing.T) {
	mw := corsTestMW(nil)
	rec := corsGET(mw, "/api/storefront", "https://octarq.org")
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("empty whitelist must not send Access-Control-Allow-Origin")
	}
	if rec.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Fatal("empty whitelist must not send Access-Control-Allow-Credentials")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("request must pass through normally, got %d", rec.Code)
	}
}

// TestCORSNonPublicEndpointGetsNothing: only endpoints the auth gate lets
// through unauthenticated are ever granted CORS, even for a whitelisted origin.
func TestCORSNonPublicEndpointGetsNothing(t *testing.T) {
	mw := corsTestMW([]string{"https://octarq.org"})
	for _, path := range []string{"/api/auth/me", "/api/products", "/api/storefront/admin", "/admin/"} {
		rec := corsGET(mw, path, "https://octarq.org")
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("non-public path %s must not get CORS headers; got ACAO %q", path, got)
		}
	}
}

// TestCORSNoOriginNoHeaders: a same-origin or non-browser request carries no
// Origin header and needs nothing from CORS.
func TestCORSNoOriginNoHeaders(t *testing.T) {
	mw := corsTestMW([]string{"https://octarq.org"})
	rec := corsGET(mw, "/api/storefront", "")
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("request without Origin must not get CORS headers")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("request without Origin must pass through, got %d", rec.Code)
	}
}

// TestCORSPreflight covers the OPTIONS path: an approved preflight is answered
// 204 with the right headers and short-circuits before the router; anything
// else falls through unanswered so the browser blocks the follow-up.
func TestCORSPreflight(t *testing.T) {
	mw := corsTestMW([]string{"https://octarq.org"})

	// Approved preflight.
	req := httptest.NewRequest(http.MethodOptions, "/api/storefront", nil)
	req.RemoteAddr = "10.0.0.1:1"
	req.Header.Set("Origin", "https://octarq.org")
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)
	req.Header.Set("Access-Control-Request-Headers", "X-Custom")
	rec := httptest.NewRecorder()
	mw.handle(rec, req, okHandler)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("approved preflight: want 204, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://octarq.org" {
		t.Fatalf("approved preflight: ACAO = %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got != http.MethodGet {
		t.Fatalf("approved preflight: ACAM = %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "X-Custom" {
		t.Fatalf("approved preflight: ACAH = %q", got)
	}
	if rec.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Fatal("preflight must never send Access-Control-Allow-Credentials")
	}
	if rec.Body.Len() != 0 {
		t.Fatal("preflight response must carry no body")
	}

	// Preflight asking for a method other than GET is not granted — the router
	// sees it and answers 405/404 without CORS headers, so the browser blocks.
	req = httptest.NewRequest(http.MethodOptions, "/api/storefront", nil)
	req.RemoteAddr = "10.0.0.1:1"
	req.Header.Set("Origin", "https://octarq.org")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	rec = httptest.NewRecorder()
	mw.handle(rec, req, okHandler)
	if rec.Code == http.StatusNoContent {
		t.Fatal("preflight for POST must not be answered")
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("preflight for POST must not be granted CORS")
	}

	// Preflight from a disallowed origin is not granted.
	req = httptest.NewRequest(http.MethodOptions, "/api/storefront", nil)
	req.RemoteAddr = "10.0.0.1:1"
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)
	rec = httptest.NewRecorder()
	mw.handle(rec, req, okHandler)
	if rec.Code == http.StatusNoContent {
		t.Fatal("preflight from a disallowed origin must not be answered")
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("preflight from a disallowed origin must not be granted CORS")
	}

	// Preflight without Access-Control-Request-Method is not a preflight; pass
	// through to the router.
	req = httptest.NewRequest(http.MethodOptions, "/api/storefront", nil)
	req.RemoteAddr = "10.0.0.1:1"
	req.Header.Set("Origin", "https://octarq.org")
	rec = httptest.NewRecorder()
	mw.handle(rec, req, okHandler)
	if rec.Code == http.StatusNoContent {
		t.Fatal("plain OPTIONS with no requested method must not be treated as a preflight")
	}

	// Preflight to a non-public endpoint is not answered either.
	req = httptest.NewRequest(http.MethodOptions, "/api/auth/me", nil)
	req.RemoteAddr = "10.0.0.1:1"
	req.Header.Set("Origin", "https://octarq.org")
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)
	rec = httptest.NewRecorder()
	mw.handle(rec, req, okHandler)
	if rec.Code == http.StatusNoContent {
		t.Fatal("preflight to a non-public endpoint must not be answered")
	}
}

// TestCORSDisabledWithoutPublicGET: if no PublicGET predicate is wired (no API
// registration), CORS never engages even with a whitelist configured.
func TestCORSDisabledWithoutPublicGET(t *testing.T) {
	mw := newMiddleware(RuntimeSettings{CORSOrigins: func() []string { return []string{"https://octarq.org"} }})
	rec := corsGET(mw, "/api/storefront", "https://octarq.org")
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("CORS must be inert when no PublicGET predicate is wired")
	}
}
