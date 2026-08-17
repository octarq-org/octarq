package api

// Scope-boundary guards between the two configuration scopes:
//
//   - tenant (org): one copy per workspace; served by the /admin shell; the
//     backend publishes it through GET /api/menus (internal/api/tenant_menu.go:709)
//     with per-row org scoping.
//   - instance (deployment): one copy for the whole install; served by the
//     /instance console shell (web/src/pages/instance/console.tsx), gated by
//     h.isInstanceAdmin (internal/api/settings.go:438).
//
// These tests lock the boundary from the outside: the tenant menu surface must
// never point into instance territory, and the largest org role (owner) must
// still be refused on the instance-settings endpoints — "org 里最大的角色说明
// 不了实例权限" is the contract written in plugin/plugin.go:251-252, and until
// now nothing pinned the owner rung (only the member rung was covered, in
// instance_readiness_test.go:248).

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/plugin"
)

// tenantMenuPlugin announces ordinary tenant-scoped sidebar entries, the shape
// every feature plugin (dns, mail, links) uses. The boundary under test: none
// of these may ever point into the /instance console or at a stale
// /settings/instance path — instance navigation lives on the console shell's
// own rail (web/src/pages/instance/console.tsx), never in the tenant /api/menus.
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

// TestTenantMenusNeverExposeInstancePaths asserts the tenant menu surface stays
// tenant-scoped: every path served by GET /api/menus must sit under the /admin
// shell's basename — never under the /instance console prefix, and free of the
// old "/settings/instance" tenant-shell instance-settings remnants.
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

// TestInstanceSettingsRefuseOrgOwner locks the org-owner rung of the
// instance-settings gate. The contract (plugin/plugin.go:251-252) is that an
// org role — including the most powerful one, owner — says nothing about
// instance privilege: every self-serve signup is owner of their own org. The
// member rung is already covered (instance_readiness_test.go:248); the owner
// rung was not.
func TestInstanceSettingsRefuseOrgOwner(t *testing.T) {
	_, srv, db := newTestHandlerWithInstance(t)

	const org = uint(904)
	ownerUID := seedOrgMember(t, db, org, "owner@x.com", "owner")
	owner := sessionCookies(t, ownerUID, org)

	// GET: an org owner must not read deployment-wide settings.
	if rec := do(srv, "GET", "/api/instance-settings", owner, ""); rec.Code != http.StatusForbidden {
		t.Errorf("org owner GET /api/instance-settings: got %d, want 403", rec.Code)
	}

	// PUT: an org owner must not mutate deployment-wide settings.
	if rec := do(srv, "PUT", "/api/instance-settings", owner, `{"appName":"pwned"}`); rec.Code != http.StatusForbidden {
		t.Errorf("org owner PUT /api/instance-settings: got %d, want 403", rec.Code)
	}

	// The configured instance admin still can: the gate is bound to the
	// instance-admin flag, not to org membership or role.
	if rec := do(srv, "GET", "/api/instance-settings", loginCookies(t, srv), ""); rec.Code != http.StatusOK {
		t.Errorf("instance admin GET /api/instance-settings: got %d, want 200", rec.Code)
	}
}
