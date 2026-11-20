package api

import (
	"net/http"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
)

type stubPurgePlugin struct {
	name string
	fn   plugin.PurgeFunc
}

func (p stubPurgePlugin) Name() string          { return p.name }
func (p stubPurgePlugin) Describe() plugin.Info { return plugin.Info{Title: p.name} }
func (p stubPurgePlugin) Models() []any         { return nil }
func (p stubPurgePlugin) Mount(m plugin.Mux, ctx *plugin.Context) {
	ctx.Provide(plugin.PurgeServiceName(p.name), p.fn)
}

func TestPurgeAccount_InvokesPluginPurgeServices(t *testing.T) {
	h, srv, db := newTestHandlerRaw(t)

	const orgA uint = 501
	const userA uint = 5001

	seedOrgFullData(t, h, orgA, userA, "SECRET-ORG-A")
	if err := db.Create(&testLink{OrgID: orgA, Slug: "guard-purge", Target: "https://example.com/guard"}).Error; err != nil {
		t.Fatalf("seed link: %v", err)
	}

	p := stubPurgePlugin{
		name: "links",
		fn: func(orgID uint) error {
			return db.Where("owner_id = ?", orgID).Delete(&testLink{}).Error
		},
	}
	h.SetPlugins([]plugin.Plugin{p})
	reg := plugin.NewRegistry()
	h.SetServiceLookup(reg.Lookup)
	pctx := &plugin.Context{Provide: reg.Provide}
	p.Mount(nil, pctx)

	cookies := sessionCookies(t, userA, orgA)

	rec := do(srv, http.MethodDelete, "/api/account/data", cookies, `{"confirm":"DELETE MY DATA"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("purge request failed: got status %d, body %s", rec.Code, rec.Body.String())
	}

	var linkCount int64
	db.Model(&testLink{}).Where("owner_id = ?", orgA).Count(&linkCount)
	if linkCount != 0 {
		t.Errorf("links rows for Org A = %d, want 0 — the links purge service never ran (plugin.PurgeFunc guard broken?)", linkCount)
	}
}

func TestPurgeAccount_RetiresCurrentAndHistoricalSlugs(t *testing.T) {
	h, srv, db := newTestHandlerRaw(t)

	const orgA uint = 601
	const userA uint = 6001
	const orgB uint = 602

	const activeSlug = "doomed-active-slug"
	const historicalSlug = "doomed-historical-slug"

	seedOrgFullData(t, h, orgA, userA, "SECRET-ORG-A")

	if err := db.Model(&models.Org{}).Where("id = ?", orgA).Update("slug", activeSlug).Error; err != nil {
		t.Fatalf("update active slug: %v", err)
	}

	if err := db.Create(&models.OrgSlugHistory{
		Slug:      historicalSlug,
		OrgID:     orgA,
		RetiredAt: time.Now(),
	}).Error; err != nil {
		t.Fatalf("seed historical slug: %v", err)
	}

	if status, err := models.CheckOrgSlugAvailable(db, historicalSlug, orgA); err != nil || status != models.SlugAvailable {
		t.Fatalf("before purge: CheckOrgSlugAvailable(%q, %d) = (%v, %v), want (SlugAvailable, nil)", historicalSlug, orgA, status, err)
	}
	if status, err := models.CheckOrgSlugAvailable(db, activeSlug, orgB); err != nil || status != models.SlugTaken {
		t.Fatalf("before purge: CheckOrgSlugAvailable(%q, %d) = (%v, %v), want (SlugTaken, nil)", activeSlug, orgB, status, err)
	}

	cookies := sessionCookies(t, userA, orgA)
	rec := do(srv, http.MethodDelete, "/api/account/data", cookies, `{"confirm":"DELETE MY DATA"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("purge request failed: got status %d, body %s", rec.Code, rec.Body.String())
	}

	status, err := models.CheckOrgSlugAvailable(db, activeSlug, orgB)
	if err != nil {
		t.Fatalf("CheckOrgSlugAvailable activeSlug orgB err: %v", err)
	}
	if status != models.SlugTaken {
		t.Errorf("CheckOrgSlugAvailable(%q, orgB=%d) = %v, want SlugTaken", activeSlug, orgB, status)
	}

	status, err = models.CheckOrgSlugAvailable(db, activeSlug, 0)
	if err != nil {
		t.Fatalf("CheckOrgSlugAvailable activeSlug new org err: %v", err)
	}
	if status != models.SlugTaken {
		t.Errorf("CheckOrgSlugAvailable(%q, targetOrgID=0) = %v, want SlugTaken", activeSlug, status)
	}

	status, err = models.CheckOrgSlugAvailable(db, historicalSlug, orgB)
	if err != nil {
		t.Fatalf("CheckOrgSlugAvailable historicalSlug orgB err: %v", err)
	}
	if status != models.SlugTaken {
		t.Errorf("CheckOrgSlugAvailable(%q, orgB=%d) = %v, want SlugTaken", historicalSlug, orgB, status)
	}

	status, err = models.CheckOrgSlugAvailable(db, historicalSlug, 0)
	if err != nil {
		t.Fatalf("CheckOrgSlugAvailable historicalSlug new org err: %v", err)
	}
	if status != models.SlugTaken {
		t.Errorf("CheckOrgSlugAvailable(%q, targetOrgID=0) = %v, want SlugTaken", historicalSlug, status)
	}

	const freeSlug = "unrelated-free-slug"
	status, err = models.CheckOrgSlugAvailable(db, freeSlug, orgB)
	if err != nil {
		t.Fatalf("CheckOrgSlugAvailable freeSlug err: %v", err)
	}
	if status != models.SlugAvailable {
		t.Errorf("CheckOrgSlugAvailable(%q, orgB=%d) = %v, want SlugAvailable", freeSlug, orgB, status)
	}
}
