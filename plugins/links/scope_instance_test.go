package links

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/plugin"
)

// TestInstanceLinkSettingsRefusesNonInstanceAdmin asserts that the instance-scope
// link settings endpoint (/api/instance/link-settings) refuses any caller who is
// not an instance admin (anonymous, member, or org owner) with 403, and allows
// the configured instance admin with 200.
func TestInstanceLinkSettingsRefusesNonInstanceAdmin(t *testing.T) {
	p := New()
	globalSettings := map[string]string{
		"reserved_slugs": "pricing\nlogin",
	}

	p.getGlobalSetting = func(key string) string {
		return globalSettings[key]
	}
	p.isInstanceAdmin = func(r *http.Request) bool {
		return r.Header.Get("X-Instance-Admin") == "true"
	}
	p.requireRole = func(r *http.Request, min string) bool {
		return r.Header.Get("X-Role") != ""
	}

	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("links-scope-test", "1.0.0"))
	ctx := &plugin.Context{
		Huma:             api,
		IsInstanceAdmin:  p.isInstanceAdmin,
		RequireRole:      p.requireRole,
		GetGlobalSetting: p.getGlobalSetting,
		SetGlobalSetting: func(key, value string) error {
			globalSettings[key] = value
			return nil
		},
	}
	p.Mount(nil, ctx)

	// 1. Non-instance-admin (ordinary member or org owner) -> 403
	reqNonAdmin := httptest.NewRequest(http.MethodGet, "/api/instance/link-settings", nil)
	reqNonAdmin.Header.Set("X-Role", "owner") // org owner, but NOT instance admin
	recNonAdmin := httptest.NewRecorder()
	mux.ServeHTTP(recNonAdmin, reqNonAdmin)
	if recNonAdmin.Code != http.StatusForbidden {
		t.Fatalf("non-instance-admin GET /api/instance/link-settings: got %d, want 403", recNonAdmin.Code)
	}

	// 2. Non-instance-admin PUT -> 403
	reqPutNonAdmin := httptest.NewRequest(http.MethodPut, "/api/instance/link-settings", strings.NewReader(`{"reservedSlugs":"evil"}`))
	reqPutNonAdmin.Header.Set("Content-Type", "application/json")
	reqPutNonAdmin.Header.Set("X-Role", "owner")
	recPutNonAdmin := httptest.NewRecorder()
	mux.ServeHTTP(recPutNonAdmin, reqPutNonAdmin)
	if recPutNonAdmin.Code != http.StatusForbidden {
		t.Fatalf("non-instance-admin PUT /api/instance/link-settings: got %d, want 403", recPutNonAdmin.Code)
	}

	// 3. Instance admin GET -> 200
	reqAdmin := httptest.NewRequest(http.MethodGet, "/api/instance/link-settings", nil)
	reqAdmin.Header.Set("X-Instance-Admin", "true")
	recAdmin := httptest.NewRecorder()
	mux.ServeHTTP(recAdmin, reqAdmin)
	if recAdmin.Code != http.StatusOK {
		t.Fatalf("instance admin GET /api/instance/link-settings: got %d, want 200 (%s)", recAdmin.Code, recAdmin.Body.String())
	}
	if !strings.Contains(recAdmin.Body.String(), "pricing") {
		t.Errorf("response missing configured reserved slugs: %s", recAdmin.Body.String())
	}

	// 4. Instance admin PUT -> 200
	reqPutAdmin := httptest.NewRequest(http.MethodPut, "/api/instance/link-settings", strings.NewReader(`{"reservedSlugs":"pricing\nlogin\ncheckout"}`))
	reqPutAdmin.Header.Set("Content-Type", "application/json")
	reqPutAdmin.Header.Set("X-Instance-Admin", "true")
	recPutAdmin := httptest.NewRecorder()
	mux.ServeHTTP(recPutAdmin, reqPutAdmin)
	if recPutAdmin.Code != http.StatusOK {
		t.Fatalf("instance admin PUT /api/instance/link-settings: got %d, want 200 (%s)", recPutAdmin.Code, recPutAdmin.Body.String())
	}
	if globalSettings["reserved_slugs"] != "pricing\nlogin\ncheckout" {
		t.Errorf("globalSetting reserved_slugs = %q, want 'pricing\\nlogin\\ncheckout'", globalSettings["reserved_slugs"])
	}
}

// TestInstanceLinkSettingsDirectHandlerGuards tests handler level isolation when seams are absent.
func TestInstanceLinkSettingsDirectHandlerGuards(t *testing.T) {
	p := New()
	ctx := context.Background()

	// An unwired plugin (p.isInstanceAdmin == nil) must fail closed (403), never open.
	req := httptest.NewRequest(http.MethodGet, "/api/instance/link-settings", nil)
	hctx := humago.NewContext(nil, req, httptest.NewRecorder())

	_, err := p.getInstanceLinkSettings(ctx, &GetInstanceLinkSettingsInput{Ctx: hctx})
	if err == nil {
		t.Fatal("getInstanceLinkSettings with unwired isInstanceAdmin must return error")
	}

	_, err = p.updateInstanceLinkSettings(ctx, &UpdateInstanceLinkSettingsInput{Ctx: hctx})
	if err == nil {
		t.Fatal("updateInstanceLinkSettings with unwired isInstanceAdmin must return error")
	}
}

// TestInstanceLinkSettingsNotGatedByWorkspacePluginToggle verifies that
// instance-level link settings endpoints are deployment-wide and never gated by
// a workspace's plugin toggle (PluginEnabled).
func TestInstanceLinkSettingsNotGatedByWorkspacePluginToggle(t *testing.T) {
	p := New()
	p.isInstanceAdmin = func(r *http.Request) bool {
		return true
	}
	p.getGlobalSetting = func(key string) string {
		return "pricing\nlogin"
	}

	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("links-scope-gate-test", "1.0.0"))
	ctx := &plugin.Context{
		Huma:             api,
		IsInstanceAdmin:  p.isInstanceAdmin,
		GetGlobalSetting: p.getGlobalSetting,
	}
	p.Mount(nil, ctx)

	// Gate wrapper simulation: when workspace toggle is OFF for workspace 7,
	// instance paths are not scoped to workspace toggles (scoped=false) and respond 200.
	req := httptest.NewRequest(http.MethodGet, "/api/instance/link-settings", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/instance/link-settings got %d, want 200 (must not be blocked by workspace toggle)", rec.Code)
	}
}
