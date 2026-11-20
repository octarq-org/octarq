package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
)

type depPlugin struct {
	name, group string
	requires    []string
	core        bool
	byDefault   bool
}

func (d depPlugin) Name() string                          { return d.name }
func (d depPlugin) Models() []any                         { return nil }
func (d depPlugin) Mount(_ plugin.Mux, _ *plugin.Context) {}
func (d depPlugin) Describe() plugin.Info {
	return plugin.Info{
		Title:            d.name,
		Group:            d.group,
		Requires:         d.requires,
		Core:             d.core,
		EnabledByDefault: d.byDefault,
	}
}
func (d depPlugin) Menus() []plugin.MenuItem {
	return []plugin.MenuItem{{ID: d.name, Label: d.name, Path: "/" + d.name, Category: "Operations"}}
}

func feature(t *testing.T, srv http.Handler, cookies []*http.Cookie, key string) featureView {
	t.Helper()
	rec := do(srv, "GET", "/api/plugins", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/plugins: got %d", rec.Code)
	}
	var feats []featureView
	if err := json.Unmarshal(rec.Body.Bytes(), &feats); err != nil {
		t.Fatalf("decode features: %v", err)
	}
	for _, f := range feats {
		if f.Key == key {
			return f
		}
	}
	t.Fatalf("feature %q not in %+v", key, feats)
	return featureView{}
}

type featureView struct {
	Key        string   `json:"key"`
	Enabled    bool     `json:"enabled"`
	Requires   []string `json:"requires"`
	RequiredBy []string `json:"requiredBy"`
}

func mailDependsOnDNS() []plugin.Plugin {
	return []plugin.Plugin{
		depPlugin{name: "dns"},
		depPlugin{name: "links", requires: []string{"dns"}},
		depPlugin{name: "mail", requires: []string{"dns", "links"}},
	}
}

