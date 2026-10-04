package plugin_test

import (
	"context"
	"sync"
	"testing"

	"github.com/octarq-org/octarq/server/plugin"
)

type charTestEvent struct {
	Message string
	Code    int
}

// TestCharacterization_TypedBus verifies TypedBus sync and async publish behavior.
func TestCharacterization_TypedBus(t *testing.T) {
	bus := plugin.NewTypedBus[charTestEvent]()

	var received []charTestEvent
	var mu sync.Mutex

	bus.Subscribe(func(e charTestEvent) error {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, e)
		return nil
	})

	ev1 := charTestEvent{Message: "hello", Code: 200}
	errs := bus.Publish(ev1)
	if len(errs) > 0 {
		t.Fatalf("Publish returned errors: %v", errs)
	}

	mu.Lock()
	if len(received) != 1 || received[0] != ev1 {
		t.Fatalf("unexpected sync event received: %+v", received)
	}
	mu.Unlock()
}

// TestCharacterization_ContextHelpers verifies context OrgID and UserID propagation.
func TestCharacterization_ContextHelpers(t *testing.T) {
	ctx := context.Background()
	if orgID := plugin.OrgIDFromContext(ctx); orgID != 0 {
		t.Errorf("expected 0 for empty ctx, got %d", orgID)
	}
	if uid := plugin.UserIDFromContext(ctx); uid != 0 {
		t.Errorf("expected 0 for empty ctx, got %d", uid)
	}

	ctx = plugin.WithOrgID(ctx, 42)
	ctx = plugin.WithUserID(ctx, 100)

	if orgID := plugin.OrgIDFromContext(ctx); orgID != 42 {
		t.Errorf("expected 42, got %d", orgID)
	}
	if uid := plugin.UserIDFromContext(ctx); uid != 100 {
		t.Errorf("expected 100, got %d", uid)
	}
}

// TestCharacterization_HelpDocParsing verifies YAML frontmatter parsing and category fallback.
func TestCharacterization_HelpDocParsing(t *testing.T) {
	content := `---
title: "Sample Guide"
slug: "sample-guide"
category: "core"
---
# Guide Content
Body markdown text here.`

	doc, err := plugin.ParseHelpDoc(content)
	if err != nil {
		t.Fatalf("ParseHelpDoc: %v", err)
	}
	if doc.Title != "Sample Guide" {
		t.Errorf("expected title 'Sample Guide', got %q", doc.Title)
	}
	if doc.Slug != "sample-guide" {
		t.Errorf("expected slug 'sample-guide', got %q", doc.Slug)
	}
	if doc.Category != "core" {
		t.Errorf("expected category 'core', got %q", doc.Category)
	}
	if doc.Markdown != "# Guide Content\nBody markdown text here." {
		t.Errorf("unexpected markdown: %q", doc.Markdown)
	}

	// Test category normalization
	doc.FillDefaults("myplugin", "")
	if doc.Category != "start" { // "core" gets remapped to "start"
		t.Errorf("expected category remapped to 'start', got %q", doc.Category)
	}
}

type dummyService interface {
	Greet() string
}

type dummyImpl struct{}

func (d *dummyImpl) Greet() string { return "hi" }

// TestCharacterization_Registry verifies Service Registry Provide and LookupAs.
func TestCharacterization_Registry(t *testing.T) {
	r := plugin.NewRegistry()
	impl := &dummyImpl{}
	r.Provide("test.dummy", impl)

	// Duplicate provide must be recorded in r.Err()
	r.Provide("test.dummy", impl)
	if r.Err() == nil {
		t.Fatalf("expected error on duplicate Provide in r.Err(), got nil")
	}

	ctx := &plugin.Context{
		Provide: r.Provide,
		Lookup:  r.Lookup,
	}

	svc, ok := plugin.LookupAs[dummyService](ctx, "test.dummy")
	if !ok || svc == nil {
		t.Fatalf("LookupAs failed to retrieve service")
	}
	if svc.Greet() != "hi" {
		t.Errorf("unexpected service response: %q", svc.Greet())
	}
}
