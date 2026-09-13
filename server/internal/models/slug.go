package models

import (
	"crypto/rand"
	"errors"
	"strings"

	"gorm.io/gorm"
)

const slugAlphabet = "abcdefghijkmnpqrstuvwxyz23456789"

func RandomSlug(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	for i := range b {
		b[i] = slugAlphabet[int(b[i])%len(slugAlphabet)]
	}
	return string(b)
}

// orgSlugLen is the length of a generated org slug. 8 characters of the
// 32-symbol alphabet is ~40 bits — far past the point where guessing another
// tenant's slug is a viable way to find them.
//
// A var rather than a const so the collision path is testable: at length 8 the
// space is too large for a test to ever exhaust, and an untested retry loop is
// how "slug already taken" turns back into a permanent lockout.
var orgSlugLen = 8

// orgSlugAttempts bounds the retry loop so a pathological table can't spin
// forever. At the real slug length a single collision is already implausible.
var orgSlugAttempts = 20

// reservedOrgSlugs are the words an org slug may not take.
//
// This is deliberately NOT the short-link reserved list (the links plugin's
// isReservedSlug): short links and orgs are different namespaces, and
// register.go used to consult the short-link one, which both over- and
// under-restricted.
//
// Today every org slug appears as a path *parameter*
// (/api/webhook/{orgSlug}/billing/{provider}), so none of these could actually
// collide. The table is here for the /{slug} and /api/{slug} shapes on the
// roadmap: by the time those land, the slugs in the wild have already been
// handed to Stripe and to IdP redirect URIs, and taking one back is a
// breaking change. Reserving them now costs nothing.
var reservedOrgSlugs = map[string]bool{
	"admin": true, "api": true, "auth": true, "login": true, "logout": true,
	"sso": true, "static": true, "assets": true, "public": true, "_": true,
	".well-known": true,
	"webhook":     true, "billing": true, "cloud": true, "customer": true,
	"delivery": true, "license": true, "portal": true, "storefront": true,
	"update": true, "updates": true,
	"new": true, "settings": true, "help": true, "docs": true,
}

// IsReservedOrgSlug reports whether slug is one of the words orgs may not take.
func IsReservedOrgSlug(slug string) bool {
	return reservedOrgSlugs[strings.ToLower(strings.TrimSpace(slug))]
}

// SlugStatus represents the availability status of an org address.
type SlugStatus int

const (
	SlugAvailable SlugStatus = iota
	SlugReserved
	SlugTaken
)

// CheckOrgSlugAvailable unifies org address availability checking across 3 sources:
// 1. Static reserved words list (reservedOrgSlugs)
// 2. Active workspaces (orgs table)
// 3. Retired workspace history (org_slug_histories table)
//
// targetOrgID is the ID of the org requesting the slug (0 for new workspace allocation).
func CheckOrgSlugAvailable(db *gorm.DB, slug string, targetOrgID uint) (SlugStatus, error) {
	if IsReservedOrgSlug(slug) {
		return SlugReserved, nil
	}

	var n int64
	query := db.Model(&Org{}).Where("slug = ?", slug)
	if targetOrgID > 0 {
		query = query.Where("id <> ?", targetOrgID)
	}
	if err := query.Count(&n).Error; err != nil {
		return SlugTaken, err
	}
	if n > 0 {
		return SlugTaken, nil
	}

	var history OrgSlugHistory
	err := db.Where("slug = ?", slug).First(&history).Error
	if err == nil {
		if targetOrgID > 0 && history.OrgID == targetOrgID {
			return SlugAvailable, nil
		}
		return SlugTaken, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return SlugTaken, err
	}

	return SlugAvailable, nil
}

// AllocateOrgSlug returns a fresh, unused org slug. It is the single entry
// point every org-creation path uses, so "how is a slug chosen" has one answer.
//
// Slugs are purely random, never derived from the founder's email or the org
// name. Deriving leaked the founder's address into public URLs (the billing
// webhook path carries the slug), and the one derivation path that skipped the
// uniqueness check let an attacker squat the slug a given email would land on
// and lock that person out of signing in for good.
//
// The caller is expected to create the Org inside the same transaction it
// passes here; the unique index on orgs.slug remains the real arbiter, so a
// racing writer loses at INSERT rather than being silently accepted.
func AllocateOrgSlug(db *gorm.DB) (string, error) {
	if db == nil {
		return "", errors.New("models: no database configured")
	}
	for range orgSlugAttempts {
		slug := RandomSlug(orgSlugLen)
		status, err := CheckOrgSlugAvailable(db, slug, 0)
		if err != nil {
			return "", err
		}
		if status == SlugAvailable {
			return slug, nil
		}
	}
	return "", errors.New("models: could not allocate a free org slug")
}
