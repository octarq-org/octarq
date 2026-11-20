package plugin

import (
	"io/fs"
	"log"
	"path"
	"sort"
	"strings"
)

type HelpDocsFS interface {
	HelpDocsFS() fs.FS
}

var helpDocExts = []string{".mdx", ".md"}

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

var helpDocLangs = []string{"zh", "es", "pt", "ja"}

func isHelpDocLangSuffix(s string) bool {
	for _, l := range helpDocLangs {
		if s == l {
			return true
		}
	}
	return false
}

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
