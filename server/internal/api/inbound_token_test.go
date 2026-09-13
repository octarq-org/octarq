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
// URL. Anyone holding it can forge inbound mail for the workspace, so GET
// /api/settings must not expose it to anyone — not even an admin. The dump
// answers with a boolean (inboundTokenSet); the raw token is returned only by
// the dedicated GET /api/settings/inbound-token endpoint, and that endpoint is
// admin-gated.
//
// The gate also has to understand API tokens: an automation reads that webhook
// secret, and comparing the membership role alone hides it from every bearer
// caller (a token has no membership row).

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

	// The masked dump must not carry the raw secret for a member…
	memberUID := seedOrgMember(t, db, 1, "member@example.com", "member")
	if body := settingsBody(t, srv, sessionCookies(t, memberUID, 1)); strings.Contains(body, secret) {
		t.Errorf("member can read the org's inbound webhook secret: body=%s", body)
	}

	// …and the dedicated endpoint refuses them.
	recMember := do(srv, http.MethodGet, "/api/settings/inbound-token", sessionCookies(t, memberUID, 1), "")
	if recMember.Code != http.StatusForbidden {
		t.Errorf("member GET /api/settings/inbound-token: got %d, want 403", recMember.Code)
	}

	// The settings dump must not leak it for an admin either — only the
	// dedicated endpoint returns the value, so the mask cannot be lifted by role.
	adminUID := seedOrgMember(t, db, 1, "admin@example.com", "admin")
	if body := settingsBody(t, srv, sessionCookies(t, adminUID, 1)); strings.Contains(body, secret) {
		t.Errorf("admin settings dump leaked the inbound webhook secret: body=%s", body)
	}
	rec := do(srv, http.MethodGet, "/api/settings/inbound-token", sessionCookies(t, adminUID, 1), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("admin GET /api/settings/inbound-token: got %d (body=%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), secret) {
		t.Errorf("admin cannot read the inbound webhook secret via its endpoint: body=%s", rec.Body.String())
	}
}

func TestInboundTokenFollowsTokenScope(t *testing.T) {
	srv, db := newTestHandler(t)
	const secret = "inbound-secret-token-scope"
	seedOrgWithInboundToken(t, db, secret)

	const adminRaw = "oct_inboundadmintoken0000000000000001"
	const memberRaw = "oct_inboundmembertoken000000000000001"
	seedMember(t, db, 21, "owner")
	seedToken(t, db, adminRaw, 21, "admin")
	seedToken(t, db, memberRaw, 21, "member")

	get := func(raw string) (int, string) {
		req := httptest.NewRequest(http.MethodGet, "/api/settings/inbound-token", nil)
		req.Header.Set("Authorization", "Bearer "+raw)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}

	if code, body := get(adminRaw); code != http.StatusOK || !strings.Contains(body, secret) {
		t.Errorf("admin-scoped token cannot read the inbound webhook secret it is meant to use: got %d (body=%s)", code, body)
	}
	if code, body := get(memberRaw); code != http.StatusForbidden {
		t.Errorf("member-scoped token read the inbound webhook secret: got %d (body=%s)", code, body)
	}
}

// TestTokenGrantingOwnerFollowsItsCap records a behaviour change, not an
// oversight. This used to be impossible for any token: the owner grant is gated
// on callerOrgRole, a token had none, so every token was refused. The stated
// reason was that handing a workspace to "a credential with no person behind
// it" should not happen silently and the audit entry would name nobody.
//
// A token now has a person behind it and the audit entry names them, so that
// reason is gone — and singling out this one endpoint would have been theatre
// anyway: an owner-capped token can already purge the workspace's data, which
// is worse. The cap is the control. So: owner-capped token held by an owner may
// grant; anything narrower may not.
func TestTokenGrantingOwnerFollowsItsCap(t *testing.T) {
	grant := func(t *testing.T, srv http.Handler, raw, email string) int {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/org/members",
			strings.NewReader(`{"email":"`+email+`","role":"owner"}`))
		req.Header.Set("Authorization", "Bearer "+raw)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		return rec.Code
	}

	t.Run("admin-capped token cannot", func(t *testing.T) {
		srv, db := newTestHandler(t)
		const raw = "oct_ownergrantcapped00000000000000001"
		seedMember(t, db, 22, "owner") // the holder could; the token may not
		seedToken(t, db, raw, 22, "admin")

		if code := grant(t, srv, raw, "nope@example.com"); code != http.StatusForbidden {
			t.Errorf("admin-capped token granting owner: got %d, want 403", code)
		}
		var count int64
		db.Model(&models.User{}).Where("email = ?", "nope@example.com").Count(&count)
		if count != 0 {
			t.Errorf("refused request still created the user (rows=%d)", count)
		}
	})

	t.Run("member-capped token held by an owner cannot", func(t *testing.T) {
		srv, db := newTestHandler(t)
		const raw = "oct_ownergrantmember00000000000000001"
		seedMember(t, db, 23, "owner")
		seedToken(t, db, raw, 23, "member")

		if code := grant(t, srv, raw, "alsonope@example.com"); code != http.StatusForbidden {
			t.Errorf("member-capped token granting owner: got %d, want 403", code)
		}
	})

	t.Run("owner-capped token held by an owner may", func(t *testing.T) {
		srv, db := newTestHandler(t)
		const raw = "oct_ownergrantowner000000000000000001"
		seedMember(t, db, 24, "owner")
		seedToken(t, db, raw, 24, "owner")

		if code := grant(t, srv, raw, "newowner@example.com"); code == http.StatusForbidden {
			t.Error("owner-capped token held by an owner was refused the owner grant")
		}
	})

	t.Run("owner-capped token held by a member cannot", func(t *testing.T) {
		srv, db := newTestHandler(t)
		const raw = "oct_ownergrantbymember0000000000001"
		seedMember(t, db, 25, "member")
		seedToken(t, db, raw, 25, "owner")

		if code := grant(t, srv, raw, "never@example.com"); code != http.StatusForbidden {
			t.Errorf("owner-capped token held by a plain member granting owner: got %d, want 403", code)
		}
	})
}
