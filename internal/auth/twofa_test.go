package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/octarq-org/octarq/internal/models"
)

// Guard test 3: SSO (LoginByIdentity) admits a TOTP-enabled account — the IdP
// owns the second factor — but MUST leave an audit trail so an operator can
// tell "SSO behind IdP MFA" from "SSO with no MFA at all".
func TestLoginByIdentityWithTOTPUserWritesBypassAudit(t *testing.T) {
	db := identityDB(t)
	m := identityManager(t, db)
	org := mustOrg(t, db, "Acme")
	user := mustUser(t, db, "bob@acme.com")
	if err := db.Model(&user).Update("totp_enabled", true).Error; err != nil {
		t.Fatalf("enable totp: %v", err)
	}
	id := oidcID("https://idp.acme.com", "sub-bob", "bob@acme.com")
	if err := bindIdentity(db, user.ID, id); err != nil {
		t.Fatalf("bind: %v", err)
	}
	id.OrgID, id.AllowJIT = org.ID, true

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	uid, err := m.LoginByIdentity(rec, req, id)
	if err != nil {
		t.Fatalf("LoginByIdentity: %v", err)
	}
	if uid != user.ID {
		t.Fatalf("LoginByIdentity returned user %d, want %d", uid, user.ID)
	}
	if len(rec.Result().Cookies()) == 0 {
		t.Fatal("LoginByIdentity issued no session cookie")
	}

	var log models.AuditLog
	if err := db.Where("action = ?", "auth.sso_login_bypassed_totp").First(&log).Error; err != nil {
		t.Fatalf("expected an auth.sso_login_bypassed_totp audit row, got none: %v", err)
	}
	if log.ActorID != user.ID || log.TargetID != user.ID {
		t.Errorf("audit actor/target = %d/%d, want %d", log.ActorID, log.TargetID, user.ID)
	}
}

// No TOTP on the account, no bypass to record: the audit row must not appear,
// so "presence of the row" stays a meaningful signal.
func TestLoginByIdentityWithoutTOTPWritesNoBypassAudit(t *testing.T) {
	db := identityDB(t)
	m := identityManager(t, db)
	org := mustOrg(t, db, "Acme")
	user := mustUser(t, db, "bob@acme.com")
	id := oidcID("https://idp.acme.com", "sub-bob", "bob@acme.com")
	if err := bindIdentity(db, user.ID, id); err != nil {
		t.Fatalf("bind: %v", err)
	}
	id.OrgID, id.AllowJIT = org.ID, true

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if _, err := m.LoginByIdentity(rec, req, id); err != nil {
		t.Fatalf("LoginByIdentity: %v", err)
	}

	var n int64
	db.Model(&models.AuditLog{}).Where("action = ?", "auth.sso_login_bypassed_totp").Count(&n)
	if n != 0 {
		t.Fatalf("login without TOTP wrote %d bypass audit row(s)", n)
	}
}

// LoginByEmail gets the same treatment: it is a plugin-facing trusted-email
// login (no in-repo caller other than app.App.loginByEmail), so a TOTP user
// arriving through it is admitted and audited, never double-challenged.
func TestLoginByEmailWithTOTPUserWritesBypassAudit(t *testing.T) {
	db := identityDB(t)
	m := identityManager(t, db)
	user := mustUser(t, db, "bob@example.com")
	if err := db.Model(&user).Update("totp_enabled", true).Error; err != nil {
		t.Fatalf("enable totp: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	uid, err := m.LoginByEmail(rec, req, "bob@example.com")
	if err != nil {
		t.Fatalf("LoginByEmail: %v", err)
	}
	if uid != user.ID {
		t.Fatalf("LoginByEmail returned user %d, want %d", uid, user.ID)
	}
	if len(rec.Result().Cookies()) == 0 {
		t.Fatal("LoginByEmail issued no session cookie")
	}

	var log models.AuditLog
	if err := db.Where("action = ?", "auth.sso_login_bypassed_totp").First(&log).Error; err != nil {
		t.Fatalf("expected an auth.sso_login_bypassed_totp audit row, got none: %v", err)
	}
	if log.ActorID != user.ID {
		t.Errorf("audit actor = %d, want %d", log.ActorID, user.ID)
	}
}

// The challenge token must refuse a forged signature and an expired token,
// and still complete for a fresh one — the OAuth 2FA redirect rides on it.
func TestTwoFAChallengeRoundTrip(t *testing.T) {
	m := identityManager(t, identityDB(t))

	tok, err := m.NewTwoFAChallenge(7)
	if err != nil {
		t.Fatalf("NewTwoFAChallenge: %v", err)
	}
	if uid := m.VerifyTwoFAChallenge(tok); uid != 7 {
		t.Fatalf("fresh challenge resolved to %d, want 7", uid)
	}

	if uid := m.VerifyTwoFAChallenge("forged.payload"); uid != 0 {
		t.Fatalf("forged challenge resolved to %d, want 0", uid)
	}

	expired, err := m.mintChallenge(7, time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatalf("mint expired challenge: %v", err)
	}
	if uid := m.VerifyTwoFAChallenge(expired); uid != 0 {
		t.Fatalf("expired challenge resolved to %d, want 0", uid)
	}
}
