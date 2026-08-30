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

// LinkEvent cleanup on delete is scoped through the owned link, so deleting (or
// attempting to delete) a link can never remove another workspace's analytics
// rows even if the ownership check above were ever skipped or reordered.
func TestDeleteLinkCleansOnlyOwnOrgEvents(t *testing.T) {
	t.Parallel()

	p, mkCtx := setupFullLinksTestDB(t)
	ctx := context.Background()

	req1 := httptest.NewRequest(http.MethodPost, "/api/links", nil)
	out1, err := p.createLink(ctx, &CreateLinkInput{Ctx: mkCtx(req1), Body: linkDTO{Slug: "org1-link", Target: "https://org1.example"}})
	if err != nil {
		t.Fatalf("createLink org1: %v", err)
	}
	if err := p.db.Create(&LinkEvent{LinkID: out1.Body.ID, IP: "1.1.1.1"}).Error; err != nil {
		t.Fatalf("seed org1 event: %v", err)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/api/links", nil)
	req2.Header.Set("X-Org-ID", "2")
	out2, err := p.createLink(ctx, &CreateLinkInput{Ctx: mkCtx(req2), Body: linkDTO{Slug: "org2-link", Target: "https://org2.example"}})
	if err != nil {
		t.Fatalf("createLink org2: %v", err)
	}
	if err := p.db.Create(&LinkEvent{LinkID: out2.Body.ID, IP: "2.2.2.2"}).Error; err != nil {
		t.Fatalf("seed org2 event: %v", err)
	}

	delCross := httptest.NewRequest(http.MethodDelete, "/api/links/1", nil)
	delCross.Header.Set("X-Org-ID", "2")
	delCross.Header.Set("X-Role", "admin")
	if _, err := p.deleteLink(ctx, &DeleteLinkInput{Ctx: mkCtx(delCross), ID: out1.Body.ID}); err == nil {
		t.Fatal("expected error when org2 deletes org1's link")
		return
	}
	var org1Events int64
	p.db.Model(&LinkEvent{}).Where("link_id = ?", out1.Body.ID).Count(&org1Events)
	if org1Events != 1 {
		t.Errorf("org1 events after cross-org delete: got %d, want 1", org1Events)
	}

	delSelf := httptest.NewRequest(http.MethodDelete, "/api/links/2", nil)
	delSelf.Header.Set("X-Org-ID", "2")
	delSelf.Header.Set("X-Role", "admin")
	if _, err := p.deleteLink(ctx, &DeleteLinkInput{Ctx: mkCtx(delSelf), ID: out2.Body.ID}); err != nil {
		t.Fatalf("deleteLink org2 self: %v", err)
	}
	var org2Events int64
	p.db.Model(&LinkEvent{}).Where("link_id = ?", out2.Body.ID).Count(&org2Events)
	if org2Events != 0 {
		t.Errorf("org2 events after self-delete: got %d, want 0", org2Events)
	}
	p.db.Model(&LinkEvent{}).Where("link_id = ?", out1.Body.ID).Count(&org1Events)
	if org1Events != 1 {
		t.Errorf("org1 events after org2 self-delete: got %d, want 1", org1Events)
	}
}

func TestLinkPasswordPreservationAndPlaintext(t *testing.T) {
	t.Parallel()

	p, mkCtx := setupFullLinksTestDB(t)
	ctx := context.Background()

	// 1. Create link with password
	pw := "secret123"
	reqCreate := httptest.NewRequest(http.MethodPost, "/api/links", nil)
	createIn := &CreateLinkInput{
		Ctx: mkCtx(reqCreate),
		Body: linkDTO{
			Slug:     "pw-test",
			Target:   "https://example.com/secret",
			Password: &pw,
		},
	}
	outCreate, err := p.createLink(ctx, createIn)
	if err != nil {
		t.Fatalf("createLink with password: %v", err)
	}
	if outCreate.Body.Password != "secret123" {
		t.Errorf("expected Password %q, got %q", "secret123", outCreate.Body.Password)
	}
	if !outCreate.Body.HasPassword {
		t.Errorf("expected HasPassword true, got false")
	}

	linkID := outCreate.Body.ID

	// 2. Update other fields without passing Password (Password == nil) -> Password MUST NOT be cleared
	reqUpdate1 := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/links/%d", linkID), nil)
	updateIn1 := &UpdateLinkInput{
		Ctx: mkCtx(reqUpdate1),
		ID:  linkID,
		Body: linkDTO{
			Title: "Updated Title",
		},
	}
	outUpdate1, err := p.updateLink(ctx, updateIn1)
	if err != nil {
		t.Fatalf("updateLink with nil password: %v", err)
	}
	if outUpdate1.Body.Password != "secret123" {
		t.Errorf("expected Password to be preserved as %q, got %q", "secret123", outUpdate1.Body.Password)
	}
	if !outUpdate1.Body.HasPassword {
		t.Errorf("expected HasPassword to remain true, got false")
	}

	// 3. Update password explicitly
	newPw := "updated456"
	reqUpdate2 := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/links/%d", linkID), nil)
	updateIn2 := &UpdateLinkInput{
		Ctx: mkCtx(reqUpdate2),
		ID:  linkID,
		Body: linkDTO{
			Password: &newPw,
		},
	}
	outUpdate2, err := p.updateLink(ctx, updateIn2)
	if err != nil {
		t.Fatalf("updateLink with new password: %v", err)
	}
	if outUpdate2.Body.Password != "updated456" {
		t.Errorf("expected Password %q, got %q", "updated456", outUpdate2.Body.Password)
	}
	if !outUpdate2.Body.HasPassword {
		t.Errorf("expected HasPassword true, got false")
	}

	// 4. Explicitly clear password with empty string
	emptyPw := ""
	reqUpdate3 := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/links/%d", linkID), nil)
	updateIn3 := &UpdateLinkInput{
		Ctx: mkCtx(reqUpdate3),
		ID:  linkID,
		Body: linkDTO{
			Password: &emptyPw,
		},
	}
	outUpdate3, err := p.updateLink(ctx, updateIn3)
	if err != nil {
		t.Fatalf("updateLink with empty password: %v", err)
	}
	if outUpdate3.Body.Password != "" {
		t.Errorf("expected Password to be cleared to \"\", got %q", outUpdate3.Body.Password)
	}
	if outUpdate3.Body.HasPassword {
		t.Errorf("expected HasPassword false after clear, got true")
	}
}
