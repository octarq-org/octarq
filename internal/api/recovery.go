package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/models"
	"golang.org/x/crypto/bcrypt"
)

// generateSecureToken creates a 32-byte (64-character hex) random token
// and its hex-encoded SHA-256 hash for database storage.
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

// hashToken returns the hex-encoded SHA-256 digest of a raw token string.
func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// primaryOrgForUser returns the lowest orgID the user belongs to as a deterministic default for notifications/system emails.
func (h *Handler) primaryOrgForUser(userID uint) uint {
	var orgMember models.OrgMember
	if h.db.Where("user_id = ?", userID).Order("org_id ASC").First(&orgMember).Error == nil {
		return orgMember.OrgID
	}
	return 0
}

// sendPasswordResetEmail best-effort delivers a password reset link to the user.
func (h *Handler) sendPasswordResetEmail(userID uint, to, resetURL string) {
	orgID := h.primaryOrgForUser(userID)
	if sendMail, ok := h.LookupService("mail.send"); ok {
		if fn, ok := sendMail.(func(orgID uint, to, subject, htmlBody, textBody string) error); ok {
			text := fmt.Sprintf("Reset your password for octarq:\n\n%s\n\nThis link expires in 1 hour.", resetURL)
			if err := fn(orgID, to, "Reset your octarq password", "", text); err != nil {
				log.Printf("password reset email to %s failed: %v", to, err)
			}
			return
		}
	}
	log.Printf("password reset email skipped for %s: mail plugin not mounted", to)
}

// sendVerificationEmail best-effort delivers an email verification link to the user.
func (h *Handler) sendVerificationEmail(userID uint, to, verifyURL string) {
	orgID := h.primaryOrgForUser(userID)
	if sendMail, ok := h.LookupService("mail.send"); ok {
		if fn, ok := sendMail.(func(orgID uint, to, subject, htmlBody, textBody string) error); ok {
			text := fmt.Sprintf("Verify your email address for octarq:\n\n%s\n\nThis link expires in 24 hours.", verifyURL)
			if err := fn(orgID, to, "Verify your octarq email", "", text); err != nil {
				log.Printf("verification email to %s failed: %v", to, err)
			}
			return
		}
	}
	log.Printf("verification email skipped for %s: mail plugin not mounted", to)
}

// --- Forgot Password ---

type ForgotPasswordInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body struct {
		Email string `json:"email"`
	}
}

func (i *ForgotPasswordInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ForgotPasswordOutput struct {
	Body struct {
		OK bool `json:"ok"`
	}
}

// POST /api/auth/forgot (public) — requests a password reset link.
// ALWAYS returns 200 {ok: true} to prevent email enumeration.
func (h *Handler) forgotPassword(ctx context.Context, input *ForgotPasswordInput) (*ForgotPasswordOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	ip := reporterIP(r)
	// Own budget, not the login one: every request counts here (there is no
	// "failed" reset), so spending login's 5-per-15min would let anyone behind a
	// shared IP be locked out of logging in by a few reset requests.
	if !h.recoveryLimiter.allow(ip) {
		return nil, huma.Error429TooManyRequests("too many attempts")
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

	rawToken, tokenHash, err := generateSecureToken()
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to generate token")
	}

	expiry := time.Now().Add(1 * time.Hour)
	// Mailing a link whose token never reached the database would send the user
	// to an "invalid or expired token" page with no way to tell why, so fail
	// loudly instead. A 500 here is enumeration-safe: it is reached only after a
	// storage fault, and an unknown address returned 200 long before this point.
	if err := h.db.Model(&user).Updates(map[string]any{
		"reset_token_hash":   tokenHash,
		"reset_token_expiry": expiry,
	}).Error; err != nil {
		return nil, huma.Error500InternalServerError("failed to store reset token")
	}

	resetURL := fmt.Sprintf("%s/admin/reset?token=%s", h.cfg.BaseURL, rawToken)
	h.sendPasswordResetEmail(user.ID, user.Email, resetURL)

	return out, nil
}

// --- Reset Password ---

type ResetPasswordInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
}

