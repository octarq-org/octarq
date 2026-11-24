package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/server/internal/auth"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
	"golang.org/x/crypto/bcrypt"
)

func generateSecureToken() (rawToken, hashHex string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	rawToken = hex.EncodeToString(b)
	hash := sha256.Sum256([]byte(rawToken))
	hashHex = hex.EncodeToString(hash[:])
	return rawToken, hashHex, nil
}

func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func (h *Handler) sendPasswordResetEmail(to, resetURL string) {
	if fn, ok := plugin.LookupServiceAs[plugin.SystemMessagingSender](h.LookupService, plugin.ServiceMessagingSendSystem); ok {
		text := fmt.Sprintf("A password reset was requested for your Octarq account.\n\nSet a new password:\n%s\n\nThis link expires in 1 hour. If you didn't request this, you can ignore this email.", resetURL)
		if err := fn(to, "Reset your Octarq password", "", text); err != nil {
			log.Printf("password reset email to %s failed: %v", to, err)
		}
		return
	}
	log.Printf("password reset email skipped for %s: mail plugin not mounted", to)
}

func (h *Handler) sendAccountExistsEmail(to, signInURL string) {
	if fn, ok := plugin.LookupServiceAs[plugin.SystemMessagingSender](h.LookupService, plugin.ServiceMessagingSendSystem); ok {
		text := fmt.Sprintf("An Octarq account already uses this address.\n\nSign in instead:\n%s\n\nIf you did not try to create an account, you can ignore this email.", signInURL)
		if err := fn(to, "Your Octarq account already exists", "", text); err != nil {
			log.Printf("account-exists email to %s failed: %v", to, err)
		}
		return
	}
	log.Printf("account-exists email skipped for %s: mail plugin not mounted", to)
}

func (h *Handler) mailReady() bool {
	fn, ok := plugin.LookupServiceAs[plugin.MessagingReady](h.LookupService, plugin.ServiceMessagingReady)
	if !ok {
		return false
	}
	return fn()
}

func (h *Handler) sendVerificationEmail(to, verifyURL string) {
	if fn, ok := plugin.LookupServiceAs[plugin.SystemMessagingSender](h.LookupService, plugin.ServiceMessagingSendSystem); ok {
		text := fmt.Sprintf("Confirm your email address for Octarq:\n\n%s\n\nThis link expires in 24 hours. If you didn't create this account, you can ignore this email.", verifyURL)
		if err := fn(to, "Verify your Octarq email", "", text); err != nil {
			log.Printf("verification email to %s failed: %v", to, err)
		}
		return
	}
	log.Printf("verification email skipped for %s: mail plugin not mounted", to)
}

type ForgotPasswordInputBody struct {
	Email string `json:"email"`
}

type ForgotPasswordInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body ForgotPasswordInputBody
}

func (i *ForgotPasswordInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ForgotPasswordOutputBody struct {
	OK bool `json:"ok"`
}

type ForgotPasswordOutput struct {
	Body ForgotPasswordOutputBody
}

func (h *Handler) forgotPassword(ctx context.Context, input *ForgotPasswordInput) (*ForgotPasswordOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	ip := reporterIP(r)
	if !h.recoveryLimiter.allow(ip) {
		return nil, huma.Error429TooManyRequests("Too many requests; wait a few minutes before trying again")
	}
	h.recoveryLimiter.recordFailure(ip)

	email := strings.ToLower(strings.TrimSpace(input.Body.Email))
	out := &ForgotPasswordOutput{}
	out.Body.OK = true

	if email == "" {
		return out, nil
	}

	var user models.User
	if h.db.Where("LOWER(email) = ?", email).First(&user).Error != nil {
		return out, nil
	}

	if user.PasswordHash == "" {
		return out, nil
	}

	rawToken, tokenHash, err := generateSecureToken()
	if err != nil {
		return nil, huma.Error500InternalServerError("Could not generate reset token; retry the request")
	}

	expiry := time.Now().Add(1 * time.Hour)
	if err := h.db.Model(&user).Updates(map[string]any{
		"reset_token_hash":   tokenHash,
		"reset_token_expiry": expiry,
	}).Error; err != nil {
		return nil, huma.Error500InternalServerError("Could not save reset token; retry the request")
	}

	resetURL := fmt.Sprintf("%s/admin/reset?token=%s", h.origin(r), rawToken)
	h.sendPasswordResetEmail(user.Email, resetURL)

	return out, nil
}

type ResetPasswordInputBody struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

type ResetPasswordInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body ResetPasswordInputBody
}

