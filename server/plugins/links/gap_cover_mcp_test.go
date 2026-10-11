package links

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/server/plugin"
)

// --- link_crawl.go / service.go ---

func TestParsePageMeta(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		body      string
		wantTitle string
		wantDesc  string
	}{
		{name: "og:title wins over title", body: `<meta property="og:title" content="OG Title"><title>Plain Title</title>`, wantTitle: "OG Title"},
		{name: "og:title after content attribute", body: `<meta content="Swapped OG" property="og:title">`, wantTitle: "Swapped OG"},
		{name: "plain title fallback", body: `<html><head><title>Hello &amp; Welcome</title></head></html>`, wantTitle: "Hello & Welcome"},
		{name: "description", body: `<meta name="description" content="A useful description">`, wantDesc: "A useful description"},
		{name: "no metadata", body: `<html><body>nothing here</body></html>`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			title, desc := parsePageMeta([]byte(c.body))
			if title != c.wantTitle {
				t.Errorf("title = %q, want %q", title, c.wantTitle)
			}
			if desc != c.wantDesc {
				t.Errorf("desc = %q, want %q", desc, c.wantDesc)
			}
		})
	}
}

func TestHandleLinkCrawl(t *testing.T) {
	p, _ := setupFullLinksTestDB(t)
	wipeLinksTables(t, p)

	if err := p.handleLinkCrawl(context.Background(), []byte("{not-json")); err == nil {
		t.Fatal("expected an error for malformed payload")
		return
	}

	if err := p.db.Create(&Link{OrgID: 1, Slug: "crawl-me", Target: "https://example.com"}).Error; err != nil {
		t.Fatalf("seed link: %v", err)
	}

	// A loopback target is refused by the SSRF-guarded client, so the crawl
	// yields no title and must leave the row untouched — without erroring.
	payload, _ := json.Marshal(map[string]any{"id": 1, "target": "http://127.0.0.1/x"})
	if err := p.handleLinkCrawl(context.Background(), payload); err != nil {
		t.Fatalf("handleLinkCrawl with unresolvable target: %v", err)
	}
	var l Link
	if err := p.db.Where("slug = ?", "crawl-me").First(&l).Error; err != nil {
		t.Fatalf("reload link: %v", err)
	}
	if l.Title != "" {
		t.Errorf("title was set without a successful fetch: %q", l.Title)
	}
}

func TestCreateLinkService(t *testing.T) {
	p, _ := setupFullLinksTestDB(t)
	wipeLinksTables(t, p)

	published := 0
	p.publishEvent = func(orgID uint, event string, data any) {
		if event == "link.create" {
			published++
		}
	}
	var cacheKeys []string
	p.deleteCache = func(_ context.Context, key string) error {
		cacheKeys = append(cacheKeys, key)
		return nil
	}

	slug, err := p.CreateLink(context.Background(), 7, "example.com/path")
	if err != nil {
		t.Fatalf("CreateLink: %v", err)
	}
	if slug == "" {
		t.Fatal("CreateLink returned an empty slug")
	}
	var l Link
	if err := p.db.Where("owner_id = ? AND slug = ?", 7, slug).First(&l).Error; err != nil {
		t.Fatalf("link row not created: %v", err)
	}
	if l.Target != "https://example.com/path" {
		t.Errorf("target not normalized: %q", l.Target)
	}
	if !l.Enabled {
		t.Error("service-created link must be enabled")
	}
	if published != 1 {
		t.Errorf("expected 1 link.create event, got %d", published)
	}
	if len(cacheKeys) != 1 {
		t.Errorf("expected 1 cache invalidation, got %d", len(cacheKeys))
	}

	if _, err := p.CreateLink(context.Background(), 0, "https://x.example"); err == nil {
		t.Error("CreateLink with orgID 0 must fail")
	}
	if _, err := p.CreateLink(context.Background(), 7, "javascript:alert(1)"); err == nil {
		t.Error("CreateLink with a dangerous target must fail")
	}
}

// --- plugin.go ---

func TestInstanceMenusAnnouncesSettingsPage(t *testing.T) {
	menus := New().InstanceMenus()
	if len(menus) != 1 || menus[0].ID != "links-instance-settings" {
		t.Fatalf("unexpected instance menus: %+v", menus)
	}
}

func TestStartClosesEngineOnContextDone(t *testing.T) {
	eng := newTestEngine(t)
	p := New()
	p.engine = eng

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		p.Start(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after context cancellation")
	}

	select {
	case _, ok := <-eng.queue:
		if ok {
			t.Error("engine queue is still open after Start returned")
		}
	default:
		t.Error("engine queue read blocked; expected it to be closed")
	}
}

func TestSplitListSkipsEmptyParts(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"a", []string{"a"}},
		{"a, b,,c", []string{"a", "b", "c"}},
		{" , , ", nil},
	}
	for _, c := range cases {
		got := splitList(c.in)
		if len(got) != len(c.want) {
			t.Errorf("splitList(%q) = %v, want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("splitList(%q) = %v, want %v", c.in, got, c.want)
			}
		}
	}
}

