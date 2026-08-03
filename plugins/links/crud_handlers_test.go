package links

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugins/dns"
	"gorm.io/gorm"
)

func setupFullLinksTestDB(t *testing.T) (*Plugin, func(req *http.Request) huma.Context) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(append(models.AllModels(), &Link{}, &LinkEvent{}, &dns.Domain{}, &dns.ProviderAccount{})...); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	p := New()
	p.db = db
	p.auth.OrgID = func(r *http.Request) uint {
		if val := r.Header.Get("X-Org-ID"); val != "" {
			var id uint
			fmt.Sscanf(val, "%d", &id)
			return id
		}
		return 1
	}
	p.requireRole = func(r *http.Request, role string) bool {
		return r.Header.Get("X-Role") != "member"
	}

	mkCtx := func(r *http.Request) huma.Context {
		if r.Header.Get("X-Org-ID") == "" {
			r.Header.Set("X-Org-ID", "1")
		}
		return humago.NewContext(nil, r, httptest.NewRecorder())
	}
	return p, mkCtx
}

func TestLinksCRUDHandlers(t *testing.T) {
	t.Parallel()

	p, mkCtx := setupFullLinksTestDB(t)
	ctx := context.Background()

	// 1. Create Link
	reqCreate := httptest.NewRequest(http.MethodPost, "/api/links", nil)
	createIn := &CreateLinkInput{
		Ctx: mkCtx(reqCreate),
		Body: linkDTO{
			Slug:   "my-promo",
			Target: "https://example.com/landing",
			Title:  "Promo Landing",
			Tags:   "marketing,promo",
		},
	}
	outCreate, err := p.createLink(ctx, createIn)
	if err != nil {
		t.Fatalf("createLink error: %v", err)
	}
	linkID := outCreate.Body.ID

	// 2. List Links
	reqList := httptest.NewRequest(http.MethodGet, "/api/links?q=Promo", nil)
	listOut, err := p.listLinks(ctx, &ListLinksInput{
		Ctx: mkCtx(reqList),
		Q:   "Promo",
		Tag: "marketing",
	})
	if err != nil || len(listOut.Body) != 1 {
		t.Fatalf("listLinks error=%v, count=%d", err, len(listOut.Body))
	}

	// 3. Update Link
	reqUp := httptest.NewRequest(http.MethodPut, "/api/links/1", nil)
	upOut, err := p.updateLink(ctx, &UpdateLinkInput{
		Ctx: mkCtx(reqUp),
		ID:  linkID,
		Body: linkDTO{
			Slug:   "my-promo-v2",
			Target: "https://example.com/landing-v2",
		},
	})
	if err != nil || upOut.Body.Slug != "my-promo-v2" {
		t.Fatalf("updateLink error=%v, out=%+v", err, upOut)
	}

	// 4. Export Links CSV
	reqExport := httptest.NewRequest(http.MethodGet, "/api/links/export.csv", nil)
	_, err = p.exportLinksCSV(ctx, &ExportLinksCSVInput{Ctx: mkCtx(reqExport)})
	if err != nil {
		t.Fatalf("exportLinksCSV error: %v", err)
	}

	// 5. Delete Link - member forbidden
	reqDelMember := httptest.NewRequest(http.MethodDelete, "/api/links/1", nil)
	reqDelMember.Header.Set("X-Role", "member")
	_, err = p.deleteLink(ctx, &DeleteLinkInput{Ctx: mkCtx(reqDelMember), ID: linkID})
	if err == nil {
		t.Error("expected 403 when deleting link as member")
	}

	// Delete Link - admin success
	reqDelAdmin := httptest.NewRequest(http.MethodDelete, "/api/links/1", nil)
	reqDelAdmin.Header.Set("X-Role", "admin")
	delOut, err := p.deleteLink(ctx, &DeleteLinkInput{Ctx: mkCtx(reqDelAdmin), ID: linkID})
	if err != nil || !delOut.Body["ok"] {
		t.Fatalf("deleteLink error=%v", err)
	}
}

func TestLinksOverview(t *testing.T) {
	t.Parallel()

	p, _ := setupFullLinksTestDB(t)
	p.db.Create(&Link{OrgID: 1, Slug: "l1", Target: "https://a.com", Clicks: 10})
	p.db.Create(&LinkEvent{LinkID: 1, Referer: "google.com", Country: "US", CreatedAt: time.Now()})

	stats := p.overview(1, false)
	if stats["totalLinks"] == 0 && stats["links"] == 0 {
		t.Errorf("overview expected non-zero links count, got %+v", stats)
	}
}
