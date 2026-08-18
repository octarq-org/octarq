package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugin"
)

func TestRecoveryMore(t *testing.T) {
	h, srv, db := newTestHandlerRaw(t)

	// 1. mailReady without service -> false
	if h.mailReady() {
		t.Error("mailReady() = true, want false when ServiceMailReady not provided")
	}

	// mailReady with service -> true
	reg := plugin.NewRegistry()
	reg.Provide(plugin.ServiceMailReady, plugin.MailReady(func() bool { return true }))
	h.SetServiceLookup(reg.Lookup)
	if !h.mailReady() {
		t.Error("mailReady() = false, want true when ServiceMailReady provided")
	}

	// 2. sendPasswordResetEmail and sendVerificationEmail
	h.sendPasswordResetEmail("user@example.com", "https://example.com/reset")
	h.sendVerificationEmail("user@example.com", "https://example.com/verify")

	// 3. forgotPassword empty email & unknown email -> 200
	rec := do(srv, "POST", "/api/auth/forgot", nil, `{"email":""}`)
	if rec.Code != http.StatusOK {
		t.Errorf("forgotPassword empty email: got %d, want 200", rec.Code)
	}

	rec = do(srv, "POST", "/api/auth/forgot", nil, `{"email":"unknown@example.com"}`)
	if rec.Code != http.StatusOK {
		t.Errorf("forgotPassword unknown email: got %d, want 200", rec.Code)
	}

	// 4. resetPassword validation: empty token & short password & expired token
	rec = do(srv, "POST", "/api/auth/reset", nil, `{"token":"","password":"newpassword123"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("resetPassword empty token: got %d, want 400", rec.Code)
	}

	rec = do(srv, "POST", "/api/auth/reset", nil, `{"token":"sometoken","password":"short"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("resetPassword short password: got %d, want 400", rec.Code)
	}

	// Create user with expired reset token
	expiredTime := time.Now().Add(-1 * time.Hour)
	user := models.User{
		Email:            "expired@example.com",
		ResetTokenHash:   hashToken("expiredtoken"),
		ResetTokenExpiry: &expiredTime,
	}
	db.Create(&user)
	rec = do(srv, "POST", "/api/auth/reset", nil, `{"token":"expiredtoken","password":"newpassword123"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("resetPassword expired token: got %d, want 400", rec.Code)
	}

	// 5. verifyEmail empty token & expired token
	req := httptest.NewRequest("GET", "/api/auth/verify-email?token=", nil)
	rec2 := httptest.NewRecorder()
	srv.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusFound || !strings.Contains(rec2.Header().Get("Location"), "invalid_token") {
		t.Errorf("verifyEmail empty token: got %d (%s)", rec2.Code, rec2.Header().Get("Location"))
	}

	userVerify := models.User{
		Email:             "expiredverify@example.com",
		VerifyTokenHash:   hashToken("expiredverifytoken"),
		VerifyTokenExpiry: &expiredTime,
	}
	db.Create(&userVerify)
	req = httptest.NewRequest("GET", "/api/auth/verify-email?token=expiredverifytoken", nil)
	rec2 = httptest.NewRecorder()
	srv.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusFound || !strings.Contains(rec2.Header().Get("Location"), "expired_token") {
		t.Errorf("verifyEmail expired token: got %d (%s)", rec2.Code, rec2.Header().Get("Location"))
	}

	// 6. resendVerification empty email & unknown email & already verified email
	h.recoveryLimiter.reset("192.0.2.1")
	rec = do(srv, "POST", "/api/auth/resend-verification", nil, `{"email":""}`)
	if rec.Code != http.StatusOK {
		t.Errorf("resendVerification empty email: got %d, want 200", rec.Code)
	}

	rec = do(srv, "POST", "/api/auth/resend-verification", nil, `{"email":"unknown@example.com"}`)
	if rec.Code != http.StatusOK {
		t.Errorf("resendVerification unknown email: got %d, want 200", rec.Code)
	}

	verifiedUser := models.User{Email: "verified@example.com", EmailVerified: true}
	db.Create(&verifiedUser)
	rec = do(srv, "POST", "/api/auth/resend-verification", nil, `{"email":"verified@example.com"}`)
	if rec.Code != http.StatusOK {
		t.Errorf("resendVerification already verified email: got %d, want 200", rec.Code)
	}

	// 7. Nil Ctx guards
	ctx := context.Background()
	if _, err := h.forgotPassword(ctx, &ForgotPasswordInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in forgotPassword")
	}
	if _, err := h.resetPassword(ctx, &ResetPasswordInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in resetPassword")
	}
	if _, err := h.verifyEmail(ctx, &VerifyEmailInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in verifyEmail")
	}
	if _, err := h.resendVerification(ctx, &ResendVerificationInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in resendVerification")
	}
}
