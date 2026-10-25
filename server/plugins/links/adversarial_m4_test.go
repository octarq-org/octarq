package links

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
)

// TestAdversarial_UpdateShortlink_PayloadAndID tests invalid payload handling,
// non-existent IDs, and input sanitization on update_shortlink.
func TestAdversarial_UpdateShortlink_PayloadAndID(t *testing.T) {
	p, _ := setupFullLinksTestDB(t)
	ctx1 := plugin.WithOrgID(context.Background(), 1)

	// Create a baseline link
	_, raw, err := p.mcpCreateLink(ctx1, nil, createLinkInput{
		Target: "https://initial.example.com",
		Slug:   "adv-update-base",
		Title:  "Initial Base",
	})
	if err != nil {
		t.Fatalf("failed to create base link: %v", err)
	}
	baseLink := raw.(linkOut)

	// 1. Invalid ID = 0
	if _, _, err := p.mcpUpdateLink(ctx1, nil, updateLinkInput{ID: 0}); err == nil {
		t.Error("expected error for ID = 0")
	}

	// 2. Non-existent ID = 99999999
	target := "https://valid.example.com"
	if _, _, err := p.mcpUpdateLink(ctx1, nil, updateLinkInput{ID: 99999999, Target: &target}); err == nil {
		t.Error("expected error for non-existent link ID")
	}

	// 3. No workspace in context
	if _, _, err := p.mcpUpdateLink(context.Background(), nil, updateLinkInput{ID: baseLink.ID, Target: &target}); err == nil {
		t.Error("expected errNoOrgInContext when context lacks org ID")
	}

	// 4. Invalid Target scheme (javascript:, data:, vbscript:)
	invalidTargets := []string{
		"javascript:alert(1)",
		"data:text/html,<script>evil()</script>",
		"vbscript:msgbox",
		"file:///etc/passwd",
	}
	for _, badTarget := range invalidTargets {
		bad := badTarget
		_, _, err := p.mcpUpdateLink(ctx1, nil, updateLinkInput{
			ID:     baseLink.ID,
			Target: &bad,
		})
		if err == nil {
			t.Errorf("expected error for dangerous target scheme %q", bad)
		}
	}

	// 5. Invalid ExpiredURL scheme
	for _, badExp := range invalidTargets {
		bad := badExp
		_, _, err := p.mcpUpdateLink(ctx1, nil, updateLinkInput{
			ID:         baseLink.ID,
			ExpiredURL: &bad,
		})
		if err == nil {
			t.Errorf("expected error for dangerous expiredUrl %q", bad)
		}
	}

	// 6. Invalid RoutingRule target
	badRules := RoutingRules{
		{Type: "geo", Match: "US", Target: "javascript:evil()"},
	}
	if _, _, err := p.mcpUpdateLink(ctx1, nil, updateLinkInput{
		ID:           baseLink.ID,
		RoutingRules: &badRules,
	}); err == nil {
		t.Error("expected error for dangerous routing rule target")
	}

	// 7. Invalid RFC3339 expiresAt format
	badDates := []string{"not-a-date", "2026-13-45", "tomorrow", "2026/10/25"}
	for _, badDate := range badDates {
		bad := badDate
		_, _, err := p.mcpUpdateLink(ctx1, nil, updateLinkInput{
			ID:        baseLink.ID,
			ExpiresAt: &bad,
		})
		if err == nil {
			t.Errorf("expected error for invalid expiresAt %q", bad)
		}
	}

	// 8. Valid RFC3339 expiresAt then clear with empty string
	validDate := "2030-12-31T23:59:59Z"
	_, _, err = p.mcpUpdateLink(ctx1, nil, updateLinkInput{
		ID:        baseLink.ID,
		ExpiresAt: &validDate,
	})
	if err != nil {
		t.Fatalf("failed to update valid expiresAt: %v", err)
	}
	var checkL Link
	p.db.First(&checkL, baseLink.ID)
	if checkL.ExpiresAt == nil || checkL.ExpiresAt.Format(time.RFC3339) != validDate {
		t.Errorf("expiresAt not updated: %+v", checkL.ExpiresAt)
	}

	// Clear expiresAt
	emptyDate := ""
	_, _, err = p.mcpUpdateLink(ctx1, nil, updateLinkInput{
		ID:        baseLink.ID,
		ExpiresAt: &emptyDate,
	})
	if err != nil {
		t.Fatalf("failed to clear expiresAt: %v", err)
	}
	var cleared Link
	p.db.First(&cleared, baseLink.ID)
	if cleared.ExpiresAt != nil {
		t.Errorf("expected expiresAt to be cleared, got: %v", cleared.ExpiresAt)
	}
}

