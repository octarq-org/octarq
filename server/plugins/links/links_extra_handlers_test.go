package links

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLinkExtraHandlers(t *testing.T) {
	t.Parallel()

	p, mkCtx := setupFullLinksTestDB(t)
	ctx := context.Background()

	lk := Link{OrgID: 1, Slug: "qr-test", Target: "https://example.com/target"}
	p.db.Create(&lk)

	reqGet := httptest.NewRequest(http.MethodGet, "/api/links/1", nil)
	outGet, err := p.getLink(ctx, &GetLinkInput{Ctx: mkCtx(reqGet), ID: lk.ID})
	if err != nil || outGet.Body.Slug != "qr-test" {
		t.Fatalf("getLink failed: %v, %+v", err, outGet)
	}

	reqMeta := httptest.NewRequest(http.MethodGet, "/api/links/metadata?url=https://example.com", nil)
	outMeta, err := p.linkMetadata(ctx, &LinkMetadataInput{Ctx: mkCtx(reqMeta), URL: "https://example.com"})
	if err != nil || outMeta.Body["favicon"] == "" {
		t.Fatalf("linkMetadata failed: %v, %+v", err, outMeta)
	}

	reqStats := httptest.NewRequest(http.MethodGet, "/api/links/1/stats", nil)
	outStats, err := p.linkStats(ctx, &LinkStatsInput{Ctx: mkCtx(reqStats), ID: lk.ID, Days: 7})
	if err != nil || outStats.Body["days"] != 7 {
		t.Fatalf("linkStats failed: %v, %+v", err, outStats)
	}

	reqQR := httptest.NewRequest(http.MethodGet, "/api/links/1/qr", nil)
	_, err = p.linkQR(ctx, &LinkQRInput{Ctx: mkCtx(reqQR), ID: lk.ID})
	if err != nil {
		t.Fatalf("linkQR failed: %v", err)
	}
}
