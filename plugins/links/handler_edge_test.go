package links

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugins/dns"
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

func TestQuickCreateLinkEdgeBranches(t *testing.T) {
	t.Parallel()
	p, mkCtx := setupFullLinksTestDB(t)
	wipeLinksTables(t, p)
	ctx := context.Background()

	req := httptest.NewRequest(http.MethodPost, "/api/links/quick", nil)
	req.Header.Set("X-Org-ID", "1")

	if _, err := p.quickCreateLink(ctx, &QuickCreateLinkInput{Ctx: mkCtx(req), Body: QuickCreateLinkBody{URL: " "}}); err == nil {
		t.Error("empty url must fail")
	}
	if _, err := p.quickCreateLink(ctx, &QuickCreateLinkInput{Ctx: mkCtx(req), Body: QuickCreateLinkBody{URL: "data:text/html,x"}}); err == nil {
		t.Error("dangerous url must fail")
	}

	p.db.Create(&dns.Domain{OrgID: 2, Name: "org2-q.example", ForLink: true})
	if _, err := p.quickCreateLink(ctx, &QuickCreateLinkInput{Ctx: mkCtx(req), Body: QuickCreateLinkBody{URL: "https://x.example", Host: "org2-q.example"}}); err == nil {
		t.Error("unowned host must be refused")
	}

	p.db.Where("key = ?", models.BaseDomainSetting).Delete(&models.Setting{})
	p.db.Create(&models.Setting{Key: models.BaseDomainSetting, Value: "octarq.test"})
	p.db.Create(&dns.Domain{OrgID: 1, Name: "own2.example", ForLink: true})
	reqH := httptest.NewRequest(http.MethodPost, "/api/links/quick", nil)
	reqH.Header.Set("X-Org-ID", "1")
	if _, err := p.quickCreateLink(ctx, &QuickCreateLinkInput{Ctx: mkCtx(reqH), Body: QuickCreateLinkBody{URL: "https://x.example"}}); err == nil {
		t.Error("hostless quick-create must be refused when a base domain is configured")
	}

	req0 := httptest.NewRequest(http.MethodPost, "/api/links/quick", nil)
	req0.Header.Set("X-Org-ID", "0")
	if _, err := p.quickCreateLink(ctx, &QuickCreateLinkInput{Ctx: mkCtx(req0), Body: QuickCreateLinkBody{URL: "https://x.example"}}); err == nil {
		t.Error("quick-create with org 0 must fail")
	}
}

