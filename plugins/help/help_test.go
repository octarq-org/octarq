package help

import (
	"context"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/plugin"
	"github.com/octarq-org/octarq/plugins/dns"
	"github.com/octarq-org/octarq/plugins/links"
	"github.com/octarq-org/octarq/plugins/mail"
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
	docOperations := plugin.HelpDoc{
		Slug:     "auto-doc",
		Title:    "Operations Doc",
		Category: "operations",
		Order:    10,
	}
	docInfra := plugin.HelpDoc{
		Slug:     "service-doc",
		Title:    "Infra Doc",
		Category: "infrastructure",
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
		helpDocs: []plugin.HelpDoc{docInfra, docOperations, docStart},
	}

	pctx := &plugin.Context{
		ActivePlugins: func() []plugin.Plugin { return []plugin.Plugin{p} },
		PluginActive:  func(uint, plugin.Plugin) bool { return true },
		OrgID:         func(*http.Request) uint { return 1 },
	}

	h := New()
	h.pctx = pctx

	// Test 1: Category sorting order (start [10] < operations [20] < infrastructure [30])
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
	if matchedService == nil { //nolint:staticcheck // SA5011 false positive: t.Fatalf above is noreturn
		t.Fatalf("service-doc not found in zh-CN query")
	}
	if matchedService.Title != "服务文档" { //nolint:staticcheck // SA5011 false positive: t.Fatalf above is noreturn
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

// bundledDocsProviders is every plugin in this build that ships documentation
// through the docs-directory convention. Adding a plugin here is the one manual
// step the convention does not remove — a plugin whose docs/ nobody lists is
// still served correctly at runtime (the aggregator finds it by interface), it
// just is not held to the checks below.
func bundledDocsProviders() map[string]plugin.HelpDocsFS {
	return map[string]plugin.HelpDocsFS{
		"help":  New(),
		"links": links.New(),
		"mail":  mail.New(),
		"dns":   dns.New(),
	}
}

// TestBundledDocsHaveTranslations guards the docs/ naming contract for every
// plugin in the OSS build. Nothing else can: a page that ships without its zh
// half still compiles, still parses, and still serves — it just silently renders
// English to a Chinese reader, which is exactly how the Pro help corpus ended up
// 100% English. The same loop catches the other failure modes that are valid
// strings and therefore invisible to the compiler: a missing title, a category
// outside the closed set, a duplicate slug shadowing another page.
//
// This walks the embedded FS directly rather than the parsed HelpDocs, because
// the specific regression it exists to catch — a page dropped in with no
// translation — produces a perfectly well-formed HelpDoc.
func TestBundledDocsHaveTranslations(t *testing.T) {
	valid := make(map[string]bool)
	for _, c := range plugin.HelpCategories() {
		valid[c.Key] = true
	}

	for name, p := range bundledDocsProviders() {
		fsys := p.HelpDocsFS()
		pages := 0

		err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			ext := filepath.Ext(d.Name())
			if ext != ".md" && ext != ".mdx" {
				return nil
			}
			base := strings.TrimSuffix(d.Name(), ext)
			// Translations are checked via their English page, not as pages.
			if filepath.Ext(base) == ".zh" {
				return nil
			}
			pages++

			zh := strings.TrimSuffix(path, ext) + ".zh" + ext
			if _, err := fs.ReadFile(fsys, zh); err != nil {
				t.Errorf("%s: %s has no Chinese translation (expected %s)", name, path, zh)
			}

			raw, err := fs.ReadFile(fsys, path)
			if err != nil {
				t.Fatalf("%s: read %s: %v", name, path, err)
			}
			doc, err := plugin.ParseHelpDoc(string(raw))
			if err != nil {
				t.Errorf("%s: %s has unparseable frontmatter: %v", name, path, err)
				return nil
			}
			if doc.Title == "" {
				t.Errorf("%s: %s has no title in its frontmatter", name, path)
			}
			if !valid[doc.Category] {
				t.Errorf("%s: %s declares category %q, which is not one of the closed set in plugin.HelpCategories()", name, path, doc.Category)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("%s: walk docs: %v", name, err)
		}
		if pages == 0 {
			t.Errorf("%s: no docs found — the plugin implements HelpDocsFS but serves nothing", name)
		}
	}
}

// TestEveryCategoryHasAtLeastOneDoc asserts that every category in plugin.HelpCategories()
// has at least one document assigned to it across all bundled plugins.
func TestEveryCategoryHasAtLeastOneDoc(t *testing.T) {
	categories := plugin.HelpCategories()
	categoryCounts := make(map[string]int)
	for _, c := range categories {
		categoryCounts[c.Key] = 0
	}

	for name, p := range bundledDocsProviders() {
		fsys := p.HelpDocsFS()
		err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			ext := filepath.Ext(d.Name())
			if ext != ".md" && ext != ".mdx" {
				return nil
			}
			base := strings.TrimSuffix(d.Name(), ext)
			if filepath.Ext(base) == ".zh" {
				return nil
			}
			raw, err := fs.ReadFile(fsys, path)
			if err != nil {
				t.Fatalf("%s: read %s: %v", name, path, err)
			}
			doc, err := plugin.ParseHelpDoc(string(raw))
			if err != nil {
				return nil
			}
			categoryCounts[doc.Category]++
			return nil
		})
		if err != nil {
			t.Fatalf("%s: walk docs: %v", name, err)
		}
	}

	for _, c := range categories {
		// commerce and licensing are contributed by Pro plugins in a composed build.
		if c.Key == "commerce" || c.Key == "licensing" {
			continue
		}
		if categoryCounts[c.Key] == 0 {
			t.Errorf("category %q has no documents — all categories in plugin.HelpCategories() must have at least one doc", c.Key)
		}
	}
}

