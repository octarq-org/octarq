package api

// A menu entry may belong to a feature other than the publishing plugin's own.
//
// The ontology plugin publishes one entry per declared entity on behalf of the
// plugin that declared it, and it is core plumbing — always mounted, always on.
// Filtering the sidebar by the *publisher's* toggle therefore offered a disabled
// plugin's data page forever. These tests pin that the entry's own feature key
// decides.

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/octarq-org/octarq/server/plugin"
)

// entityHost is the core publisher: on in every workspace, and it announces a
// page that belongs to somebody else.
type entityHost struct{}

func (entityHost) Name() string                          { return "ontology" }
func (entityHost) Models() []any                         { return nil }
func (entityHost) Mount(_ plugin.Mux, _ *plugin.Context) {}
func (entityHost) Describe() plugin.Info {
	return plugin.Info{Title: "Data", Core: true, EnabledByDefault: true}
}
func (entityHost) Menus() []plugin.MenuItem {
	return []plugin.MenuItem{
		{ID: "data-runbooks", Label: "Runbooks", Path: "/data/runbooks", Category: "Workspace", Feature: "runbooks"},
		// An entry with no feature belongs to the publisher, as before.
		{ID: "data-tickets", Label: "Tickets", Path: "/data/tickets", Category: "Workspace"},
	}
}

// runbooks is the declaring module: off unless a workspace turns it on.
type runbooks struct{}

func (runbooks) Name() string                          { return "runbooks" }
func (runbooks) Models() []any                         { return nil }
func (runbooks) Mount(_ plugin.Mux, _ *plugin.Context) {}
func (runbooks) Describe() plugin.Info                 { return plugin.Info{Title: "Runbooks"} }

// menuIDs reads the sidebar the backend serves.
func menuIDs(t *testing.T, srv http.Handler, cookies []*http.Cookie) map[string]bool {
	t.Helper()
	rec := do(srv, "GET", "/api/menus", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list menus: got %d (%s)", rec.Code, rec.Body.String())
	}
	var menus []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &menus); err != nil {
		t.Fatalf("decode menus: %v (%s)", err, rec.Body.String())
	}
	out := make(map[string]bool, len(menus))
	for _, m := range menus {
		out[m.ID] = true
	}
	return out
}

// TestAForeignFeatureMenuFollowsItsOwnSwitch is the reported bug: disabling
// Runbooks left its data page in the sidebar, because the entry came from a core
// plugin that is always on.
func TestAForeignFeatureMenuFollowsItsOwnSwitch(t *testing.T) {
	h, srv, _ := newTestHandlerWithInstance(t)
	h.SetPlugins([]plugin.Plugin{entityHost{}, runbooks{}})
	cookies := loginCookies(t, srv)

	if ids := menuIDs(t, srv, cookies); ids["data-runbooks"] {
		t.Fatal("the runbooks entry is served while its plugin has never been enabled")
	}
	// The publisher's own entry is unaffected.
	if ids := menuIDs(t, srv, cookies); !ids["data-tickets"] {
		t.Fatal("an entry with no feature was filtered out")
	}

	if rec := do(srv, "PUT", "/api/plugins/runbooks", cookies, `{"enabled":true}`); rec.Code != http.StatusOK {
		t.Fatalf("enable runbooks: got %d (%s)", rec.Code, rec.Body.String())
	}
	if ids := menuIDs(t, srv, cookies); !ids["data-runbooks"] {
		t.Fatal("the runbooks entry is missing while its plugin is enabled")
	}

	if rec := do(srv, "PUT", "/api/plugins/runbooks", cookies, `{"enabled":false}`); rec.Code != http.StatusOK {
		t.Fatalf("disable runbooks: got %d (%s)", rec.Code, rec.Body.String())
	}
	ids := menuIDs(t, srv, cookies)
	if ids["data-runbooks"] {
		t.Fatal("the runbooks entry is still served after its plugin was disabled")
	}
	if !ids["data-tickets"] {
		t.Fatal("disabling runbooks took an unrelated entry with it")
	}
}

// TestThePluginManagerShowsAPageOnItsOwnersCard pins the other half of the
// attribution: the Data card is always on, so listing a disabled module's page
// there would tell an admin the page belongs to a feature they cannot switch
// off. The entry goes on the card whose toggle governs it.
func TestThePluginManagerShowsAPageOnItsOwnersCard(t *testing.T) {
	h, srv, _ := newTestHandlerWithInstance(t)
	h.SetPlugins([]plugin.Plugin{entityHost{}, runbooks{}})
	cookies := loginCookies(t, srv)

	rec := do(srv, "GET", "/api/plugins", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list features: got %d (%s)", rec.Code, rec.Body.String())
	}
	var feats []struct {
		Key   string `json:"key"`
		Menus []struct {
			Path string `json:"path"`
		} `json:"menus"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &feats); err != nil {
		t.Fatalf("decode features: %v (%s)", err, rec.Body.String())
	}

	paths := map[string][]string{}
	for _, f := range feats {
		for _, m := range f.Menus {
			paths[f.Key] = append(paths[f.Key], m.Path)
		}
	}
	if hasPath(paths["ontology"], "/data/runbooks") {
		t.Fatalf("the Data card lists %v; a page its toggle cannot govern belongs to its owner", paths["ontology"])
	}
	if !hasPath(paths["ontology"], "/data/tickets") {
		t.Fatalf("the Data card lost its own entry: %v", paths["ontology"])
	}
	if !hasPath(paths["runbooks"], "/data/runbooks") {
		t.Fatalf("the runbooks card does not list its page: %v", paths["runbooks"])
	}
}

func hasPath(paths []string, want string) bool {
	for _, p := range paths {
		if p == want {
			return true
		}
	}
	return false
}
