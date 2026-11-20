package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOverviewMore(t *testing.T) {
	h, srv, _ := newTestHandlerRaw(t)
	cookies := loginCookies(t, srv)

	rec := do(srv, "GET", "/api/overview?includeBot=true", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/overview?includeBot=true: got %d", rec.Code)
	}

	rec = do(srv, "GET", "/api/overview", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauth overview: got %d, want 401", rec.Code)
	}

	if _, err := h.overview(context.Background(), &OverviewInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in overview")
	}
}

func TestHostOrgMore(t *testing.T) {
	h, _, _ := newTestHandlerRaw(t)

	req := httptest.NewRequest("GET", "http://example.com", nil)
	if oid := h.brandingOrg(req); oid != 0 {
		t.Errorf("brandingOrg(unauth example.com) = %d, want 0", oid)
	}

	if oid := h.orgIDForHost(""); oid != 0 {
		t.Errorf("orgIDForHost(\"\") = %d, want 0", oid)
	}
	hNil := &Handler{db: nil}
	if oid := hNil.orgIDForHost("example.com"); oid != 0 {
		t.Errorf("orgIDForHost(nil db) = %d, want 0", oid)
	}

	c := &hostOrgCache{}
	for i := 0; i < 1030; i++ {
		c.put(string(rune(i)), uint(i))
	}
	if len(c.entries) > 1024 {
		t.Errorf("cache entries length %d exceeds 1024", len(c.entries))
	}
}