func TestDisableBlockedWhileDependentEnabled(t *testing.T) {
	h, srv, _ := newTestHandlerWithInstance(t)
	h.SetPlugins(mailDependsOnDNS())
	cookies := loginCookies(t, srv)

	if rec := do(srv, "PUT", "/api/plugins/mail", cookies, `{"enabled":true}`); rec.Code != http.StatusOK {
		t.Fatalf("enable mail: got %d", rec.Code)
	}

	rec := do(srv, "PUT", "/api/plugins/dns", cookies, `{"enabled":false}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("disable dns while mail is on: got %d, want 409", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, "mail") && !strings.Contains(body, "links") {
		t.Errorf("409 body should name the dependents that block the change, got %s", body)
	}

	if f := feature(t, srv, cookies, "dns"); !f.Enabled {
		t.Error("dns should still be enabled after the rejected disable")
	}
}

func TestDisableSucceedsOnceDependentsAreOff(t *testing.T) {
	h, srv, _ := newTestHandlerWithInstance(t)
	h.SetPlugins(mailDependsOnDNS())
	cookies := loginCookies(t, srv)

	do(srv, "PUT", "/api/plugins/mail", cookies, `{"enabled":true}`)

	if rec := do(srv, "PUT", "/api/plugins/mail", cookies, `{"enabled":false}`); rec.Code != http.StatusOK {
		t.Fatalf("disable mail: got %d", rec.Code)
	}
	if rec := do(srv, "PUT", "/api/plugins/links", cookies, `{"enabled":false}`); rec.Code != http.StatusOK {
		t.Fatalf("disable links: got %d", rec.Code)
	}
	if rec := do(srv, "PUT", "/api/plugins/dns", cookies, `{"enabled":false}`); rec.Code != http.StatusOK {
		t.Fatalf("disable dns once nothing depends on it: got %d, want 200", rec.Code)
	}
	if f := feature(t, srv, cookies, "dns"); f.Enabled {
		t.Error("dns should be disabled")
	}
}

func TestEnableCascadesDependencyChain(t *testing.T) {
	h, srv, db := newTestHandlerWithInstance(t)
	h.SetPlugins(mailDependsOnDNS())
	cookies := loginCookies(t, srv)

	if rec := do(srv, "PUT", "/api/plugins/mail", cookies, `{"enabled":true}`); rec.Code != http.StatusOK {
		t.Fatalf("enable mail: got %d", rec.Code)
	}

	for _, key := range []string{"mail", "links", "dns"} {
		if f := feature(t, srv, cookies, key); !f.Enabled {
			t.Errorf("%s should have been enabled by the cascade", key)
		}
	}

	var rows []models.PluginSetting
	db.Where("plugin IN ?", []string{"mail", "links", "dns"}).Find(&rows)
	if len(rows) != 3 {
		t.Errorf("want a PluginSetting row per cascaded feature, got %d: %+v", len(rows), rows)
	}
}

func TestEnableWithDependencyCycleTerminates(t *testing.T) {
	h, srv, _ := newTestHandlerWithInstance(t)
	h.SetPlugins([]plugin.Plugin{
		depPlugin{name: "a", requires: []string{"b"}},
		depPlugin{name: "b", requires: []string{"a"}},
	})
	cookies := loginCookies(t, srv)

	done := make(chan int, 1)
	go func() { done <- do(srv, "PUT", "/api/plugins/a", cookies, `{"enabled":true}`).Code }()

	select {
	case code := <-done:
		if code != http.StatusOK {
			t.Fatalf("cyclic enable: got %d", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("enable with a dependency cycle did not terminate — cycle guard is missing")
	}
}

func TestRequiredByReflectsOnlyEnabledDependents(t *testing.T) {
	h, srv, _ := newTestHandlerWithInstance(t)
	h.SetPlugins(mailDependsOnDNS())
	cookies := loginCookies(t, srv)

	if f := feature(t, srv, cookies, "dns"); len(f.RequiredBy) != 0 {
		t.Errorf("dns.requiredBy should be empty while every dependent is off, got %v", f.RequiredBy)
	}

	do(srv, "PUT", "/api/plugins/links", cookies, `{"enabled":true}`)

	f := feature(t, srv, cookies, "dns")
	if len(f.RequiredBy) == 0 {
		t.Fatal("dns.requiredBy should name links once links is enabled")
	}
	for _, d := range f.RequiredBy {
		if d == "mail" {
			t.Errorf("disabled mail must not appear in dns.requiredBy, got %v", f.RequiredBy)
		}
	}

	if lf := feature(t, srv, cookies, "links"); len(lf.Requires) == 0 || lf.Requires[0] != "dns" {
		t.Errorf("links.requires should be [dns], got %v", lf.Requires)
	}
}

func TestGroupedDependencyResolvesToFeatureKey(t *testing.T) {
	h, srv, _ := newTestHandlerWithInstance(t)
	h.SetPlugins([]plugin.Plugin{
		depPlugin{name: "issuer", group: "commerce"},
		depPlugin{name: "portal", requires: []string{"issuer"}},
	})
	cookies := loginCookies(t, srv)

	if rec := do(srv, "PUT", "/api/plugins/portal", cookies, `{"enabled":true}`); rec.Code != http.StatusOK {
		t.Fatalf("enable portal: got %d", rec.Code)
	}
	if f := feature(t, srv, cookies, "commerce"); !f.Enabled {
		t.Error("enabling portal should cascade the commerce group on via issuer")
	}
	if rec := do(srv, "PUT", "/api/plugins/commerce", cookies, `{"enabled":false}`); rec.Code != http.StatusConflict {
		t.Errorf("disabling commerce while portal depends on issuer: got %d, want 409", rec.Code)
	}
}

func TestPluginToggleIsolatedPerOrg(t *testing.T) {
	h, srv, db := newTestHandlerWithInstance(t)
	h.SetPlugins(mailDependsOnDNS())

	const orgA, orgB = uint(901), uint(902)
	uidA := seedOrgMember(t, db, orgA, "a@x.com", "owner")
	uidB := seedOrgMember(t, db, orgB, "b@x.com", "owner")
	sessA := sessionCookies(t, uidA, orgA)
	sessB := sessionCookies(t, uidB, orgB)

	if rec := do(srv, "PUT", "/api/plugins/mail", sessA, `{"enabled":true}`); rec.Code != http.StatusOK {
		t.Fatalf("org A enable mail: got %d", rec.Code)
	}

	for _, key := range []string{"mail", "links", "dns"} {
		if f := feature(t, srv, sessB, key); f.Enabled {
			t.Errorf("org B's %s should be untouched by org A's cascade", key)
		}
	}

	if rec := do(srv, "PUT", "/api/plugins/dns", sessB, `{"enabled":false}`); rec.Code != http.StatusOK {
		t.Errorf("org B disable dns: got %d, want 200 (org A's mail must not block it)", rec.Code)
	}

	var rowsA []models.PluginSetting
	db.Where("org_id = ?", orgA).Find(&rowsA)
	for _, r := range rowsA {
		if !r.Enabled {
			t.Errorf("org A's %s was flipped off by org B's request", r.Plugin)
		}
	}
}

func TestInstancePluginsRequiresInstanceAdmin(t *testing.T) {
	h, srv, db := newTestHandlerWithInstance(t)
	h.SetPlugins([]plugin.Plugin{
		depPlugin{name: "dns"},
		depPlugin{name: "licensing", core: true},
	})

	const org = uint(903)
	ownerUID := seedOrgMember(t, db, org, "owner@x.com", "owner")
	if rec := do(srv, "GET", "/api/instance/plugins", sessionCookies(t, ownerUID, org), ""); rec.Code != http.StatusForbidden {
		t.Fatalf("org owner reading /api/instance/plugins: got %d, want 403", rec.Code)
	}

	rec := do(srv, "GET", "/api/instance/plugins", loginCookies(t, srv), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("instance admin: got %d, want 200", rec.Code)
	}
	var out []struct {
		Name string `json:"name"`
		Core bool   `json:"core"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var sawCore bool
	for _, p := range out {
		if p.Name == "licensing" && p.Core {
			sawCore = true
		}
	}
	if !sawCore {
		t.Errorf("instance view should include Core plugins the org view hides, got %+v", out)
	}
}

func TestInstancePluginsRejectsAnonymous(t *testing.T) {
	h, srv, _ := newTestHandlerWithInstance(t)
	h.SetPlugins([]plugin.Plugin{depPlugin{name: "dns"}})

	if rec := do(srv, "GET", "/api/instance/plugins", nil, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: got %d, want 401", rec.Code)
	}
}

func TestDependencyConflictBodyShape(t *testing.T) {
	h, srv, _ := newTestHandlerWithInstance(t)
	h.SetPlugins(mailDependsOnDNS())
	cookies := loginCookies(t, srv)

	do(srv, "PUT", "/api/plugins/mail", cookies, `{"enabled":true}`)
	rec := do(srv, "PUT", "/api/plugins/dns", cookies, `{"enabled":false}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("got %d, want 409", rec.Code)
	}

	var body struct {
		Feature    string   `json:"feature"`
		Dependents []string `json:"dependents"`
		Detail     string   `json:"detail"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode 409 body: %v (raw: %s)", err, rec.Body.String())
	}
	if body.Feature != "dns" {
		t.Errorf("feature = %q, want dns", body.Feature)
	}
	if len(body.Dependents) == 0 {
		t.Fatalf("dependents must be populated, got %s", rec.Body.String())
	}
	for i := 1; i < len(body.Dependents); i++ {
		if body.Dependents[i-1] > body.Dependents[i] {
			t.Errorf("dependents should be sorted, got %v", body.Dependents)
		}
	}
	if body.Detail == "" {
		t.Error("detail should carry a human-readable message")
	}
}
