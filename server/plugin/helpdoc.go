package plugin

import (
	"log"
	"strings"

	"gopkg.in/yaml.v3"
)

// HelpCategory is a sidebar group of help docs. The set is closed and lives
// here so a doc only has to name its category — labels never live in the shell.
type HelpCategory struct {
	Key    string            `json:"key"`    // "start", "access", …
	Order  int               `json:"order"`  // group ordering
	Icon   string            `json:"icon"`   // lucide key, e.g. "book-open"
	Labels map[string]string `json:"labels"` // lang → label; "en" is required
}

// HelpCategories returns the closed set of 6 help categories.
func HelpCategories() []HelpCategory {
	return []HelpCategory{
		{
			Key:   "start",
			Order: 10,
			Icon:  "book-open",
			Labels: map[string]string{
				"en": "Start & platform",
				"zh": "入门与平台",
				"es": "Introducción y plataforma",
				"pt": "Introdução e plataforma",
				"ja": "はじめに・プラットフォーム",
			},
		},
		{
			Key:   "operations",
			Order: 20,
			Icon:  "workflow",
			Labels: map[string]string{
				"en": "Operations",
				"zh": "运营",
				"es": "Operaciones",
				"pt": "Operações",
				"ja": "運用",
			},
		},
		{
			Key:   "services",
			Order: 30,
			Icon:  "layers",
			Labels: map[string]string{
				"en": "Services & plugins",
				"zh": "服务与插件",
				"es": "Servicios y plugins",
				"pt": "Serviços e plugins",
				"ja": "サービスとプラグイン",
			},
		},
		{
			Key:   "access",
			Order: 40,
			Icon:  "shield-check",
			Labels: map[string]string{
				"en": "Access & security",
				"zh": "访问与安全",
				"es": "Acceso y seguridad",
				"pt": "Acesso e segurança",
				"ja": "アクセスとセキュリティ",
			},
		},
		{
			Key:   "developers",
			Order: 50,
			Icon:  "code-2",
			Labels: map[string]string{
				"en": "Developers & API",
				"zh": "开发者与 API",
				"es": "Desarrolladores y API",
				"pt": "Desenvolvedores e API",
				"ja": "開発者と API",
			},
		},
		{
			Key:   "recipes",
			Order: 60,
			Icon:  "sparkles",
			Labels: map[string]string{
				"en": "Recipes & blueprints",
				"zh": "配方与最佳实践",
				"es": "Recetas y plantillas",
				"pt": "Receitas e modelos",
				"ja": "レシピとブループリント",
			},
		},
	}
}

var helpCategoryMap = func() map[string]HelpCategory {
	m := make(map[string]HelpCategory)
	for _, c := range HelpCategories() {
		m[c.Key] = c
	}
	return m
}()

// CompareHelpDocs orders HelpDocs first by category order, then doc Order, then Title.
func CompareHelpDocs(a, b HelpDoc) bool {
	catAOrder := 9999
	if c, ok := helpCategoryMap[a.Category]; ok {
		catAOrder = c.Order
	}
	catBOrder := 9999
	if c, ok := helpCategoryMap[b.Category]; ok {
		catBOrder = c.Order
	}

	if catAOrder != catBOrder {
		return catAOrder < catBOrder
	}
	if a.Order != b.Order {
		return a.Order < b.Order
	}
	return a.Title < b.Title
}

// HelpDocTranslation provides localized strings for a HelpDoc.
type HelpDocTranslation struct {
	Title    string `yaml:"title" json:"title,omitempty"`
	Category string `yaml:"category" json:"category,omitempty"`
	Markdown string `yaml:"markdown" json:"markdown,omitempty"`
}

