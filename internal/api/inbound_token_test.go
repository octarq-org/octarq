package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/internal/models"
	"gorm.io/gorm"
)

// P2-10: org.InboundToken is the per-tenant secret in the inbound-email webhook
// URL. Anyone holding it can forge inbound mail for the workspace, so it must
// not appear in GET /api/settings for a plain member.
//
// The gate also has to understand API tokens: this endpoint is where an
// automation reads that webhook secret, and comparing the membership role alone
// hides it from every bearer caller (a token has no membership row).

// seedOrgWithInboundToken ensures org 1 exists carrying a known secret. The base
// fixture has no org row, and getSettings mints a fresh random token for an org
// it can find — either way the value would not be one the test can look for.
func seedOrgWithInboundToken(t *testing.T, db *gorm.DB, secret string) {
	t.Helper()
	org := models.Org{ID: 1, Name: "Test", Slug: "test-" + t.Name(), InboundToken: secret}
	if err := db.Save(&org).Error; err != nil {
		t.Fatalf("seed org: %v", err)
	}
}

func settingsBody(t *testing.T, srv http.Handler, cookies []*http.Cookie) string {
	t.Helper()
	rec := do(srv, http.MethodGet, "/api/settings", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/settings: got %d (body=%s)", rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

func TestInboundTokenHiddenFromMember(t *testing.T) {
	srv, db := newTestHandler(t)

	// Give the org a known inbound token so a leak is unmistakable in the body.
	const secret = "inbound-secret-do-not-leak"
	seedOrgWithInboundToken(t, db, secret)

	memberUID := seedOrgMember(t, db, 1, "member@example.com", "member")
	if body := settingsBody(t, srv, sessionCookies(t, memberUID, 1)); strings.Contains(body, secret) {
		t.Errorf("member can read the org's inbound webhook secret: body=%s", body)
	}

	// Positive half: an admin still gets it, so the assertion above cannot be
	// satisfied by the field simply having disappeared for everybody.
	adminUID := seedOrgMember(t, db, 1, "admin@example.com", "admin")
	if body := settingsBody(t, srv, sessionCookies(t, adminUID, 1)); !strings.Contains(body, secret) {
		t.Errorf("admin cannot read the inbound webhook secret: body=%s", body)
	}
}

func TestInboundTokenFollowsTokenScope(t *testing.T) {
	srv, db := newTestHandler(t)
	const secret = "inbound-secret-token-scope"
	seedOrgWithInboundToken(t, db, secret)

	const adminRaw = "oct_inboundadmintoken0000000000000001"
	const memberRaw = "oct_inboundmembertoken000000000000001"
	seedToken(t, db, adminRaw, "admin")
	seedToken(t, db, memberRaw, "member")

	get := func(raw string) string {
		req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
		req.Header.Set("Authorization", "Bearer "+raw)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /api/settings with token: got %d (body=%s)", rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}

	if body := get(adminRaw); !strings.Contains(body, secret) {
		t.Errorf("admin-scoped token cannot read the inbound webhook secret it is meant to use: body=%s", body)
	}
	if body := get(memberRaw); strings.Contains(body, secret) {
		t.Errorf("member-scoped token read the inbound webhook secret: body=%s", body)
	}
}

// TestTokenCannotGrantOwnerRole pins a deliberate asymmetry rather than an
// oversight. addOrgMember gates the owner grant on callerOrgRole, which a token
// never has, so no token can create or re-grade an owner — not even an
// unrestricted legacy one. Handing ownership of a workspace to a credential with
// no person behind it is not something an automation should do silently, and the
// audit entry would name nobody.
func TestTokenCannotGrantOwnerRole(t *testing.T) {
	srv, db := newTestHandler(t)
	const raw = "oct_ownergranttoken00000000000000001"
	seedToken(t, db, raw, "owner")

	req := httptest.NewRequest(http.MethodPost, "/api/org/members",
		strings.NewReader(`{"email":"newowner@example.com","role":"owner"}`))
	req.Header.Set("Authorization", "Bearer "+raw)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("owner-scoped token granting the owner role: got %d, want 403 (body=%s)", rec.Code, rec.Body.String())
	}
	var count int64
	db.Model(&models.User{}).Where("email = ?", "newowner@example.com").Count(&count)
	if count != 0 {
		t.Errorf("refused request still created the user (rows=%d)", count)
	}
}
