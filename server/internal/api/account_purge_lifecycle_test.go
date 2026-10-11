package api

import (
	"net/http"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugins/links"
)

// TestPurgeAccount_InvokesPluginPurgeServices guards the plugin.PurgeFunc
// contract: the purge must reach every mounted plugin's purge service through
// LookupServiceAs[plugin.PurgeFunc]. The core purge transaction never touches
// plugin tables, so a link row surviving the purge means the plugin service
// never ran. Short-circuiting the guard leaves the row behind and reds this
// test.
func TestPurgeAccount_InvokesPluginPurgeServices(t *testing.T) {
	h, srv, db := newTestHandlerRaw(t)

	const orgA uint = 501
	const userA uint = 5001

	seedOrgFullData(t, h, orgA, userA, "SECRET-ORG-A")
	if err := db.Create(&links.Link{OrgID: orgA, Slug: "guard-purge", Target: "https://example.com/guard"}).Error; err != nil {
		t.Fatalf("seed link: %v", err)
	}
	cookies := sessionCookies(t, userA, orgA)

	rec := do(srv, http.MethodDelete, "/api/account/data", cookies, `{"confirm":"DELETE MY DATA"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("purge request failed: got status %d, body %s", rec.Code, rec.Body.String())
	}

	var linkCount int64
	db.Model(&links.Link{}).Where("owner_id = ?", orgA).Count(&linkCount)
	if linkCount != 0 {
		t.Errorf("links rows for Org A = %d, want 0 — the links purge service never ran (plugin.PurgeFunc guard broken?)", linkCount)
	}
}

// TestPurgeAccount_RetiresCurrentAndHistoricalSlugs guards address symmetry:
// purging a workspace must retire both its active slug and any historical slugs
// from prior renames, making every address the org ever held permanently SlugTaken
// for other orgs or new registrations.
func TestPurgeAccount_RetiresCurrentAndHistoricalSlugs(t *testing.T) {
	h, srv, db := newTestHandlerRaw(t)

	const orgA uint = 601
	const userA uint = 6001
	const orgB uint = 602

	const activeSlug = "doomed-active-slug"
	const historicalSlug = "doomed-historical-slug"

	seedOrgFullData(t, h, orgA, userA, "SECRET-ORG-A")

	// Set custom active slug for orgA
	if err := db.Model(&models.Org{}).Where("id = ?", orgA).Update("slug", activeSlug).Error; err != nil {
		t.Fatalf("update active slug: %v", err)
	}

	// Seed historical slug from a prior rename for orgA
	if err := db.Create(&models.OrgSlugHistory{
		Slug:      historicalSlug,
		OrgID:     orgA,
		RetiredAt: time.Now(),
	}).Error; err != nil {
		t.Fatalf("seed historical slug: %v", err)
	}

	// Sanity check before purge: orgA can reclaim its own historical slug,
	// while other orgs cannot take either slug.
	if status, err := models.CheckOrgSlugAvailable(db, historicalSlug, orgA); err != nil || status != models.SlugAvailable {
		t.Fatalf("before purge: CheckOrgSlugAvailable(%q, %d) = (%v, %v), want (SlugAvailable, nil)", historicalSlug, orgA, status, err)
	}
	if status, err := models.CheckOrgSlugAvailable(db, activeSlug, orgB); err != nil || status != models.SlugTaken {
		t.Fatalf("before purge: CheckOrgSlugAvailable(%q, %d) = (%v, %v), want (SlugTaken, nil)", activeSlug, orgB, status, err)
	}

	// Purge Org A
	cookies := sessionCookies(t, userA, orgA)
	rec := do(srv, http.MethodDelete, "/api/account/data", cookies, `{"confirm":"DELETE MY DATA"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("purge request failed: got status %d, body %s", rec.Code, rec.Body.String())
	}

	// 1. Another org (orgB) checking the purged org's active slug -> SlugTaken
	status, err := models.CheckOrgSlugAvailable(db, activeSlug, orgB)
	if err != nil {
		t.Fatalf("CheckOrgSlugAvailable activeSlug orgB err: %v", err)
	}
	if status != models.SlugTaken {
		t.Errorf("CheckOrgSlugAvailable(%q, orgB=%d) = %v, want SlugTaken", activeSlug, orgB, status)
	}

	// 2. New workspace registration (targetOrgID = 0) checking the purged org's active slug -> SlugTaken
	status, err = models.CheckOrgSlugAvailable(db, activeSlug, 0)
	if err != nil {
		t.Fatalf("CheckOrgSlugAvailable activeSlug new org err: %v", err)
	}
	if status != models.SlugTaken {
		t.Errorf("CheckOrgSlugAvailable(%q, targetOrgID=0) = %v, want SlugTaken", activeSlug, status)
	}

	// 3. Historical slug from prior rename is still SlugTaken for another org
	status, err = models.CheckOrgSlugAvailable(db, historicalSlug, orgB)
	if err != nil {
		t.Fatalf("CheckOrgSlugAvailable historicalSlug orgB err: %v", err)
	}
	if status != models.SlugTaken {
		t.Errorf("CheckOrgSlugAvailable(%q, orgB=%d) = %v, want SlugTaken", historicalSlug, orgB, status)
	}

	// 4. Historical slug from prior rename is still SlugTaken for new workspace registration
	status, err = models.CheckOrgSlugAvailable(db, historicalSlug, 0)
	if err != nil {
		t.Fatalf("CheckOrgSlugAvailable historicalSlug new org err: %v", err)
	}
	if status != models.SlugTaken {
		t.Errorf("CheckOrgSlugAvailable(%q, targetOrgID=0) = %v, want SlugTaken", historicalSlug, status)
	}

	// 5. Unrelated brand new slug remains available
	const freeSlug = "unrelated-free-slug"
	status, err = models.CheckOrgSlugAvailable(db, freeSlug, orgB)
	if err != nil {
		t.Fatalf("CheckOrgSlugAvailable freeSlug err: %v", err)
	}
	if status != models.SlugAvailable {
		t.Errorf("CheckOrgSlugAvailable(%q, orgB=%d) = %v, want SlugAvailable", freeSlug, orgB, status)
	}
}
