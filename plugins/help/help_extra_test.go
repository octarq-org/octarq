package help

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/plugin"
)

func TestPluginMetadataAndLifecycle(t *testing.T) {
	p := New()
	if p.Name() != "help" {
		t.Errorf("expected name 'help', got %q", p.Name())
	}
	desc := p.Describe()
	if desc.Title != "Help" || !desc.Core {
		t.Errorf("unexpected Describe output: %+v", desc)
	}
	if p.Models() != nil {
		t.Errorf("expected Models to be nil, got %v", p.Models())
	}
	menus := p.Menus()
	if len(menus) != 1 || menus[0].ID != "help" || menus[0].Category != "footer" {
		t.Errorf("unexpected Menus output: %+v", menus)
	}
	if p.HelpDocsFS() == nil {
		t.Error("expected non-nil HelpDocsFS")
	}

	// Test Mount
	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("Help API", "1.0.0"))
	pctx := &plugin.Context{
		Huma: api,
		OrgID: func(r *http.Request) uint {
			return 1
		},
	}
	p.Mount(nil, pctx)
}

func TestListCategoriesEndpoint(t *testing.T) {
	p := New()
	input := &ListCategoriesInput{}
	req := httptest.NewRequest("GET", "/api/help/categories", nil)
	humaCtx := humago.NewContext(nil, req, nil)
	if errs := input.Resolve(humaCtx); len(errs) > 0 {
		t.Fatalf("Resolve failed: %v", errs)
	}

	out, err := p.listCategories(context.Background(), input)
	if err != nil {
		t.Fatalf("listCategories error: %v", err)
	}
	expectedCats := plugin.HelpCategories()
	if len(out.Body) != len(expectedCats) {
		t.Fatalf("expected %d categories, got %d", len(expectedCats), len(out.Body))
	}
	for i, c := range out.Body {
		if c.Key != expectedCats[i].Key {
			t.Errorf("category %d key mismatch: got %q, want %q", i, c.Key, expectedCats[i].Key)
		}
	}
}

func TestNilHumaContextHandling(t *testing.T) {
	p := New()
	_, err := p.listDocs(context.Background(), &ListDocsInput{Ctx: nil})
	if err == nil {
		t.Error("expected error for nil huma context in listDocs, got nil")
	}
	_, err = p.getDoc(context.Background(), &GetDocInput{Ctx: nil, Slug: "intro"})
	if err == nil {
		t.Error("expected error for nil huma context in getDoc, got nil")
	}
}

func TestGetDocNotFound(t *testing.T) {
	p := New()
	p.pctx = &plugin.Context{
		OrgID:         func(r *http.Request) uint { return 1 },
		ActivePlugins: func() []plugin.Plugin { return nil },
		PluginActive:  func(uint, plugin.Plugin) bool { return true },
	}

	req := httptest.NewRequest("GET", "/api/help/docs/non-existent-slug-xyz", nil)
	humaCtx := humago.NewContext(nil, req, nil)
	input := &GetDocInput{Slug: "non-existent-slug-xyz"}
	input.Resolve(humaCtx)

	_, err := p.getDoc(context.Background(), input)
	if err == nil {
		t.Error("expected 404 error for non-existent doc, got nil")
	}
}