// TestBundledDocSlugsAreUnique pins the property the aggregator cannot fix. It
// de-duplicates slugs ACROSS plugins by prefixing the loser, so a collision
// there degrades to an ugly URL; two pages colliding INSIDE one plugin, or a
// slug two plugins both want, silently costs a page its canonical /help/<slug>.
func TestBundledDocSlugsAreUnique(t *testing.T) {
	seen := make(map[string]string)
	for name, p := range bundledDocsProviders() {
		for _, d := range plugin.LoadHelpDocs(p.HelpDocsFS()) {
			if prev, dup := seen[d.Slug]; dup {
				t.Errorf("slug %q is claimed by both %s and %s", d.Slug, prev, name)
			}
			seen[d.Slug] = name
		}
	}
}

// TestFilenameIsTheSlug pins the half of the convention a reader relies on: the
// URL /help/<slug> is predictable from the file tree. Frontmatter may still
// override the slug — LoadHelpDocs honours it — but nothing in this build should
// need to, and a page that quietly did would be findable only by grep.
func TestFilenameIsTheSlug(t *testing.T) {
	for name, p := range bundledDocsProviders() {
		fsys := p.HelpDocsFS()
		for _, d := range plugin.LoadHelpDocs(fsys) {
			found := false
			for _, ext := range []string{".md", ".mdx"} {
				if _, err := fs.ReadFile(fsys, filepath.Join("docs", d.Slug+ext)); err == nil {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("%s: doc %q (%s) has no docs/%s.md(x) — its slug comes from frontmatter, so the file name no longer predicts the URL", name, d.Title, d.Slug, d.Slug)
			}
			// A slug ending in a language tag means a translation was served as
			// a page: every doc then appears twice in the sidebar, once per
			// language. LoadHelpDocs recognises a closed list of suffixes, so
			// this is what shipping a locale it has not been taught looks like.
			if ext := filepath.Ext(d.Slug); ext != "" && len(ext) <= 4 {
				t.Errorf("%s: slug %q ends in %q — if that is a language tag, add it to helpDocLangs so the file is treated as a translation rather than a page of its own", name, d.Slug, ext)
			}
		}
	}
}

// docsFSPlugin is a plugin whose docs come from the directory convention.
type docsFSPlugin struct {
	name string
	fsys fs.FS
}

func (m *docsFSPlugin) Name() string                              { return m.name }
func (m *docsFSPlugin) Models() []any                             { return nil }
func (m *docsFSPlugin) Mount(mux plugin.Mux, ctx *plugin.Context) {}
func (m *docsFSPlugin) HelpDocsFS() fs.FS                         { return m.fsys }

// TestSameNamedPluginsKeepTheirOwnDocs covers the case of two mounted plugins
// answering the same Name(). That is deliberate — the core help plugin and a
// downstream help module can be two halves of one feature and share a feature
// key so one toggle governs both — so the docs cache must not treat the name as
// an identity. When it did, the second half read back the first half's pages:
// every core doc was served twice (the duplicate shadow-renamed to help-<slug> by
// the slug dedup) and the downstream-only page vanished.
func TestSameNamedPluginsKeepTheirOwnDocs(t *testing.T) {
	page := func(slug, title string) *fstest.MapFile {
		return &fstest.MapFile{Data: []byte("---\ntitle: " + title + "\n---\n\nbody\n")}
	}
	oss := &docsFSPlugin{name: "help", fsys: fstest.MapFS{
		"docs/getting-started.md": page("getting-started", "Getting Started"),
	}}
	pro := &docsFSPlugin{name: "help", fsys: fstest.MapFS{
		"docs/overview.md": page("overview", "Pro & Elite Overview"),
	}}

	slugsOf := func(pl plugin.Plugin) []string {
		var out []string
		for _, d := range pluginDocs(pl) {
			out = append(out, d.Slug)
		}
		return out
	}

	// Order matters: the bug only appeared on the second plugin, after the first
	// had populated the cache under the shared name.
	if got := slugsOf(oss); len(got) != 1 || got[0] != "getting-started" {
		t.Fatalf("first plugin: got %v, want [getting-started]", got)
	}
	if got := slugsOf(pro); len(got) != 1 || got[0] != "overview" {
		t.Errorf("second plugin with the same Name(): got %v, want [overview] — it is reading the other plugin's cached docs", got)
	}
	// And the cache must still be a cache: a repeat call is served from it and
	// must return the same plugin's pages, not the other's.
	if got := slugsOf(oss); len(got) != 1 || got[0] != "getting-started" {
		t.Errorf("first plugin on the cached path: got %v, want [getting-started]", got)
	}
}

// TestBundledDocLinksResolve walks every in-app link the OSS corpus writes and
// fails on one that lands nowhere. Cross-references are the whole navigation
// story of the help area — each page ends in a "Related" list — and a dead one
// is invisible to every other check here: the markdown is well-formed, the page
// renders, and the reader only finds out by clicking. Three ways they went dead
// before this test existed:
//
//   - /help/plugins/dns/dns — the docs-site path, which is not the in-app one;
//   - /help/portal — a slug that was renamed to customer-portal;
//   - /help/<pro-slug> from an OSS page, dead in every OSS build.
//
// The canonical in-app form is /help/<slug>, two segments. The viewer expands it
// to /help/<category>/<slug>, so a page that changes category keeps its links.
func TestBundledDocLinksResolve(t *testing.T) {
	slugs := make(map[string]bool)
	for _, p := range bundledDocsProviders() {
		for _, d := range plugin.LoadHelpDocs(p.HelpDocsFS()) {
			slugs[d.Slug] = true
		}
	}

	link := regexp.MustCompile(`\]\((/help/[^)]*)\)`)
	for name, p := range bundledDocsProviders() {
		for _, d := range plugin.LoadHelpDocs(p.HelpDocsFS()) {
			for _, m := range link.FindAllStringSubmatch(d.Markdown, -1) {
				target := strings.TrimPrefix(m[1], "/help/")
				if strings.Contains(target, "/") {
					t.Errorf("%s: %s links to %s — the in-app form is /help/<slug>, with no category or plugin path", name, d.Slug, m[1])
					continue
				}
				if !slugs[strings.SplitN(target, "#", 2)[0]] {
					t.Errorf("%s: %s links to %s, which no page in this build serves", name, d.Slug, m[1])
				}
			}
		}
	}
}
