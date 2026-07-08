// Package hello is a minimal, copy-me example of a led plugin: the Go half of a
// full-stack feature. It implements plugin.Plugin (Name/Models/Mount) plus the
// optional plugin.MenuProvider, and exposes one trivial read-only endpoint.
//
// The JS half lives in ./web and implements the frontend `UIPlugin` contract
// against @led/plugin-sdk. Together they show the symmetry led is built on:
// a Go module + a JS package, each conforming to its side's plugin contract,
// composed into the app WITHOUT forking it — the Go plugin mounted via
// app.App.Use(...), the UI plugin composed at build time via registerUIPlugin.
package hello

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/octarq-org/led/plugin"
)

// Plugin is the exported unit a host wires up with app.App.Use(hello.Plugin{}).
type Plugin struct{}

// Name is the stable identifier — matches the frontend UIPlugin's `name` so the
// two halves of the feature are traceable to each other.
func (Plugin) Name() string { return "hello" }

// Models returns the GORM models this plugin owns. This example is stateless.
func (Plugin) Models() []any { return nil }

// Mount registers the plugin's HTTP routes on the shared API mux. Every route
// is auto-gated: if the "hello" feature is disabled for the caller's workspace,
// the app answers 404 before the handler runs — which is exactly the state the
// frontend page renders its neutral "not in this build" fallback for.
func (Plugin) Mount(mux plugin.Mux, ctx *plugin.Context) {
	mux.Handle("GET /api/hello/ping", ctx.Guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": "hello from the example plugin",
			"time":    time.Now().UTC().Format(time.RFC3339),
		})
	})))
}

// Menus contributes a sidebar link for the frontend. The Category ("operations")
// is placed by the same areaForCategory logic core menus use. Implementing this
// optional interface is how a plugin gets a nav entry without the frontend
// hardcoding it.
func (Plugin) Menus() []plugin.MenuItem {
	return []plugin.MenuItem{
		{ID: "hello", Label: "Hello", Path: "/hello", Icon: "👋", Category: "operations"},
	}
}

// Compile-time assertions that Plugin satisfies the contracts it claims.
var (
	_ plugin.Plugin       = Plugin{}
	_ plugin.MenuProvider = Plugin{}
)
