package plugin

import (
	"errors"
	"testing"
)

type aiSlicePlugin struct {
	name     string
	prefixes []string
}

func (p aiSlicePlugin) Name() string        { return p.name }
func (p aiSlicePlugin) Models() []any       { return nil }
func (p aiSlicePlugin) Mount(Mux, *Context) {}
func (p aiSlicePlugin) UnscopedPrefixes() []string {
	return p.prefixes
}

func TestCollectUnscopedDeclarationsAiSlice(t *testing.T) {
	got, err := CollectUnscopedDeclarations([]Plugin{
		aiSlicePlugin{name: "commerce", prefixes: []string{"/api/storefront", "/api/storefront"}},
		aiSlicePlugin{name: "licensing", prefixes: []string{"/api/license"}},
	})
	if err != nil || len(got) != 2 {
		t.Fatalf("CollectUnscopedDeclarations = (%+v, %v)", got, err)
	}
	if _, err := CollectUnscopedDeclarations([]Plugin{
		aiSlicePlugin{name: "a", prefixes: []string{"/api/storefront"}},
		aiSlicePlugin{name: "b", prefixes: []string{"/api/storefront"}},
	}); !errors.Is(err, ErrUnscopedDuplicate) {
		t.Errorf("duplicate = %v, want ErrUnscopedDuplicate", err)
	}
	if _, err := CollectUnscopedDeclarations([]Plugin{
		aiSlicePlugin{name: "broken", prefixes: []string{"api/broken"}},
	}); !errors.Is(err, ErrUnscopedInvalid) {
		t.Errorf("invalid = %v, want ErrUnscopedInvalid", err)
	}
}
