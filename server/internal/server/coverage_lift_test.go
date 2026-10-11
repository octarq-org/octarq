package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/octarq-org/octarq/server/config"
)

func TestCoverageLiftNewDevWebProxyErrors(t *testing.T) {
	db := domainsDB(t)

	// 1. Invalid DevWebProxy URL
	badCfg := &config.Config{DevWebProxy: "://invalid-url"}
	_, err := New(badCfg, db, mockAPI{}, nil, nil, nil, RuntimeSettings{})
	if err == nil || !strings.Contains(err.Error(), "invalid OCTARQ_DEV_WEB_PROXY URL") {
		t.Fatalf("expected invalid URL error, got %v", err)
	}

	// 2. webFS is nil without dev proxy
	_, err = New(&config.Config{}, db, mockAPI{}, nil, nil, nil, RuntimeSettings{})
	if err == nil || !strings.Contains(err.Error(), "webFS is nil") {
		t.Fatalf("expected webFS is nil error, got %v", err)
	}

	// 3. Dev proxy configured with webFS present (exercises reading index.html when devProxy != nil)
	// and triggering devProxy.ErrorHandler on unreachable backend
	goodProxyCfg := &config.Config{DevWebProxy: "http://127.0.0.1:54321"}
	webFS := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html>index</html>")},
	}
	s, err := New(goodProxyCfg, db, mockAPI{}, nil, webFS, nil, RuntimeSettings{})
	if err != nil {
		t.Fatalf("New with devProxy failed: %v", err)
	}

	req := httptest.NewRequest("GET", "/admin/test", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 Bad Gateway from dev proxy, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "OCTARQ_DEV_WEB_PROXY failed to connect") {
		t.Fatalf("unexpected error response body: %s", rec.Body.String())
	}
}

func TestCoverageLiftSpecPathsAndInstance(t *testing.T) {
	db := domainsDB(t, testDomain{Name: "short.ly", ForLink: true})
	webFS := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html>dashboard</html>")},
	}
	s, err := New(&config.Config{}, db, mockAPI{}, nil, webFS, nil, RuntimeSettings{})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	// 1. Spec paths
	specPathsToTest := []string{"/openapi.json", "/openapi.yaml", "/docs", "/schemas/item.json"}
	for _, p := range specPathsToTest {
		req := httptest.NewRequest("GET", p, nil)
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || rec.Body.String() != "api response" {
			t.Fatalf("expected api response for %s, got code %d body %q", p, rec.Code, rec.Body.String())
		}
	}

	// 2. /instance on forbidden host
	req := httptest.NewRequest("GET", "/instance", nil)
	req.Host = "short.ly"
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for /instance on short.ly, got %d", rec.Code)
	}

	// 3. /instance on allowed host
	req = httptest.NewRequest("GET", "/instance", nil)
	req.Host = "admin.example.com"
	rec = httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<html>dashboard</html>") {
		t.Fatalf("expected 200 for /instance on admin host, got %d", rec.Code)
	}
}

func TestCoverageLiftRootFallbackAndAssets(t *testing.T) {
	db := domainsDB(t)
	webFS := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html>dashboard</html>")},
	}
	fallbackCalled := false
	fallback := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallbackCalled = true
		w.WriteHeader(http.StatusTeapot)
	})

	s, err := New(&config.Config{}, db, mockAPI{}, fallback, webFS, nil, RuntimeSettings{})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	req := httptest.NewRequest("GET", "/custom-unrouted-slug", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if !fallbackCalled || rec.Code != http.StatusTeapot {
		t.Fatalf("expected fallback handler to be called with 418, got code %d", rec.Code)
	}

	// AssetExists with nil assets
	s.assets = nil
	if s.assetExists("file.txt") {
		t.Fatalf("expected assetExists to return false for nil assets")
	}
}

func TestCoverageLiftMiddlewareBranches(t *testing.T) {
	// 1. clientIP with no port in RemoteAddr
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.100"
	if ip := clientIP(req); ip != "192.168.1.100" {
		t.Fatalf("expected raw RemoteAddr, got %q", ip)
	}

	// 2. Rate limiter budget with unseen IP and over-limit IP
	rl := newRateLimiter()
	now := time.Now()
	limit, rem, _ := rl.budget(tierAPI, "1.2.3.4", now)
	if limit <= 0 || rem != limit {
		t.Fatalf("expected fresh budget, got limit=%d rem=%d", limit, rem)
	}

	// Consume more than limit
	rl.counters["1|1.2.3.4"] = &rlCounter{
		count:   limit + 5,
		resetAt: now.Add(time.Minute),
	}
	_, rem, _ = rl.budget(tierAPI, "1.2.3.4", now)
	if rem != 0 {
		t.Fatalf("expected rem=0 when over limit, got %d", rem)
	}

	// 3. refreshConfig with custom RateLimits function
	mw := newMiddleware(RuntimeSettings{
		RateLimits: func() (authRPM, apiRPM, redirectRPM int) {
			return 10, 20, 30
		},
		CORSOrigins: func() []string {
			return []string{"https://cors.example.com"}
		},
	})
	mw.refreshConfig(now.Add(settingsRefreshInterval + time.Hour))
	if mw.limiter.limits[tierAPI] != 20 {
		t.Fatalf("expected updated rate limit 20, got %d", mw.limiter.limits[tierAPI])
	}

	// 4. applyCORS with non-GET method
	rec := httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/data", nil)
	req.Header.Set("Origin", "https://cors.example.com")
	if mw.applyCORS(rec, req) {
		t.Fatalf("applyCORS should return false for POST")
	}

	// 5. setSecurityHeaders with X-Forwarded-Proto: https
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	setSecurityHeaders(rec, req)
	if h := rec.Header().Get("Strict-Transport-Security"); h == "" {
		t.Fatalf("expected Strict-Transport-Security header to be set")
	}

	// 6. Error status (>=500) sets span error status
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/error-endpoint", nil)
	mw.handle(rec, req, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}
