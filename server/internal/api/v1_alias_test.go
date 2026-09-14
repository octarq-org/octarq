package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestV1AliasReachesCoreRoute pins the published /api/v1/... alias: the docs and
// CHANGELOG hand out that form, so it must reach the same handler as /api/...
// rather than 404.
func TestV1AliasReachesCoreRoute(t *testing.T) {
	srv, _ := newTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/health: got %d body %s, want 200", rec.Code, rec.Body.String())
	}

	// Body, not just the status: an alias handler that swallows the request
	// without forwarding it still leaves the recorder at a default 200.
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("GET /api/v1/health: undecodable body %q: %v", rec.Body.String(), err)
	}
	if resp["status"] != "healthy" {
		t.Errorf("GET /api/v1/health: status = %v, want healthy", resp["status"])
	}
}

// TestV1AliasReachesPluginRoute covers the routes that actually broke: plugins
// mount onto the mux *after* Routes() returns, and the billing/mail webhook URLs
// the product hands to operators are plugin routes in the v1 form. A GET on a
// POST-only path is 405 when the route exists and 404 when it does not.
func TestV1AliasReachesPluginRoute(t *testing.T) {
	srv, _ := newTestHandler(t)

	const path = "/webhook/acme/email/inbound/tok"

	req := httptest.NewRequest(http.MethodGet, "/api"+path, nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code == http.StatusNotFound {
		t.Fatalf("precondition: GET /api%s is 404, the unprefixed route is gone", path)
	}
	want := rec.Code

	req = httptest.NewRequest(http.MethodGet, "/api/v1"+path, nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != want {
		t.Fatalf("GET /api/v1%s: got %d, want %d (same as unprefixed)", path, rec.Code, want)
	}
}
