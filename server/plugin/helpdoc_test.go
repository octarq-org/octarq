package plugin

import (
	"testing"
)

func TestParseHelpDoc(t *testing.T) {
	rawMDX := `---
slug: test-doc
title: Test Title
category: test
order: 1
feature: test_feat
translations:
  zh:
    title: 测试标题
---

# Hello World
This is markdown content.`

	doc, err := ParseHelpDoc(rawMDX)
	if err != nil {
		t.Fatalf("unexpected error parsing help doc: %v", err)
	}

	if doc.Slug != "test-doc" {
		t.Errorf("expected slug 'test-doc', got %q", doc.Slug)
	}
	if doc.Title != "Test Title" {
		t.Errorf("expected title 'Test Title', got %q", doc.Title)
	}
	if doc.Category != "test" {
		t.Errorf("expected category 'test', got %q", doc.Category)
	}
	if doc.Order != 1 {
		t.Errorf("expected order 1, got %d", doc.Order)
	}
	if doc.Feature != "test_feat" {
		t.Errorf("expected feature 'test_feat', got %q", doc.Feature)
	}
	if doc.Markdown != "# Hello World\nThis is markdown content." {
		t.Errorf("expected markdown content, got %q", doc.Markdown)
	}

	zh, ok := doc.Translations["zh"]
	if !ok {
		t.Fatalf("expected 'zh' translation present")
	}
	if zh.Title != "测试标题" {
		t.Errorf("expected zh title '测试标题', got %q", zh.Title)
	}
}

func TestWithTranslation(t *testing.T) {
	rawEn := `---
slug: getting-started
title: English Title
---
English Content`

	rawZh := `---
title: 中文标题
---
中文内容`

	doc := MustParseHelpDoc(rawEn).WithTranslation("zh", rawZh)

	if doc.Title != "English Title" {
		t.Errorf("expected main title 'English Title', got %q", doc.Title)
	}

	zh, ok := doc.Translations["zh"]
	if !ok {
		t.Fatalf("expected 'zh' translation")
	}
	if zh.Title != "中文标题" {
		t.Errorf("expected zh title '中文标题', got %q", zh.Title)
	}
	if zh.Markdown != "中文内容" {
		t.Errorf("expected zh markdown '中文内容', got %q", zh.Markdown)
	}
}

func TestFillDefaultsFallsBackToClosedSet(t *testing.T) {
	doc := HelpDoc{
		Slug:     "custom-doc",
		Title:    "Custom Doc",
		Category: "unknown_category_key",
	}

	doc.FillDefaults("custom-plugin", "")

	if _, ok := helpCategoryMap[doc.Category]; !ok {
		t.Fatalf("expected category inside the closed set, got %q", doc.Category)
	}
}

func TestFillDefaultsKeepsDeclaredClosedSetCategory(t *testing.T) {
	for _, key := range []string{"start", "operations", "infrastructure", "commerce", "settings", "licensing"} {
		doc := HelpDoc{Slug: "doc", Category: key}
		doc.FillDefaults("plugin", "")
		if doc.Category != key {
			t.Errorf("category %q: expected %q, got %q", key, key, doc.Category)
		}
	}
}

func TestFillDefaultsDerivesFromPluginCategory(t *testing.T) {
	cases := map[string]string{
		CategoryInfrastructure: "infrastructure",
		CategorySecurity:       "settings",
		CategoryCommerce:       "commerce",
		CategoryMarketing:      "operations",
		CategoryMessaging:      "operations",
		CategoryAI:             "operations",
		CategoryUtilities:      "operations",
	}
	for in, want := range cases {
		doc := HelpDoc{Slug: "doc"}
		doc.FillDefaults("plugin", in)
		if doc.Category != want {
			t.Errorf("plugin category %q: expected %q, got %q", in, want, doc.Category)
		}
	}

	doc := HelpDoc{Slug: "doc"}
	doc.FillDefaults("custom-plugin", "")
	if doc.Category != "start" {
		t.Errorf("missing plugin category: expected start, got %q", doc.Category)
	}
}

func TestParseHelpDocDescriptionAndRoutes(t *testing.T) {
	raw := "---\ntitle: Domains\ndescription: Bind a custom domain.\ncategory: infrastructure\norder: 10\nroutes:\n  - /domains\n  - /domains/new\n---\n\nBody"
	doc, err := ParseHelpDoc(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if doc.Description != "Bind a custom domain." {
		t.Errorf("unexpected description %q", doc.Description)
	}
	if len(doc.Routes) != 2 || doc.Routes[0] != "/domains" || doc.Routes[1] != "/domains/new" {
		t.Errorf("unexpected routes %#v", doc.Routes)
	}
}

func TestParseHelpDocActions(t *testing.T) {
	raw := "---\ntitle: Domains\nactions:\n  - domains.create\n  - domains.verify\n---\n\nBody"
	doc, err := ParseHelpDoc(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(doc.Actions) != 2 || doc.Actions[0] != "domains.create" || doc.Actions[1] != "domains.verify" {
		t.Errorf("unexpected actions %#v", doc.Actions)
	}
}

func TestHelpPlaceholdersAreClosedSet(t *testing.T) {
	for _, key := range []string{"app_name", "domain", "org_slug"} {
		if !IsHelpPlaceholder(key) {
			t.Errorf("expected %q to be a known placeholder", key)
		}
	}
	if IsHelpPlaceholder("token") || IsHelpPlaceholder("secret") {
		t.Error("secret-like keys must not be placeholders")
	}
	if len(HelpPlaceholders()) != 3 {
		t.Errorf("expected 3 placeholders, got %d", len(HelpPlaceholders()))
	}
}
