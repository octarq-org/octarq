package plugin

import (
	"testing"
)

func TestParseHelpDoc(t *testing.T) {
	rawMDX := `---
slug: test-doc
title: Test Title
scope: plugins
category: test
group: Testing
groupOrder: 10
order: 1
feature: test_feat
translations:
  zh:
    title: 测试标题
    group: 测试组
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
	if doc.Scope != "plugins" {
		t.Errorf("expected scope 'plugins', got %q", doc.Scope)
	}
	if doc.Category != "test" {
		t.Errorf("expected category 'test', got %q", doc.Category)
	}
	if doc.Group != "Testing" {
		t.Errorf("expected group 'Testing', got %q", doc.Group)
	}
	if doc.GroupOrder != 10 {
		t.Errorf("expected groupOrder 10, got %d", doc.GroupOrder)
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
	if zh.Group != "测试组" {
		t.Errorf("expected zh group '测试组', got %q", zh.Group)
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
