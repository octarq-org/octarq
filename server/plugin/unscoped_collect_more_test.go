package plugin

import (
	"errors"
	"testing"
)

type collectStub struct {
	prefixes []string
	name     string
}

func (p collectStub) Name() string        { return p.name }
func (p collectStub) Models() []any       { return nil }
func (p collectStub) Mount(Mux, *Context) {}
func (p collectStub) UnscopedPrefixes() []string {
	return p.prefixes
}

func TestCollectUnscopedDeclarationsMore(t *testing.T) {
	got, err := CollectUnscopedDeclarations([]Plugin{
		collectStub{name: "commerce", prefixes: []string{"/api/storefront", "/api/storefront"}},
		collectStub{name: "licensing", prefixes: []string{"/api/license"}},
	})
	if err != nil {
		t.Fatalf("CollectUnscopedDeclarations: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("declarations = %+v, want 2", got)
	}

	if _, err := CollectUnscopedDeclarations([]Plugin{
		collectStub{name: "commerce", prefixes: []string{"/api/storefront"}},
		collectStub{name: "licensing", prefixes: []string{"/api/storefront"}},
	}); !errors.Is(err, ErrUnscopedDuplicate) {
		t.Errorf("duplicate error = %v, want ErrUnscopedDuplicate", err)
	}

	if _, err := CollectUnscopedDeclarations([]Plugin{
		collectStub{name: "broken", prefixes: []string{"api/broken"}},
	}); !errors.Is(err, ErrUnscopedInvalid) {
		t.Errorf("invalid error = %v, want ErrUnscopedInvalid", err)
	}

	if got := UnscopedPrefixes(barePlugin{}); got != nil {
		t.Errorf("plain plugin prefixes = %v, want nil", got)
	}
}
