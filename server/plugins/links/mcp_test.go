package links

import (
	"context"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
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
	_, rawOut, err = p.mcpUpdateLink(ctx1, nil, updateLinkInput{
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

func TestMCPWriteEdgeCases(t *testing.T) {
	p, _ := setupFullLinksTestDB(t)
	ctx := plugin.WithOrgID(context.Background(), 1)

	// Reserved slug
	_, _, err := p.mcpCreateLink(ctx, nil, createLinkInput{
		Target: "https://example.com",
		Slug:   "admin",
	})
	if err == nil {
		t.Error("expected error for reserved slug")
	}

	// Host not owned
	_, _, err = p.mcpCreateLink(ctx, nil, createLinkInput{
		Target: "https://example.com",
		Host:   "unowned.com",
	})
	if err == nil {
		t.Error("expected error for unowned host")
	}

	// Update with invalid target
	invTarget := "ftp://bad"
	_, _, err = p.mcpUpdateLink(ctx, nil, updateLinkInput{
		ID:     999999,
		Target: &invTarget,
	})
	if err == nil {
		t.Error("expected error for link not found")
	}

	// Update with ID=0
	_, _, err = p.mcpUpdateLink(ctx, nil, updateLinkInput{
		ID: 0,
	})
	if err == nil {
		t.Error("expected error for ID=0")
	}

	// Update with no org
	_, _, err = p.mcpUpdateLink(context.Background(), nil, updateLinkInput{
		ID: 1,
	})
	if err == nil {
		t.Error("expected error for no org")
	}

	// Delete with ID=0
	_, _, err = p.mcpDeleteLink(ctx, nil, deleteLinkInput{ID: 0})
	if err == nil {
		t.Error("expected error for delete ID=0")
	}

	// Delete with no org
	_, _, err = p.mcpDeleteLink(context.Background(), nil, deleteLinkInput{ID: 1})
	if err == nil {
		t.Error("expected error for delete no org")
	}

	// Delete not found
	_, _, err = p.mcpDeleteLink(ctx, nil, deleteLinkInput{ID: 999999})
	if err == nil {
		t.Error("expected error for delete not found")
	}

	// Batch create empty
	_, _, err = p.mcpBatchCreateLinks(ctx, nil, batchCreateLinksInput{})
	if err == nil {
		t.Error("expected error for empty batch")
	}

	// Batch create error in item
	_, _, err = p.mcpBatchCreateLinks(ctx, nil, batchCreateLinksInput{
		Links: []createLinkInput{
			{Target: "javascript:bad"},
		},
	})
	if err == nil {
		t.Error("expected error for invalid link in batch")
	}

	// List links no org
	_, _, err = p.mcpListLinks(context.Background(), nil, listLinksInput{})
	if err == nil {
		t.Error("expected error for list no org")
	}

	// Export links
	exp, err := p.mcpExportLinks(ctx, 1)
	if err != nil {
		t.Fatalf("mcpExportLinks: %v", err)
	}
	if exp == nil {
		t.Error("expected non-nil export")
	}
}

func TestMCPRegisteredToolsIncludeAllRequired(t *testing.T) {
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1.0"}, nil)
	client := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "1.0"}, nil)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer serverSession.Close()
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer clientSession.Close()

	p := New()
	p.RegisterMCP(server)

	res, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	var names []string
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	required := []string{
		"octarq_network__list_links",
		"octarq_network__create_link",
		"octarq_network__update_link",
		"octarq_network__update_shortlink",
		"octarq_network__toggle_shortlink",
		"octarq_network__get_link_analytics",
		"octarq_network__delete_link",
		"octarq_network__batch_create_links",
	}
	for _, reqName := range required {
		if !slicesContains(names, reqName) {
			t.Errorf("tool %q not registered; got: %v", reqName, names)
		}
	}
}

func TestMCPUpdateLinkFullFields(t *testing.T) {
	p, _ := setupFullLinksTestDB(t)
	ctx := plugin.WithOrgID(context.Background(), 1)

	// Create a link
	res, rawOut, err := p.mcpCreateLink(ctx, nil, createLinkInput{
		Target: "https://example.com/initial",
		Slug:   "initial-slug",
		Title:  "Initial Title",
	})
	if err != nil || res.IsError {
		t.Fatalf("create failed: %v", err)
	}
	created := rawOut.(linkOut)

	// Update all full fields
	newTarget := "https://example.com/updated-full"
	newSlug := "updated-slug"
	newHost := ""
	newTitle := "Updated Title Full"
	newTags := "tagA,tagB"
	newNote := "Updated administrative note"
	newPass := "secret123"
	newLimit := int64(500)
	expStr := "2030-01-01T00:00:00Z"
	expURL := "https://example.com/expired"
	rules := RoutingRules{
		{Type: "geo", Match: "US", Target: "https://us.example.com"},
		{Type: "split", Weight: 50, Target: "https://b.example.com"},
	}
	enabled := false
	archived := true

	updateRes, rawUpdated, err := p.mcpUpdateLink(ctx, nil, updateLinkInput{
		ID:           created.ID,
		Target:       &newTarget,
		Slug:         &newSlug,
		Host:         &newHost,
		Title:        &newTitle,
		Tags:         &newTags,
		Note:         &newNote,
		Password:     &newPass,
		ClickLimit:   &newLimit,
		ExpiresAt:    &expStr,
		ExpiredURL:   &expURL,
		RoutingRules: &rules,
		Enabled:      &enabled,
		Archived:     &archived,
	})
	if err != nil || updateRes.IsError {
		t.Fatalf("update full fields failed: %v", err)
	}
	out := rawUpdated.(linkOut)
	if out.Slug != newSlug || out.Target != newTarget || out.Title != newTitle || out.Tags != newTags || out.Enabled != enabled || out.Archived != archived {
		t.Fatalf("unexpected linkOut: %+v", out)
	}

	// Verify in DB directly
	var l Link
	if err := p.db.Where("id = ?", created.ID).First(&l).Error; err != nil {
		t.Fatalf("query DB: %v", err)
	}
	if l.Password != newPass {
		t.Errorf("password = %q, want %q", l.Password, newPass)
	}
	if l.ClickLimit != newLimit {
		t.Errorf("clickLimit = %d, want %d", l.ClickLimit, newLimit)
	}
	if l.ExpiredURL != expURL {
		t.Errorf("expiredUrl = %q, want %q", l.ExpiredURL, expURL)
	}
	if l.ExpiresAt == nil || l.ExpiresAt.Format(time.RFC3339) != expStr {
		t.Errorf("expiresAt = %v, want %s", l.ExpiresAt, expStr)
	}
	if len(l.RoutingRules) != 2 || l.RoutingRules[0].Match != "US" {
		t.Errorf("routingRules mismatch: %+v", l.RoutingRules)
	}
}

func TestMCPToggleShortlink(t *testing.T) {
	p, _ := setupFullLinksTestDB(t)
	ctx := plugin.WithOrgID(context.Background(), 1)

	// Create link (default enabled = true)
	_, rawOut, err := p.mcpCreateLink(ctx, nil, createLinkInput{
		Target: "https://example.com/toggle",
		Slug:   "toggle-slug",
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	linkID := rawOut.(linkOut).ID

	// 1. Toggle without explicit enabled (true -> false)
	res, rawOut, err := p.mcpToggleShortlink(ctx, nil, toggleShortlinkInput{ID: linkID})
	if err != nil || res.IsError {
		t.Fatalf("toggle without param failed: %v", err)
	}
	if rawOut.(linkOut).Enabled != false {
		t.Errorf("expected Enabled == false, got true")
	}

	// 2. Toggle without explicit enabled again (false -> true)
	res, rawOut, err = p.mcpToggleShortlink(ctx, nil, toggleShortlinkInput{ID: linkID})
	if err != nil || res.IsError {
		t.Fatalf("toggle without param 2 failed: %v", err)
	}
	if rawOut.(linkOut).Enabled != true {
		t.Errorf("expected Enabled == true, got false")
	}

	// 3. Set explicit enabled false
	f := false
	res, rawOut, err = p.mcpToggleShortlink(ctx, nil, toggleShortlinkInput{ID: linkID, Enabled: &f})
	if err != nil || res.IsError {
		t.Fatalf("toggle explicit false failed: %v", err)
	}
	if rawOut.(linkOut).Enabled != false {
		t.Errorf("expected Enabled == false, got true")
	}

	// 4. Missing ID
	if _, _, err := p.mcpToggleShortlink(ctx, nil, toggleShortlinkInput{ID: 0}); err == nil {
		t.Error("expected error for ID=0")
	}

	// 5. Cross-tenant isolation
	ctx2 := plugin.WithOrgID(context.Background(), 2)
	if _, _, err := p.mcpToggleShortlink(ctx2, nil, toggleShortlinkInput{ID: linkID}); err == nil {
		t.Error("expected error for cross-tenant toggle")
	}
}

func TestMCPGetLinkAnalytics(t *testing.T) {
	p, _ := setupFullLinksTestDB(t)
	ctx := plugin.WithOrgID(context.Background(), 1)

	_, rawOut, err := p.mcpCreateLink(ctx, nil, createLinkInput{
		Target: "https://example.com/stats",
		Slug:   "stats-slug",
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	linkID := rawOut.(linkOut).ID

	// Seed some LinkEvents
	now := time.Now()
	p.db.Create(&LinkEvent{LinkID: linkID, CreatedAt: now, Country: "US", Device: "Mobile", Referer: "https://google.com", Fingerprint: "fp1"})
	p.db.Create(&LinkEvent{LinkID: linkID, CreatedAt: now, Country: "US", Device: "Desktop", Referer: "https://google.com", Fingerprint: "fp1"})
	p.db.Create(&LinkEvent{LinkID: linkID, CreatedAt: now, Country: "JP", Device: "Mobile", Referer: "https://twitter.com", Fingerprint: "fp2"})

	// Query UV stats
	res, rawStats, err := p.mcpGetLinkAnalytics(ctx, nil, getLinkAnalyticsInput{
		ID:     linkID,
		Days:   30,
		Metric: "uv",
	})
	if err != nil || res.IsError {
		t.Fatalf("get analytics failed: %v", err)
	}
	stats := rawStats.(map[string]any)
	if stats["total"].(int64) != 3 {
		t.Errorf("total = %v, want 3", stats["total"])
	}
	if stats["metric"] != "uv" {
		t.Errorf("metric = %v, want uv", stats["metric"])
	}

	// Query PV stats
	_, rawStatsPV, err := p.mcpGetLinkAnalytics(ctx, nil, getLinkAnalyticsInput{
		ID:     linkID,
		Days:   7,
		Metric: "pv",
	})
	if err != nil {
		t.Fatalf("get analytics pv failed: %v", err)
	}
	statsPV := rawStatsPV.(map[string]any)
	if statsPV["metric"] != "pv" {
		t.Errorf("metric = %v, want pv", statsPV["metric"])
	}

	// Cross-tenant isolation
	ctx2 := plugin.WithOrgID(context.Background(), 2)
	if _, _, err := p.mcpGetLinkAnalytics(ctx2, nil, getLinkAnalyticsInput{ID: linkID}); err == nil {
		t.Error("expected error for cross-tenant get analytics")
	}
}

func TestLinksCapabilities(t *testing.T) {
	p := New()
	// Unconfigured / no geo
	out, err := p.linksCapabilities(context.Background(), nil)
	if err != nil {
		t.Fatalf("linksCapabilities: %v", err)
	}
	if out.Body.GeoReady != false || out.Body.Mode != "unavailable" {
		t.Errorf("unwired capabilities: %+v", out.Body)
	}

	// Configured with mock GeoLookup
	p.ctx = &plugin.Context{
		GeoLookup: func(ip string) (string, string, string) {
			if ip == "8.8.8.8" {
				return "US", "CA", "Mountain View"
			}
			return "", "", ""
		},
	}
	outReady, err := p.linksCapabilities(context.Background(), nil)
	if err != nil {
		t.Fatalf("linksCapabilities ready: %v", err)
	}
	if outReady.Body.GeoReady != true || outReady.Body.Mode != "configured" {
		t.Errorf("wired capabilities: %+v", outReady.Body)
	}
}
