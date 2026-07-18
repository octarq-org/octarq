package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/octarq-org/octarq/plugin"
)

type fakePlugin struct{}

func (fakePlugin) Name() string                          { return "fake" }
func (fakePlugin) Models() []any                         { return nil }
func (fakePlugin) Mount(_ plugin.Mux, _ *plugin.Context) {}
func (fakePlugin) Menus() []plugin.MenuItem {
	return []plugin.MenuItem{{ID: "fake", Label: "Fake", Path: "/fake", Category: "Operations"}}
}

// groupedPlugin is a member of a multi-plugin feature.
type groupedPlugin struct {
	name, group, path string
}

func (g groupedPlugin) Name() string                          { return g.name }
func (g groupedPlugin) Models() []any                         { return nil }
func (g groupedPlugin) Mount(_ plugin.Mux, _ *plugin.Context) {}
func (g groupedPlugin) Describe() plugin.Info                 { return plugin.Info{Title: "Commerce", Group: g.group} }
func (g groupedPlugin) Menus() []plugin.MenuItem {
	return []plugin.MenuItem{{ID: g.name, Label: g.name, Path: g.path, Category: "Commerce"}}
}

// corePlugin is always-on plumbing; it must be hidden from the registry.
type corePlugin struct{}

func (corePlugin) Name() string                          { return "licensing" }
func (corePlugin) Models() []any                         { return nil }
func (corePlugin) Mount(_ plugin.Mux, _ *plugin.Context) {}
func (corePlugin) Describe() plugin.Info                 { return plugin.Info{Core: true} }

// TestPluginGroupingAndCore verifies grouped plugins collapse into one feature
// toggled together, and core plugins are excluded from the registry.
func TestPluginGroupingAndCore(t *testing.T) {
	h, srv, _ := newTestHandlerWithInstance(t)
	h.SetPlugins([]plugin.Plugin{
		groupedPlugin{name: "product", group: "commerce", path: "/storefront"},
		groupedPlugin{name: "billing", group: "commerce", path: "/billing"},
		corePlugin{},
	})
	cookies := loginCookies(t, srv)

	// Registry: one "commerce" feature (not two), no core plugin.
	rec := do(srv, "GET", "/api/plugins", cookies, "")
	var feats []struct {
		Key   string `json:"key"`
		Title string `json:"title"`
		Menus []struct {
			Path string `json:"path"`
		} `json:"menus"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &feats)
	if len(feats) != 1 || feats[0].Key != "commerce" {
		t.Fatalf("want a single 'commerce' feature, got %+v", feats)
	}
	if feats[0].Title != "Commerce" || len(feats[0].Menus) != 2 {
		t.Fatalf("commerce should carry both member menus + title, got %+v", feats[0])
	}

	// Enabling the group surfaces both members' menus at once.
	if rec := do(srv, "PUT", "/api/plugins/commerce", cookies, `{"enabled":true}`); rec.Code != http.StatusOK {
		t.Fatalf("enable commerce: got %d", rec.Code)
	}
	if !menuHasPath(t, srv, cookies, "/storefront") || !menuHasPath(t, srv, cookies, "/billing") {
		t.Fatal("both grouped menus should appear once the feature is enabled")
	}
}

// TestPluginRegistryToggleAndMenuFilter verifies plugins are opt-in per
// workspace: disabled by default, toggleable by an admin, and their menus only
// appear in /api/menus once enabled.
func TestPluginRegistryToggleAndMenuFilter(t *testing.T) {
	h, srv, _ := newTestHandlerWithInstance(t)
	h.SetPlugins([]plugin.Plugin{fakePlugin{}})
	cookies := loginCookies(t, srv)

	// Default: registered but disabled, and its menu is absent.
	if got := pluginEnabled(t, srv, cookies, "fake"); got {
		t.Fatal("plugin should be disabled by default")
	}
	if menuHasPath(t, srv, cookies, "/fake") {
		t.Fatal("disabled plugin menu should not appear")
	}

	// Enable it.
	if rec := do(srv, "PUT", "/api/plugins/fake", cookies, `{"enabled":true}`); rec.Code != http.StatusOK {
		t.Fatalf("enable: got %d (%s)", rec.Code, rec.Body.String())
	}
	if got := pluginEnabled(t, srv, cookies, "fake"); !got {
		t.Fatal("plugin should be enabled after toggle")
	}
	if !menuHasPath(t, srv, cookies, "/fake") {
		t.Fatal("enabled plugin menu should appear in /api/menus")
	}

	// Unknown plugin → 404.
	if rec := do(srv, "PUT", "/api/plugins/nope", cookies, `{"enabled":true}`); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown plugin: got %d, want 404", rec.Code)
	}
}

// defaultOnPlugin is a user-toggleable feature that starts enabled (opt-out),
// like the hello example — Info.EnabledByDefault, not Core.
type defaultOnPlugin struct{}

func (defaultOnPlugin) Name() string                          { return "demo" }
func (defaultOnPlugin) Models() []any                         { return nil }
func (defaultOnPlugin) Mount(_ plugin.Mux, _ *plugin.Context) {}
func (defaultOnPlugin) Describe() plugin.Info {
	return plugin.Info{Title: "Demo", EnabledByDefault: true}
}
func (defaultOnPlugin) Menus() []plugin.MenuItem {
	return []plugin.MenuItem{{ID: "demo", Label: "Demo", Path: "/demo", Category: "Operations"}}
}

// TestPluginEnabledByDefault verifies EnabledByDefault flips the pre-toggle
// default to on while keeping the feature listed and toggleable off.
func TestPluginEnabledByDefault(t *testing.T) {
	h, srv, _ := newTestHandlerWithInstance(t)
	h.SetPlugins([]plugin.Plugin{defaultOnPlugin{}})
	cookies := loginCookies(t, srv)

	// No toggle yet: on by default, listed in the manager, menu present.
	if !pluginEnabled(t, srv, cookies, "demo") {
		t.Fatal("EnabledByDefault plugin should be on before any toggle")
	}
	if !menuHasPath(t, srv, cookies, "/demo") {
		t.Fatal("default-on plugin menu should appear")
	}

	// Still toggleable off — the default only sets the pre-toggle state.
	if rec := do(srv, "PUT", "/api/plugins/demo", cookies, `{"enabled":false}`); rec.Code != http.StatusOK {
		t.Fatalf("disable demo: got %d", rec.Code)
	}
	if pluginEnabled(t, srv, cookies, "demo") {
		t.Fatal("plugin should be off after explicit disable")
	}
	if menuHasPath(t, srv, cookies, "/demo") {
		t.Fatal("disabled plugin menu should disappear")
	}
}

func pluginEnabled(t *testing.T, srv http.Handler, cookies []*http.Cookie, name string) bool {
	t.Helper()
	rec := do(srv, "GET", "/api/plugins", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list plugins: got %d", rec.Code)
	}
	var list []struct {
		Key     string `json:"key"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode plugins: %v", err)
	}
	for _, p := range list {
		if p.Key == name {
			return p.Enabled
		}
	}
	t.Fatalf("feature %q not in registry", name)
	return false
}

func menuHasPath(t *testing.T, srv http.Handler, cookies []*http.Cookie, path string) bool {
	t.Helper()
	rec := do(srv, "GET", "/api/menus", cookies, "")
	var menus []struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &menus); err != nil {
		t.Fatalf("decode menus: %v", err)
	}
	for _, m := range menus {
		if m.Path == path {
			return true
		}
	}
	return false
}
