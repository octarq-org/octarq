package hello

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/octarq-org/octarq/server/plugin"
)

type Plugin struct{}

type Greeter interface {
	Greet(who string) string
}

type greeter struct{}

func (greeter) Greet(who string) string { return "hello, " + who + "!" }

func (Plugin) Name() string { return "hello" }

func (Plugin) Describe() plugin.Info {
	return plugin.Info{Title: "Hello", EnabledByDefault: true}
}

func (Plugin) Models() []any { return nil }

func (Plugin) Mount(mux plugin.Mux, ctx *plugin.Context) {
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

func (Plugin) Menus() []plugin.MenuItem {
	return []plugin.MenuItem{
		{ID: "hello", Label: "Hello", Path: "/hello", Icon: "puzzle", Category: "Workspace", Area: "operations"},
	}
}

func (Plugin) InstanceMenus() []plugin.MenuItem {
	return []plugin.MenuItem{
		{ID: "hello-instance", Label: "Hello (instance)", Path: "/hello", Icon: "sparkles"},
	}
}

var (
	_ plugin.Plugin               = Plugin{}
	_ plugin.MenuProvider         = Plugin{}
	_ plugin.InstanceMenuProvider = Plugin{}
	_ plugin.Describer            = Plugin{}
)
