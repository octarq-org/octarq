package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/octarq-org/octarq/server/plugin"
)

type roleGatedPlugin struct{}

func (roleGatedPlugin) Name() string                          { return "gated" }
func (roleGatedPlugin) Models() []any                         { return nil }
func (roleGatedPlugin) Mount(_ plugin.Mux, _ *plugin.Context) {}
func (roleGatedPlugin) Menus() []plugin.MenuItem {
	return []plugin.MenuItem{
		{ID: "gated", Label: "Gated", Path: "/gated", Category: "System", RequiredRole: "admin"},
		{ID: "open", Label: "Open", Path: "/open", Category: "System"},
	}
}

func TestMenusCarryRequiredRole(t *testing.T) {
	h, srv, _ := newTestHandlerWithInstance(t)
	h.SetPlugins([]plugin.Plugin{roleGatedPlugin{}})

	cookies := loginCookies(t, srv)
	if rec := do(srv, "PUT", "/api/plugins/gated", cookies, `{"enabled":true}`); rec.Code != http.StatusOK {
		t.Fatalf("enable plugin: got %d (%s)", rec.Code, rec.Body.String())
	}

	rec := do(srv, "GET", "/api/menus", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list menus: got %d (%s)", rec.Code, rec.Body.String())
	}
	var menus []struct {
		ID           string `json:"id"`
		RequiredRole string `json:"requiredRole"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &menus); err != nil {
		t.Fatalf("decode menus: %v (%s)", err, rec.Body.String())
	}

	got := map[string]string{}
	for _, m := range menus {
		got[m.ID] = m.RequiredRole
	}
	if _, ok := got["gated"]; !ok {
		t.Fatalf("gated menu missing from /api/menus: %s", rec.Body.String())
	}
	if got["gated"] != "admin" {
		t.Errorf("gated requiredRole = %q, want \"admin\"", got["gated"])
	}
	if got["open"] != "" {
		t.Errorf("open requiredRole = %q, want empty", got["open"])
	}
}