func TestIsReservedSlug(t *testing.T) {
	p := New()
	if !p.isReservedSlug("ADMIN") {
		t.Error("builtin reserved slug must be case-insensitively reserved")
	}
	if p.isReservedSlug("sales") {
		t.Error("ordinary slug must not be reserved")
	}

	p.getGlobalSetting = func(key string) string {
		if key == "reserved_slugs" {
			return "support, helpdesk"
		}
		return ""
	}
	if !p.isReservedSlug("support") {
		t.Error("globally configured reserved slug not honored")
	}
	if !p.isReservedSlug("HELPDESK") {
		t.Error("globally configured reserved slug must be case-insensitive")
	}
}

func TestCleanupEventsRemovesOldRows(t *testing.T) {
	p, _ := setupFullLinksTestDB(t)
	wipeLinksTables(t, p)

	old := time.Now().AddDate(0, 0, -60)
	if err := p.db.Create(&LinkEvent{LinkID: 1, CreatedAt: old}).Error; err != nil {
		t.Fatalf("seed old event: %v", err)
	}
	if err := p.db.Create(&LinkEvent{LinkID: 1, CreatedAt: old.Add(-time.Hour)}).Error; err != nil {
		t.Fatalf("seed second old event: %v", err)
	}
	if err := p.db.Create(&LinkEvent{LinkID: 1, CreatedAt: time.Now()}).Error; err != nil {
		t.Fatalf("seed recent event: %v", err)
	}

	p.cleanupEvents(context.Background(), 30)
	var remaining int64
	p.db.Model(&LinkEvent{}).Count(&remaining)
	if remaining != 1 {
		t.Errorf("expected 1 event to survive cleanup, got %d", remaining)
	}

	// Non-positive retention is a no-op that must not touch anything.
	p.cleanupEvents(context.Background(), 0)
	var still int64
	p.db.Model(&LinkEvent{}).Count(&still)
	if still != 1 {
		t.Errorf("cleanup with retention 0 deleted rows: %d remain", still)
	}
}

// --- mcp.go ---

func TestRegisterMCPRegistersListLinksTool(t *testing.T) {
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

	New().RegisterMCP(server)

	res, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	var names []string
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	if !slicesContains(names, "octarq_network__list_links") {
		t.Errorf("octarq_network__list_links tool not registered; got %v", names)
	}
}

func slicesContains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func TestMCPListLinks(t *testing.T) {
	p, _ := setupFullLinksTestDB(t)
	wipeLinksTables(t, p)
	if err := p.db.Create(&Link{OrgID: 1, Slug: "aa", Target: "https://a.example", Tags: "promo"}).Error; err != nil {
		t.Fatalf("seed aa: %v", err)
	}
	if err := p.db.Create(&Link{OrgID: 1, Slug: "bb", Target: "https://b.example", Host: "go.example"}).Error; err != nil {
		t.Fatalf("seed bb: %v", err)
	}
	if err := p.db.Create(&Link{OrgID: 2, Slug: "cc", Target: "https://c.example"}).Error; err != nil {
		t.Fatalf("seed cc: %v", err)
	}

	ctx := plugin.WithOrgID(context.Background(), 1)

	// No org in context is refused.
	if _, _, err := p.mcpListLinks(context.Background(), nil, listLinksInput{}); err == nil {
		t.Fatal("expected errNoOrgInContext without a workspace in context")
		return
	}

	result, out, err := p.mcpListLinks(ctx, nil, listLinksInput{})
	if err != nil {
		t.Fatalf("mcpListLinks: %v", err)
	}
	list, ok := out.([]linkOut)
	if !ok {
		t.Fatalf("unexpected output type %T", out)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 links for org 1, got %d", len(list))
	}
	text := ""
	if tc, ok := result.Content[0].(*mcp.TextContent); ok {
		text = tc.Text
	}
	if !strings.Contains(text, "aa") {
		t.Errorf("json result missing link aa: %s", text)
	}

	_, filtered, err := p.mcpListLinks(ctx, nil, listLinksInput{Host: "go.example"})
	if err != nil {
		t.Fatalf("host-filtered call: %v", err)
	}
	if len(filtered.([]linkOut)) != 1 || filtered.([]linkOut)[0].Slug != "bb" {
		t.Errorf("host filter: got %+v", filtered)
	}

	_, tagged, err := p.mcpListLinks(ctx, nil, listLinksInput{Tag: "promo"})
	if err != nil {
		t.Fatalf("tag-filtered call: %v", err)
	}
	if len(tagged.([]linkOut)) != 1 || tagged.([]linkOut)[0].Slug != "aa" {
		t.Errorf("tag filter: got %+v", tagged)
	}

	if _, _, err := p.mcpListLinks(ctx, nil, listLinksInput{Limit: -3}); err != nil {
		t.Errorf("negative limit: %v", err)
	}
	if _, _, err := p.mcpListLinks(ctx, nil, listLinksInput{Limit: 500}); err != nil {
		t.Errorf("oversized limit: %v", err)
	}
}
