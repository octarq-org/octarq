package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Jungley8/led/plugin"
)

type fakePlugin struct{}

func (fakePlugin) Name() string                          { return "fake" }
func (fakePlugin) Models() []any                         { return nil }
func (fakePlugin) Mount(_ plugin.Mux, _ *plugin.Context) {}
func (fakePlugin) Menus() []plugin.MenuItem {
	return []plugin.MenuItem{{ID: "fake", Label: "Fake", Path: "/fake", Category: "Operations"}}
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

func pluginEnabled(t *testing.T, srv http.Handler, cookies []*http.Cookie, name string) bool {
	t.Helper()
	rec := do(srv, "GET", "/api/plugins", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list plugins: got %d", rec.Code)
	}
	var list []struct {
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode plugins: %v", err)
	}
	for _, p := range list {
		if p.Name == name {
			return p.Enabled
		}
	}
	t.Fatalf("plugin %q not in registry", name)
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
