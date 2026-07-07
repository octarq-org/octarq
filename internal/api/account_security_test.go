package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/octarq-org/led/internal/models"
	"github.com/pquerna/otp/totp"
)

// loginCookies runs the real login endpoint and returns the session cookies.
func loginCookies(t *testing.T, srv http.Handler) []*http.Cookie {
	t.Helper()
	rec := do(srv, "POST", "/api/auth/login", nil, `{"username":"admin","password":"pw"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("login: got %d (%s)", rec.Code, rec.Body.String())
	}
	return rec.Result().Cookies()
}

func TestLogoutAllRevokesExistingCookie(t *testing.T) {
	srv, db := newTestHandler(t)
	cookies := loginCookies(t, srv)

	// The freshly issued cookie works against a protected endpoint.
	if rec := do(srv, "GET", "/api/overview", cookies, ""); rec.Code != http.StatusOK {
		t.Fatalf("pre-logout overview: got %d, want 200", rec.Code)
	}

	// Log out everywhere: this bumps the caller's SessionEpoch.
	if rec := do(srv, "POST", "/api/auth/logout-all", cookies, ""); rec.Code != http.StatusOK {
		t.Fatalf("logout-all: got %d (%s)", rec.Code, rec.Body.String())
	}

	var count int64
	db.Model(&models.Session{}).Count(&count)
	if count != 0 {
		t.Fatalf("outstanding sessions: got %d, want 0", count)
	}

	// The old cookie is now stale (epoch mismatch) → 401.
	if rec := do(srv, "GET", "/api/overview", cookies, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("post-logout overview: got %d, want 401", rec.Code)
	}

	// A fresh login mints a cookie under the new epoch and works again.
	if rec := do(srv, "GET", "/api/overview", loginCookies(t, srv), ""); rec.Code != http.StatusOK {
		t.Fatalf("re-login overview: got %d, want 200", rec.Code)
	}
}

func TestTwoFactorEnrollmentAndLogin(t *testing.T) {
	srv, _ := newTestHandler(t)
	cookies := loginCookies(t, srv)

	// Setup → get the pending secret.
	rec := do(srv, "POST", "/api/auth/2fa/setup", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("2fa setup: got %d (%s)", rec.Code, rec.Body.String())
	}
	var setup struct {
		Secret     string `json:"secret"`
		OTPAuthURL string `json:"otpauthUrl"`
	}
	json.Unmarshal(rec.Body.Bytes(), &setup)
	if setup.Secret == "" || setup.OTPAuthURL == "" {
		t.Fatalf("setup missing secret/url: %s", rec.Body.String())
	}

	// Enable with a valid code → get recovery codes.
	code, _ := totp.GenerateCode(setup.Secret, time.Now())
	rec = do(srv, "POST", "/api/auth/2fa/enable", cookies, `{"code":"`+code+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("2fa enable: got %d (%s)", rec.Code, rec.Body.String())
	}
	var enable struct {
		RecoveryCodes []string `json:"recoveryCodes"`
	}
	json.Unmarshal(rec.Body.Bytes(), &enable)
	if len(enable.RecoveryCodes) != recoveryCodeCount {
		t.Fatalf("recovery codes: got %d, want %d", len(enable.RecoveryCodes), recoveryCodeCount)
	}

	// Status reflects enabled.
	rec = do(srv, "GET", "/api/auth/2fa/status", cookies, "")
	if rec.Code != http.StatusOK || !containsJSON(rec.Body.Bytes(), "enabled", true) {
		t.Fatalf("2fa status: got %d (%s)", rec.Code, rec.Body.String())
	}

	// Login now defers the session pending a second factor.
	rec = do(srv, "POST", "/api/auth/login", nil, `{"username":"admin","password":"pw"}`)
	if rec.Code != http.StatusOK || !containsJSON(rec.Body.Bytes(), "twoFactorRequired", true) {
		t.Fatalf("login should require 2FA: got %d (%s)", rec.Code, rec.Body.String())
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Fatal("login must not set a session cookie when 2FA is required")
	}

	// Verify with a TOTP code → session set.
	code, _ = totp.GenerateCode(setup.Secret, time.Now())
	rec = do(srv, "POST", "/api/auth/2fa/verify", nil,
		`{"username":"admin","password":"pw","code":"`+code+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("2fa verify (totp): got %d (%s)", rec.Code, rec.Body.String())
	}
	if len(rec.Result().Cookies()) == 0 {
		t.Fatal("2fa verify must set a session cookie")
	}

	// A recovery code also completes login.
	rec = do(srv, "POST", "/api/auth/2fa/verify", nil,
		`{"username":"admin","password":"pw","code":"`+enable.RecoveryCodes[0]+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("2fa verify (recovery): got %d (%s)", rec.Code, rec.Body.String())
	}

	// The consumed recovery code cannot be reused.
	rec = do(srv, "POST", "/api/auth/2fa/verify", nil,
		`{"username":"admin","password":"pw","code":"`+enable.RecoveryCodes[0]+`"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("reused recovery code: got %d, want 401", rec.Code)
	}
}

func TestAddOrgMemberWithoutSMTPStillReturnsLink(t *testing.T) {
	srv, db := newTestHandler(t)
	const orgID = uint(1)
	adminUID := seedOrgMember(t, db, orgID, "owner@example.com", "owner")
	adminSession := sessionCookies(t, adminUID, orgID)

	// No SMTPSender is configured for this org — the invite must still succeed
	// and return the link.
	rec := do(srv, "POST", "/api/org/members", adminSession,
		`{"email":"`+t.Name()+`+invitee@example.com","role":"member"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("addOrgMember: got %d (%s)", rec.Code, rec.Body.String())
	}
	var res struct {
		Ok        bool   `json:"ok"`
		InviteURL string `json:"inviteUrl"`
	}
	json.Unmarshal(rec.Body.Bytes(), &res)
	if !res.Ok || res.InviteURL == "" {
		t.Fatalf("expected ok+inviteUrl, got %s", rec.Body.String())
	}
}

// containsJSON reports whether the JSON object in b has key == want (bool).
func containsJSON(b []byte, key string, want bool) bool {
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		return false
	}
	v, ok := m[key].(bool)
	return ok && v == want
}
