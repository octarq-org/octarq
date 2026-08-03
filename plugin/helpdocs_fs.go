package plugin

import (
	"io/fs"
	"log"
	"path"
	"sort"
	"strings"
)

// HelpDocsFS is the docs-directory convention: a plugin embeds the directory its
// documentation lives in and returns it, instead of hand-building HelpDoc values
// in Go.
//
//	//go:embed docs
//	var docsFS embed.FS
//
//	func (p *Plugin) HelpDocsFS() fs.FS { return docsFS }
//
// Everything about a page — its slug, title, category, order — is declared
// once, in that page's own frontmatter.
//
// A plugin may implement this, HelpProvider, or both. The two are concatenated,
// so a plugin can ship static pages from disk alongside pages it generates at
// runtime.
type HelpDocsFS interface {
	HelpDocsFS() fs.FS
}

// helpDocExts lists recognized documentation file extensions (.mdx and .md).
var helpDocExts = []string{".mdx", ".md"}

// LoadHelpDocs walks fsys and returns one HelpDoc per documentation page.
//
// The naming IS the contract:
//
//	docs/webhooks.mdx      -> a page, slug "webhooks"
//	docs/webhooks.zh.mdx   -> its Chinese translation, never a page of its own
//
// A page's slug defaults to its file name, so frontmatter is only needed to set
// title/category/order or to override the slug. The walk is recursive and slugs
// come from the base name, so subdirectories organise files without showing up
// in any URL.
//
// Malformed or unreadable files are logged and skipped rather than returned as
// an error: a plugin whose one doc fails to parse should still boot and serve
// every other plugin's docs.
func LoadHelpDocs(fsys fs.FS) []HelpDoc {
	if fsys == nil {
		return nil
	}

	var docs []HelpDoc
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			log.Printf("[help] warning: cannot walk %q: %v", p, err)
			return nil
		}
		if d.IsDir() {
			return nil
		}
		base, ok := helpDocBase(d.Name())
		if !ok {
			return nil
		}

		raw, err := fs.ReadFile(fsys, p)
		if err != nil {
			log.Printf("[help] warning: cannot read %q: %v", p, err)
			return nil
		}
		doc := ParseHelpDocSafe(string(raw))
		if doc.Slug == "" {
			doc.Slug = base
		}
		if doc.Title == "" {
			log.Printf("[help] warning: doc %q has no title in frontmatter, falling back to its slug", p)
			doc.Title = doc.Slug
		}

		// Translations sit next to their English page, under the same base name.
		dir := path.Dir(p)
		for lang, raw := range readHelpDocTranslations(fsys, dir, base) {
			doc = doc.WithTranslation(lang, raw)
		}

		docs = append(docs, doc)
		return nil
	})
	if err != nil {
		log.Printf("[help] warning: help docs walk failed: %v", err)
	}

	sort.Slice(docs, func(i, j int) bool { return CompareHelpDocs(docs[i], docs[j]) })
	return docs
}

// helpDocBase reports whether name is an English page and returns its slug base.
// A ".<lang>" segment before the extension marks a translation, which is pulled
// in by its English page and must never be listed as a page itself — otherwise
// every translated doc would appear twice in the sidebar, once in each language.
func helpDocBase(name string) (string, bool) {
	for _, ext := range helpDocExts {
		if !strings.HasSuffix(name, ext) {
			continue
		}
		base := strings.TrimSuffix(name, ext)
		if base == "" {
			return "", false
		}
		if lang := path.Ext(base); lang != "" && isHelpDocLangSuffix(lang[1:]) {
			return "", false
		}
		return base, true
	}
	return "", false
}

// helpDocLangs is the closed set of translation suffixes. It is closed on
// purpose: a page legitimately named "docs.v2.mdx" must stay a page, and only an
// explicit list can tell that apart from a translation.
var helpDocLangs = []string{"zh", "es", "pt", "ja"}

func isHelpDocLangSuffix(s string) bool {
	for _, l := range helpDocLangs {
		if s == l {
			return true
		}
	}
	return false
}

// readHelpDocTranslations returns the raw contents of every "<base>.<lang>.<ext>"
// sibling of a page, keyed by language.
func readHelpDocTranslations(fsys fs.FS, dir, base string) map[string]string {
	var out map[string]string
	for _, lang := range helpDocLangs {
		for _, ext := range helpDocExts {
			raw, err := fs.ReadFile(fsys, path.Join(dir, base+"."+lang+ext))
			if err != nil {
				continue
			}
			if out == nil {
				out = make(map[string]string, len(helpDocLangs))
			}
			out[lang] = string(raw)
			break
		}
	}
	return out
}
