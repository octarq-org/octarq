package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/pquerna/otp/totp"
)

func loginCookies(t *testing.T, srv http.Handler) []*http.Cookie {
	t.Helper()
	rec := do(srv, "POST", "/api/auth/login", nil, `{"email":"admin","password":"pw"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("login: got %d (%s)", rec.Code, rec.Body.String())
	}
	return rec.Result().Cookies()
}

func TestLogoutAllRevokesExistingCookie(t *testing.T) {
	srv, db := newTestHandler(t)
	cookies := loginCookies(t, srv)

	if rec := do(srv, "GET", "/api/overview", cookies, ""); rec.Code != http.StatusOK {
		t.Fatalf("pre-logout overview: got %d, want 200", rec.Code)
	}

	if rec := do(srv, "POST", "/api/auth/logout-all", cookies, ""); rec.Code != http.StatusOK {
		t.Fatalf("logout-all: got %d (%s)", rec.Code, rec.Body.String())
	}

	var count int64
	db.Model(&models.Session{}).Count(&count)
	if count != 0 {
		t.Fatalf("outstanding sessions: got %d, want 0", count)
	}

	if rec := do(srv, "GET", "/api/overview", cookies, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("post-logout overview: got %d, want 401", rec.Code)
	}

	if rec := do(srv, "GET", "/api/overview", loginCookies(t, srv), ""); rec.Code != http.StatusOK {
		t.Fatalf("re-login overview: got %d, want 200", rec.Code)
	}
}

func TestTwoFactorEnrollmentAndLogin(t *testing.T) {
	srv, _ := newTestHandler(t)
	cookies := loginCookies(t, srv)

	rec := do(srv, "POST", "/api/auth/2fa/setup", cookies, `{"password":"pw"}`)
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

	rec = do(srv, "GET", "/api/auth/2fa/status", cookies, "")
	if rec.Code != http.StatusOK || !containsJSON(rec.Body.Bytes(), "enabled", true) {
		t.Fatalf("2fa status: got %d (%s)", rec.Code, rec.Body.String())
	}

	rec = do(srv, "POST", "/api/auth/login", nil, `{"email":"admin","password":"pw"}`)
	if rec.Code != http.StatusOK || !containsJSON(rec.Body.Bytes(), "twoFactorRequired", true) {
		t.Fatalf("login should require 2FA: got %d (%s)", rec.Code, rec.Body.String())
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Fatal("login must not set a session cookie when 2FA is required")
	}

	code, _ = totp.GenerateCode(setup.Secret, time.Now())
	rec = do(srv, "POST", "/api/auth/2fa/verify", nil,
		`{"email":"admin","password":"pw","code":"`+code+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("2fa verify (totp): got %d (%s)", rec.Code, rec.Body.String())
	}
	if len(rec.Result().Cookies()) == 0 {
		t.Fatal("2fa verify must set a session cookie")
	}

	rec = do(srv, "POST", "/api/auth/2fa/verify", nil,
		`{"email":"admin","password":"pw","code":"`+enable.RecoveryCodes[0]+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("2fa verify (recovery): got %d (%s)", rec.Code, rec.Body.String())
	}

	rec = do(srv, "POST", "/api/auth/2fa/verify", nil,
		`{"email":"admin","password":"pw","code":"`+enable.RecoveryCodes[0]+`"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("reused recovery code: got %d, want 401", rec.Code)
	}
}

func TestAddOrgMemberWithoutSMTPStillSucceeds(t *testing.T) {
	srv, db := newTestHandler(t)
	const orgID = uint(1)
	adminUID := seedOrgMember(t, db, orgID, "owner@example.com", "owner")
	adminSession := sessionCookies(t, adminUID, orgID)

	email := t.Name() + "+invitee@example.com"
	rec := do(srv, "POST", "/api/org/members", adminSession,
		`{"email":"`+email+`","role":"member"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("addOrgMember: got %d (%s)", rec.Code, rec.Body.String())
	}
	var res struct {
		Ok bool `json:"ok"`
	}
	json.Unmarshal(rec.Body.Bytes(), &res)
	if !res.Ok {
		t.Fatalf("expected ok, got %s", rec.Body.String())
	}
	var user models.User
	if err := db.Where("email = ?", email).First(&user).Error; err != nil {
		t.Fatalf("invited user not created without a mail sender: %v", err)
	}
	if user.InviteTokenHash == "" {
		t.Fatal("no pending invite stored for the invited user")
	}
}

func containsJSON(b []byte, key string, want bool) bool {
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		return false
	}
	v, ok := m[key].(bool)
	return ok && v == want
}

func TestTwoFAEnrolmentRequiresPassword(t *testing.T) {
	srv, db := newTestHandler(t)
	cookies := loginCookies(t, srv)

	for _, body := range []string{`{}`, `{"password":""}`, `{"password":"wrong"}`} {
		if rec := do(srv, "POST", "/api/auth/2fa/setup", cookies, body); rec.Code != http.StatusUnauthorized {
			t.Errorf("setup with %s: got %d, want 401", body, rec.Code)
		}
	}
	var user models.User
	if err := db.Where("email = ?", "admin").First(&user).Error; err == nil && user.TOTPSecret != "" {
		t.Error("a rejected setup still stored a pending TOTP secret")
	}
}