// TestAdversarial_UpdateShortlink_ConflictingSlugsAndHost tests unique slug
// constraints on same host and workspace host ownership.
func TestAdversarial_UpdateShortlink_ConflictingSlugsAndHost(t *testing.T) {
	p, _ := setupFullLinksTestDB(t)
	ctx1 := plugin.WithOrgID(context.Background(), 1)

	// Create Link A with slug "slug-alpha"
	_, rawA, err := p.mcpCreateLink(ctx1, nil, createLinkInput{
		Target: "https://alpha.example.com",
		Slug:   "slug-alpha",
	})
	if err != nil {
		t.Fatalf("create Link A: %v", err)
	}
	linkA := rawA.(linkOut)

	// Create Link B with slug "slug-beta"
	_, rawB, err := p.mcpCreateLink(ctx1, nil, createLinkInput{
		Target: "https://beta.example.com",
		Slug:   "slug-beta",
	})
	if err != nil {
		t.Fatalf("create Link B: %v", err)
	}
	linkB := rawB.(linkOut)

	// 1. Attempt to update Link B's slug to "slug-alpha" (conflicting slug)
	targetSlug := "slug-alpha"
	_, _, err = p.mcpUpdateLink(ctx1, nil, updateLinkInput{
		ID:   linkB.ID,
		Slug: &targetSlug,
	})
	if err == nil {
		t.Error("expected unique constraint error when updating to conflicting slug")
	}

	// Verify both links remain untouched in DB
	var checkA, checkB Link
	p.db.First(&checkA, linkA.ID)
	p.db.First(&checkB, linkB.ID)
	if checkA.Slug != "slug-alpha" || checkB.Slug != "slug-beta" {
		t.Fatalf("slugs corrupted after conflict: A=%q, B=%q", checkA.Slug, checkB.Slug)
	}

	// 2. Updating Link A to its own existing slug should succeed
	sameSlug := "slug-alpha"
	newTitle := "Same Slug Updated Title"
	_, rawOut, err := p.mcpUpdateLink(ctx1, nil, updateLinkInput{
		ID:    linkA.ID,
		Slug:  &sameSlug,
		Title: &newTitle,
	})
	if err != nil {
		t.Fatalf("expected update with identical slug to succeed: %v", err)
	}
	if rawOut.(linkOut).Title != newTitle {
		t.Errorf("title not updated: %+v", rawOut)
	}

	// 3. Updating to reserved slug should fail
	reservedSlug := "admin"
	_, _, err = p.mcpUpdateLink(ctx1, nil, updateLinkInput{
		ID:   linkA.ID,
		Slug: &reservedSlug,
	})
	if err == nil {
		t.Error("expected error when updating to reserved slug 'admin'")
	}

	// 4. Updating to unowned host should fail
	unownedHost := "unowned-domain.org"
	_, _, err = p.mcpUpdateLink(ctx1, nil, updateLinkInput{
		ID:   linkA.ID,
		Host: &unownedHost,
	})
	if err == nil {
		t.Error("expected error when updating to unowned host")
	}
}