func (i *ResetPasswordInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ResetPasswordOutput struct {
	Body struct {
		OK bool `json:"ok"`
	}
}

// POST /api/auth/reset (public) — completes password reset using secret token.
func (h *Handler) resetPassword(ctx context.Context, input *ResetPasswordInput) (*ResetPasswordOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	token := strings.TrimSpace(input.Body.Token)
	password := input.Body.Password
	if token == "" {
		return nil, huma.Error400BadRequest("token is required")
	}
	if len(password) < 8 {
		return nil, huma.Error400BadRequest("password must be at least 8 characters")
	}

	hash := hashToken(token)
	var user models.User
	if err := h.db.Where("reset_token_hash = ?", hash).First(&user).Error; err != nil {
		return nil, huma.Error400BadRequest("invalid or expired token")
	}

	if user.ResetTokenExpiry == nil || time.Now().After(*user.ResetTokenExpiry) {
		return nil, huma.Error400BadRequest("invalid or expired token")
	}

	pwHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to hash password")
	}

	// Update user password & clear reset token. No session_epoch bump: nothing
	// read that column, so it never revoked anything — the session deletion
	// below is what does.
	//
	// Order matters: the sessions must outlive a failed write. Revoking first (or
	// unconditionally) logs the user out of every device while leaving the old
	// password in place, and the ok:true response tells them the opposite.
	if err := h.db.Model(&user).Updates(map[string]any{
		"password_hash":      string(pwHash),
		"reset_token_hash":   "",
		"reset_token_expiry": nil,
	}).Error; err != nil {
		return nil, huma.Error500InternalServerError("failed to update password")
	}

	// Invalidate all active sessions for this user
	var sessions []models.Session
	h.db.Where("user_id = ?", user.ID).Find(&sessions)
	for _, s := range sessions {
		_ = h.auth.Cache().Delete(ctx, "session:"+s.Token)
	}
	h.db.Where("user_id = ?", user.ID).Delete(&models.Session{})

	out := &ResetPasswordOutput{}
	out.Body.OK = true
	return out, nil
}

// --- Verify Email ---

type VerifyEmailInput struct {
	Ctx   huma.Context `hidden:"true"`
	Token string       `query:"token"`
}

func (i *VerifyEmailInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

// GET /api/auth/verify-email (public) — verifies email token and 302 redirects to login.
func (h *Handler) verifyEmail(ctx context.Context, input *VerifyEmailInput) (*struct{}, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
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

	// Claiming verified=1 on a failed write leaves the account unverified with a
	// token that has apparently been spent. Stay in the redirect contract.
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

// --- Resend Verification Email ---

type ResendVerificationInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body struct {
		Email string `json:"email"`
	}
}

func (i *ResendVerificationInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ResendVerificationOutput struct {
	Body struct {
		OK bool `json:"ok"`
	}
}

// POST /api/auth/resend-verification (public) — resends email verification link.
// ALWAYS returns 200 {ok: true}. Rate-limited via loginLimiter.
func (h *Handler) resendVerification(ctx context.Context, input *ResendVerificationInput) (*ResendVerificationOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	ip := reporterIP(r)
	// Own budget, not the login one: every request counts here (there is no
	// "failed" reset), so spending login's 5-per-15min would let anyone behind a
	// shared IP be locked out of logging in by a few reset requests.
	if !h.recoveryLimiter.allow(ip) {
		return nil, huma.Error429TooManyRequests("too many attempts")
	}
	h.recoveryLimiter.recordFailure(ip)

	email := strings.ToLower(strings.TrimSpace(input.Body.Email))
	out := &ResendVerificationOutput{}
	out.Body.OK = true

	if email == "" {
		return out, nil
	}

	var user models.User
	if h.db.Where("LOWER(email) = ?", email).First(&user).Error != nil {
		return out, nil
	}

	if user.EmailVerified {
		return out, nil
	}

	rawToken, tokenHash, err := generateSecureToken()
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to generate token")
	}

	expiry := time.Now().Add(24 * time.Hour)
	// Same reasoning as forgotPassword: an unstored token makes the mailed link
	// dead on arrival. Unknown and already-verified addresses returned 200 above,
	// so this 500 says nothing about the account.
	if err := h.db.Model(&user).Updates(map[string]any{
		"verify_token_hash":   tokenHash,
		"verify_token_expiry": expiry,
	}).Error; err != nil {
		return nil, huma.Error500InternalServerError("failed to store verification token")
	}

	verifyURL := fmt.Sprintf("%s/api/auth/verify-email?token=%s", h.cfg.BaseURL, rawToken)
	h.sendVerificationEmail(user.ID, user.Email, verifyURL)

	return out, nil
}
