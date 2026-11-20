package archtest

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/plugin"
	"github.com/octarq-org/octarq/server/plugins/builtin"
)

func coreDocsProviders() map[string]plugin.HelpDocsFS {
	out := map[string]plugin.HelpDocsFS{}
	for _, p := range builtin.Default() {
		if docs, ok := p.(plugin.HelpDocsFS); ok {
			out[p.Name()] = docs
		}
	}
	return out
}

func TestCoreHelpDocsHaveTranslationsAndExcludeSpecMd(t *testing.T) {
	checked := 0
	for name, p := range coreDocsProviders() {
		fsys := p.HelpDocsFS()
		docs := plugin.LoadHelpDocs(fsys)
		if len(docs) == 0 {
			continue
		}
		checked++
		for _, d := range docs {
			zh, ok := d.Translations["zh"]
			if !ok || strings.TrimSpace(zh.Markdown) == "" {
				t.Errorf("%s: doc %q is missing Chinese (.zh) translation", name, d.Slug)
			}
		}
		err := fs.WalkDir(fsys, ".", func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return err
			}
			lower := strings.ToLower(entry.Name())
			if lower == "spec.md" || strings.HasSuffix(lower, ".spec.md") {
				t.Errorf("%s: SPEC.md must never be embedded in HelpDocsFS (%s)", name, path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("%s: walk docs: %v", name, err)
		}
	}
	if checked == 0 {
		t.Fatal("no builtin plugin ships help docs; the guard checked nothing")
	}
}

func TestCoreHelpDocSlugsAreUniqueAcrossPlugins(t *testing.T) {
	seen := make(map[string]string)
	for name, p := range coreDocsProviders() {
		for _, d := range plugin.LoadHelpDocs(p.HelpDocsFS()) {
			if prev, dup := seen[d.Slug]; dup {
				t.Errorf("slug %q is claimed by both %s and %s", d.Slug, prev, name)
			}
			seen[d.Slug] = name
		}
	}
}
