package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/octarq-org/octarq/plugin"
)

type actionTestPlugin struct{}

func (actionTestPlugin) Name() string                          { return "action_test" }
func (actionTestPlugin) Models() []any                         { return nil }
func (actionTestPlugin) Mount(_ plugin.Mux, _ *plugin.Context) {}
func (actionTestPlugin) Actions() []plugin.Action {
	return []plugin.Action{
		{ID: "action_test.create", Label: "New Test Item", Path: "/test?create=1", Icon: "plus", Category: "Test", Order: 5, RequiredRole: "admin"},
	}
}

type noActionPlugin struct{}

func (noActionPlugin) Name() string                          { return "no_action" }
func (noActionPlugin) Models() []any                         { return nil }
func (noActionPlugin) Mount(_ plugin.Mux, _ *plugin.Context) {}

func TestActionsAPI(t *testing.T) {
	t.Run("ActionProvider plugin exposes actions when enabled, hidden when disabled", func(t *testing.T) {
		h, srv, _ := newTestHandlerWithInstance(t)
		h.SetPlugins([]plugin.Plugin{actionTestPlugin{}})

		cookies := loginCookies(t, srv)

		// 1. When plugin is disabled (default), its action does NOT appear
		rec := do(srv, "GET", "/api/actions", cookies, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /api/actions got %d (%s)", rec.Code, rec.Body.String())
		}
		var actions []Action
		if err := json.Unmarshal(rec.Body.Bytes(), &actions); err != nil {
			t.Fatalf("decode actions: %v (%s)", err, rec.Body.String())
		}
		if len(actions) != 0 {
			t.Fatalf("expected 0 actions when plugin is disabled, got %d", len(actions))
		}

		// Enable plugin
		if rec := do(srv, "PUT", "/api/plugins/action_test", cookies, `{"enabled":true}`); rec.Code != http.StatusOK {
			t.Fatalf("enable plugin got %d (%s)", rec.Code, rec.Body.String())
		}

		// 2. When plugin is enabled, its action appears in GET /api/actions
		rec = do(srv, "GET", "/api/actions", cookies, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /api/actions got %d (%s)", rec.Code, rec.Body.String())
		}
		actions = nil
		if err := json.Unmarshal(rec.Body.Bytes(), &actions); err != nil {
			t.Fatalf("decode actions: %v (%s)", err, rec.Body.String())
		}
		if len(actions) != 1 {
			t.Fatalf("expected 1 action when plugin is enabled, got %d", len(actions))
		}
		act := actions[0]
		if act.ID != "action_test.create" || act.Label != "New Test Item" || act.Path != "/test?create=1" || act.Icon != "plus" || act.Category != "Test" || act.Order != 5 {
			t.Errorf("unexpected action fields: %+v", act)
		}
		// 4. RequiredRole passes through to JSON
		if act.RequiredRole != "admin" {
			t.Errorf("expected RequiredRole \"admin\", got %q", act.RequiredRole)
		}
	})

	t.Run("Plugin without ActionProvider returns empty array [] not null", func(t *testing.T) {
		h, srv, _ := newTestHandlerWithInstance(t)
		h.SetPlugins([]plugin.Plugin{noActionPlugin{}})

		cookies := loginCookies(t, srv)

		// Enable noActionPlugin
		if rec := do(srv, "PUT", "/api/plugins/no_action", cookies, `{"enabled":true}`); rec.Code != http.StatusOK {
			t.Fatalf("enable plugin got %d (%s)", rec.Code, rec.Body.String())
		}

		rec := do(srv, "GET", "/api/actions", cookies, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /api/actions got %d (%s)", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if body != "[]" && body != "[]\n" {
			t.Fatalf("expected [] for actions, got %q", body)
		}
	})
}
