package hello

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/octarq-org/octarq/plugin"
)

// TestGreeterService exposes the greeter both directly and through a registry,
// the documented lookup path for consumers.
func TestGreeterService(t *testing.T) {
	var g Greeter = greeter{}
	if got := g.Greet("world"); got != "hello, world!" {
		t.Errorf("Greet = %q", got)
	}

	reg := plugin.NewRegistry()
	reg.Provide("hello.greeter", Greeter(greeter{}))
	got, ok := plugin.LookupServiceAs[Greeter](reg.Lookup, "hello.greeter")
	if !ok {
		t.Fatal("greeter not resolvable from the registry")
	}
	if got.Greet("octarq") != "hello, octarq!" {
		t.Errorf("registry Greet = %q", got.Greet("octarq"))
	}
}

// TestPluginDescription pins the example's identity: name, models, describe and
// both menu scopes it declares.
func TestPluginDescription(t *testing.T) {
	p := Plugin{}

	var _ plugin.Plugin = p
	var _ plugin.MenuProvider = p
	var _ plugin.InstanceMenuProvider = p
	var _ plugin.Describer = p

	if p.Name() != "hello" {
		t.Errorf("Name = %q, want hello", p.Name())
	}
	info := p.Describe()
	if !info.EnabledByDefault {
		t.Error("example plugin must be enabled by default")
	}
	if models := p.Models(); models != nil {
		t.Errorf("Models = %v, want nil (stateless)", models)
	}

	menus := p.Menus()
	if len(menus) != 1 || menus[0].ID != "hello" || menus[0].Icon != "puzzle" {
		t.Errorf("Menus = %+v", menus)
	}
	instance := p.InstanceMenus()
	if len(instance) != 1 || instance[0].Path != "/hello" || instance[0].Icon != "sparkles" {
		t.Errorf("InstanceMenus = %+v", instance)
	}
}

// TestMountRegistersRouteAndService mounts the plugin on a real mux with a
// pass-through Guard and asserts the /api/hello/ping route answers with the
// plugin's JSON envelope.
func TestMountRegistersRouteAndService(t *testing.T) {
	mux := http.NewServeMux()
	reg := plugin.NewRegistry()
	ctx := &plugin.Context{
		Provide: reg.Provide,
		Guard:   func(h http.Handler) http.Handler { return h },
	}
	Plugin{}.Mount(mux, ctx)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/hello/ping", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("ping = %d, want 200", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if body["message"] != "hello from the example plugin" {
		t.Errorf("message = %q", body["message"])
	}
	if body["time"] == "" {
		t.Error("missing RFC3339 time field")
	}

	if _, ok := plugin.LookupServiceAs[Greeter](reg.Lookup, "hello.greeter"); !ok {
		t.Error("Mount did not Provide hello.greeter")
	}
}

// TestMountToleratesNilProvide covers hosts whose Context predates the Provide
// field: the plugin must still mount routes without panicking.
func TestMountToleratesNilProvide(t *testing.T) {
	mux := http.NewServeMux()
	Plugin{}.Mount(mux, &plugin.Context{
		Guard: func(h http.Handler) http.Handler { return h },
	})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/hello/ping", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("ping = %d, want 200", rec.Code)
	}
}
