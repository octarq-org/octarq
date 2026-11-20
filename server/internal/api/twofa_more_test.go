package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
)

func TestTwoFAMore(t *testing.T) {
	srv, db := newTestHandler(t)
	adminCookies := loginCookies(t, srv)

	var user models.User
	db.Where("email = ?", "admin").First(&user)
	hash, _ := bcrypt.GenerateFromPassword([]byte("pw"), bcrypt.MinCost)
	db.Model(&user).Update("password_hash", string(hash))

	rec := do(srv, "GET", "/api/auth/2fa/status", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauth 2fa status: got %d, want 401", rec.Code)
	}

	rec = do(srv, "GET", "/api/auth/2fa/status", adminCookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("auth 2fa status: got %d (%s)", rec.Code, rec.Body.String())
	}

	rec = do(srv, "POST", "/api/auth/2fa/setup", adminCookies, `{"password":"wrong"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("setup 2fa wrong password: got %d, want 401", rec.Code)
	}

	rec = do(srv, "POST", "/api/auth/2fa/setup", adminCookies, `{"password":"pw"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("setup 2fa valid: got %d (%s)", rec.Code, rec.Body.String())
	}
	var setupResp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &setupResp)
	secret := setupResp["secret"].(string)

	rec = do(srv, "POST", "/api/auth/2fa/enable", adminCookies, `{"code":"000000"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("enable 2fa invalid code: got %d, want 400", rec.Code)
	}

	validCode, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("generate valid totp code: %v", err)
	}
	rec = do(srv, "POST", "/api/auth/2fa/enable", adminCookies, fmt.Sprintf(`{"code":%q}`, validCode))
	if rec.Code != http.StatusOK {
		t.Fatalf("enable 2fa valid code: got %d (%s)", rec.Code, rec.Body.String())
	}
	var enableResp struct {
		OK            bool     `json:"ok"`
		RecoveryCodes []string `json:"recoveryCodes"`
	}
	json.Unmarshal(rec.Body.Bytes(), &enableResp)
	if len(enableResp.RecoveryCodes) != 8 {
		t.Fatalf("expected 8 recovery codes, got %d", len(enableResp.RecoveryCodes))
	}

	recoveryCode := enableResp.RecoveryCodes[0]
	rec = do(srv, "POST", "/api/auth/2fa/disable", adminCookies, fmt.Sprintf(`{"code":%q,"password":"pw"}`, recoveryCode))
	if rec.Code != http.StatusOK {
		t.Fatalf("disable 2fa with recovery code: got %d (%s)", rec.Code, rec.Body.String())
	}

	h, _, _ := newTestHandlerRaw(t)
	plainCodes, hashedCodes, err := generateRecoveryCodes(8)
	if err != nil || len(plainCodes) != 8 || len(hashedCodes) != 8 {
		t.Fatalf("generateRecoveryCodes: %v", err)
	}

	user.TOTPSecret, _ = h.cipher.Encrypt([]byte(secret))
	user.LastTOTPCode = validCode
	if h.verifyTOTPOrRecovery(&user, validCode) {
		t.Error("expected replay of same validCode to be rejected")
	}

	bHashes, _ := json.Marshal(hashedCodes)
	user.RecoveryCodes = string(bHashes)
	user.LastTOTPCode = ""
	user.TOTPSecret = ""
	if !h.verifyTOTPOrRecovery(&user, plainCodes[0]) {
		t.Errorf("expected verifyTOTPOrRecovery with plain recovery code %q to succeed", plainCodes[0])
	}
	var remainingHashes []string
	json.Unmarshal([]byte(user.RecoveryCodes), &remainingHashes)
	if len(remainingHashes) != 7 {
		t.Errorf("expected 7 remaining recovery codes after consuming 1, got %d", len(remainingHashes))
	}

	if h.verifyTOTPOrRecovery(&user, "") {
		t.Error("expected verifyTOTPOrRecovery with empty code to return false")
	}

	user.RecoveryCodes = `["bad-hash"]`
	if h.verifyTOTPOrRecovery(&user, "invalid-code") {
		t.Error("expected verifyTOTPOrRecovery with invalid recovery code to return false")
	}

	ssoUser := models.User{Email: "sso_2fa@example.com", PasswordHash: ""}
	db.Create(&ssoUser)
	db.Create(&models.OrgMember{OrgID: 1, UserID: ssoUser.ID, Role: "member"})
	ssoCookies := sessionCookies(t, ssoUser.ID, 1)
	rec = do(srv, "POST", "/api/auth/2fa/setup", ssoCookies, `{}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("sso setup 2fa: got %d (%s)", rec.Code, rec.Body.String())
	}

	rec = do(srv, "POST", "/api/auth/2fa/setup", adminCookies, `{}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("local user empty password setup 2fa: got %d, want 401", rec.Code)
	}

	noSecretUser := models.User{Email: "nosecret@example.com"}
	db.Create(&noSecretUser)
	db.Create(&models.OrgMember{OrgID: 1, UserID: noSecretUser.ID, Role: "member"})
	noSecretCookies := sessionCookies(t, noSecretUser.ID, 1)
	rec = do(srv, "POST", "/api/auth/2fa/enable", noSecretCookies, `{"code":"123456"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("enable 2fa without secret: got %d, want 400", rec.Code)
	}

	rec = do(srv, "POST", "/api/auth/2fa/disable", noSecretCookies, `{"code":"123456"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("disable 2fa when disabled: got %d, want 200", rec.Code)
	}

	ctx := context.Background()
	if _, err := h.twoFAStatus(ctx, &TwoFAStatusInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in twoFAStatus")
	}
	if _, err := h.setup2FA(ctx, &Setup2FAInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in setup2FA")
	}
	if _, err := h.enable2FA(ctx, &Enable2FAInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in enable2FA")
	}
	if _, err := h.disable2FA(ctx, &Disable2FAInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in disable2FA")
	}
}
