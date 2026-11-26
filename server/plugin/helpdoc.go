package plugin

import (
	"log"
	"strings"

	"gopkg.in/yaml.v3"
)

type HelpCategory struct {
	Key    string            `json:"key"`
	Order  int               `json:"order"`
	Icon   string            `json:"icon"`
	Labels map[string]string `json:"labels"`
}

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
				"ja": "オペレーション",
			},
		},
		{
			Key:   "infrastructure",
			Order: 30,
			Icon:  "boxes",
			Labels: map[string]string{
				"en": "Infrastructure",
				"zh": "基础设施",
				"es": "Infraestructura",
				"pt": "Infraestrutura",
				"ja": "インフラ",
			},
		},
		{
			Key:   "commerce",
			Order: 40,
			Icon:  "wallet",
			Labels: map[string]string{
				"en": "Commerce",
				"zh": "商务",
				"es": "Comercio",
				"pt": "Comércio",
				"ja": "コマース",
			},
		},
		{
			Key:   "settings",
			Order: 50,
			Icon:  "settings",
			Labels: map[string]string{
				"en": "Settings",
				"zh": "设置",
				"es": "Configuración",
				"pt": "Configurações",
				"ja": "設定",
			},
		},
		{
			Key:   "licensing",
			Order: 60,
			Icon:  "key",
			Labels: map[string]string{
				"en": "Editions & licensing",
				"zh": "版本与授权",
				"es": "Ediciones y licencias",
				"pt": "Edições e licenças",
				"ja": "エディションとライセンス",
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

var helpPlaceholderKeys = []string{"app_name", "domain", "org_slug"}

func HelpPlaceholders() []string {
	return append([]string(nil), helpPlaceholderKeys...)
}

func IsHelpPlaceholder(key string) bool {
	for _, k := range helpPlaceholderKeys {
		if k == key {
			return true
		}
	}
	return false
}

type HelpDocTranslation struct {
	Title       string `yaml:"title" json:"title,omitempty"`
	Description string `yaml:"description" json:"description,omitempty"`
	Category    string `yaml:"category" json:"category,omitempty"`
	Markdown    string `yaml:"markdown" json:"markdown,omitempty"`
}

type HelpDoc struct {
	Slug         string                        `yaml:"slug" json:"slug"`
	Title        string                        `yaml:"title" json:"title"`
	Description  string                        `yaml:"description" json:"description,omitempty"`
	Category     string                        `yaml:"category" json:"category"`
	Order        int                           `yaml:"order" json:"order"`
	Feature      string                        `yaml:"feature" json:"feature,omitempty"`
	Routes       []string                      `yaml:"routes" json:"routes,omitempty"`
	Actions      []string                      `yaml:"actions" json:"actions,omitempty"`
	Markdown     string                        `yaml:"-" json:"markdown"`
	Translations map[string]HelpDocTranslation `yaml:"translations" json:"translations,omitempty"`
}

func (h HelpDoc) WithTranslation(lang, raw string) HelpDoc {
	if h.Translations == nil {
		h.Translations = make(map[string]HelpDocTranslation)
	}
	trDoc, err := ParseHelpDoc(raw)
	tr := h.Translations[lang]
	if err == nil && (trDoc.Title != "" || trDoc.Description != "" || trDoc.Category != "" || trDoc.Markdown != "") {
		if trDoc.Title != "" {
			tr.Title = trDoc.Title
		}
		if trDoc.Description != "" {
			tr.Description = trDoc.Description
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

var helpCategoryForPluginCategory = map[string]string{
	CategoryInfrastructure: "infrastructure",
	CategorySecurity:       "settings",
	CategoryCommerce:       "commerce",
	CategoryMarketing:      "operations",
	CategoryMessaging:      "operations",
	CategoryAI:             "operations",
	CategoryUtilities:      "operations",
}

func (h *HelpDoc) FillDefaults(pluginName string, pluginCategory string) {
	if h.Category == "" {
		h.Category = helpCategoryForPluginCategory[pluginCategory]
	}
	h.Category = strings.ToLower(h.Category)
	if _, ok := helpCategoryMap[h.Category]; !ok {
		log.Printf("[help] warning: doc %q from plugin %q has unknown category %q, falling back to 'start'", h.Slug, pluginName, h.Category)
		h.Category = "start"
	}
}

func MustParseHelpDoc(content string) HelpDoc {
	doc, err := ParseHelpDoc(content)
	if err != nil {
		panic(err)
	}
	return doc
}