// HelpDoc is one page of in-app documentation contributed by a plugin.
type HelpDoc struct {
	Slug         string                        `yaml:"slug" json:"slug"`
	Title        string                        `yaml:"title" json:"title"`
	Category     string                        `yaml:"category" json:"category"`
	Order        int                           `yaml:"order" json:"order"`
	Feature      string                        `yaml:"feature" json:"feature,omitempty"`
	Markdown     string                        `yaml:"-" json:"markdown"`
	Translations map[string]HelpDocTranslation `yaml:"translations" json:"translations,omitempty"`
}

// WithTranslation parses translation content (which can also contain YAML frontmatter or raw markdown)
// and adds/updates the translation for the specified language key (e.g. "zh").
func (h HelpDoc) WithTranslation(lang, raw string) HelpDoc {
	if h.Translations == nil {
		h.Translations = make(map[string]HelpDocTranslation)
	}
	trDoc, err := ParseHelpDoc(raw)
	tr := h.Translations[lang]
	if err == nil && (trDoc.Title != "" || trDoc.Category != "" || trDoc.Markdown != "") {
		if trDoc.Title != "" {
			tr.Title = trDoc.Title
		}
		if trDoc.Category != "" {
			tr.Category = trDoc.Category
		}
		tr.Markdown = trDoc.Markdown
	} else {
		tr.Markdown = strings.TrimSpace(raw)
	}
	h.Translations[lang] = tr
	return h
}

// ParseHelpDoc parses an MDX/MD string with optional YAML frontmatter.
func ParseHelpDoc(content string) (HelpDoc, error) {
	var doc HelpDoc
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "---") {
		doc.Markdown = trimmed
		return doc, nil
	}

	rest := trimmed[3:]
	if idx := strings.Index(rest, "\n"); idx != -1 {
		rest = rest[idx+1:]
	}

	endIdx := strings.Index(rest, "---")
	if endIdx == -1 {
		doc.Markdown = trimmed
		return doc, nil
	}

	frontmatterYAML := rest[:endIdx]
	markdownBody := strings.TrimSpace(rest[endIdx+3:])

	if err := yaml.Unmarshal([]byte(frontmatterYAML), &doc); err != nil {
		return doc, err
	}
	doc.Markdown = markdownBody
	return doc, nil
}

// ParseHelpDocSafe parses an MDX/MD string and returns a fallback doc with markdown body on error without panic.
func ParseHelpDocSafe(content string) HelpDoc {
	doc, err := ParseHelpDoc(content)
	if err != nil {
		log.Printf("[help] warning: failed to parse help doc: %v", err)
		return HelpDoc{
			Markdown: strings.TrimSpace(content),
		}
	}
	return doc
}

// FillDefaults automatically populates Category based on taxonomy defaults if missing or invalid.
func (h *HelpDoc) FillDefaults(pluginName string, pluginCategory string) {
	if h.Category == "" {
		if pluginCategory != "" {
			h.Category = pluginCategory
		} else {
			h.Category = pluginName
		}
	}
	h.Category = strings.ToLower(h.Category)

	// Remap known legacy categories or validate against closed set
	switch h.Category {
	case "getting-started", "start", "general", "platform", "core":
		h.Category = "start"
	case "access", "security", "identity":
		h.Category = "access"
	case "automation", "api", "apis":
		h.Category = "automation"
	case "services", "infrastructure", "dns", "ddns", "short-links", "links", "mail", "messaging", "marketing":
		h.Category = "services"
	case "commerce", "billing", "finance":
		h.Category = "commerce"
	case "licensing", "editions":
		h.Category = "licensing"
	default:
		if _, ok := helpCategoryMap[h.Category]; !ok {
			log.Printf("[help] warning: doc %q from plugin %q has unknown category %q, falling back to 'services'", h.Slug, pluginName, h.Category)
			h.Category = "services"
		}
	}
}

// MustParseHelpDoc calls ParseHelpDoc and panics if an error occurs.
func MustParseHelpDoc(content string) HelpDoc {
	doc, err := ParseHelpDoc(content)
	if err != nil {
		panic(err)
	}
	return doc
}
