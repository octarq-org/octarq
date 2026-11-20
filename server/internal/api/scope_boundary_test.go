package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/plugin"
)

type tenantMenuPlugin struct{}

func (tenantMenuPlugin) Name() string                          { return "tenantmenu" }
func (tenantMenuPlugin) Models() []any                         { return nil }
func (tenantMenuPlugin) Mount(_ plugin.Mux, _ *plugin.Context) {}
func (tenantMenuPlugin) Menus() []plugin.MenuItem {
	return []plugin.MenuItem{
		{ID: "domains", Label: "DNS", Path: "/domains", Icon: "globe", Category: "Network"},
		{ID: "mail", Label: "Mail", Path: "/mail", Icon: "mail", Category: "Messaging"},
		{ID: "links", Label: "Links", Path: "/links", Icon: "link-2", Category: "Marketing"},
	}
}

func TestTenantMenusNeverExposeInstancePaths(t *testing.T) {
	h, srv, _ := newTestHandlerWithInstance(t)
	h.SetPlugins([]plugin.Plugin{tenantMenuPlugin{}})

	cookies := loginCookies(t, srv)
	if rec := do(srv, "PUT", "/api/plugins/tenantmenu", cookies, `{"enabled":true}`); rec.Code != http.StatusOK {
		t.Fatalf("enable tenantmenu plugin: got %d (%s)", rec.Code, rec.Body.String())
	}

	rec := do(srv, "GET", "/api/menus", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list menus: got %d (%s)", rec.Code, rec.Body.String())
	}
	var menus []struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &menus); err != nil {
		t.Fatalf("decode menus: %v (%s)", err, rec.Body.String())
	}
	if len(menus) == 0 {
		t.Fatal("menus is empty; the assertion below would be vacuous")
	}

	for _, m := range menus {
		if m.Path == "/instance" || strings.HasPrefix(m.Path, "/instance/") {
			t.Errorf("tenant menu exposes instance-console path %q: instance navigation belongs to the /instance shell, not /api/menus", m.Path)
		}
		if m.Path == "/settings/instance" || strings.HasPrefix(m.Path, "/settings/instance/") {
			t.Errorf("tenant menu carries stale /settings/instance remnant %q", m.Path)
		}
	}
}

func TestInstanceSettingsRefuseOrgOwner(t *testing.T) {
	_, srv, db := newTestHandlerRaw(t)

	const org = uint(904)
	ownerUID := seedOrgMember(t, db, org, "owner@x.com", "owner")
	owner := sessionCookies(t, ownerUID, org)

	if rec := do(srv, "GET", "/api/instance-settings", owner, ""); rec.Code != http.StatusForbidden {
		t.Errorf("org owner GET /api/instance-settings: got %d, want 403", rec.Code)
	}

	if rec := do(srv, "PUT", "/api/instance-settings", owner, `{"appName":"pwned"}`); rec.Code != http.StatusForbidden {
		t.Errorf("org owner PUT /api/instance-settings: got %d, want 403", rec.Code)
	}

	if rec := do(srv, "GET", "/api/instance-settings", loginCookies(t, srv), ""); rec.Code != http.StatusOK {
		t.Errorf("instance admin GET /api/instance-settings: got %d, want 200", rec.Code)
	}
}
