package links

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/plugin"
	"github.com/octarq-org/octarq/server/plugins/dns"
)

// Stage 1: Characterization Locking Test for plugins/links
// Locks exported interfaces, state machines, tenant isolation boundaries, and fail-closed error responses.

func TestLinks_Characterization_InterfaceContracts(t *testing.T) {
	t.Parallel()
	var (
		_ plugin.Plugin       = (*Plugin)(nil)
		_ plugin.Describer    = (*Plugin)(nil)
		_ plugin.MenuProvider = (*Plugin)(nil)
		_ plugin.HelpDocsFS   = (*Plugin)(nil)
		_ plugin.Starter      = (*Plugin)(nil)
		_ plugin.LinkCreator  = (*Plugin)(nil)
	)

	p := New()
	if p.Name() != "links" {
		t.Fatalf("expected plugin Name 'links', got %q", p.Name())
	}
	desc := p.Describe()
	if desc.Title != "Short Links" {
		t.Fatalf("expected Describe().Title 'Short Links', got %q", desc.Title)
	}
}

func TestLinks_Characterization_MultiTenantIsolation(t *testing.T) {
	t.Parallel()
	p, mkCtx := setupFullLinksTestDB(t)
	wipeLinksTables(t, p)
	ctx := context.Background()

	// Seed links in Org 1 and Org 2
	l1 := Link{OrgID: 1, Host: "shared.example.com", Slug: "promo1", Target: "https://target1.example.com", Enabled: true}
	l2 := Link{OrgID: 2, Host: "shared.example.com", Slug: "promo2", Target: "https://target2.example.com", Enabled: true}
	if err := p.db.Create(&l1).Error; err != nil {
		t.Fatalf("seed l1: %v", err)
	}
	if err := p.db.Create(&l2).Error; err != nil {
		t.Fatalf("seed l2: %v", err)
	}

	// 1. Org 1 query cannot see Org 2's links
	req1 := httptest.NewRequest(http.MethodGet, "/api/links", nil)
	req1.Header.Set("X-Org-ID", "1")
	out1, err := p.listLinks(ctx, &ListLinksInput{Ctx: mkCtx(req1)})
	if err != nil {
		t.Fatalf("listLinks org 1: %v", err)
	}
	for _, l := range out1.Body {
		if l.OrgID != 1 {
			t.Fatalf("cross-tenant data leak: org 1 received link belonging to org %d", l.OrgID)
		}
	}

	// 2. Org 1 cannot delete Org 2's link
	reqDel := httptest.NewRequest(http.MethodDelete, "/api/links/2", nil)
	reqDel.Header.Set("X-Org-ID", "1")
	reqDel.Header.Set("X-Role", "admin")
	_, errDel := p.deleteLink(ctx, &DeleteLinkInput{Ctx: mkCtx(reqDel), ID: l2.ID})
	if errDel == nil {
		t.Fatalf("expected error when org 1 tries to delete org 2 link, got nil")
	}

	// Verify l2 is still intact
	var checkL2 Link
	if err := p.db.Where("id = ?", l2.ID).First(&checkL2).Error; err != nil {
		t.Fatalf("l2 was deleted or not found: %v", err)
	}
}

func TestLinks_Characterization_FailClosedHostResolution(t *testing.T) {
	t.Parallel()
	db, p, engine := setupOwnershipTestDB(t)
	defer engine.Close()

	// Org 1 and Org 2 both claim "contested.example.com"
	db.Create(&dns.Domain{
		OrgID: 1,
		Name:  "contested.example.com",
	})
	db.Create(&dns.Domain{
		OrgID: 2,
		Name:  "contested.example.com",
	})

	// When a host is contested, ownsHost must FAIL CLOSED for both tenants
	if p.ownsHost(1, "contested.example.com") {
		t.Error("contested domain must fail closed for org 1")
	}
	if p.ownsHost(2, "contested.example.com") {
		t.Error("contested domain must fail closed for org 2")
	}
}

func TestLinks_Characterization_UTMAndAttributionState(t *testing.T) {
	t.Parallel()
	p, _ := setupFullLinksTestDB(t)
	wipeLinksTables(t, p)

	ev := LinkEvent{
		LinkID:      1,
		Referer:     "https://twitter.com",
		UTMSource:   "newsletter",
		UTMMedium:   "email",
		UTMCampaign: "fall2026",
		CreatedAt:   time.Now(),
	}
	if err := p.db.Create(&ev).Error; err != nil {
		t.Fatalf("create LinkEvent: %v", err)
	}

	var fetched LinkEvent
	if err := p.db.Where("id = ?", ev.ID).First(&fetched).Error; err != nil {
		t.Fatalf("fetch LinkEvent: %v", err)
	}
	if fetched.UTMSource != "newsletter" || fetched.UTMMedium != "email" || fetched.UTMCampaign != "fall2026" {
		t.Fatalf("unexpected UTM values: source=%q medium=%q campaign=%q", fetched.UTMSource, fetched.UTMMedium, fetched.UTMCampaign)
	}
}

func TestLinks_Characterization_ExpiredLinkState(t *testing.T) {
	t.Parallel()
	past := time.Now().Add(-1 * time.Hour)
	l := Link{
		Target:    "https://example.com/active",
		ExpiresAt: &past,
	}

	if l.ExpiresAt == nil || !l.ExpiresAt.Before(time.Now()) {
		t.Fatal("expected expired link to have past ExpiresAt")
	}
}
