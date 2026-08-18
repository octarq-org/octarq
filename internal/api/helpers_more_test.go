package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/octarq-org/octarq/internal/auth"
	"github.com/octarq-org/octarq/plugin"
)

func TestHelpersMore(t *testing.T) {
	h, _, _ := newTestHandlerRaw(t)

	// 1. reporterIP with trustProxy = true
	trustProxy = true
	defer func() { trustProxy = false }()

	reqXFF := httptest.NewRequest("GET", "http://example.com", nil)
	reqXFF.Header.Set("X-Forwarded-For", "203.0.113.195, 70.41.3.18, 150.172.238.178")
	if ip := reporterIP(reqXFF); ip != "203.0.113.195" {
		t.Errorf("reporterIP(XFF) = %q, want 203.0.113.195", ip)
	}

	reqXRealIP := httptest.NewRequest("GET", "http://example.com", nil)
	reqXRealIP.Header.Set("X-Real-IP", "198.51.100.1")
	if ip := reporterIP(reqXRealIP); ip != "198.51.100.1" {
		t.Errorf("reporterIP(X-Real-IP) = %q, want 198.51.100.1", ip)
	}

	// 2. splitHostPort without port
	if _, _, err := splitHostPort("localhost"); err == nil {
		t.Error("expected error for splitHostPort without port")
	}

	// 3. orgDB with 0 org
	reqNoOrg := httptest.NewRequest("GET", "http://example.com", nil)
	q := h.orgDB(reqNoOrg)
	if q == nil {
		t.Error("expected orgDB(no org) to return query")
	}

	// 4. RequireRole wrapper
	if h.RequireRole(reqNoOrg, "admin") {
		t.Error("expected RequireRole with no org to return false")
	}

	// 5. RequirePerm
	if h.RequirePerm(nil, "perm", "admin") {
		t.Error("expected RequirePerm(nil) to return false")
	}

	// Token request with role cap
	ctx := auth.WithTokenID(context.Background(), 10)
	ctx = auth.WithTokenRole(ctx, "member")
	reqToken := httptest.NewRequest("GET", "http://example.com", nil).WithContext(ctx)
	// Token capped at member asking for owner -> false
	if h.RequirePerm(reqToken, "perm", "owner") {
		t.Error("expected capped token RequirePerm to return false")
	}

	// Custom permission resolver
	plugin.SetPermResolver(func(r *http.Request, permKey string) (bool, bool) {
		if permKey == "custom.granted" {
			return true, true
		}
		if permKey == "custom.denied" {
			return false, true
		}
		return false, false
	})
	reqRegular := httptest.NewRequest("GET", "http://example.com", nil)
	if !h.RequirePerm(reqRegular, "custom.granted", "admin") {
		t.Error("expected custom.granted to return true")
	}
	if h.RequirePerm(reqRegular, "custom.denied", "member") {
		t.Error("expected custom.denied to return false")
	}

	// 6. Audit wrapper
	h.Audit(reqRegular, "test.action", "test", 1, map[string]any{"k": "v"})
}
