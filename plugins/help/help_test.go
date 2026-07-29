package help

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/plugin"
)

type mockPlugin struct {
	name     string
	helpDocs []plugin.HelpDoc
}

func (m *mockPlugin) Name() string                              { return m.name }
func (m *mockPlugin) Models() []any                             { return nil }
func (m *mockPlugin) Mount(mux plugin.Mux, ctx *plugin.Context) {}

func (m *mockPlugin) HelpDocs() []plugin.HelpDoc {
	return m.helpDocs
}

type noHelpPlugin struct {
	name string
}

func (m *noHelpPlugin) Name() string                              { return m.name }
func (m *noHelpPlugin) Models() []any                             { return nil }
func (m *noHelpPlugin) Mount(mux plugin.Mux, ctx *plugin.Context) {}

func TestHelpDocs(t *testing.T) {
	org1Enabled := true
	org2Enabled := false

	pluginA := &mockPlugin{
		name: "plugin-a",
		helpDocs: []plugin.HelpDoc{
			{Slug: "doc-a", Title: "Doc A", Group: "A", Markdown: "Hello <b>world</b>"},
		},
	}
	pluginB := &mockPlugin{
		name: "plugin-b",
		helpDocs: []plugin.HelpDoc{
			{Slug: "doc-a", Title: "Doc A Shadow", Group: "B", Markdown: "Collision"},
		},
	}
	noHelp := &noHelpPlugin{name: "no-help"}

	allPlugins := []plugin.Plugin{pluginA, pluginB, noHelp}

	pctx := &plugin.Context{
		ActivePlugins: func() []plugin.Plugin {
			return allPlugins
		},
		PluginActive: func(orgID uint, p plugin.Plugin) bool {
			if orgID == 1 {
				return org1Enabled
			}
			if orgID == 2 {
				return org2Enabled
			}
			return true
		},
		OrgID: func(r *http.Request) uint {
			// Stub orgID based on Header
			if r.Header.Get("X-Org") == "2" {
				return 2
			}
			return 1
		},
		RequireRole: func(*http.Request, string) bool { return true },
	}

	h := New()
	h.pctx = pctx

	getDocs := func(orgID uint) []plugin.HelpDoc {
		req := httptest.NewRequest("GET", "/api/help/docs", nil)
		if orgID == 2 {
			req.Header.Set("X-Org", "2")
		} else {
			req.Header.Set("X-Org", "1")
		}
		humaCtx := humago.NewContext(nil, req, nil)
		input := &ListDocsInput{}
		input.Resolve(humaCtx)
		out, err := h.listDocs(context.Background(), input)
		if err != nil {
			t.Fatalf("listDocs error: %v", err)
		}
		// Map DocMeta to HelpDoc for easy comparison
		res := make([]plugin.HelpDoc, len(out.Body))
		for i, d := range out.Body {
			res[i] = plugin.HelpDoc{Slug: d.Slug, Title: d.Title}
		}
		return res
	}

	// 1. Org 1 (enabled) sees both plugins, with deterministic slug resolution
	docs1 := getDocs(1)
	if len(docs1) != 2 {
		t.Fatalf("expected 2 docs, got %d", len(docs1))
	}
	// Slug collision: plugin-b's doc-a should become plugin-b-doc-a (or plugin-a-doc-a depending on order)
	// Because ActivePlugins returns [pluginA, pluginB], A gets "doc-a" and B gets "plugin-b-doc-a"
	hasDocA := false
	hasDocBCollision := false
	for _, d := range docs1 {
		if d.Slug == "doc-a" {
			hasDocA = true
		}
		if d.Slug == "plugin-b-doc-a" {
			hasDocBCollision = true
		}
	}
	if !hasDocA || !hasDocBCollision {
		t.Errorf("slug collision not resolved deterministically: %v", docs1)
	}

	// 2. Org 2 (disabled) sees no docs
	docs2 := getDocs(2)
	if len(docs2) != 0 {
		t.Errorf("expected 0 docs for org 2, got %d", len(docs2))
	}

	// 3. Raw HTML is escaped
	req := httptest.NewRequest("GET", "/api/help/docs/doc-a", nil)
	humaCtx := humago.NewContext(nil, req, nil)
	getInput := &GetDocInput{Slug: "doc-a"}
	getInput.Resolve(humaCtx)
	docOut, err := h.getDoc(context.Background(), getInput)
	if err != nil {
		t.Fatalf("getDoc error: %v", err)
	}
	html := docOut.Body.HTML
	if html != "<p>Hello <!-- raw HTML omitted -->world<!-- raw HTML omitted --></p>\n" {
		t.Errorf("HTML not escaped properly: %q", html)
	}
}