// TestAdversarial_UpdateShortlink_TenantIsolation verifies that Org 2 cannot update
// or hijack links owned by Org 1.
func TestAdversarial_UpdateShortlink_TenantIsolation(t *testing.T) {
	p, _ := setupFullLinksTestDB(t)
	ctx1 := plugin.WithOrgID(context.Background(), 1)
	ctx2 := plugin.WithOrgID(context.Background(), 2)
	ctx3 := plugin.WithOrgID(context.Background(), 3)

	_, rawA, err := p.mcpCreateLink(ctx1, nil, createLinkInput{
		Target: "https://org1.example.com",
		Slug:   "org1-private-link",
		Title:  "Org 1 Secret",
	})
	if err != nil {
		t.Fatalf("create Link in Org 1: %v", err)
	}
	linkID := rawA.(linkOut).ID

	hijackTarget := "https://hacked.example.com"
	hijackTitle := "Hijacked by Org 2"

	// Org 2 tries to update
	_, _, err = p.mcpUpdateLink(ctx2, nil, updateLinkInput{
		ID:     linkID,
		Target: &hijackTarget,
		Title:  &hijackTitle,
	})
	if err == nil {
		t.Fatal("security violation: Org 2 updated Org 1's link")
	}
	if err.Error() != "link not found" {
		t.Errorf("expected 'link not found', got %q", err.Error())
	}

	// Org 3 tries to update
	_, _, err = p.mcpUpdateLink(ctx3, nil, updateLinkInput{
		ID:     linkID,
		Target: &hijackTarget,
	})
	if err == nil {
		t.Fatal("security violation: Org 3 updated Org 1's link")
	}

	// Verify DB row remains unchanged
	var link Link
	p.db.First(&link, linkID)
	if link.Target != "https://org1.example.com" || link.Title != "Org 1 Secret" {
		t.Fatalf("Org 1 link corrupted: %+v", link)
	}
}

// TestAdversarial_ToggleShortlink_StressAndIsolation tests repeated toggling,
// explicit boolean flags, tenant isolation, and cache invalidation.
func TestAdversarial_ToggleShortlink_StressAndIsolation(t *testing.T) {
	p, _ := setupFullLinksTestDB(t)
	ctx1 := plugin.WithOrgID(context.Background(), 1)
	ctx2 := plugin.WithOrgID(context.Background(), 2)

	var invalidatedKeys []string
	p.deleteCache = func(_ context.Context, key string) error {
		invalidatedKeys = append(invalidatedKeys, key)
		return nil
	}

	_, raw, err := p.mcpCreateLink(ctx1, nil, createLinkInput{
		Target: "https://toggle.example.com",
		Slug:   "adv-toggle",
	})
	if err != nil {
		t.Fatalf("create link: %v", err)
	}
	linkID := raw.(linkOut).ID

	// 1. Missing ID
	if _, _, err := p.mcpToggleShortlink(ctx1, nil, toggleShortlinkInput{ID: 0}); err == nil {
		t.Error("expected error for toggle ID = 0")
	}

	// 2. Non-existent ID
	if _, _, err := p.mcpToggleShortlink(ctx1, nil, toggleShortlinkInput{ID: 99999999}); err == nil {
		t.Error("expected error for non-existent ID")
	}

	// 3. No org in context
	if _, _, err := p.mcpToggleShortlink(context.Background(), nil, toggleShortlinkInput{ID: linkID}); err == nil {
		t.Error("expected errNoOrgInContext")
	}

	// 4. Cross-tenant isolation: Org 2 cannot toggle Org 1's link
	if _, _, err := p.mcpToggleShortlink(ctx2, nil, toggleShortlinkInput{ID: linkID}); err == nil {
		t.Fatal("security violation: Org 2 toggled Org 1's link")
	}

	// 5. Stress toggle sequence: 10 repeated toggles without explicit enabled
	expectedState := true // started true
	for i := 0; i < 10; i++ {
		expectedState = !expectedState
		res, rawOut, err := p.mcpToggleShortlink(ctx1, nil, toggleShortlinkInput{ID: linkID})
		if err != nil || res.IsError {
			t.Fatalf("toggle iteration %d failed: %v", i, err)
		}
		out := rawOut.(linkOut)
		if out.Enabled != expectedState {
			t.Errorf("iter %d: expected enabled=%v, got %v", i, expectedState, out.Enabled)
		}

		// Verify in DB directly
		var l Link
		p.db.First(&l, linkID)
		if l.Enabled != expectedState {
			t.Errorf("iter %d DB: expected enabled=%v, got %v", i, expectedState, l.Enabled)
		}
	}

	// 6. Explicit enabled=false when already false
	falseVal := false
	trueVal := true
	if expectedState {
		// Set to false first
		p.mcpToggleShortlink(ctx1, nil, toggleShortlinkInput{ID: linkID, Enabled: &falseVal})
	}
	// Now explicitly set false again
	_, rawOut, err := p.mcpToggleShortlink(ctx1, nil, toggleShortlinkInput{ID: linkID, Enabled: &falseVal})
	if err != nil {
		t.Fatalf("explicit false failed: %v", err)
	}
	if rawOut.(linkOut).Enabled != false {
		t.Errorf("expected false, got true")
	}

	// Explicitly set true twice
	p.mcpToggleShortlink(ctx1, nil, toggleShortlinkInput{ID: linkID, Enabled: &trueVal})
	_, rawOut, err = p.mcpToggleShortlink(ctx1, nil, toggleShortlinkInput{ID: linkID, Enabled: &trueVal})
	if err != nil {
		t.Fatalf("explicit true failed: %v", err)
	}
	if rawOut.(linkOut).Enabled != true {
		t.Errorf("expected true, got false")
	}

	// 7. Verify cache invalidation occurred for each toggle
	expectedKey := "link:redirect::adv-toggle"
	foundKey := false
	for _, k := range invalidatedKeys {
		if k == expectedKey {
			foundKey = true
			break
		}
	}
	if !foundKey {
		t.Errorf("expected cache key %q in invalidated keys: %v", expectedKey, invalidatedKeys)
	}
}

