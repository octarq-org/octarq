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
// This is deliberately NOT the short-link reserved list (api.isReservedSlug):
// short links and orgs are different namespaces, and register.go used to
// consult the short-link one, which both over- and under-restricted.
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

// LegacyEmailSlug reproduces the pre-randomization derivation
// ("foo@bar.com" → "foo-bar-com") for one purpose only: recognizing an existing
// org whose slug still spells out its founder's address, so the UI can offer to
// rename it before that slug goes public in an SSO login URL. Never call it to
// mint a slug — AllocateOrgSlug is the only way to do that.
func LegacyEmailSlug(email string) string {
	r := strings.NewReplacer("@", "-", ".", "-", "_", "-", "+", "-")
	return r.Replace(strings.ToLower(strings.TrimSpace(email)))
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
		if reservedOrgSlugs[slug] {
			continue
		}
		var n int64
		if err := db.Model(&Org{}).Where("slug = ?", slug).Count(&n).Error; err != nil {
			return "", err
		}
		if n == 0 {
			return slug, nil
		}
	}
	return "", errors.New("models: could not allocate a free org slug")
}
