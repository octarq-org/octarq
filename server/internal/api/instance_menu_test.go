package api

// Guard tests for GET /api/instance/menus — the instance-console analog of
// /api/menus. Case 4 (scope isolation) is the core guard of the seam: a plugin
// must never be able to leak one scope's pages into the other shell.

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/octarq-org/octarq/plugin"
)

// dualScopePlugin implements both MenuProvider and InstanceMenuProvider so a
// single test plugin can exercise both shells at once. EnabledByDefault keeps
// it live for the caller's workspace (the tenant side is per-org gated).
type dualScopePlugin struct {
	name          string
	tenantMenus   []plugin.MenuItem
	instanceMenus []plugin.MenuItem
}

func (d dualScopePlugin) Name() string                          { return d.name }
func (d dualScopePlugin) Models() []any                         { return nil }
func (d dualScopePlugin) Mount(_ plugin.Mux, _ *plugin.Context) {}
func (d dualScopePlugin) Describe() plugin.Info {
	return plugin.Info{Title: d.name, EnabledByDefault: true}
}
func (d dualScopePlugin) Menus() []plugin.MenuItem         { return d.tenantMenus }
func (d dualScopePlugin) InstanceMenus() []plugin.MenuItem { return d.instanceMenus }

// TestInstanceMenusRejectsAnonymous keeps the endpoint behind auth entirely —
// 401 before the instance-admin check, so it can't be probed unauthenticated.
func TestInstanceMenusRejectsAnonymous(t *testing.T) {
	h, srv, _ := newTestHandlerWithInstance(t)
	h.SetPlugins([]plugin.Plugin{dualScopePlugin{name: "hello"}})

	if rec := do(srv, "GET", "/api/instance/menus", nil, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: got %d, want 401", rec.Code)
	}
}

// TestInstanceMenusRequiresInstanceAdmin: an ordinary org owner must not read
// the instance menu list — it reveals which deployment-wide surfaces exist.
func TestInstanceMenusRequiresInstanceAdmin(t *testing.T) {
	h, srv, db := newTestHandlerWithInstance(t)
	h.SetPlugins([]plugin.Plugin{dualScopePlugin{name: "hello"}})

	const org = uint(904)
	ownerUID := seedOrgMember(t, db, org, "owner@x.com", "owner")
	if rec := do(srv, "GET", "/api/instance/menus", sessionCookies(t, ownerUID, org), ""); rec.Code != http.StatusForbidden {
		t.Fatalf("org owner reading /api/instance/menus: got %d, want 403", rec.Code)
	}
}

// TestInstanceMenusServedToInstanceAdmin: the configured instance admin gets
// the list, and the test plugin's InstanceMenus() entry is in it.
func TestInstanceMenusServedToInstanceAdmin(t *testing.T) {
	h, srv, _ := newTestHandlerWithInstance(t)
	h.SetPlugins([]plugin.Plugin{
		dualScopePlugin{
			name: "hello",
			instanceMenus: []plugin.MenuItem{
				{ID: "hello-instance", Label: "Hello (instance)", Path: "/hello", Icon: "sparkles"},
			},
		},
	})

	rec := do(srv, "GET", "/api/instance/menus", loginCookies(t, srv), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("instance admin: got %d, want 200", rec.Code)
	}
	var out []MenuItem
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var found bool
	for _, m := range out {
		if m.ID == "hello-instance" && m.Path == "/hello" {
			found = true
		}
	}
	if !found {
		t.Errorf("instance admin should see the plugin's instance menu, got %+v", out)
	}
}

// TestInstanceMenusScopeIsolation is the core guard: the SAME plugin
// implementing both providers must never leak one scope's pages into the
// other shell — /api/menus must not contain /instance-only, and
// /api/instance/menus must not contain /tenant-only.
func TestInstanceMenusScopeIsolation(t *testing.T) {
	h, srv, db := newTestHandlerWithInstance(t)
	h.SetPlugins([]plugin.Plugin{
		dualScopePlugin{
			name:          "dual",
			tenantMenus:   []plugin.MenuItem{{ID: "dual-tenant", Label: "Tenant only", Path: "/tenant-only", Category: "Workspace"}},
			instanceMenus: []plugin.MenuItem{{ID: "dual-instance", Label: "Instance only", Path: "/instance-only", Icon: "server"}},
		},
	})

	// Instance side, as the instance admin.
	rec := do(srv, "GET", "/api/instance/menus", loginCookies(t, srv), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("instance admin: got %d, want 200", rec.Code)
	}
	var instanceMenus []MenuItem
	if err := json.Unmarshal(rec.Body.Bytes(), &instanceMenus); err != nil {
		t.Fatalf("decode instance menus: %v", err)
	}
	for _, m := range instanceMenus {
		if m.Path == "/tenant-only" {
			t.Errorf("instance menus leaked a tenant page: %+v", m)
		}
	}

	// Tenant side, as an org member of the same instance.
	const org = uint(905)
	ownerUID := seedOrgMember(t, db, org, "owner@x.com", "owner")
	rec = do(srv, "GET", "/api/menus", sessionCookies(t, ownerUID, org), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("org owner reading /api/menus: got %d, want 200", rec.Code)
	}
	var tenantMenus []MenuItem
	if err := json.Unmarshal(rec.Body.Bytes(), &tenantMenus); err != nil {
		t.Fatalf("decode tenant menus: %v", err)
	}
	var sawTenant bool
	for _, m := range tenantMenus {
		if m.Path == "/instance-only" {
			t.Errorf("tenant menus leaked an instance page: %+v", m)
		}
		if m.ID == "dual-tenant" {
			sawTenant = true
		}
	}
	if !sawTenant {
		t.Errorf("tenant menus should include the plugin's own tenant entry, got %+v", tenantMenus)
	}
}

// TestInstanceMenusSortedOrder pins the stable ordering contract: Order asc,
// then ID asc.
func TestInstanceMenusSortedOrder(t *testing.T) {
	h, srv, _ := newTestHandlerWithInstance(t)
	h.SetPlugins([]plugin.Plugin{
		dualScopePlugin{
			name: "hello",
			instanceMenus: []plugin.MenuItem{
				{ID: "b", Label: "B", Path: "/b", Order: 2},
				{ID: "a", Label: "A", Path: "/a", Order: 1},
				{ID: "c", Label: "C", Path: "/c", Order: 1},
			},
		},
	})

	rec := do(srv, "GET", "/api/instance/menus", loginCookies(t, srv), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("instance admin: got %d, want 200", rec.Code)
	}
	var out []MenuItem
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var ids []string
	for _, m := range out {
		ids = append(ids, m.ID)
	}
	want := []string{"a", "c", "b"} // Order 1 before Order 2; ties by ID
	if len(ids) != len(want) {
		t.Fatalf("menu count: got %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("menu order: got %v, want %v", ids, want)
		}
	}
}
