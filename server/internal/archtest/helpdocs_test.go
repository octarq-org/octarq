package archtest

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/plugin"
	"github.com/octarq-org/octarq/server/plugins/dns"
	"github.com/octarq-org/octarq/server/plugins/help"
	"github.com/octarq-org/octarq/server/plugins/links"
	"github.com/octarq-org/octarq/server/plugins/mail"
)

func coreDocsProviders() map[string]plugin.HelpDocsFS {
	return map[string]plugin.HelpDocsFS{
		"help":  help.New(),
		"links": links.New(),
		"mail":  mail.New(),
		"dns":   dns.New(),
	}
}

func TestCoreHelpDocsHaveTranslationsAndExcludeSpecMd(t *testing.T) {
	for name, p := range coreDocsProviders() {
		fsys := p.HelpDocsFS()
		docs := plugin.LoadHelpDocs(fsys)
		if len(docs) == 0 {
			t.Errorf("%s: expected at least one help doc", name)
		}
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
}
