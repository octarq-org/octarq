package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/octarq-org/octarq/config"
	"github.com/octarq-org/octarq/internal/crypto"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugin"
)

// TestManagerGuards covers the early-return guards in the async/revocation
// helpers that a stateless manager (no DB) and empty inputs hit.
func TestManagerGuards(t *testing.T) {
	m := testManager(t) // no DB attached

	m.touchToken(3) // must not panic with a nil DB

	if n := m.RevokeUserOrgSessions(1, 1); n != 0 {
		t.Errorf("RevokeUserOrgSessions without DB = %d, want 0", n)
	}
	if n := m.RevokeUserOrgTokens(1, 1); n != 0 {
		t.Errorf("RevokeUserOrgTokens without DB = %d, want 0", n)
	}

	db := identityDB(t)
	m = m.WithDB(db)
	if n := m.RevokeUserOrgSessions(0, 1); n != 0 {
		t.Errorf("RevokeUserOrgSessions(user 0) = %d, want 0", n)
	}
	if n := m.RevokeUserOrgSessions(1, 0); n != 0 {
		t.Errorf("RevokeUserOrgSessions(org 0) = %d, want 0", n)
	}
	if n := m.RevokeUserOrgSessions(1, 1); n != 0 {
		t.Errorf("RevokeUserOrgSessions with no rows = %d, want 0", n)
	}

	// TouchSession no-ops for a missing cookie and an unknown token.
	m.TouchSession(httptest.NewRequest("GET", "/", nil))
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: "no-such-token"})
	m.TouchSession(req)
}

// TestRegisterEmptyID covers the early return in methods.Register.
func TestRegisterEmptyID(t *testing.T) {
	Register(AuthMethod{})
}

// TestBindIdentityNormalizeError surfaces the normalize failure through
// bindIdentity rather than only through the direct helper call.
func TestBindIdentityNormalizeError(t *testing.T) {
	db := identityDB(t)
	user := mustUser(t, db, "a@acme.com")
	if err := bindIdentity(db, user.ID, plugin.ExternalIdentity{Provider: "oidc"}); err == nil {
		t.Fatal("bindIdentity accepted an identity missing issuer/subject")
		return
	}
	var n int64
	db.Model(&models.UserIdentity{}).Count(&n)
	if n != 0 {
		t.Errorf("failed bind wrote %d row(s)", n)
	}
}

// TestUserHasTOTPMissingUser returns false for a non-existent uid.
func TestUserHasTOTPMissingUser(t *testing.T) {
	m := testManager(t).WithDB(identityDB(t))
	if m.userHasTOTP(999999) {
		t.Error("userHasTOTP true for a missing user")
	}
}

// TestResolveBoundIdentityWithoutOrg joins a bound identity's first org when no
// org is named on the assertion (the joinOrg fallback).
func TestResolveBoundIdentityWithoutOrg(t *testing.T) {
	db := identityDB(t)
	user := mustUser(t, db, "bob@acme.com")
	id := oidcID("https://idp.acme.com", "sub-bob", "bob@acme.com")
	if err := bindIdentity(db, user.ID, id); err != nil {
		t.Fatalf("bind: %v", err)
	}
	uid, orgID, err := resolveIdentity(db, id)
	if err != nil {
		t.Fatalf("resolveIdentity: %v", err)
	}
	if uid != user.ID || orgID == 0 {
		t.Errorf("resolve = (%d, %d), want user %d and a created org", uid, orgID, user.ID)
	}
}

// TestRegistrationAllowedEdge reads the setting default (on when absent) and a
// nil DB (off).
func TestRegistrationAllowedEdge(t *testing.T) {
	db := identityDB(t)
	if !registrationAllowed(db) {
		t.Error("registrationAllowed defaulted to off without a setting row")
	}
	if registrationAllowed(nil) {
		t.Error("registrationAllowed returned true for a nil DB")
	}
}

// TestUpsertUserByEmailEmpty refuses the empty address.
func TestUpsertUserByEmailEmpty(t *testing.T) {
	db := identityDB(t)
	if _, _, err := UpsertUserByEmail(db, "   ", true); err == nil {
		t.Fatal("UpsertUserByEmail accepted an empty address")
		return
	}
}

// TestDecryptedSettingGarbage refuses an undecryptable stored secret.
func TestDecryptedSettingGarbage(t *testing.T) {
	db := identityDB(t)
	db.Create(&models.Setting{Key: "oauth.x.client_secret", Value: "not-encrypted"})
	h := NewOAuthHandler(db, New(&config.Config{SecretKey: "s"}, crypto.New("s")).WithDB(db), crypto.New("s"))
	if got := h.decryptedSetting("oauth.x.client_secret"); got != "" {
		t.Errorf("decryptedSetting on garbage = %q, want empty", got)
	}
}
