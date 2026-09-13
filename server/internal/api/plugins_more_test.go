package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugin"
)

type depPluginA struct{}

func (depPluginA) Name() string                      { return "pluginA" }
func (depPluginA) Models() []any                     { return nil }
func (depPluginA) Init(*plugin.Context) error        { return nil }
func (depPluginA) Mount(plugin.Mux, *plugin.Context) {}
func (depPluginA) Describe() plugin.Info {
	return plugin.Info{
		Title:            "Plugin A",
		Category:         plugin.CategoryUtilities,
		EnabledByDefault: true,
	}
}

type depPluginB struct{}

func (depPluginB) Name() string                      { return "pluginB" }
func (depPluginB) Models() []any                     { return nil }
func (depPluginB) Init(*plugin.Context) error        { return nil }
func (depPluginB) Mount(plugin.Mux, *plugin.Context) {}
func (depPluginB) Describe() plugin.Info {
	return plugin.Info{
		Title:            "Plugin B",
		Category:         plugin.CategoryUtilities,
		EnabledByDefault: true,
		Requires:         []string{"pluginA"},
	}
}

func TestPluginsMore(t *testing.T) {
	h, srv, db := newTestHandlerRaw(t)
	adminCookies := loginCookies(t, srv)

	// Create member
	memberUser := models.User{Email: "plugmember@example.com"}
	db.Create(&memberUser)
	db.Create(&models.OrgMember{OrgID: 1, UserID: memberUser.ID, Role: "member"})
	memberCookies := sessionCookies(t, memberUser.ID, 1)

	// 1. List Instance Plugins
	// unauth -> 401
	rec := do(srv, "GET", "/api/instance/plugins", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauth list instance plugins: got %d, want 401", rec.Code)
	}

	// member -> 403
	rec = do(srv, "GET", "/api/instance/plugins", memberCookies, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("member list instance plugins: got %d, want 403", rec.Code)
	}

	// admin -> 200
	rec = do(srv, "GET", "/api/instance/plugins", adminCookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("admin list instance plugins: got %d (%s)", rec.Code, rec.Body.String())
	}

	// 2. updatePlugin unknown feature -> 404
	rec = do(srv, "PUT", "/api/plugins/nonexistent-feature", adminCookies, `{"enabled":true}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("update unknown plugin: got %d, want 404", rec.Code)
	}

	// 3. dependencyConflictError test
	depErr := &dependencyConflictError{
		Feature:    "dns",
		Dependents: []string{"mail", "links"},
	}
	if depErr.GetStatus() != http.StatusConflict {
		t.Errorf("depErr status = %d, want 409", depErr.GetStatus())
	}
	if depErr.Error() != "dns is required by mail, links" {
		t.Errorf("depErr error = %q", depErr.Error())
	}
	b, err := json.Marshal(depErr)
	if err != nil || len(b) == 0 {
		t.Errorf("depErr marshal error: %v", err)
	}

	// 4. Plugin with dependencies
	h.plugins = []plugin.Plugin{depPluginA{}, depPluginB{}}
	// Both A and B enabled by default
	// Disabling A when B is enabled -> 409 Conflict
	rec = do(srv, "PUT", "/api/plugins/pluginA", adminCookies, `{"enabled":false}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 conflict when disabling plugin with enabled dependents, got %d (%s)", rec.Code, rec.Body.String())
	}

	// Disable B first
	rec = do(srv, "PUT", "/api/plugins/pluginB", adminCookies, `{"enabled":false}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("disable pluginB: got %d (%s)", rec.Code, rec.Body.String())
	}
	// Now disable A -> 200 OK
	rec = do(srv, "PUT", "/api/plugins/pluginA", adminCookies, `{"enabled":false}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("disable pluginA: got %d (%s)", rec.Code, rec.Body.String())
	}

	// Enabling B when A is disabled -> cascades enabling A
	rec = do(srv, "PUT", "/api/plugins/pluginB", adminCookies, `{"enabled":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("cascade enable pluginB: got %d (%s)", rec.Code, rec.Body.String())
	}

	// 5. Direct calls with nil Ctx
	ctx := context.Background()
	if _, err := h.listPlugins(ctx, &ListPluginsInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in listPlugins")
	}
	if _, err := h.updatePlugin(ctx, &UpdatePluginInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in updatePlugin")
	}
	if _, err := h.listInstancePlugins(ctx, &ListInstancePluginsInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in listInstancePlugins")
	}
}
