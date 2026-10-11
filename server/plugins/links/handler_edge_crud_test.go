package links

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugins/dns"
)

func TestListLinksFiltersAndPaging(t *testing.T) {
	t.Parallel()
	p, mkCtx := setupFullLinksTestDB(t)
	wipeLinksTables(t, p)
	ctx := context.Background()

	mk := func(slug, host, tags string, archived bool) {
		if err := p.db.Create(&Link{OrgID: 1, Host: host, Slug: slug, Target: "https://t.example", Title: "T " + slug, Tags: tags, Archived: archived}).Error; err != nil {
			t.Fatalf("seed %s: %v", slug, err)
		}
	}
	mk("one", "h1.example", "marketing", false)
	mk("two", "h1.example", "sales", false)
	mk("three", "h2.example", "marketing", true)

	req := httptest.NewRequest(http.MethodGet, "/api/links", nil)
	req.Header.Set("X-Org-ID", "1")

	// Active only by default.
	out, err := p.listLinks(ctx, &ListLinksInput{Ctx: mkCtx(req)})
	if err != nil {
		t.Fatalf("listLinks: %v", err)
	}
	if len(out.Body) != 2 {
		t.Errorf("default list: got %d active links, want 2", len(out.Body))
	}

	// Archived filter exposes the hidden row.
	reqA := httptest.NewRequest(http.MethodGet, "/api/links?archived=1", nil)
	outA, err := p.listLinks(ctx, &ListLinksInput{Ctx: mkCtx(reqA), Archived: "1"})
	if err != nil {
		t.Fatalf("archived list: %v", err)
	}
	if len(outA.Body) != 1 || outA.Body[0].Slug != "three" {
		t.Errorf("archived list: got %+v", outA.Body)
	}

	// Tag filter.
	outT, err := p.listLinks(ctx, &ListLinksInput{Ctx: mkCtx(req), Tag: "sales"})
	if err != nil {
		t.Fatalf("tag list: %v", err)
	}
	if len(outT.Body) != 1 || outT.Body[0].Slug != "two" {
		t.Errorf("tag list: got %+v", outT.Body)
	}

	// Host filter.
	outH, err := p.listLinks(ctx, &ListLinksInput{Ctx: mkCtx(req), Host: "h2.example"})
	if err != nil {
		t.Fatalf("host list: %v", err)
	}
	if len(outH.Body) != 0 {
		t.Errorf("host list for h2.example: got %d links, want 0 (only archived lives there)", len(outH.Body))
	}

	// Limit clamps: negative/zero -> 50 default, >500 -> 50.
	if _, err := p.listLinks(ctx, &ListLinksInput{Ctx: mkCtx(req), Limit: 600}); err != nil {
		t.Errorf("oversized limit: %v", err)
	}
	paged, err := p.listLinks(ctx, &ListLinksInput{Ctx: mkCtx(req), Limit: 1})
	if err != nil {
		t.Fatalf("paged list: %v", err)
	}
	if len(paged.Body) != 1 {
		t.Errorf("limit=1: got %d links", len(paged.Body))
	}

	// Q matches title/slug/target/note/tags.
	outQ, err := p.listLinks(ctx, &ListLinksInput{Ctx: mkCtx(req), Q: "T two"})
	if err != nil {
		t.Fatalf("q list: %v", err)
	}
	if len(outQ.Body) != 1 || outQ.Body[0].Slug != "two" {
		t.Errorf("q list: got %+v", outQ.Body)
	}

	// Unauthenticated (org 0) is refused.
	req0 := httptest.NewRequest(http.MethodGet, "/api/links", nil)
	req0.Header.Set("X-Org-ID", "0")
	if _, err := p.listLinks(ctx, &ListLinksInput{Ctx: mkCtx(req0)}); err == nil {
		t.Error("listLinks with org 0 must fail")
	}
}

func TestGetLinkScopedToOrg(t *testing.T) {
	t.Parallel()
	p, mkCtx := setupFullLinksTestDB(t)
	wipeLinksTables(t, p)
	ctx := context.Background()

	mine := Link{OrgID: 1, Slug: "mine", Target: "https://x"}
	if err := p.db.Create(&mine).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	theirs := Link{OrgID: 2, Slug: "theirs", Target: "https://y"}
	if err := p.db.Create(&theirs).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/links/1", nil)
	out, err := p.getLink(ctx, &GetLinkInput{Ctx: mkCtx(req), ID: mine.ID})
	if err != nil {
		t.Fatalf("getLink own: %v", err)
	}
	if out.Body.Slug != "mine" {
		t.Errorf("got %+v", out.Body)
	}

	// Another org's link is invisible.
	req2 := httptest.NewRequest(http.MethodGet, "/api/links/2", nil)
	if _, err := p.getLink(ctx, &GetLinkInput{Ctx: mkCtx(req2), ID: theirs.ID}); err == nil {
		t.Error("cross-org getLink must 404")
	}

	// Missing id.
	if _, err := p.getLink(ctx, &GetLinkInput{Ctx: mkCtx(req), ID: 999}); err == nil {
		t.Error("missing link must 404")
	}
}

