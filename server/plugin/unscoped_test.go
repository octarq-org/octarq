package plugin_test

import (
	"errors"
	"testing"

	"github.com/octarq-org/octarq/server/plugin"
)

func TestValidateUnscopedDeclaration(t *testing.T) {
	for _, ok := range []string{"/api/license/verify", "/api/license/", "/api/storefront", "/oauth/callback"} {
		if err := plugin.ValidateUnscopedDeclaration(ok); err != nil {
			t.Errorf("ValidateUnscopedDeclaration(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{
		"",
		" ",
		"/",
		"/api",
		"/api/",
		"/api/license/verify ",
		" /api/license",
		"/api/space here",
		"/api/{id}",
		"/api/*",
		"/api/x?",
		"api/x",
		"/api//x",
	} {
		if err := plugin.ValidateUnscopedDeclaration(bad); !errors.Is(err, plugin.ErrUnscopedInvalid) {
			t.Errorf("ValidateUnscopedDeclaration(%q) = %v, want ErrUnscopedInvalid", bad, err)
		}
	}
}

type unscopedPlugin struct {
	name     string
	prefixes []string
}

func (p unscopedPlugin) Name() string                      { return p.name }
func (p unscopedPlugin) Models() []any                     { return nil }
func (p unscopedPlugin) Mount(plugin.Mux, *plugin.Context) {}
func (p unscopedPlugin) UnscopedPrefixes() []string        { return p.prefixes }

func TestUnscopedPrefixesOfPlainPlugin(t *testing.T) {
	if got := plugin.UnscopedPrefixes(namedPlugin{name: "plain"}); got != nil {
		t.Errorf("UnscopedPrefixes = %v, want nil", got)
	}
}

func TestCollectUnscopedDeclarations(t *testing.T) {
	got, err := plugin.CollectUnscopedDeclarations([]plugin.Plugin{
		unscopedPlugin{name: "commerce", prefixes: []string{"/api/storefront", "/api/storefront"}},
		unscopedPlugin{name: "licensing", prefixes: []string{"/api/license"}},
	})
	if err != nil {
		t.Fatalf("CollectUnscopedDeclarations: %v", err)
	}
	if len(got) != 2 || got[0].Path != "/api/storefront" || got[1].Path != "/api/license" {
		t.Errorf("declarations = %+v", got)
	}

	if _, err := plugin.CollectUnscopedDeclarations([]plugin.Plugin{
		unscopedPlugin{name: "commerce", prefixes: []string{"/api/storefront"}},
		unscopedPlugin{name: "licensing", prefixes: []string{"/api/storefront"}},
	}); !errors.Is(err, plugin.ErrUnscopedDuplicate) {
		t.Errorf("cross-plugin duplicate error = %v, want ErrUnscopedDuplicate", err)
	}

	if _, err := plugin.CollectUnscopedDeclarations([]plugin.Plugin{
		unscopedPlugin{name: "broken", prefixes: []string{"api/broken"}},
	}); !errors.Is(err, plugin.ErrUnscopedInvalid) {
		t.Errorf("invalid declaration error = %v, want ErrUnscopedInvalid", err)
	}
}
