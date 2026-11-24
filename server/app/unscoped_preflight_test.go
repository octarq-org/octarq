package app

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/plugin"
)

type unscopedStubPlugin struct {
	name     string
	prefixes []string
}

func (p unscopedStubPlugin) Name() string                      { return p.name }
func (p unscopedStubPlugin) Models() []any                     { return nil }
func (p unscopedStubPlugin) Mount(plugin.Mux, *plugin.Context) {}

func (p unscopedStubPlugin) UnscopedPrefixes() []string { return p.prefixes }

func TestPreflightUnscopedClaims(t *testing.T) {
	routes := newRouteRegistry()
	routes.claim("commerce", "POST /api/storefront/checkout", false)
	routes.claim("commerce", "GET /api/storefront/orders/{id}", false)
	routes.claim("license", "GET /api/license", false)

	okPlugins := []plugin.Plugin{
		unscopedStubPlugin{name: "commerce", prefixes: []string{"/api/storefront/"}},
		unscopedStubPlugin{name: "license", prefixes: []string{"/api/license"}},
	}
	if err := preflightUnscopedClaims(okPlugins, routes); err != nil {
		t.Fatalf("expected declarations to pass preflight, got %v", err)
	}

	t.Run("declaration with no matching route", func(t *testing.T) {
		err := preflightUnscopedClaims([]plugin.Plugin{
			unscopedStubPlugin{name: "ghost", prefixes: []string{"/api/ghost/"}},
		}, routes)
		if err == nil || !strings.Contains(err.Error(), "/api/ghost/") {
			t.Fatalf("expected a startup error naming the declaration, got %v", err)
		}
	})

	t.Run("declaration captures another plugin's route", func(t *testing.T) {
		err := preflightUnscopedClaims([]plugin.Plugin{
			unscopedStubPlugin{name: "commerce", prefixes: []string{"/api/"}},
		}, routes)
		if err == nil {
			t.Fatal("expected a startup error for a route captured from another plugin")
		}
	})
}

func TestGatedMuxUnscopedDeclarationReachesHandler(t *testing.T) {
	matcher, err := newUnscopedMatcher([]plugin.Plugin{
		unscopedStubPlugin{name: "commerce", prefixes: []string{"/api/storefront/"}},
	})
	if err != nil {
		t.Fatalf("build matcher: %v", err)
	}
	enabled := func(r *http.Request, pluginName string) (bool, bool) {
		if isPlatformReservedPath(r.URL.Path) || matcher.Matches(r.URL.Path) {
			return false, false
		}
		return false, true
	}

	real := http.NewServeMux()
	gm := &gatedMux{real: real, plugin: "commerce", enabled: enabled}
	gm.HandleFunc("/api/storefront/checkout", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	gm.HandleFunc("/api/billing/config", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	real.ServeHTTP(rec, httptest.NewRequest("GET", "/api/storefront/checkout", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("declared unscoped route = %d, want 200", rec.Code)
	}

	rec = httptest.NewRecorder()
	real.ServeHTTP(rec, httptest.NewRequest("GET", "/api/billing/config", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("scoped route for a disabled plugin = %d, want 404", rec.Code)
	}

	err = preflightUnscopedClaims([]plugin.Plugin{
		unscopedStubPlugin{name: "commerce", prefixes: []string{"/api/storefront/"}},
	}, func() *routeRegistry {
		r := newRouteRegistry()
		r.claim("commerce", "POST /api/storefront/checkout", false)
		return r
	}())
	if err != nil {
		t.Fatalf("preflight: %v", err)
	}
}

func TestCollectUnscopedDeclarationsRejectsInvalid(t *testing.T) {
	if _, err := plugin.CollectUnscopedDeclarations([]plugin.Plugin{
		unscopedStubPlugin{name: "a", prefixes: []string{""}},
	}); !errors.Is(err, plugin.ErrUnscopedInvalid) {
		t.Fatalf("empty declaration error = %v, want ErrUnscopedInvalid", err)
	}

	if _, err := plugin.CollectUnscopedDeclarations([]plugin.Plugin{
		unscopedStubPlugin{name: "a", prefixes: []string{"/api/one/"}},
		unscopedStubPlugin{name: "b", prefixes: []string{"/api/one/"}},
	}); !errors.Is(err, plugin.ErrUnscopedDuplicate) {
		t.Fatalf("duplicate declaration error = %v, want ErrUnscopedDuplicate", err)
	}
}
