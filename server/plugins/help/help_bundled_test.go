package help

import (
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/octarq-org/octarq/server/plugin"
)

func bundledDocsProviders() map[string]plugin.HelpDocsFS {
	return map[string]plugin.HelpDocsFS{
		"help": New(),
	}
}

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
		if c.Key == "commerce" || c.Key == "licensing" || c.Key == "infrastructure" || c.Key == "operations" {
			continue
		}
		if categoryCounts[c.Key] == 0 {
			t.Errorf("category %q has no documents — all categories in plugin.HelpCategories() must have at least one doc", c.Key)
		}
	}
}

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
			if ext := filepath.Ext(d.Slug); ext != "" && len(ext) <= 4 {
				t.Errorf("%s: slug %q ends in %q — if that is a language tag, add it to helpDocLangs so the file is treated as a translation rather than a page of its own", name, d.Slug, ext)
			}
		}
	}
}

type docsFSPlugin struct {
	name string
	fsys fs.FS
}

func (m *docsFSPlugin) Name() string                              { return m.name }
func (m *docsFSPlugin) Models() []any                             { return nil }
func (m *docsFSPlugin) Mount(mux plugin.Mux, ctx *plugin.Context) {}
func (m *docsFSPlugin) HelpDocsFS() fs.FS                         { return m.fsys }

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

	if got := slugsOf(oss); len(got) != 1 || got[0] != "getting-started" {
		t.Fatalf("first plugin: got %v, want [getting-started]", got)
	}
	if got := slugsOf(pro); len(got) != 1 || got[0] != "overview" {
		t.Errorf("second plugin with the same Name(): got %v, want [overview] — it is reading the other plugin's cached docs", got)
	}
	if got := slugsOf(oss); len(got) != 1 || got[0] != "getting-started" {
		t.Errorf("first plugin on the cached path: got %v, want [getting-started]", got)
	}
}

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
