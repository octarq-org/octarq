package app

import (
	"net/http"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/plugin"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
}

func TestDuplicateRouteIsRefusedNotPanicked(t *testing.T) {
	routes := newRouteRegistry()
	real := http.NewServeMux()

	pro := &recordingMux{real: real, routes: routes, plugin: "commerce"}
	third := &recordingMux{real: real, routes: routes, plugin: "acme-shop"}

	pro.Handle("POST /api/products", okHandler())

	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("duplicate route reached ServeMux and panicked: %v", rec)
		}
	}()
	third.Handle("POST /api/products", okHandler())

	err := routes.Err()
	if err == nil {
		t.Fatal("expected a startup error for the duplicate route, got nil")
		return
	}
	for _, want := range []string{"/api/products", "commerce", "acme-shop"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q — an operator cannot tell which plugins collided", err, want)
		}
	}
}

func TestDistinctRoutesAreAccepted(t *testing.T) {
	routes := newRouteRegistry()
	real := http.NewServeMux()
	a := &recordingMux{real: real, routes: routes, plugin: "dns"}
	b := &recordingMux{real: real, routes: routes, plugin: "mail"}

	a.Handle("GET /api/domains", okHandler())
	a.HandleFunc("POST /api/domains", func(w http.ResponseWriter, r *http.Request) {})
	b.Handle("GET /api/emails", okHandler())

	if err := routes.Err(); err != nil {
		t.Fatalf("unexpected collision: %v", err)
	}
	for _, tc := range []struct{ method, path string }{
		{"GET", "/api/domains"}, {"POST", "/api/domains"}, {"GET", "/api/emails"},
	} {
		req, _ := http.NewRequest(tc.method, "http://x"+tc.path, nil)
		if h, pattern := real.Handler(req); h == nil || pattern == "" {
			t.Errorf("%s %s was not registered on the real mux", tc.method, tc.path)
		}
	}
}

func TestWhitespaceVariantsAreTheSameRoute(t *testing.T) {
	routes := newRouteRegistry()
	if !routes.claim("a", "GET /api/thing", false) {
		t.Fatal("first claim rejected")
	}
	if routes.claim("b", "  GET   /api/thing  ", false) {
		t.Error("whitespace variant was treated as a different route")
	}
	if routes.Err() == nil {
		t.Error("expected a collision error")
	}
}

func TestThirdPartyMustUseReservedNamespace(t *testing.T) {
	routes := newRouteRegistry()
	real := http.NewServeMux()
	third := &recordingMux{real: real, routes: routes, plugin: "acme", thirdParty: true}

	third.Handle("POST /api/products", okHandler())

	err := routes.Err()
	if err == nil {
		t.Fatal("third-party plugin claimed a bare /api noun without being refused")
		return
	}
	if !strings.Contains(err.Error(), "/api/x/acme") {
		t.Errorf("error %q does not tell the author which namespace to use", err)
	}
	req, _ := http.NewRequest("POST", "http://x/api/products", nil)
	if h, _ := real.Handler(req); h != nil {
		if _, pattern := real.Handler(req); pattern != "" {
			t.Error("the refused route was still registered on the real mux")
		}
	}
}

func TestThirdPartyInsideNamespaceIsAccepted(t *testing.T) {
	routes := newRouteRegistry()
	real := http.NewServeMux()
	third := &recordingMux{real: real, routes: routes, plugin: "acme", thirdParty: true}

	third.Handle("POST /api/x/acme/things", okHandler())
	third.Handle("GET /api/x/acme", okHandler())
	third.Handle("GET /acme-landing", okHandler())

	if err := routes.Err(); err != nil {
		t.Fatalf("legal third-party routes were refused: %v", err)
	}
}

func TestInTreePluginsKeepBareNouns(t *testing.T) {
	routes := newRouteRegistry()
	real := http.NewServeMux()
	inTree := &recordingMux{real: real, routes: routes, plugin: "dns", thirdParty: false}

	inTree.Handle("GET /api/domains", okHandler())
	inTree.Handle("GET /api/provider-accounts", okHandler())

	if err := routes.Err(); err != nil {
		t.Fatalf("in-tree bare nouns were refused: %v", err)
	}
}

func TestIsThirdPartyPkg(t *testing.T) {
	cases := []struct {
		pkg  string
		want bool
	}{
		{"github.com/octarq-org/octarq/server/plugins/help", false},
		{"github.com/octarq-org/octarq-extra/modules/commerce", false},
		{"github.com/acme/octarq-shop", true},
		{"example.com/internal/plugin", true},
		{"", false},
	}
	for _, c := range cases {
		if got := isThirdPartyPkg(c.pkg); got != c.want {
			t.Errorf("isThirdPartyPkg(%q) = %v, want %v", c.pkg, got, c.want)
		}
	}
}

func TestPluginIsThirdPartyUsesPackagePath(t *testing.T) {
	if pluginIsThirdParty(&stubRoutePlugin{}) {
		t.Error("an in-repo plugin was classified as third-party")
	}
}

func TestPatternPath(t *testing.T) {
	cases := map[string]string{
		"POST /api/products":      "/api/products",
		"/api/products":           "/api/products",
		"example.com/api/x/a/b":   "/api/x/a/b",
		"GET example.com/api/foo": "/api/foo",
		"nopath":                  "nopath",
	}
	for in, want := range cases {
		if got := patternPath(in); got != want {
			t.Errorf("patternPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPluginIsThirdParty_Nil(t *testing.T) {
	if pluginIsThirdParty(nil) {
		t.Errorf("expected false for nil plugin")
	}
}

type stubRoutePlugin struct{}

func (*stubRoutePlugin) Name() string                      { return "stub" }
func (*stubRoutePlugin) Models() []any                     { return nil }
func (*stubRoutePlugin) Mount(plugin.Mux, *plugin.Context) {}
