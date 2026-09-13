package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/models"
)

func TestVerify2FACoverage(t *testing.T) {
	h, _, db := newTestHandlerRaw(t)
	disableEmailVerification(t, db)
	ctx := context.Background()

	// Purge settings cache to ensure it reads from db

	t.Run("Rate limit exceeded", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/2fa/verify", nil)
		req.RemoteAddr = "127.0.0.1:1234"
		rec := httptest.NewRecorder()
		humaCtx := humago.NewContext(nil, req, rec)

		// Exhaust the rate limit
		for i := 0; i < 15; i++ {
			h.loginLimiter.recordFailure("127.0.0.1")
		}

		_, err := h.verify2FA(ctx, &Verify2FAInput{Ctx: humaCtx})
		if err == nil || !strings.Contains(err.Error(), "too many failed login attempts") {
			t.Errorf("expected rate limit error, got %v", err)
		}

		// Reset for further tests
		h.loginLimiter.reset("127.0.0.1")
	})

	t.Run("Challenge - forged challenge", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/2fa/verify", nil)
		req.RemoteAddr = "127.0.0.1:1235"

		token := "fake.token.here"
		req.AddCookie(&http.Cookie{Name: "octarq_2fa_challenge", Value: token})

		rec := httptest.NewRecorder()
		humaCtx := humago.NewContext(nil, req, rec)

		_, err := h.verify2FA(ctx, &Verify2FAInput{Ctx: humaCtx})
		if err == nil || !strings.Contains(err.Error(), "invalid 2FA challenge") {
			t.Errorf("expected invalid 2FA challenge error, got %v", err)
		}
	})

	t.Run("Challenge - missing user", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/2fa/verify", nil)
		req.RemoteAddr = "127.0.0.1:1236"

		token, _ := h.auth.NewTwoFAChallenge(999)
		req.AddCookie(&http.Cookie{Name: "octarq_2fa_challenge", Value: token})

		rec := httptest.NewRecorder()
		humaCtx := humago.NewContext(nil, req, rec)

		_, err := h.verify2FA(ctx, &Verify2FAInput{Ctx: humaCtx})
		if err == nil || !strings.Contains(err.Error(), "invalid 2FA challenge") {
			t.Errorf("expected invalid 2FA challenge error (missing user), got %v", err)
		}
	})

	t.Run("Challenge - TOTP disabled", func(t *testing.T) {
		user := models.User{Email: "nototp@example.com", PasswordHash: "x", TOTPEnabled: false}
		db.Create(&user)

		req := httptest.NewRequest(http.MethodPost, "/api/auth/2fa/verify", nil)
		req.RemoteAddr = "127.0.0.1:1237"

		token, _ := h.auth.NewTwoFAChallenge(user.ID)
		req.AddCookie(&http.Cookie{Name: "octarq_2fa_challenge", Value: token})

		rec := httptest.NewRecorder()
		humaCtx := humago.NewContext(nil, req, rec)

		_, err := h.verify2FA(ctx, &Verify2FAInput{Ctx: humaCtx})
		if err == nil || !strings.Contains(err.Error(), "invalid 2FA challenge") {
			t.Errorf("expected invalid 2FA challenge error (TOTP disabled), got %v", err)
		}
	})

	t.Run("Challenge - Missing org member", func(t *testing.T) {
		user := models.User{Email: "noorg@example.com", PasswordHash: "x", TOTPEnabled: true}
		db.Create(&user)

		req := httptest.NewRequest(http.MethodPost, "/api/auth/2fa/verify", nil)
		req.RemoteAddr = "127.0.0.1:1238"

		token, _ := h.auth.NewTwoFAChallenge(user.ID)
		req.AddCookie(&http.Cookie{Name: "octarq_2fa_challenge", Value: token})

		rec := httptest.NewRecorder()
		humaCtx := humago.NewContext(nil, req, rec)

		_, err := h.verify2FA(ctx, &Verify2FAInput{Ctx: humaCtx})
		if err == nil || !strings.Contains(err.Error(), "invalid 2FA challenge") {
			t.Errorf("expected invalid 2FA challenge error (missing org member), got %v", err)
		}
	})

	t.Run("Email Verification Required - non admin", func(t *testing.T) {
		// Since sqlite might fail on insert with ON CONFLICT, use save
		setting := models.Setting{Key: "require_email_verification", Value: "true"}
		db.Save(&setting)

		user := models.User{Email: "unverified@example.com", PasswordHash: "$2a$10$notarealhashbutnonempty0000000000000000000000000000", TOTPEnabled: true, EmailVerified: false}
		db.Create(&user)
		db.Create(&models.OrgMember{UserID: user.ID, OrgID: 1})

		req := httptest.NewRequest(http.MethodPost, "/api/auth/2fa/verify", nil)
		req.RemoteAddr = "127.0.0.1:1239"

		token, _ := h.auth.NewTwoFAChallenge(user.ID)
		req.AddCookie(&http.Cookie{Name: "octarq_2fa_challenge", Value: token})

		rec := httptest.NewRecorder()
		humaCtx := humago.NewContext(nil, req, rec)

		_, err := h.verify2FA(ctx, &Verify2FAInput{
			Ctx: humaCtx,
			Body: Verify2FAInputBody{
				Code: "123456",
			},
		})
		if err == nil || !strings.Contains(err.Error(), "email verification required") {
			t.Errorf("expected email verification required error, got %v", err)
		}

		setting.Value = "false"
		db.Save(&setting)
	})

	t.Run("Email Verification Required - Instance admin auto-verify", func(t *testing.T) {
		setting := models.Setting{Key: "require_email_verification", Value: "true"}
		db.Save(&setting)

		user := models.User{Email: "admin2@example.com", PasswordHash: "x", TOTPEnabled: true, EmailVerified: false, IsInstanceAdmin: true}
		db.Create(&user)
		db.Create(&models.OrgMember{UserID: user.ID, OrgID: 1})

		req := httptest.NewRequest(http.MethodPost, "/api/auth/2fa/verify", nil)
		req.RemoteAddr = "127.0.0.1:1240"
		token, _ := h.auth.NewTwoFAChallenge(user.ID)
		req.AddCookie(&http.Cookie{Name: "octarq_2fa_challenge", Value: token})

		rec := httptest.NewRecorder()
		humaCtx := humago.NewContext(nil, req, rec)

		_, err := h.verify2FA(ctx, &Verify2FAInput{
			Ctx: humaCtx,
			Body: Verify2FAInputBody{
				Code: "123456",
			},
		})
		if err == nil || !strings.Contains(err.Error(), "invalid 2FA code") {
			t.Errorf("expected invalid 2FA code since we gave dummy code, got %v", err)
		}

		var updatedUser models.User
		db.First(&updatedUser, user.ID)
		if !updatedUser.EmailVerified {
			t.Errorf("expected instance admin to be auto-verified")
		}

		setting.Value = "false"
		db.Save(&setting)
	})
}
func TestVerify2FAResolve(t *testing.T) {
	input := Verify2FAInput{}
	errs := input.Resolve(nil) // It just sets the ctx
	if len(errs) != 0 {
		t.Errorf("expected 0 errors, got %d", len(errs))
	}
}
