package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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

func TestCORSDisallowedOriginGetsNothing(t *testing.T) {
	mw := corsTestMW([]string{"https://octarq.org"})
	for _, evil := range []string{
		"https://evil.example",
		"https://octarq.org.evil.com",
		"https://evil.com/?x=https://octarq.org",
		"http://octarq.org",
		"https://octarq.org:8443",
	} {
		rec := corsGET(mw, "/api/storefront", evil)
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("origin %q must not match whitelist entry https://octarq.org; got ACAO %q", evil, got)
		}
	}
}

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

func TestCORSNonPublicEndpointGetsNothing(t *testing.T) {
	mw := corsTestMW([]string{"https://octarq.org"})
	for _, path := range []string{"/api/auth/me", "/api/products", "/api/storefront/admin", "/admin/"} {
		rec := corsGET(mw, path, "https://octarq.org")
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("non-public path %s must not get CORS headers; got ACAO %q", path, got)
		}
	}
}

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

func TestCORSPreflight(t *testing.T) {
	mw := corsTestMW([]string{"https://octarq.org"})

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

	req = httptest.NewRequest(http.MethodOptions, "/api/storefront", nil)
	req.RemoteAddr = "10.0.0.1:1"
	req.Header.Set("Origin", "https://octarq.org")
	rec = httptest.NewRecorder()
	mw.handle(rec, req, okHandler)
	if rec.Code == http.StatusNoContent {
		t.Fatal("plain OPTIONS with no requested method must not be treated as a preflight")
	}

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

func TestCORSDisabledWithoutPublicGET(t *testing.T) {
	mw := newMiddleware(RuntimeSettings{CORSOrigins: func() []string { return []string{"https://octarq.org"} }})
	rec := corsGET(mw, "/api/storefront", "https://octarq.org")
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("CORS must be inert when no PublicGET predicate is wired")
	}
}
