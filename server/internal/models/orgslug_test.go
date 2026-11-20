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
		if err := db.Create(&Org{Name: "x", Slug: slug}).Error; err != nil {
			t.Fatalf("create org: %v", err)
		}
	}
	if len(seen) != 50 {
		t.Fatalf("got %d distinct slugs out of 50 — allocation is not random", len(seen))
	}
}

func TestAllocateOrgSlugSkipsTaken(t *testing.T) {
	db := slugTestDB(t)
	defer func(n, a int) { orgSlugLen, orgSlugAttempts = n, a }(orgSlugLen, orgSlugAttempts)
	orgSlugLen = 1
	orgSlugAttempts = 500

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

	if err := db.Create(&Org{Name: "last", Slug: free}).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	if got, err := AllocateOrgSlug(db); err == nil {
		t.Fatalf("allocated %q from an exhausted space, want an error", got)
		return
	}
}

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
		return
	}
}

func TestAllocateOrgSlugNilDB(t *testing.T) {
	_, err := AllocateOrgSlug(nil)
	if err == nil {
		t.Fatal("expected error when db is nil")
		return
	}
}

func TestCheckOrgSlugAvailable(t *testing.T) {
	db := slugTestDB(t)

	status, err := CheckOrgSlugAvailable(db, "admin", 0)
	if err != nil || status != SlugReserved {
		t.Fatalf("expected SlugReserved for 'admin', got status=%v err=%v", status, err)
	}

	org := Org{Name: "Alpha", Slug: "alpha"}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}

	status, err = CheckOrgSlugAvailable(db, "alpha", 0)
	if err != nil || status != SlugTaken {
		t.Fatalf("expected SlugTaken for existing org slug, got status=%v err=%v", status, err)
	}

	status, err = CheckOrgSlugAvailable(db, "alpha", org.ID)
	if err != nil || status != SlugAvailable {
		t.Fatalf("expected SlugAvailable when targetOrgID matches existing org, got status=%v err=%v", status, err)
	}

	if err := db.Create(&OrgSlugHistory{Slug: "retired-beta", OrgID: 10}).Error; err != nil {
		t.Fatalf("create history: %v", err)
	}

	status, err = CheckOrgSlugAvailable(db, "retired-beta", 10)
	if err != nil || status != SlugAvailable {
		t.Fatalf("expected SlugAvailable when reclaiming historical slug, got status=%v err=%v", status, err)
	}

	status, err = CheckOrgSlugAvailable(db, "retired-beta", 20)
	if err != nil || status != SlugTaken {
		t.Fatalf("expected SlugTaken when claiming another org's historical slug, got status=%v err=%v", status, err)
	}

	status, err = CheckOrgSlugAvailable(db, "brand-new-slug", 0)
	if err != nil || status != SlugAvailable {
		t.Fatalf("expected SlugAvailable for new slug, got status=%v err=%v", status, err)
	}
}
