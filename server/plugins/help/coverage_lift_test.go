package help

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/server/plugin"
)

type dummyDescriberPlugin struct {
	plugin.Plugin
}

func (d *dummyDescriberPlugin) Name() string { return "dummy" }

func (d *dummyDescriberPlugin) Describe() plugin.Info {
	return plugin.Info{
		Title:    "Dummy",
		Category: "services",
	}
}

func (d *dummyDescriberPlugin) HelpDocs() []plugin.HelpDoc {
	return []plugin.HelpDoc{
		{
			Slug:    "feat-doc",
			Title:   "Feature Doc",
			Feature: "disabled_feature",
		},
		{
			Slug:  "lang-doc",
			Title: "Language Doc",
			Translations: map[string]plugin.HelpDocTranslation{
				"zh": {
					Title:    "中文标题",
					Markdown: "# 中文内容",
				},
			},
		},
	}
}

func TestCoverageLift_HelpPluginHost(t *testing.T) {
	p := New()
	th := &plugin.TestHost{}
	p.SetHost(th)
	if p.Host() != th {
		t.Fatalf("expected host %v, got %v", th, p.Host())
	}
}

func TestCoverageLift_ParseAsideOpenerEmpty(t *testing.T) {
	kind, title, ok := parseAsideOpener(":::   ")
	if ok || kind != "" || title != "" {
		t.Fatalf("expected false for whitespace aside opener, got ok=%v, kind=%s, title=%s", ok, kind, title)
	}
}

func TestCoverageLift_GetDocsBranches(t *testing.T) {
	p := New()
	dummy := &dummyDescriberPlugin{}
	p.pctx = &plugin.Context{
		ActivePlugins: func() []plugin.Plugin {
			return []plugin.Plugin{dummy}
		},
		PluginActive: func(orgID uint, pl plugin.Plugin) bool {
			return true
		},
		FeatureActive: func(orgID uint, feat string) bool {
			if feat == "disabled_feature" {
				return false
			}
			return true
		},
		Host: &plugin.TestHost{
			SessionMock: &plugin.TestSession{
				OrgIDFn: func(r *http.Request) uint { return 1 },
			},
		},
	}

	docs := p.getDocs(1, "zh")
	if len(docs) != 1 {
		t.Fatalf("expected 1 doc (feat-doc skipped), got %d", len(docs))
	}
	if docs[0].Title != "中文标题" {
		t.Fatalf("expected title 中文标题, got %q", docs[0].Title)
	}
	if docs[0].Category != "services" {
		t.Fatalf("expected category services from Describer, got %q", docs[0].Category)
	}

	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("Help API", "1.0.0"))
	p.pctx.Huma = api

	req := httptest.NewRequest("GET", "/api/help/docs/lang-doc?lang=zh", nil)
	humaCtx := humago.NewContext(nil, req, nil)
	input := &GetDocInput{Slug: "lang-doc"}
	if errs := input.Resolve(humaCtx); len(errs) > 0 {
		t.Fatalf("Resolve failed: %v", errs)
	}
	out, err := p.getDoc(context.Background(), input)
	if err != nil {
		t.Fatalf("getDoc failed: %v", err)
	}
	if out.Body.Title != "中文标题" {
		t.Fatalf("expected translated title, got %q", out.Body.Title)
	}
}
