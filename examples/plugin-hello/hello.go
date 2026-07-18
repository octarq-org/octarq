// Package hello is a minimal, copy-me example of a octarq plugin: the Go half of a
// full-stack feature. It implements plugin.Plugin (Name/Models/Mount) plus the
// optional plugin.MenuProvider, and exposes one trivial read-only endpoint.
//
// The JS half lives in ./web and implements the frontend `UIPlugin` contract
// against @octarq-org/plugin-sdk. Together they show the symmetry octarq is built on:
// a Go module + a JS package, each conforming to its side's plugin contract,
// composed into the app WITHOUT forking it — the Go plugin mounted via
// app.App.Use(...), the UI plugin composed at build time via registerUIPlugin.
package hello

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/octarq-org/octarq/plugin"
)

// Plugin is the exported unit a host wires up with app.App.Use(hello.Plugin{}).
type Plugin struct{}

// Greeter is the service this plugin offers to OTHER plugins through the
// inter-plugin registry (ctx.Provide in Mount, name "hello.greeter" — the
// "<pluginName>.<service>" convention). The provider owns the interface; a
// consumer imports this package for the type only and resolves the
// implementation lazily — in its Start (which the app runs only after ALL
// plugins have mounted) or per-request, never in its own Mount:
//
//	func (p *Consumer) Mount(mux plugin.Mux, ctx *plugin.Context) { p.ctx = ctx }
//
//	func (p *Consumer) Start(ctx context.Context) {
//		g, ok := plugin.LookupAs[hello.Greeter](p.ctx, "hello.greeter")
//		if !ok {
//			return // hello isn't in this build — degrade gracefully
//		}
//		_ = g.Greet("world")
//	}
//
//	var (
//		_ plugin.Plugin  = (*Consumer)(nil)
//		_ plugin.Starter = (*Consumer)(nil)
//	)
type Greeter interface {
	Greet(who string) string
}

type greeter struct{}

func (greeter) Greet(who string) string { return "hello, " + who + "!" }

// Name is the stable identifier — matches the frontend UIPlugin's `name` so the
// two halves of the feature are traceable to each other.
func (Plugin) Name() string { return "hello" }

// Describe makes the example on by default: a fresh workspace sees the Hello
// feature without having to enable it first, yet it stays user-toggleable (not
// Core) so the plugin manager still lists it and can turn it off. EnabledByDefault
// flips the pre-toggle default from opt-in to opt-out.
func (Plugin) Describe() plugin.Info {
	return plugin.Info{Title: "Hello", EnabledByDefault: true}
}

// Models returns the GORM models this plugin owns. This example is stateless.
func (Plugin) Models() []any { return nil }

// Mount registers the plugin's HTTP routes on the shared API mux. Every route
// is auto-gated: if the "hello" feature is disabled for the caller's workspace,
// the app answers 404 before the handler runs — which is exactly the state the
// frontend page renders its neutral "not in this build" fallback for.
func (Plugin) Mount(mux plugin.Mux, ctx *plugin.Context) {
	// Offer Greeter to other plugins. Mount is the only place to Provide; a
	// duplicate name is a startup error, so the app fails fast on collisions.
	// The nil check follows the Context evolution policy: tolerate hosts that
	// predate a field.
	if ctx.Provide != nil {
		ctx.Provide("hello.greeter", Greeter(greeter{}))
	}

	mux.Handle("GET /api/hello/ping", ctx.Guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": "hello from the example plugin",
			"time":    time.Now().UTC().Format(time.RFC3339),
		})
	})))
}

// Menus contributes a sidebar link for the frontend. Category names the
// sidebar GROUP the entry joins — by convention it equals the group's label
// ("Workspace" is the group holding Overview); a category with no matching
// group creates one, placed in a top-level area by the shared areaForCategory
// keyword routing. Implementing this optional interface is how a plugin gets a
// nav entry without the frontend hardcoding it.
func (Plugin) Menus() []plugin.MenuItem {
	return []plugin.MenuItem{
		{ID: "hello", Label: "Hello", Path: "/hello", Icon: "👋", Category: "Workspace"},
	}
}

// Compile-time assertions that Plugin satisfies the contracts it claims.
var (
	_ plugin.Plugin       = Plugin{}
	_ plugin.MenuProvider = Plugin{}
	_ plugin.Describer    = Plugin{}
)
