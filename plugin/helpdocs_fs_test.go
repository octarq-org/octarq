package plugin

import (
	"testing"
	"testing/fstest"
)

func docsFor(t *testing.T, fsys fstest.MapFS) map[string]HelpDoc {
	t.Helper()
	out := make(map[string]HelpDoc)
	for _, d := range LoadHelpDocs(fsys) {
		out[d.Slug] = d
	}
	return out
}

func TestLoadHelpDocsFileNameIsTheSlug(t *testing.T) {
	docs := docsFor(t, fstest.MapFS{
		"docs/webhooks.mdx": {Data: []byte("---\ntitle: Webhooks\ncategory: automation\n---\nbody")},
	})
	if len(docs) != 1 {
		t.Fatalf("expected 1 doc, got %d", len(docs))
	}
	d := docs["webhooks"]
	if d.Title != "Webhooks" || d.Category != "automation" {
		t.Errorf("frontmatter not applied: %+v", d)
	}
	if d.Markdown != "body" {
		t.Errorf("markdown = %q, want %q", d.Markdown, "body")
	}
}

func TestLoadHelpDocsFrontmatterOverridesSlug(t *testing.T) {
	docs := docsFor(t, fstest.MapFS{
		"docs/legacy.md": {Data: []byte("---\nslug: renamed\ntitle: T\n---\nbody")},
	})
	if _, ok := docs["renamed"]; !ok {
		t.Errorf("explicit slug not honoured, got %v", docs)
	}
}

// A page with no frontmatter at all is the shape most of octarq-pro's module
// docs shipped in, so it must degrade to something serveable rather than an
// untitled ghost entry.
func TestLoadHelpDocsBareMarkdownFallsBackToFileName(t *testing.T) {
	docs := docsFor(t, fstest.MapFS{
		"docs/infra.md": {Data: []byte("# Infra\n\nbody")},
	})
	d, ok := docs["infra"]
	if !ok {
		t.Fatalf("bare markdown page dropped: %v", docs)
	}
	if d.Title != "infra" {
		t.Errorf("title = %q, want the slug as fallback", d.Title)
	}
}

// The one thing the naming contract must never get wrong: a translation is not
// a page. Getting this backwards puts every doc in the sidebar twice, once per
// language, which is what makes it worth a test of its own.
func TestLoadHelpDocsTranslationsAreNotPages(t *testing.T) {
	docs := docsFor(t, fstest.MapFS{
		"docs/mcp.mdx":    {Data: []byte("---\ntitle: MCP\n---\nen")},
		"docs/mcp.zh.mdx": {Data: []byte("---\ntitle: MCP 服务器\n---\nzh")},
	})
	if len(docs) != 1 {
		t.Fatalf("expected 1 page, got %d: %v", len(docs), docs)
	}
	tr, ok := docs["mcp"].Translations["zh"]
	if !ok {
		t.Fatal("zh sibling was not attached as a translation")
	}
	if tr.Title != "MCP 服务器" || tr.Markdown != "zh" {
		t.Errorf("translation not parsed: %+v", tr)
	}
}

// ".zh" is a language; ".v2" is part of the name. Only the closed list of
// language suffixes may steal a file's page-hood.
func TestLoadHelpDocsDottedNameIsNotATranslation(t *testing.T) {
	docs := docsFor(t, fstest.MapFS{
		"docs/api.v2.mdx": {Data: []byte("---\ntitle: API v2\n---\nbody")},
	})
	if _, ok := docs["api.v2"]; !ok {
		t.Errorf("api.v2.mdx was mistaken for a translation: %v", docs)
	}
}

// Subdirectories organise files without appearing in any URL, and .md/.mdx are
// interchangeable — octarq-pro writes .md, the OSS plugins write .mdx.
func TestLoadHelpDocsWalksSubdirectoriesAndBothExtensions(t *testing.T) {
	docs := docsFor(t, fstest.MapFS{
		"docs/top.mdx":              {Data: []byte("---\ntitle: Top\n---\na")},
		"docs/nested/deep.md":       {Data: []byte("---\ntitle: Deep\n---\nb")},
		"docs/nested/deep.zh.md":    {Data: []byte("zh body")},
		"docs/nested/more/leaf.mdx": {Data: []byte("---\ntitle: Leaf\n---\nc")},
	})
	for _, slug := range []string{"top", "deep", "leaf"} {
		if _, ok := docs[slug]; !ok {
			t.Errorf("missing %q, got %v", slug, docs)
		}
	}
	if docs["deep"].Translations["zh"].Markdown != "zh body" {
		t.Errorf("translation not paired inside a subdirectory: %+v", docs["deep"])
	}
}

// One unparseable page must not take the rest of the plugin's docs down with
// it — a help system that fails closed on a typo is worse than one that serves
// nine pages and logs about the tenth.
func TestLoadHelpDocsSurvivesABadPage(t *testing.T) {
	docs := docsFor(t, fstest.MapFS{
		"docs/good.mdx": {Data: []byte("---\ntitle: Good\n---\nok")},
		"docs/bad.mdx":  {Data: []byte("---\ntitle: [unterminated\n---\nbody")},
	})
	if _, ok := docs["good"]; !ok {
		t.Errorf("a malformed sibling took down the whole directory: %v", docs)
	}
}

func TestLoadHelpDocsNilFS(t *testing.T) {
	if got := LoadHelpDocs(nil); got != nil {
		t.Errorf("LoadHelpDocs(nil) = %v, want nil", got)
	}
}

func TestLoadHelpDocsSorted(t *testing.T) {
	got := LoadHelpDocs(fstest.MapFS{
		"docs/b.mdx": {Data: []byte("---\ntitle: B\ncategory: services\norder: 1\n---\n")},
		"docs/a.mdx": {Data: []byte("---\ntitle: A\ncategory: start\norder: 99\n---\n")},
	})
	if len(got) != 2 || got[0].Slug != "a" {
		// "start" sorts before "services" regardless of per-doc order.
		t.Errorf("docs not sorted by category then order: %v", got)
	}
}