// TestAdversarial_GetLinkAnalytics_BoundariesAndEdgeCases tests day boundaries
// (0, 365, 366, negative, large), metric variants, empty events, and tenant isolation.
func TestAdversarial_GetLinkAnalytics_BoundariesAndEdgeCases(t *testing.T) {
	p, _ := setupFullLinksTestDB(t)
	ctx1 := plugin.WithOrgID(context.Background(), 1)
	ctx2 := plugin.WithOrgID(context.Background(), 2)

	_, raw, err := p.mcpCreateLink(ctx1, nil, createLinkInput{
		Target: "https://analytics.example.com",
		Slug:   "adv-analytics",
	})
	if err != nil {
		t.Fatalf("create link: %v", err)
	}
	linkID := raw.(linkOut).ID

	// 1. Missing ID / non-existent / no org
	if _, _, err := p.mcpGetLinkAnalytics(ctx1, nil, getLinkAnalyticsInput{ID: 0}); err == nil {
		t.Error("expected error for ID = 0")
	}
	if _, _, err := p.mcpGetLinkAnalytics(ctx1, nil, getLinkAnalyticsInput{ID: 99999999}); err == nil {
		t.Error("expected error for non-existent ID")
	}
	if _, _, err := p.mcpGetLinkAnalytics(context.Background(), nil, getLinkAnalyticsInput{ID: linkID}); err == nil {
		t.Error("expected errNoOrgInContext")
	}

	// 2. Cross-tenant isolation
	if _, _, err := p.mcpGetLinkAnalytics(ctx2, nil, getLinkAnalyticsInput{ID: linkID}); err == nil {
		t.Fatal("security violation: Org 2 accessed Org 1 analytics")
	}

	// 3. Analytics on link with zero events (empty database)
	dayCases := []struct {
		inputDays int
		wantDays  int
	}{
		{inputDays: 0, wantDays: 30},       // 0 -> default 30
		{inputDays: -1, wantDays: 30},      // negative -> default 30
		{inputDays: -500, wantDays: 30},    // negative -> default 30
		{inputDays: 1, wantDays: 1},        // lower bound valid
		{inputDays: 7, wantDays: 7},        // valid
		{inputDays: 30, wantDays: 30},      // default
		{inputDays: 365, wantDays: 365},    // upper bound valid
		{inputDays: 366, wantDays: 30},     // overflow -> default 30
		{inputDays: 1000000, wantDays: 30}, // large overflow -> default 30
	}

	for _, tc := range dayCases {
		res, rawOut, err := p.mcpGetLinkAnalytics(ctx1, nil, getLinkAnalyticsInput{
			ID:   linkID,
			Days: tc.inputDays,
		})
		if err != nil || res.IsError {
			t.Fatalf("days=%d failed: %v", tc.inputDays, err)
		}
		stats := rawOut.(map[string]any)
		if stats["days"] != tc.wantDays {
			t.Errorf("days input %d: got %v, want %d", tc.inputDays, stats["days"], tc.wantDays)
		}
		if stats["total"].(int64) != 0 {
			t.Errorf("expected 0 total events, got %v", stats["total"])
		}
	}

	// 4. Metric normalization
	metricCases := []struct {
		inputMetric string
		wantMetric  string
	}{
		{"uv", "uv"},
		{"UV", "uv"},
		{"pv", "pv"},
		{"PV", "pv"},
		{"  Pv  ", "pv"},
		{"invalid", "uv"},
		{"", "uv"},
		{"random_string", "uv"},
	}
	for _, mc := range metricCases {
		_, rawOut, err := p.mcpGetLinkAnalytics(ctx1, nil, getLinkAnalyticsInput{
			ID:     linkID,
			Days:   30,
			Metric: mc.inputMetric,
		})
		if err != nil {
			t.Fatalf("metric %q failed: %v", mc.inputMetric, err)
		}
		stats := rawOut.(map[string]any)
		if stats["metric"] != mc.wantMetric {
			t.Errorf("metric input %q: got %q, want %q", mc.inputMetric, stats["metric"], mc.wantMetric)
		}
	}

	// 5. Seed diverse events and verify UV deduplication vs PV counting
	now := time.Now()
	// FP "user-1": 3 visits from Google on Desktop (same user)
	p.db.Create(&LinkEvent{LinkID: linkID, CreatedAt: now, Fingerprint: "user-1", Referer: "https://google.com/search", Device: "Desktop", Country: "US"})
	p.db.Create(&LinkEvent{LinkID: linkID, CreatedAt: now, Fingerprint: "user-1", Referer: "https://google.com/search", Device: "Desktop", Country: "US"})
	p.db.Create(&LinkEvent{LinkID: linkID, CreatedAt: now, Fingerprint: "user-1", Referer: "https://google.com/search", Device: "Desktop", Country: "US"})
	// FP "user-2": 1 visit from Twitter on Mobile
	p.db.Create(&LinkEvent{LinkID: linkID, CreatedAt: now, Fingerprint: "user-2", Referer: "https://t.co/abc", Device: "Mobile", Country: "JP"})

	// Query UV
	_, rawUV, err := p.mcpGetLinkAnalytics(ctx1, nil, getLinkAnalyticsInput{ID: linkID, Metric: "uv"})
	if err != nil {
		t.Fatalf("query UV: %v", err)
	}
	statsUV := rawUV.(map[string]any)
	if statsUV["total"].(int64) != 4 {
		t.Errorf("UV total events: got %v, want 4", statsUV["total"])
	}
	countriesUV := statsUV["countries"].([]models.StatKV)
	// US should have count 1 in UV (deduplicated across user-1)
	for _, c := range countriesUV {
		if c.Key == "US" && c.Count != 1 {
			t.Errorf("UV country US: count = %d, want 1", c.Count)
		}
	}

	// Query PV
	_, rawPV, err := p.mcpGetLinkAnalytics(ctx1, nil, getLinkAnalyticsInput{ID: linkID, Metric: "pv"})
	if err != nil {
		t.Fatalf("query PV: %v", err)
	}
	statsPV := rawPV.(map[string]any)
	countriesPV := statsPV["countries"].([]models.StatKV)
	// US should have count 3 in PV (raw count)
	for _, c := range countriesPV {
		if c.Key == "US" && c.Count != 3 {
			t.Errorf("PV country US: count = %d, want 3", c.Count)
		}
	}
}

