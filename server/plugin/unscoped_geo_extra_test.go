package plugin

import (
	"errors"
	"testing"
)

type geoSlicePlugin struct {
	name     string
	prefixes []string
}

func (p geoSlicePlugin) Name() string        { return p.name }
func (p geoSlicePlugin) Models() []any       { return nil }
func (p geoSlicePlugin) Mount(Mux, *Context) {}
func (p geoSlicePlugin) UnscopedPrefixes() []string {
	return p.prefixes
}

func TestCollectUnscopedDeclarationsGeoSlice(t *testing.T) {
	got, err := CollectUnscopedDeclarations([]Plugin{
		geoSlicePlugin{name: "commerce", prefixes: []string{"/api/storefront", "/api/storefront"}},
		geoSlicePlugin{name: "licensing", prefixes: []string{"/api/license"}},
	})
	if err != nil || len(got) != 2 {
		t.Fatalf("CollectUnscopedDeclarations = (%+v, %v)", got, err)
	}
	if _, err := CollectUnscopedDeclarations([]Plugin{
		geoSlicePlugin{name: "a", prefixes: []string{"/api/storefront"}},
		geoSlicePlugin{name: "b", prefixes: []string{"/api/storefront"}},
	}); !errors.Is(err, ErrUnscopedDuplicate) {
		t.Errorf("duplicate = %v", err)
	}
	if _, err := CollectUnscopedDeclarations([]Plugin{
		geoSlicePlugin{name: "broken", prefixes: []string{"api/broken"}},
	}); !errors.Is(err, ErrUnscopedInvalid) {
		t.Errorf("invalid = %v", err)
	}
}
