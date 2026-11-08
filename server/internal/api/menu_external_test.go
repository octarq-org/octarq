package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/octarq-org/octarq/server/plugin"
)

// ownFrontendPlugin declares a menu entry for a frontend it mounts itself with
// Context.HandleStatic, plus one ordinary dashboard route.
type ownFrontendPlugin struct{}

func (ownFrontendPlugin) Name() string                          { return "ownfront" }
func (ownFrontendPlugin) Models() []any                         { return nil }
func (ownFrontendPlugin) Mount(_ plugin.Mux, _ *plugin.Context) {}
func (ownFrontendPlugin) Menus() []plugin.MenuItem {
	return []plugin.MenuItem{
		{ID: "desk", Label: "Desk", Path: "/desk", Category: "System", External: true},
		{ID: "plain", Label: "Plain", Path: "/plain", Category: "System"},
	}
}

// The shell renders a menu entry as an in-app route unless the item says it is
// external; a static frontend mounted outside the dashboard SPA then lands on
// the "unavailable" page. This asserts the flag survives plugin declaration →
// /api/menus JSON.
func TestMenusCarryExternal(t *testing.T) {
	h, srv, _ := newTestHandlerWithInstance(t)
	h.SetPlugins([]plugin.Plugin{ownFrontendPlugin{}})

	cookies := loginCookies(t, srv)
	if rec := do(srv, "PUT", "/api/plugins/ownfront", cookies, `{"enabled":true}`); rec.Code != http.StatusOK {
		t.Fatalf("enable plugin: got %d (%s)", rec.Code, rec.Body.String())
	}

	rec := do(srv, "GET", "/api/menus", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list menus: got %d (%s)", rec.Code, rec.Body.String())
	}
	var menus []struct {
		ID       string `json:"id"`
		External bool   `json:"external"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &menus); err != nil {
		t.Fatalf("decode menus: %v (%s)", err, rec.Body.String())
	}

	got := map[string]bool{}
	for _, m := range menus {
		got[m.ID] = m.External
	}
	if _, ok := got["desk"]; !ok {
		t.Fatalf("desk menu missing from /api/menus: %s", rec.Body.String())
	}
	if !got["desk"] {
		t.Error("desk external = false, want true")
	}
	// A dashboard route must stay internal, or the sidebar turns it into a
	// full page load for no reason.
	if got["plain"] {
		t.Error("plain external = true, want false")
	}
}