// TestAdversarial_CapabilitiesEndpoint tests the capabilities & geo-status endpoints
// across unwired, unready, and configured states.
func TestAdversarial_CapabilitiesEndpoint(t *testing.T) {
	p := New()

	// 1. Completely unconfigured plugin
	out, err := p.linksCapabilities(context.Background(), nil)
	if err != nil {
		t.Fatalf("capabilities unwired failed: %v", err)
	}
	if out.Body.GeoReady != false || out.Body.Mode != "unavailable" {
		t.Errorf("expected GeoReady=false, mode='unavailable'; got %+v", out.Body)
	}

	// 2. GeoLookup returns empty string (no DB present on host)
	p.ctx = &plugin.Context{
		GeoLookup: func(ip string) (string, string, string) {
			return "", "", ""
		},
	}
	out, err = p.linksCapabilities(context.Background(), nil)
	if err != nil {
		t.Fatalf("capabilities empty geo failed: %v", err)
	}
	if out.Body.GeoReady != false || out.Body.Mode != "unavailable" {
		t.Errorf("expected GeoReady=false when GeoLookup returns empty; got %+v", out.Body)
	}

	// 3. GeoLookup returns valid country
	p.ctx = &plugin.Context{
		GeoLookup: func(ip string) (string, string, string) {
			if ip == "8.8.8.8" {
				return "US", "CA", "Mountain View"
			}
			return "", "", ""
		},
	}
	out, err = p.linksCapabilities(context.Background(), nil)
	if err != nil {
		t.Fatalf("capabilities configured geo failed: %v", err)
	}
	if out.Body.GeoReady != true || out.Body.Mode != "configured" {
		t.Errorf("expected GeoReady=true, mode='configured'; got %+v", out.Body)
	}

	// 4. Test HTTP routes via Huma mux (/api/links/capabilities and /api/links/geo-status)
	router := http.NewServeMux()
	api := humago.New(router, huma.DefaultConfig("Links Test API", "1.0.0"))
	p.ctx.Huma = api
	p.registerRoutes(p.ctx)

	for _, path := range []string{"/api/links/capabilities", "/api/links/geo-status"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("path %s returned HTTP %d, want 200", path, rec.Code)
		}
		var resp struct {
			GeoReady bool   `json:"geoReady"`
			Mode     string `json:"mode"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response for %s: %v", path, err)
		}
		if !resp.GeoReady || resp.Mode != "configured" {
			t.Errorf("path %s unexpected body: %+v", path, resp)
		}
	}
}

// TestAdversarial_MCPClientSession_EndToEnd executes tool calls through real
// in-memory MCP Client/Server sessions for update_shortlink, toggle_shortlink,
// and get_link_analytics.
func TestAdversarial_MCPClientSession_EndToEnd(t *testing.T) {
	ctx := context.Background()
	p, _ := setupFullLinksTestDB(t)

	// Seed link in Org 1
	l := Link{
		OrgID:   1,
		Slug:    "mcp-e2e-link",
		Target:  "https://e2e.example.com",
		Enabled: true,
	}
	if err := p.db.Create(&l).Error; err != nil {
		t.Fatalf("seed link: %v", err)
	}

	// Connect MCP Server and Client with Org 1 context
	server := mcp.NewServer(&mcp.Implementation{Name: "test-octarq", Version: "1.0"}, nil)
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0"}, nil)
	p.RegisterMCP(server)

	sTrans, cTrans := mcp.NewInMemoryTransports()
	sSes, err := server.Connect(plugin.WithOrgID(ctx, 1), sTrans, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer sSes.Close()
	cSes, err := client.Connect(ctx, cTrans, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer cSes.Close()

	// 1. Call octarq_network__update_shortlink over MCP protocol
	newTarget := "https://e2e-updated.example.com"
	newTitle := "MCP Updated Title"
	updateRes, err := cSes.CallTool(ctx, &mcp.CallToolParams{
		Name: "octarq_network__update_shortlink",
		Arguments: map[string]any{
			"id":     l.ID,
			"target": newTarget,
			"title":  newTitle,
		},
	})
	if err != nil {
		t.Fatalf("CallTool update_shortlink failed: %v", err)
	}
	if len(updateRes.Content) == 0 {
		t.Fatal("empty response content from update_shortlink")
	}
	text := updateRes.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, newTarget) || !strings.Contains(text, newTitle) {
		t.Errorf("update_shortlink response missing updated fields: %s", text)
	}

	// 2. Call octarq_network__toggle_shortlink over MCP protocol
	toggleRes, err := cSes.CallTool(ctx, &mcp.CallToolParams{
		Name: "octarq_network__toggle_shortlink",
		Arguments: map[string]any{
			"id": l.ID,
		},
	})
	if err != nil {
		t.Fatalf("CallTool toggle_shortlink failed: %v", err)
	}
	toggleText := toggleRes.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(toggleText, `"enabled": false`) {
		t.Errorf("toggle_shortlink response missing 'enabled: false': %s", toggleText)
	}

	// 3. Call octarq_network__get_link_analytics over MCP protocol
	analyticsRes, err := cSes.CallTool(ctx, &mcp.CallToolParams{
		Name: "octarq_network__get_link_analytics",
		Arguments: map[string]any{
			"id":     l.ID,
			"days":   14,
			"metric": "pv",
		},
	})
	if err != nil {
		t.Fatalf("CallTool get_link_analytics failed: %v", err)
	}
	analyticsText := analyticsRes.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(analyticsText, `"days": 14`) || !strings.Contains(analyticsText, `"metric": "pv"`) {
		t.Errorf("get_link_analytics response missing parameters: %s", analyticsText)
	}
}