func TestUpdateLinkEdgeBranches(t *testing.T) {
	t.Parallel()
	p, mkCtx := setupFullLinksTestDB(t)
	wipeLinksTables(t, p)
	ctx := context.Background()

	if err := p.db.Create(&Link{OrgID: 1, Slug: "u1", Target: "https://a.example"}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	linkID := func() uint { var l Link; p.db.Where("slug = ?", "u1").First(&l); return l.ID }()

	req := httptest.NewRequest(http.MethodPut, "/api/links/1", nil)
	req.Header.Set("X-Org-ID", "1")

	// Reserved slug refused.
	if _, err := p.updateLink(ctx, &UpdateLinkInput{Ctx: mkCtx(req), ID: linkID, Body: linkDTO{Slug: "api"}}); err == nil {
		t.Error("updating to a reserved slug must fail")
	}
	// Dangerous target refused, original untouched.
	if _, err := p.updateLink(ctx, &UpdateLinkInput{Ctx: mkCtx(req), ID: linkID, Body: linkDTO{Target: "javascript:alert(1)"}}); err == nil {
		t.Error("updating to a javascript target must fail")
	}
	// Unowned host refused.
	p.db.Create(&dns.Domain{OrgID: 2, Name: "org2-up.example", ForLink: true})
	if _, err := p.updateLink(ctx, &UpdateLinkInput{Ctx: mkCtx(req), ID: linkID, Body: linkDTO{Host: "org2-up.example"}}); err == nil {
		t.Error("updating to an unowned host must fail")
	}
	// Host required when a base domain is configured.
	p.db.Where("key = ?", models.BaseDomainSetting).Delete(&models.Setting{})
	p.db.Create(&models.Setting{Key: models.BaseDomainSetting, Value: "octarq.test"})
	p.db.Create(&dns.Domain{OrgID: 1, Name: "own-up.example", ForLink: true})
	reqH := httptest.NewRequest(http.MethodPut, "/api/links/1", nil)
	reqH.Header.Set("X-Org-ID", "1")
	if _, err := p.updateLink(ctx, &UpdateLinkInput{Ctx: mkCtx(reqH), ID: linkID, Body: linkDTO{Host: ""}}); err == nil {
		t.Error("clearing host must fail when a base domain is configured")
	}

	// Missing id.
	if _, err := p.updateLink(ctx, &UpdateLinkInput{Ctx: mkCtx(req), ID: 999, Body: linkDTO{Note: "x"}}); err == nil {
		t.Error("updating a missing link must fail")
	}

	// Full update: slug rename invalidates both the old and the new cache key.
	p.db.Where("key = ?", models.BaseDomainSetting).Delete(&models.Setting{})
	var invalidations []string
	p.deleteCache = func(_ context.Context, key string) error { invalidations = append(invalidations, key); return nil }

	exp := time.Now().Add(24 * time.Hour)
	enabled := false
	pw := "pw"
	out, err := p.updateLink(ctx, &UpdateLinkInput{Ctx: mkCtx(req), ID: linkID, Body: linkDTO{
		Slug: "u1-renamed", Target: "https://b.example", Note: "n", Title: "t", Tags: "x",
		Password: &pw, ExpiresAt: &exp, ExpiredURL: "old.example", ClickLimit: 9, Archived: &enabled, Enabled: &enabled,
	}})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if out.Body.Slug != "u1-renamed" || out.Body.Target != "https://b.example" || out.Body.ClickLimit != 9 || out.Body.Password != "pw" {
		t.Errorf("update did not persist: %+v", out.Body)
	}
	if out.Body.ExpiredURL != "https://old.example" {
		t.Errorf("expiredUrl not normalized: %q", out.Body.ExpiredURL)
	}
	if len(invalidations) != 2 {
		t.Errorf("expected old+new cache keys invalidated, got %v", invalidations)
	}
}

func TestDeleteLinkNotFound(t *testing.T) {
	t.Parallel()
	p, mkCtx := setupFullLinksTestDB(t)
	wipeLinksTables(t, p)
	ctx := context.Background()

	req := httptest.NewRequest(http.MethodDelete, "/api/links/999", nil)
	req.Header.Set("X-Org-ID", "1")
	req.Header.Set("X-Role", "admin")
	if _, err := p.deleteLink(ctx, &DeleteLinkInput{Ctx: mkCtx(req), ID: 999}); err == nil {
		t.Error("deleting a missing link must fail")
	}
}

func TestExportLinksCSVBody(t *testing.T) {
	t.Parallel()
	p, _ := setupFullLinksTestDB(t)
	wipeLinksTables(t, p)
	ctx := context.Background()

	if err := p.db.Create(&Link{OrgID: 1, Slug: "csv1", Host: "go.example", Target: "https://csv.example/x", Title: "CSV One", Clicks: 3}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/links/export.csv", nil)
	rec := httptest.NewRecorder()
	hctx := humago.NewContext(nil, req, rec)
	if _, err := p.exportLinksCSV(ctx, &ExportLinksCSVInput{Ctx: hctx}); err != nil {
		t.Fatalf("exportLinksCSV: %v", err)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("content-type = %q, want text/csv", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "ID,Host,Slug,Target,Title,Clicks,CreatedAt") {
		t.Errorf("missing CSV header: %q", body)
	}
	if !strings.Contains(body, "csv1") || !strings.Contains(body, "https://csv.example/x") || !strings.Contains(body, "CSV One") {
		t.Errorf("missing link row: %q", body)
	}
}

func TestLinkStatsWithEvents(t *testing.T) {
	t.Parallel()
	p, mkCtx := setupFullLinksTestDB(t)
	wipeLinksTables(t, p)
	ctx := context.Background()

	now := time.Now()
	link := Link{
		OrgID: 1, Slug: "stats1", Target: "https://s.example",
		RoutingRules: RoutingRules{{Type: "split", Weight: 50, Target: "https://v1.example"}},
	}
	if err := p.db.Create(&link).Error; err != nil {
		t.Fatalf("seed link: %v", err)
	}
	for _, ev := range []LinkEvent{
		{LinkID: link.ID, CreatedAt: now, Referer: "https://google.com", Country: "US", Device: "Desktop", Browser: "Chrome", Variant: "https://v1.example", Fingerprint: "f1"},
		{LinkID: link.ID, CreatedAt: now.Add(-time.Hour), Referer: "https://twitter.com", Country: "JP", Device: "Mobile", Browser: "Safari", Fingerprint: "f2", Variant: "https://v1.example"},
		{LinkID: link.ID, CreatedAt: now.Add(-48 * time.Hour), Referer: "https://google.com", Country: "US", Device: "Desktop", Browser: "Chrome", Variant: "https://v1.example", Fingerprint: "f3"},
	} {
		if err := p.db.Create(&ev).Error; err != nil {
			t.Fatalf("seed event: %v", err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/links/1/stats?metric=uv", nil)
	out, err := p.linkStats(ctx, &LinkStatsInput{Ctx: mkCtx(req), ID: link.ID, Days: 30, Metric: "uv"})
	if err != nil {
		t.Fatalf("linkStats: %v", err)
	}
	if out.Body["total"].(int64) != 3 {
		t.Errorf("total = %v, want 3", out.Body["total"])
	}
	if out.Body["metric"] != "uv" {
		t.Errorf("metric = %v, want uv", out.Body["metric"])
	}
	referers := out.Body["referers"].([]models.StatKV)
	if len(referers) != 2 {
		t.Errorf("referers = %+v", referers)
	}
	channels := out.Body["channels"].([]models.StatKV)
	if len(channels) != 2 {
		t.Errorf("channels = %+v", channels)
	}
	countries := out.Body["countries"].([]models.StatKV)
	if len(countries) != 2 {
		t.Errorf("countries = %+v", countries)
	}
	variants := out.Body["variants"].([]models.StatKV)
	if len(variants) != 1 || variants[0].Count != 3 {
		t.Errorf("variants = %+v, want 1 variant with 3 events (UV dedups to 3 distinct fingerprints)", variants)
	}

	// pv metric and a 7-day window exclude the 48h-old event.
	outPV, err := p.linkStats(ctx, &LinkStatsInput{Ctx: mkCtx(req), ID: link.ID, Days: 7, Metric: "pv"})
	if err != nil {
		t.Fatalf("linkStats pv: %v", err)
	}
	if outPV.Body["total"].(int64) != 3 {
		t.Errorf("pv total = %v, want 3 (total is windowless)", outPV.Body["total"])
	}
	if outPV.Body["days"] != 7 {
		t.Errorf("days = %v, want 7", outPV.Body["days"])
	}
	if len(outPV.Body["series"].([]models.StatKV)) == 0 {
		t.Error("expected non-empty daily series")
	}

	// Missing link.
	if _, err := p.linkStats(ctx, &LinkStatsInput{Ctx: mkCtx(req), ID: 999, Days: 30}); err == nil {
		t.Error("stats for a missing link must fail")
	}
}

func TestLinkQRWritesPNG(t *testing.T) {
	t.Parallel()
	p, mkCtx := setupFullLinksTestDB(t)
	wipeLinksTables(t, p)
	ctx := context.Background()

	seeded := &Link{OrgID: 1, Slug: "qr1", Host: "go.example", Target: "https://qr.example"}
	if err := p.db.Create(seeded).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/links/1/qr", nil)
	rec := httptest.NewRecorder()
	hctx := humago.NewContext(nil, req, rec)
	if _, err := p.linkQR(ctx, &LinkQRInput{Ctx: hctx, ID: seeded.ID}); err != nil {
		t.Fatalf("linkQR: %v", err)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("content-type = %q, want image/png", ct)
	}
	png := rec.Body.Bytes()
	if len(png) < 8 || string(png[:8]) != "\x89PNG\r\n\x1a\n" {
		t.Errorf("body is not a PNG: %d bytes, prefix %q", len(png), png[:min(len(png), 8)])
	}
	img, err := pngDecode(png)
	if err != nil {
		t.Fatalf("decode PNG: %v", err)
	}
	bounds := img.Bounds()
	if bounds.Dx() != 320 || bounds.Dy() != 320 {
		t.Errorf("QR dimensions = %dx%d, want 320x320", bounds.Dx(), bounds.Dy())
	}

	req404 := httptest.NewRequest(http.MethodGet, "/api/links/999/qr", nil)
	if _, err := p.linkQR(ctx, &LinkQRInput{Ctx: mkCtx(req404), ID: 999}); err == nil {
		t.Error("QR for missing link must fail")
	}
}

func pngDecode(data []byte) (image.Image, error) {
	return png.Decode(bytes.NewReader(data))
}
