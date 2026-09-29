package links

import (
	"context"
	"testing"

	"github.com/octarq-org/octarq/server/plugin"
)

func TestMCPLinksLifecycle(t *testing.T) {
	p, _ := setupFullLinksTestDB(t)

	ctx1 := plugin.WithOrgID(context.Background(), 1)
	ctx2 := plugin.WithOrgID(context.Background(), 2)

	// 1. Create Link in Org 1
	enabled := true
	res, rawOut, err := p.mcpCreateLink(ctx1, nil, createLinkInput{
		Target:  "https://example.com/hello",
		Slug:    "my-slug",
		Title:   "My Title",
		Tags:    "tag1,tag2",
		Enabled: &enabled,
	})
	if err != nil {
		t.Fatalf("mcpCreateLink: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success, got error result")
	}
	created := rawOut.(linkOut)
	if created.Slug != "my-slug" || created.Target != "https://example.com/hello" {
		t.Fatalf("unexpected linkOut: %+v", created)
	}

	// 2. Cross-tenant isolation on Update
	newTarget := "https://example.com/updated"
	_, _, err = p.mcpUpdateLink(ctx2, nil, updateLinkInput{
		ID:     created.ID,
		Target: &newTarget,
	})
	if err == nil {
		t.Error("expected error when org 2 tries to update org 1 link")
	}

	// 3. Update in Org 1
	newTitle := "Updated Title"
	res, rawOut, err = p.mcpUpdateLink(ctx1, nil, updateLinkInput{
		ID:     created.ID,
		Target: &newTarget,
		Title:  &newTitle,
	})
	if err != nil {
		t.Fatalf("mcpUpdateLink: %v", err)
	}
	updated := rawOut.(linkOut)
	if updated.Target != newTarget || updated.Title != newTitle {
		t.Fatalf("unexpected updated link: %+v", updated)
	}

	// 4. Batch Create
	_, rawBatch, err := p.mcpBatchCreateLinks(ctx1, nil, batchCreateLinksInput{
		Links: []createLinkInput{
			{Target: "https://example.com/b1", Slug: "b1"},
			{Target: "https://example.com/b2", Slug: "b2"},
		},
	})
	if err != nil {
		t.Fatalf("mcpBatchCreateLinks: %v", err)
	}
	batch := rawBatch.([]linkOut)
	if len(batch) != 2 {
		t.Fatalf("expected 2 links created in batch, got %d", len(batch))
	}

	// 5. List with tag filter
	_, rawList, err := p.mcpListLinks(ctx1, nil, listLinksInput{
		Tag: "tag1",
	})
	if err != nil {
		t.Fatalf("mcpListLinks: %v", err)
	}
	linksList := rawList.([]linkOut)
	if len(linksList) != 1 || linksList[0].Slug != "my-slug" {
		t.Fatalf("expected 1 tagged link, got %+v", linksList)
	}

	// 6. Delete Link
	_, _, err = p.mcpDeleteLink(ctx1, nil, deleteLinkInput{
		ID: created.ID,
	})
	if err != nil {
		t.Fatalf("mcpDeleteLink: %v", err)
	}

	// Verify it's gone
	_, _, err = p.mcpUpdateLink(ctx1, nil, updateLinkInput{
		ID:     created.ID,
		Target: &newTarget,
	})
	if err == nil {
		t.Error("expected error updating deleted link")
	}
}

func TestMCPCreateLinkValidation(t *testing.T) {
	p, _ := setupFullLinksTestDB(t)
	ctx := plugin.WithOrgID(context.Background(), 1)

	// Empty target
	_, _, err := p.mcpCreateLink(ctx, nil, createLinkInput{Target: ""})
	if err == nil {
		t.Error("expected error for empty target")
	}

	// Invalid target
	_, _, err = p.mcpCreateLink(ctx, nil, createLinkInput{Target: "javascript:alert(1)"})
	if err == nil {
		t.Error("expected error for invalid target scheme")
	}

	// No org in context
	_, _, err = p.mcpCreateLink(context.Background(), nil, createLinkInput{Target: "https://ok.com"})
	if err == nil {
		t.Error("expected error when no org in context")
	}
}
