package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInstanceBuildRequiresAuth(t *testing.T) {
	_, srv, _ := newTestHandlerRaw(t)

	req := httptest.NewRequest(http.MethodGet, "/api/instance/build", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET /api/instance/build: expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

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

func TestInstanceBuildRequiresInstanceAdmin(t *testing.T) {
	_, srv, db := newTestHandlerRaw(t)

	const org = uint(101)
	memberUID := seedOrgMember(t, db, org, "member@x.com", "member")
	req := httptest.NewRequest(http.MethodGet, "/api/instance/build", nil)
	for _, c := range sessionCookies(t, memberUID, org) {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("member GET /api/instance/build: expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

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
