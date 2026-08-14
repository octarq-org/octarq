package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestInstanceBuildRequiresAuth pins that the build-info endpoint is gated by
// the dashboard session. version + commit fingerprint a self-hosted instance
// for CVE-version scanning, so an anonymous caller must never get an answer.
func TestInstanceBuildRequiresAuth(t *testing.T) {
	_, srv, _ := newTestHandlerRaw(t)

	req := httptest.NewRequest(http.MethodGet, "/api/instance/build", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET /api/instance/build: expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestInstanceBuildNotPublicPinned guards the allowlist in public_endpoints.go
// DIRECTLY — reading the product's own map, not a copy of it. If anyone ever
// adds /api/instance/build to publicExactPaths, this test goes red on the next
// run. The registry test in public_endpoints_test.go would also fail, but an
// explicit pin fails with a message that says why this route in particular
// must stay private.
func TestInstanceBuildNotPublicPinned(t *testing.T) {
	if publicExactPaths["/api/instance/build"] {
		t.Fatal("/api/instance/build must NOT be in publicExactPaths: build info " +
			"fingerprints the instance's octarq version for CVE scanning; keep it " +
			"behind the dashboard session")
	}
	if isPublicPath("/api/instance/build") {
		t.Fatal("/api/instance/build must require a session, but isPublicPath returned true")
	}
}

// TestInstanceBuildAuthenticated verifies the route serves its payload to a
// logged-in caller. The values themselves are build-time injection, so the
// test pins shape and non-empty fields, not specific strings.
func TestInstanceBuildAuthenticated(t *testing.T) {
	_, srv, _ := newTestHandlerRaw(t)

	req := httptest.NewRequest(http.MethodGet, "/api/instance/build", nil)
	for _, c := range loginCookies(t, srv) {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("authenticated GET /api/instance/build: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var res map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("invalid json output: %v", err)
	}
	for _, k := range []string{"version", "commit", "builtAt"} {
		v, ok := res[k].(string)
		if !ok || v == "" {
			t.Errorf("expected non-empty string key %q in response, got %#v", k, res[k])
		}
	}
}
