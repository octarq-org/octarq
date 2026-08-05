package models

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func slugTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(&Org{}, &OrgSlugHistory{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// A slug must never be derivable from anything the caller supplies. The old
// derivation put the founder's email into /api/webhook/{orgSlug}/..., a path
// that reaches Stripe and every proxy in between.
func TestAllocateOrgSlugIsNotDerivedFromInput(t *testing.T) {
	db := slugTestDB(t)
	seen := map[string]bool{}
	for range 50 {
		slug, err := AllocateOrgSlug(db)
		if err != nil {
			t.Fatalf("allocate: %v", err)
		}
		if len(slug) != orgSlugLen {
			t.Fatalf("slug %q: want %d chars", slug, orgSlugLen)
		}
		if strings.ContainsAny(slug, "@.+_") {
			t.Fatalf("slug %q is not URL-safe", slug)
		}
		seen[slug] = true
		// Occupy it, so the next call has to find a different one.
		if err := db.Create(&Org{Name: "x", Slug: slug}).Error; err != nil {
			t.Fatalf("create org: %v", err)
		}
	}
	if len(seen) != 50 {
		t.Fatalf("got %d distinct slugs out of 50 — allocation is not random", len(seen))
	}
}

// The squatting lockout (§6.3): a taken slug must never be handed out again, or
// the second org's INSERT dies on the unique index and its owner can never sign
// in. Shrinking the slug to one character makes the whole space enumerable, so
// this asserts the table lookup rather than relying on 40 bits not colliding.
func TestAllocateOrgSlugSkipsTaken(t *testing.T) {
	db := slugTestDB(t)
	defer func(n, a int) { orgSlugLen, orgSlugAttempts = n, a }(orgSlugLen, orgSlugAttempts)
	orgSlugLen = 1
	orgSlugAttempts = 500 // one free slug in 32; 20 tries would be a coin flip

	// Occupy every single-character slug but one.
	free := string(slugAlphabet[7])
	for _, c := range slugAlphabet {
		if string(c) == free {
			continue
		}
		if err := db.Create(&Org{Name: "squatter", Slug: string(c)}).Error; err != nil {
			t.Fatalf("create org: %v", err)
		}
	}

	got, err := AllocateOrgSlug(db)
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	if got != free {
		t.Fatalf("allocated %q, the only free slug is %q", got, free)
	}

	// With the space fully exhausted it must fail loudly, not return a duplicate.
	if err := db.Create(&Org{Name: "last", Slug: free}).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	if got, err := AllocateOrgSlug(db); err == nil {
		t.Fatalf("allocated %q from an exhausted space, want an error", got)
	}
}

// Reserved words are org-scoped, not the short-link list: an org must not be
// able to take a slug that a future /{slug} route needs.
func TestReservedOrgSlugs(t *testing.T) {
	for _, s := range []string{"admin", "API", " sso ", ".well-known", "storefront"} {
		if !IsReservedOrgSlug(s) {
			t.Errorf("IsReservedOrgSlug(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"acme", "shortlink", ""} {
		if IsReservedOrgSlug(s) {
			t.Errorf("IsReservedOrgSlug(%q) = true, want false", s)
		}
	}
}

func TestAllocateOrgSlugSkipsHistory(t *testing.T) {
	db := slugTestDB(t)
	defer func(n, a int) { orgSlugLen, orgSlugAttempts = n, a }(orgSlugLen, orgSlugAttempts)
	orgSlugLen = 1
	orgSlugAttempts = 500

	// Occupy every single-character slug except one in active Orgs, and put that last one into OrgSlugHistory.
	freeInOrg := string(slugAlphabet[7])
	for _, c := range slugAlphabet {
		if string(c) == freeInOrg {
			continue
		}
		if err := db.Create(&Org{Name: "active", Slug: string(c)}).Error; err != nil {
			t.Fatalf("create org: %v", err)
		}
	}
	if err := db.Create(&OrgSlugHistory{Slug: freeInOrg, OrgID: 99}).Error; err != nil {
		t.Fatalf("create history: %v", err)
	}

	if got, err := AllocateOrgSlug(db); err == nil {
		t.Fatalf("allocated %q from a space occupied by active orgs and retired history, want an error", got)
	}
}