func TestLinkMetadataHandler(t *testing.T) {
	t.Parallel()
	p, mkCtx := setupFullLinksTestDB(t)
	wipeLinksTables(t, p)
	ctx := context.Background()

	req := httptest.NewRequest(http.MethodGet, "/api/links/metadata?url=127.0.0.1", nil)
	out, err := p.linkMetadata(ctx, &LinkMetadataInput{Ctx: mkCtx(req), URL: "127.0.0.1"})
	if err != nil {
		t.Fatalf("linkMetadata: %v", err)
	}
	body := out.Body
	if body["favicon"] != "https://127.0.0.1/favicon.ico" {
		t.Errorf("favicon = %v", body["favicon"])
	}
	// The SSRF-guarded client refuses loopback, so title/description stay empty
	// rather than a network call happening in the test.
	if body["title"] != "" || body["description"] != "" {
		t.Errorf("unexpected fetched metadata: %v", body)
	}

	bad := []struct{ input string }{
		{""},
		{"javascript:alert(1)"},
		{"file:///etc/passwd"},
	}
	for _, c := range bad {
		_, err := p.linkMetadata(ctx, &LinkMetadataInput{Ctx: mkCtx(req), URL: c.input})
		if err == nil {
			t.Errorf("linkMetadata(%q) must fail", c.input)
		}
	}

	req0 := httptest.NewRequest(http.MethodGet, "/", nil)
	req0.Header.Set("X-Org-ID", "0")
	if _, err := p.linkMetadata(ctx, &LinkMetadataInput{Ctx: mkCtx(req0), URL: "example.com"}); err == nil {
		t.Error("linkMetadata with org 0 must fail")
	}
}

func TestCreateLinkEdgeBranches(t *testing.T) {
	t.Parallel()
	p, mkCtx := setupFullLinksTestDB(t)
	wipeLinksTables(t, p)
	ctx := context.Background()

	req := httptest.NewRequest(http.MethodPost, "/api/links", nil)
	req.Header.Set("X-Org-ID", "1")

	// Empty target.
	if _, err := p.createLink(ctx, &CreateLinkInput{Ctx: mkCtx(req), Body: linkDTO{Target: "  "}}); err == nil {
		t.Error("empty target must fail")
	}
	// Dangerous scheme.
	if _, err := p.createLink(ctx, &CreateLinkInput{Ctx: mkCtx(req), Body: linkDTO{Slug: "x", Target: "javascript:alert(1)"}}); err == nil {
		t.Error("javascript target must fail")
	}
	// Reserved slug.
	if _, err := p.createLink(ctx, &CreateLinkInput{Ctx: mkCtx(req), Body: linkDTO{Slug: "admin", Target: "https://x.example"}}); err == nil {
		t.Error("reserved slug must be refused")
	}

	// Host owned by another org -> 403.
	p.db.Create(&dns.Domain{OrgID: 2, Name: "org2-host.example", ForLink: true})
	if _, err := p.createLink(ctx, &CreateLinkInput{Ctx: mkCtx(req), Body: linkDTO{Slug: "h", Host: "org2-host.example", Target: "https://x.example"}}); err == nil {
		t.Error("unowned host must be refused")
	}

	// Multi-tenant mode with a base domain + org link host: hostless links are refused.
	p.db.Where("key = ?", models.BaseDomainSetting).Delete(&models.Setting{})
	p.db.Create(&models.Setting{Key: models.BaseDomainSetting, Value: "octarq.test"})
	p.db.Create(&dns.Domain{OrgID: 1, Name: "own.example", ForLink: true})
	reqH := httptest.NewRequest(http.MethodPost, "/api/links", nil)
	reqH.Header.Set("X-Org-ID", "1")
	if _, err := p.createLink(ctx, &CreateLinkInput{Ctx: mkCtx(reqH), Body: linkDTO{Slug: "nohost", Target: "https://x.example"}}); err == nil {
		t.Error("hostless link must be refused when a base domain is configured")
	}
	// The base-domain scenario is test-scoped; drop it so the remaining creates
	// in this test are host-optional again.
	p.db.Where("key = ?", models.BaseDomainSetting).Delete(&models.Setting{})

	// Duplicate (host, slug) -> 409.
	if _, err := p.createLink(ctx, &CreateLinkInput{Ctx: mkCtx(req), Body: linkDTO{Slug: "dup", Target: "https://x.example"}}); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if _, err := p.createLink(ctx, &CreateLinkInput{Ctx: mkCtx(req), Body: linkDTO{Slug: "dup", Target: "https://y.example"}}); err == nil {
		t.Error("duplicate slug must be refused")
	}

	// Success path with hooks: enabled=false respected, audit + event + cache.
	audited := 0
	p.audit = func(*http.Request, string, string, uint, map[string]any) { audited++ }
	published := 0
	p.publishEvent = func(uint, string, any) { published++ }
	var invalidations []string
	p.deleteCache = func(_ context.Context, key string) error { invalidations = append(invalidations, key); return nil }

	disabled := false
	out, err := p.createLink(ctx, &CreateLinkInput{Ctx: mkCtx(req), Body: linkDTO{Slug: "ok", Target: "https://ok.example", Enabled: &disabled}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if out.Body.Slug != "ok" || out.Body.Target != "https://ok.example" {
		t.Errorf("unexpected created link: %+v", out.Body)
	}
	if audited != 1 || published != 1 || len(invalidations) != 1 {
		t.Errorf("hooks: audit=%d published=%d cache=%d, want all 1", audited, published, len(invalidations))
	}
}
