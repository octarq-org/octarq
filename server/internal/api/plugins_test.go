package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/plugin"
)

type fakePlugin struct{}

func (fakePlugin) Name() string                          { return "fake" }
func (fakePlugin) Models() []any                         { return nil }
func (fakePlugin) Mount(_ plugin.Mux, _ *plugin.Context) {}
func (fakePlugin) Menus() []plugin.MenuItem {
	return []plugin.MenuItem{{ID: "fake", Label: "Fake", Path: "/fake", Category: "Operations"}}
}

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

type corePlugin struct{}

func (corePlugin) Name() string                          { return "licensing" }
func (corePlugin) Models() []any                         { return nil }
func (corePlugin) Mount(_ plugin.Mux, _ *plugin.Context) {}
func (corePlugin) Describe() plugin.Info                 { return plugin.Info{Core: true} }

func TestPluginGroupingAndCore(t *testing.T) {
	h, srv, _ := newTestHandlerWithInstance(t)
	h.SetPlugins([]plugin.Plugin{
		groupedPlugin{name: "product", group: "commerce", path: "/storefront"},
		groupedPlugin{name: "billing", group: "commerce", path: "/billing"},
		corePlugin{},
	})
	cookies := loginCookies(t, srv)

	rec := do(srv, "GET", "/api/plugins", cookies, "")
	var feats []struct {
		Key     string `json:"key"`
		Title   string `json:"title"`
		Enabled bool   `json:"enabled"`
		Core    bool   `json:"core"`
		Menus   []struct {
			Path string `json:"path"`
		} `json:"menus"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &feats)
	byKey := map[string]int{}
	for i, f := range feats {
		byKey[f.Key] = i
	}
	if len(feats) != 2 || len(byKey) != 2 {
		t.Fatalf("want exactly the 'commerce' and 'licensing' features, got %+v", feats)
	}

	commerce := feats[byKey["commerce"]]
	if commerce.Title != "Commerce" || len(commerce.Menus) != 2 {
		t.Fatalf("commerce should carry both member menus + title, got %+v", commerce)
	}
	if commerce.Core {
		t.Fatal("commerce is toggleable and must not be flagged core")
	}

	core, ok := byKey["licensing"]
	if !ok {
		t.Fatalf("core plumbing must be listed, got %+v", feats)
	}
	if !feats[core].Core || !feats[core].Enabled {
		t.Fatalf("core plumbing must report core+enabled, got %+v", feats[core])
	}

	if rec := do(srv, "PUT", "/api/plugins/licensing", cookies, `{"enabled":false}`); rec.Code != http.StatusNotFound {
		t.Fatalf("disabling core plumbing must 404, got %d", rec.Code)
	}

	if rec := do(srv, "PUT", "/api/plugins/commerce", cookies, `{"enabled":true}`); rec.Code != http.StatusOK {
		t.Fatalf("enable commerce: got %d", rec.Code)
	}
	if !menuHasPath(t, srv, cookies, "/storefront") || !menuHasPath(t, srv, cookies, "/billing") {
		t.Fatal("both grouped menus should appear once the feature is enabled")
	}
}

func TestPluginRegistryToggleAndMenuFilter(t *testing.T) {
	h, srv, _ := newTestHandlerWithInstance(t)
	h.SetPlugins([]plugin.Plugin{fakePlugin{}})
	cookies := loginCookies(t, srv)

	if got := pluginEnabled(t, srv, cookies, "fake"); got {
		t.Fatal("plugin should be disabled by default")
	}
	if menuHasPath(t, srv, cookies, "/fake") {
		t.Fatal("disabled plugin menu should not appear")
	}

	if rec := do(srv, "PUT", "/api/plugins/fake", cookies, `{"enabled":true}`); rec.Code != http.StatusOK {
		t.Fatalf("enable: got %d (%s)", rec.Code, rec.Body.String())
	}
	if got := pluginEnabled(t, srv, cookies, "fake"); !got {
		t.Fatal("plugin should be enabled after toggle")
	}
	if !menuHasPath(t, srv, cookies, "/fake") {
		t.Fatal("enabled plugin menu should appear in /api/menus")
	}

	if rec := do(srv, "PUT", "/api/plugins/nope", cookies, `{"enabled":true}`); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown plugin: got %d, want 404", rec.Code)
	}
}

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

func TestPluginEnabledByDefault(t *testing.T) {
	h, srv, _ := newTestHandlerWithInstance(t)
	h.SetPlugins([]plugin.Plugin{defaultOnPlugin{}})
	cookies := loginCookies(t, srv)

	if !pluginEnabled(t, srv, cookies, "demo") {
		t.Fatal("EnabledByDefault plugin should be on before any toggle")
	}
	if h.PluginEnabled(0, "demo") {
		t.Fatal("PluginEnabled(0, key) must fail closed even for a default-on feature")
	}
	if !h.FeatureDefaultEnabled("demo") {
		t.Fatal("FeatureDefaultEnabled should report the declared default")
	}
	if !menuHasPath(t, srv, cookies, "/demo") {
		t.Fatal("default-on plugin menu should appear")
	}

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

type coreHalfPlugin struct{}

func (coreHalfPlugin) Name() string                          { return "help" }
func (coreHalfPlugin) Models() []any                         { return nil }
func (coreHalfPlugin) Mount(_ plugin.Mux, _ *plugin.Context) {}
func (coreHalfPlugin) Describe() plugin.Info                 { return plugin.Info{Title: "Help", Core: true} }
func (coreHalfPlugin) Menus() []plugin.MenuItem {
	return []plugin.MenuItem{{ID: "help", Label: "Help", Path: "/help", Category: "footer"}}
}

type contentHalfPlugin struct{}

func (contentHalfPlugin) Name() string                          { return "help" }
func (contentHalfPlugin) Models() []any                         { return nil }
func (contentHalfPlugin) Mount(_ plugin.Mux, _ *plugin.Context) {}
func (contentHalfPlugin) Describe() plugin.Info                 { return plugin.Info{Title: "Help"} }

func TestCoreIsStickyAcrossFeatureHalves(t *testing.T) {
	h, srv, _ := newTestHandlerWithInstance(t)
	h.SetPlugins([]plugin.Plugin{contentHalfPlugin{}, coreHalfPlugin{}})
	cookies := loginCookies(t, srv)

	rec := do(srv, "GET", "/api/plugins", cookies, "")
	var feats []struct {
		Key     string `json:"key"`
		Enabled bool   `json:"enabled"`
		Core    bool   `json:"core"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &feats); err != nil {
		t.Fatalf("decode plugins: %v", err)
	}
	found := false
	for _, f := range feats {
		if f.Key != "help" {
			continue
		}
		found = true
		if !f.Core || !f.Enabled {
			t.Fatalf("a feature with a Core half must be listed core+enabled, got %+v", f)
		}
	}
	if !found {
		t.Fatalf("the help feature must be listed, got %+v", feats)
	}

	if rec := do(srv, "PUT", "/api/plugins/help", cookies, `{"enabled":false}`); rec.Code != http.StatusNotFound {
		t.Fatalf("disable core feature: got %d, want 404", rec.Code)
	}
	if !menuHasPath(t, srv, cookies, "/help") {
		t.Fatal("core feature menu must stay visible")
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

func TestPluginToggleSchemaNameNotLogout(t *testing.T) {
	_, srv, _ := newTestHandlerWithInstance(t)

	req := httptest.NewRequest("GET", "/openapi.json", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("openapi.json returned %d", rec.Code)
	}

	var doc struct {
		Paths map[string]struct {
			Put struct {
				Responses map[string]struct {
					Content map[string]struct {
						Schema struct {
							Ref string `json:"$ref"`
						} `json:"schema"`
					} `json:"content"`
				} `json:"responses"`
			} `json:"put"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("parse openapi: %v", err)
	}

	var raw struct {
		Paths map[string]struct {
			Put struct {
				Responses map[string]any `json:"responses"`
			} `json:"put"`
		} `json:"paths"`
	}
	json.Unmarshal(rec.Body.Bytes(), &raw)
	out, _ := json.MarshalIndent(raw.Paths["/api/plugins/{name}"].Put.Responses, "", " ")
	fmt.Printf("=== SCHEMA_OUTPUT ===\n%s\n=====================\n", string(out))

	pathData, ok := doc.Paths["/api/plugins/{name}"]
	if !ok {
		t.Fatal("missing /api/plugins/{name} in openapi")
	}
	resp, ok := pathData.Put.Responses["200"]
	if !ok {
		t.Fatal("missing 200 response")
	}
	content, ok := resp.Content["application/json"]
	if !ok {
		t.Fatal("missing application/json content")
	}

	ref := content.Schema.Ref
	if strings.Contains(strings.ToLower(ref), "logout") {
		t.Fatalf("PUT /api/plugins/{name} schema must not be a logout type, got: %s", ref)
	}
}
