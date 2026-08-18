package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
)

func TestMaskIPServer(t *testing.T) {
	cases := map[string]string{
		"127.0.0.1":        "127.0.0.1",
		"::1":              "::1",
		"192.168.1.100":    "192.168.1.*",
		"10.0.0.1":         "10.0.0.*",
		"2001:db8:85a3::1": "2001:db8:85a3::*",
		"unknown":          "unknown",
	}
	for in, want := range cases {
		if got := maskIPServer(in); got != want {
			t.Errorf("maskIPServer(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestChangeEmailFlows(t *testing.T) {
	srv, db := newTestHandler(t)
	adminCookies := loginCookies(t, srv)

	var adminUser models.User
	db.Where("email = ?", "admin").First(&adminUser)
	hash, _ := bcrypt.GenerateFromPassword([]byte("pw"), bcrypt.MinCost)
	db.Model(&adminUser).Updates(map[string]any{
		"email":         "admin@example.com",
		"password_hash": string(hash),
	})
	adminUser.Email = "admin@example.com"

	// 1. Unauthenticated -> 401
	rec := do(srv, "PUT", "/api/auth/email", nil, `{"newEmail":"admin2@example.com","currentPassword":"pw"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauth change email: got %d, want 401", rec.Code)
	}

	// 2. Invalid email format -> 400
	rec = do(srv, "PUT", "/api/auth/email", adminCookies, `{"newEmail":"invalid-email","currentPassword":"pw"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid email: got %d, want 400", rec.Code)
	}

	// 3. Same email as current -> 200 (no-op)
	rec = do(srv, "PUT", "/api/auth/email", adminCookies, `{"newEmail":"admin@example.com","currentPassword":"pw"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("same email: got %d, want 200 (%s)", rec.Code, rec.Body.String())
	}

	// 4. SSO user without password hash -> 400
	ssoUser := models.User{Email: "sso_auth@example.com", PasswordHash: ""}
	db.Create(&ssoUser)
	ssoCookies := sessionCookies(t, ssoUser.ID, 1)
	rec = do(srv, "PUT", "/api/auth/email", ssoCookies, `{"newEmail":"new_sso@example.com","currentPassword":""}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("sso user change email: got %d, want 400", rec.Code)
	}

	// 5. Wrong current password -> 400
	rec = do(srv, "PUT", "/api/auth/email", adminCookies, `{"newEmail":"admin2@example.com","currentPassword":"wrong"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("wrong password: got %d, want 400", rec.Code)
	}

	// 6. Conflict with existing user -> 409
	otherUser := models.User{Email: "taken@example.com"}
	db.Create(&otherUser)
	rec = do(srv, "PUT", "/api/auth/email", adminCookies, `{"newEmail":"taken@example.com","currentPassword":"pw"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("taken email: got %d, want 409", rec.Code)
	}

	// 7. Successful email change (require_email_verification = false)
	disableEmailVerification(t, db)
	rec = do(srv, "PUT", "/api/auth/email", adminCookies, `{"newEmail":"admin_new@example.com","currentPassword":"pw"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("successful change email: got %d (%s)", rec.Code, rec.Body.String())
	}
	var reloaded models.User
	db.First(&reloaded, adminUser.ID)
	if reloaded.Email != "admin_new@example.com" {
		t.Errorf("email not updated in DB: %q", reloaded.Email)
	}

	// 8. Successful email change (require_email_verification = true)
	db.Save(&models.Setting{Key: keyRequireEmailVerification, Value: "true"})
	rec = do(srv, "PUT", "/api/auth/email", adminCookies, `{"newEmail":"admin_verified@example.com","currentPassword":"pw"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("change email with verification: got %d (%s)", rec.Code, rec.Body.String())
	}
	db.First(&reloaded, adminUser.ID)
	if reloaded.VerifyTokenHash == "" {
		t.Errorf("expected VerifyTokenHash to be set")
	}

	// 9. Nil Ctx guards on auth methods
	h, _, _ := newTestHandlerRaw(t)
	ctx := context.Background()
	if _, err := h.loginHuma(ctx, &LoginInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in loginHuma")
	}
	if _, err := h.verify2FA(ctx, &Verify2FAInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in verify2FA")
	}
	if _, err := h.logoutAll(ctx, &LogoutAllInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in logoutAll")
	}
	if _, err := h.changePassword(ctx, &ChangePasswordInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in changePassword")
	}
	if _, err := h.listSessions(ctx, &ListSessionsInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in listSessions")
	}
	if _, err := h.revokeSession(ctx, &RevokeSessionInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in revokeSession")
	}
	if _, err := h.logout(ctx, &LogoutInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in logout")
	}
	if _, err := h.me(ctx, &MeInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in me")
	}
	if _, err := h.changeEmail(ctx, &ChangeEmailInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in changeEmail")
	}
	if _, err := h.acceptInvite(ctx, &AcceptInviteInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in acceptInvite")
	}
}

func TestAuthSessionsMeAndInvite(t *testing.T) {
	h, srv, db := newTestHandlerRaw(t)
	adminCookies := loginCookies(t, srv)

	// 1. GET /api/auth/me unauth -> 401
	rec := do(srv, "GET", "/api/auth/me", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauth me: got %d, want 401", rec.Code)
	}

	// 2. GET /api/auth/me auth -> 200
	rec = do(srv, "GET", "/api/auth/me", adminCookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("auth me: got %d (%s)", rec.Code, rec.Body.String())
	}

	// 3. GET /api/auth/sessions auth -> 200
	rec = do(srv, "GET", "/api/auth/sessions", adminCookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list sessions: got %d", rec.Code)
	}

	// 4. DELETE /api/auth/sessions/999999 -> 404
	rec = do(srv, "DELETE", "/api/auth/sessions/999999", adminCookies, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("revoke nonexistent session: got %d, want 404", rec.Code)
	}

	// 5. POST /api/auth/invite/accept with invalid token -> 400
	h.recoveryLimiter.reset("192.0.2.1")
	rec = do(srv, "POST", "/api/auth/invite/accept", nil, `{"token":"invalid-token","password":"newpassword123"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("accept invalid invite token: got %d, want 400", rec.Code)
	}

	// 6. POST /api/auth/invite/accept with short password -> 400
	h.recoveryLimiter.reset("192.0.2.1")
	rec = do(srv, "POST", "/api/auth/invite/accept", nil, `{"token":"invalid-token","password":"short"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("accept invite short password: got %d, want 400", rec.Code)
	}

	// Expired invite token -> 400
	expiredTime := time.Now().Add(-1 * time.Hour)
	expUser := models.User{
		Email:           "expinvite@example.com",
		InviteTokenHash: hashToken("expiredinvitetoken"),
		InviteExpiresAt: &expiredTime,
	}
	db.Create(&expUser)
	h.recoveryLimiter.reset("192.0.2.1")
	rec = do(srv, "POST", "/api/auth/invite/accept", nil, `{"token":"expiredinvitetoken","password":"validpassword123"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("accept expired invite token: got %d, want 400", rec.Code)
	}

	// Valid invite token -> 200
	futureTime := time.Now().Add(24 * time.Hour)
	validUser := models.User{
		Email:           "validinvite@example.com",
		InviteTokenHash: hashToken("goodinvitetoken"),
		InviteExpiresAt: &futureTime,
	}
	db.Create(&validUser)
	db.Create(&models.OrgMember{OrgID: 1, UserID: validUser.ID, Role: "member"})
	h.recoveryLimiter.reset("192.0.2.1")
	rec = do(srv, "POST", "/api/auth/invite/accept", nil, `{"token":"goodinvitetoken","password":"validpassword123"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("accept valid invite token: got %d (%s)", rec.Code, rec.Body.String())
	}

	// 7. POST /api/auth/2fa/verify
	// Create user with 2FA enabled
	secretKey, _ := totp.Generate(totp.GenerateOpts{Issuer: "octarq", AccountName: "totp2fa@example.com"})
	totpSecret := secretKey.Secret()
	encSecret, _ := h.cipher.Encrypt([]byte(totpSecret))
	hashPw, _ := bcrypt.GenerateFromPassword([]byte("strongpassword123"), bcrypt.MinCost)
	twofaUser := models.User{
		Email:         "totp2fa@example.com",
		PasswordHash:  string(hashPw),
		TOTPEnabled:   true,
		TOTPSecret:    encSecret,
		EmailVerified: true,
	}
	db.Create(&twofaUser)
	db.Create(&models.OrgMember{OrgID: 1, UserID: twofaUser.ID, Role: "member"})

	// Wrong password -> 401
	rec = do(srv, "POST", "/api/auth/2fa/verify", nil, `{"email":"totp2fa@example.com","password":"wrong","code":"123456"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("verify2FA wrong password: got %d, want 401", rec.Code)
	}

	// Wrong TOTP code -> 401
	rec = do(srv, "POST", "/api/auth/2fa/verify", nil, `{"email":"totp2fa@example.com","password":"strongpassword123","code":"000000"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("verify2FA wrong code: got %d, want 401", rec.Code)
	}

	// Valid TOTP code -> 200
	validCode, _ := totp.GenerateCode(totpSecret, time.Now())
	rec = do(srv, "POST", "/api/auth/2fa/verify", nil, fmt.Sprintf(`{"email":"totp2fa@example.com","password":"strongpassword123","code":%q}`, validCode))
	if rec.Code != http.StatusOK {
		t.Fatalf("verify2FA valid code: got %d (%s)", rec.Code, rec.Body.String())
	}

	// 8. POST /api/auth/password
	// Unauth -> 401
	rec = do(srv, "POST", "/api/auth/password", nil, `{"currentPassword":"pw","newPassword":"newpassword123"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauth change password: got %d, want 401", rec.Code)
	}

	// Empty password hash -> 400
	rec = do(srv, "POST", "/api/auth/password", adminCookies, `{"currentPassword":"pw","newPassword":"newpassword123"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty password hash change password: got %d, want 400", rec.Code)
	}

	// Set password hash and valid email for admin
	var adminUser models.User
	db.Where("email = ?", "admin").First(&adminUser)
	pwHash, _ := bcrypt.GenerateFromPassword([]byte("currentpw123"), bcrypt.MinCost)
	db.Model(&adminUser).Updates(map[string]any{"password_hash": string(pwHash), "email": "admin@example.com"})

	// Short new password -> 400
	rec = do(srv, "POST", "/api/auth/password", adminCookies, `{"currentPassword":"currentpw123","newPassword":"short"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("short new password: got %d, want 400", rec.Code)
	}

	// Wrong current password -> 400
	rec = do(srv, "POST", "/api/auth/password", adminCookies, `{"currentPassword":"wrong","newPassword":"newpassword123"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("wrong current password: got %d, want 400", rec.Code)
	}

	// Valid password change -> 200
	rec = do(srv, "POST", "/api/auth/password", adminCookies, `{"currentPassword":"currentpw123","newPassword":"newpassword123"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("valid change password: got %d (%s)", rec.Code, rec.Body.String())
	}

	// 9. PUT /api/auth/email
	// Unauth -> 401
	rec = do(srv, "PUT", "/api/auth/email", nil, `{"newEmail":"admin2@example.com","currentPassword":"newpassword123"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauth change email: got %d, want 401", rec.Code)
	}

	// Invalid email -> 400
	rec = do(srv, "PUT", "/api/auth/email", adminCookies, `{"newEmail":"notanemail","currentPassword":"newpassword123"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid new email: got %d, want 400", rec.Code)
	}

	// Same email -> 200 no-op
	rec = do(srv, "PUT", "/api/auth/email", adminCookies, `{"newEmail":"admin@example.com","currentPassword":"newpassword123"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("same email change: got %d, want 200", rec.Code)
	}

	// Wrong current password -> 400
	rec = do(srv, "PUT", "/api/auth/email", adminCookies, `{"newEmail":"newadmin@example.com","currentPassword":"wrongpassword"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("wrong password change email: got %d, want 400", rec.Code)
	}

	// Existing email collision -> 409
	rec = do(srv, "PUT", "/api/auth/email", adminCookies, `{"newEmail":"totp2fa@example.com","currentPassword":"newpassword123"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("existing email collision: got %d, want 409", rec.Code)
	}

	// Valid email change -> 200
	rec = do(srv, "PUT", "/api/auth/email", adminCookies, `{"newEmail":"brandnewadmin@example.com","currentPassword":"newpassword123"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("valid change email: got %d (%s)", rec.Code, rec.Body.String())
	}

	// 10. locationFromGeo helper
	if loc := locationFromGeo("US", "New York"); loc != "New York, US" {
		t.Errorf("locationFromGeo(US, New York) = %q, want New York, US", loc)
	}
	if loc := locationFromGeo("US", ""); loc != "US" {
		t.Errorf("locationFromGeo(US, \"\") = %q, want US", loc)
	}
	if loc := locationFromGeo("", "Paris"); loc != "Paris" {
		t.Errorf("locationFromGeo(\"\", Paris) = %q, want Paris", loc)
	}
	if loc := locationFromGeo("", ""); loc != "" {
		t.Errorf("locationFromGeo(\"\", \"\") = %q, want \"\"", loc)
	}

	// 11. Revoke own session
	// List sessions to find own session ID
	rec = do(srv, "GET", "/api/auth/sessions", adminCookies, "")
	var sessList []models.Session
	if err := json.Unmarshal(rec.Body.Bytes(), &sessList); err == nil && len(sessList) > 0 {
		rec = do(srv, "DELETE", fmt.Sprintf("/api/auth/sessions/%d", sessList[0].ID), adminCookies, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("revoke own session: got %d (%s)", rec.Code, rec.Body.String())
		}
	}

	// 12. POST /api/auth/logout
	// Unauth -> 200
	rec = do(srv, "POST", "/api/auth/logout", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("unauth logout: got %d, want 200", rec.Code)
	}

	// Auth -> 200
	freshAdminCookies := loginCookies(t, srv)
	rec = do(srv, "POST", "/api/auth/logout", freshAdminCookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("auth logout: got %d", rec.Code)
	}
}

func TestDirectAuthUnauthHandlers(t *testing.T) {
	h, _, _ := newTestHandlerRaw(t)
	ctx := context.Background()
	req := httptest.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()
	humaCtx := humago.NewContext(nil, req, w)

	if _, err := h.me(ctx, &MeInput{Ctx: humaCtx}); err == nil {
		t.Error("expected 401 for direct me")
	}
	if _, err := h.changePassword(ctx, &ChangePasswordInput{Ctx: humaCtx}); err == nil {
		t.Error("expected 401 for direct changePassword")
	}
	if _, err := h.changeEmail(ctx, &ChangeEmailInput{Ctx: humaCtx}); err == nil {
		t.Error("expected 401 for direct changeEmail")
	}
	if _, err := h.listSessions(ctx, &ListSessionsInput{Ctx: humaCtx}); err == nil {
		t.Error("expected 401 for direct listSessions")
	}
	if _, err := h.revokeSession(ctx, &RevokeSessionInput{Ctx: humaCtx}); err == nil {
		t.Error("expected 401 for direct revokeSession")
	}
}
