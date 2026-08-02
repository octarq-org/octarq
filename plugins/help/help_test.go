package help

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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
			{Slug: "doc-a", Title: "Doc A", Markdown: "Hello <b>world</b>"},
		},
	}
	pluginB := &mockPlugin{
		name: "plugin-b",
		helpDocs: []plugin.HelpDoc{
			{Slug: "doc-a", Title: "Doc A Shadow", Markdown: "Collision"},
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
		FeatureActive: func(orgID uint, featureKey string) bool {
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

	// 2. Org 2 (disabled) sees no docs for disabled plugins
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

func TestHelpDocsCategorySortingAndTranslation(t *testing.T) {
	// Create docs in 3 different categories with intentionally reversed Order to ensure category order wins
	docStart := plugin.HelpDoc{
		Slug:     "start-doc",
		Title:    "Start Doc",
		Category: "start",
		Order:    50,
	}
	docAutomation := plugin.HelpDoc{
		Slug:     "auto-doc",
		Title:    "Auto Doc",
		Category: "automation",
		Order:    10,
	}
	docServices := plugin.HelpDoc{
		Slug:     "service-doc",
		Title:    "Service Doc",
		Category: "services",
		Order:    1,
		Translations: map[string]plugin.HelpDocTranslation{
			"zh": {
				Title:    "服务文档",
				Category: "licensing",
				Markdown: "中文正文",
			},
		},
	}

	p := &mockPlugin{
		name:     "test-plugin",
		helpDocs: []plugin.HelpDoc{docServices, docAutomation, docStart},
	}

	pctx := &plugin.Context{
		ActivePlugins: func() []plugin.Plugin { return []plugin.Plugin{p} },
		PluginActive:  func(uint, plugin.Plugin) bool { return true },
		OrgID:         func(*http.Request) uint { return 1 },
	}

	h := New()
	h.pctx = pctx

	// Test 1: Category sorting order (start [10] < automation [30] < services [40])
	docs := h.getDocs(1, "en")
	if len(docs) != 3 {
		t.Fatalf("expected 3 docs, got %d", len(docs))
	}
	if docs[0].Slug != "start-doc" || docs[1].Slug != "auto-doc" || docs[2].Slug != "service-doc" {
		t.Errorf("incorrect doc ordering: got %s, %s, %s", docs[0].Slug, docs[1].Slug, docs[2].Slug)
	}

	// Test 2: zh-CN matches "zh" translation and updates Title & Category
	docsZh := h.getDocs(1, "zh-CN")
	var matchedService *plugin.HelpDoc
	for _, d := range docsZh {
		if d.Slug == "service-doc" {
			matchedService = &d
			break
		}
	}
	if matchedService == nil {
		t.Fatalf("service-doc not found in zh-CN query")
	}
	if matchedService.Title != "服务文档" {
		t.Errorf("expected translated title '服务文档', got %q", matchedService.Title)
	}
	if matchedService.Category != "licensing" {
		t.Errorf("expected translated category 'licensing', got %q", matchedService.Category)
	}
	if matchedService.Markdown != "中文正文" {
		t.Errorf("expected translated markdown '中文正文', got %q", matchedService.Markdown)
	}
}

func TestParseHelpDocSafeFallback(t *testing.T) {
	badYAML := `---
title: [invalid yaml
---
Body Content`

	doc := plugin.ParseHelpDocSafe(badYAML)
	if doc.Markdown == "" {
		t.Errorf("expected fallback doc with markdown body, got empty string")
	}
}

// TestBundledDocsHaveTranslations guards the content/ naming contract. Nothing
// else can: a page that ships without its zh half still compiles, still parses,
// and still serves — it just silently renders English to a Chinese reader, which
// is exactly how the Pro help corpus ended up 100% English. The same loop also
// catches the failure modes that are valid strings and therefore invisible to
// the compiler: a missing title, a category outside the closed set, a duplicate
// slug shadowing another page.
func TestBundledDocsHaveTranslations(t *testing.T) {
	entries, err := content.ReadDir("content")
	if err != nil {
		t.Fatalf("read embedded content dir: %v", err)
	}

	valid := make(map[string]bool)
	for _, c := range plugin.HelpCategories() {
		valid[c.Key] = true
	}

	pages := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".mdx") || strings.HasSuffix(name, ".zh.mdx") {
			continue
		}
		pages++
		base := strings.TrimSuffix(name, ".mdx")

		if _, err := content.ReadFile("content/" + base + ".zh.mdx"); err != nil {
			t.Errorf("%s has no Chinese translation: expected content/%s.zh.mdx", name, base)
		}

		raw, err := content.ReadFile("content/" + name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		doc, err := plugin.ParseHelpDoc(string(raw))
		if err != nil {
			t.Errorf("%s has unparseable frontmatter: %v", name, err)
			continue
		}
		if doc.Title == "" {
			t.Errorf("%s has no title in its frontmatter", name)
		}
		if !valid[doc.Category] {
			t.Errorf("%s declares category %q, which is not one of the closed set in plugin.HelpCategories()", name, doc.Category)
		}
	}

	if pages == 0 {
		t.Fatal("no docs found in content/ — the loader would serve nothing")
	}

	// The aggregator only de-duplicates slugs ACROSS plugins (by prefixing the
	// loser). Two pages colliding inside this one plugin would silently drop one.
	seen := make(map[string]string)
	for _, d := range parsedHelpDocs() {
		if prev, dup := seen[d.Slug]; dup {
			t.Errorf("slug %q is claimed by both %q and %q", d.Slug, prev, d.Title)
		}
		seen[d.Slug] = d.Title
	}
}