func (i *ResetPasswordInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ResetPasswordOutputBody struct {
	OK bool `json:"ok"`
}

type ResetPasswordOutput struct {
	Body ResetPasswordOutputBody
}

func (h *Handler) resetPassword(ctx context.Context, input *ResetPasswordInput) (*ResetPasswordOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	ip := reporterIP(r)
	if !h.recoveryLimiter.allow(ip) {
		return nil, huma.Error429TooManyRequests("Too many requests; wait a few minutes before trying again")
	}
	h.recoveryLimiter.recordFailure(ip)

	token := strings.TrimSpace(input.Body.Token)
	password := input.Body.Password
	if token == "" {
		return nil, huma.Error400BadRequest("Reset token is required; check the link in your email")
	}
	if len(password) < 8 {
		return nil, huma.Error400BadRequest("Password must be at least 8 characters; choose a longer password")
	}

	hash := hashToken(token)
	var user models.User
	if err := h.db.Where("reset_token_hash = ?", hash).First(&user).Error; err != nil {
		return nil, huma.Error400BadRequest("Reset token is invalid or expired; request a new reset link")
	}

	if user.ResetTokenExpiry == nil || time.Now().After(*user.ResetTokenExpiry) {
		return nil, huma.Error400BadRequest("Reset token is invalid or expired; request a new reset link")
	}

	pwHash, err := bcrypt.GenerateFromPassword([]byte(password), auth.BcryptCost)
	if err != nil {
		return nil, huma.Error500InternalServerError("Could not process password; retry the request")
	}

	if err := h.db.Model(&user).Updates(map[string]any{
		"password_hash":      string(pwHash),
		"reset_token_hash":   "",
		"reset_token_expiry": nil,
	}).Error; err != nil {
		return nil, huma.Error500InternalServerError("Could not update password; retry the request")
	}

	var sessions []models.Session
	h.db.Where("user_id = ?", user.ID).Find(&sessions)
	for _, s := range sessions {
		if err := h.auth.Cache().Delete(ctx, "session:"+s.Token); err != nil {
			slog.Warn("failed to delete session cache on password reset", "token", s.Token, "err", err)
		}
	}
	h.db.Where("user_id = ?", user.ID).Delete(&models.Session{})

	out := &ResetPasswordOutput{}
	out.Body.OK = true
	return out, nil
}

type VerifyEmailInput struct {
	Ctx   huma.Context `hidden:"true"`
	Token string       `query:"token"`
}

func (i *VerifyEmailInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

func (h *Handler) verifyEmail(ctx context.Context, input *VerifyEmailInput) (*struct{}, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, w := humago.Unwrap(input.Ctx)
	token := strings.TrimSpace(input.Token)
	if token == "" {
		http.Redirect(w, r, "/admin/login?error=invalid_token", http.StatusFound)
		return nil, nil
	}

	hash := hashToken(token)
	var user models.User
	if err := h.db.Where("verify_token_hash = ?", hash).First(&user).Error; err != nil {
		http.Redirect(w, r, "/admin/login?error=invalid_token", http.StatusFound)
		return nil, nil
	}

	if user.VerifyTokenExpiry == nil || time.Now().After(*user.VerifyTokenExpiry) {
		http.Redirect(w, r, "/admin/login?error=expired_token", http.StatusFound)
		return nil, nil
	}

	if err := h.db.Model(&user).Updates(map[string]any{
		"email_verified":      true,
		"verify_token_hash":   "",
		"verify_token_expiry": nil,
	}).Error; err != nil {
		http.Redirect(w, r, "/admin/login?error=verification_failed", http.StatusFound)
		return nil, nil
	}

	http.Redirect(w, r, "/admin/login?verified=1", http.StatusFound)
	return nil, nil
}

type ResendVerificationInputBody struct {
	Email string `json:"email"`
}

type ResendVerificationInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body ResendVerificationInputBody
}

func (i *ResendVerificationInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ResendVerificationOutputBody struct {
	OK             bool `json:"ok"`
	MailConfigured bool `json:"mailConfigured"`
}

type ResendVerificationOutput struct {
	Body ResendVerificationOutputBody
}

func (h *Handler) resendVerification(ctx context.Context, input *ResendVerificationInput) (*ResendVerificationOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	ip := reporterIP(r)
	if !h.recoveryLimiter.allow(ip) {
		return nil, huma.Error429TooManyRequests("Too many requests; wait a few minutes before trying again")
	}
	h.recoveryLimiter.recordFailure(ip)

	out := &ResendVerificationOutput{}
	out.Body.OK = true
	out.Body.MailConfigured = h.mailReady()

	email := strings.ToLower(strings.TrimSpace(input.Body.Email))

	if email == "" {
		return out, nil
	}

	var user models.User
	if h.db.Where("LOWER(email) = ?", email).First(&user).Error != nil {
		return out, nil
	}

	if user.EmailVerified {
		h.sendAccountExistsEmail(user.Email, h.origin(r)+"/admin/login")
		return out, nil
	}

	rawToken, tokenHash, err := generateSecureToken()
	if err != nil {
		return nil, huma.Error500InternalServerError("Could not generate verification token; retry the request")
	}

	expiry := time.Now().Add(24 * time.Hour)
	if err := h.db.Model(&user).Updates(map[string]any{
		"verify_token_hash":   tokenHash,
		"verify_token_expiry": expiry,
	}).Error; err != nil {
		return nil, huma.Error500InternalServerError("Could not save verification token; retry the request")
	}

	verifyURL := fmt.Sprintf("%s/api/auth/verify-email?token=%s", h.origin(r), rawToken)
	h.sendVerificationEmail(user.Email, verifyURL)

	return out, nil
}
