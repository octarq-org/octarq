package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/server/internal/models"
	"golang.org/x/crypto/bcrypt"
)

type ChangeEmailInputBody struct {
	NewEmail        string `json:"newEmail" doc:"The replacement email address"`
	CurrentPassword string `json:"currentPassword,omitempty" doc:"Current password for verification"`
}

type ChangeEmailInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body ChangeEmailInputBody
}

func (i *ChangeEmailInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ChangeEmailOutputBody struct {
	OK               bool   `json:"ok"`
	Email            string `json:"email"`
	VerificationSent bool   `json:"verificationSent"`
}

type ChangeEmailOutput struct {
	Body ChangeEmailOutputBody
}

// changeEmail updates the authenticated user's email address.
// PUT /api/auth/email
func (h *Handler) changeEmail(ctx context.Context, input *ChangeEmailInput) (*ChangeEmailOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	ip := reporterIP(r)
	if !h.loginLimiter.allow(ip) {
		return nil, huma.Error429TooManyRequests("Too many requests; wait a few minutes before trying again")
	}

	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}

	newEmail := strings.ToLower(strings.TrimSpace(input.Body.NewEmail))
	if addr, err := mail.ParseAddress(newEmail); err != nil || addr.Address != newEmail || !strings.Contains(newEmail, "@") {
		return nil, huma.Error400BadRequest("Valid email address required; enter an address in the form user@example.com")
	}

	uid := h.auth.UserID(r)
	var user models.User
	if err := h.db.First(&user, uid).Error; err != nil {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}

	if strings.EqualFold(user.Email, newEmail) {
		out := &ChangeEmailOutput{}
		out.Body.OK = true
		out.Body.Email = user.Email
		return out, nil
	}

	if user.PasswordHash != "" {
		if input.Body.CurrentPassword == "" || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.Body.CurrentPassword)) != nil {
			h.loginLimiter.recordFailure(ip)
			return nil, huma.Error400BadRequest("Current password is incorrect; check your password and try again")
		}
	} else if user.IsInstanceAdmin {
		if input.Body.CurrentPassword == "" || !h.auth.CheckAdminPassword(input.Body.CurrentPassword) {
			h.loginLimiter.recordFailure(ip)
			return nil, huma.Error400BadRequest("Current password is incorrect; check your password and try again")
		}
	} else {
		return nil, huma.Error400BadRequest("Account managed by an external identity provider; update your email through your provider")
	}

	var existing models.User
	if h.db.Where("LOWER(email) = ? AND id <> ?", newEmail, user.ID).First(&existing).Error == nil {
		return nil, huma.NewError(http.StatusConflict, "An account with this email address already exists; use a different email address")
	}

	oldEmail := user.Email
	verificationSent := false

	updates := map[string]any{
		"email":          newEmail,
		"email_verified": false,
	}

	if h.requireEmailVerification() {
		rawToken, tokenHash, err := generateSecureToken()
		if err != nil {
			return nil, huma.Error500InternalServerError("Could not generate verification token; retry the request")
		}
		expiry := time.Now().Add(24 * time.Hour)
		updates["verify_token_hash"] = tokenHash
		updates["verify_token_expiry"] = expiry

		verifyURL := fmt.Sprintf("%s/api/auth/verify-email?token=%s", h.origin(r), rawToken)
		h.sendVerificationEmail(newEmail, verifyURL)
		verificationSent = true
	} else {
		updates["verify_token_hash"] = ""
		updates["verify_token_expiry"] = nil
	}

	if err := h.db.Model(&user).Updates(updates).Error; err != nil {
		return nil, huma.Error500InternalServerError("Could not update email address; retry the request")
	}

	h.audit(r, "user.update_email", "user", user.ID, map[string]any{
		"oldEmail": oldEmail,
		"newEmail": newEmail,
	})

	// Revoke every session except the one making this request.
	currentID := h.auth.SessionID(r)
	var sessions []models.Session
	h.db.Where("user_id = ? AND id <> ?", uid, currentID).Find(&sessions)
	reqCtx := r.Context()
	for _, s := range sessions {
		if err := h.auth.Cache().Delete(reqCtx, "session:"+s.Token); err != nil {
			slog.Warn("failed to delete session from cache on email change", "token", s.Token, "err", err)
		}
	}
	h.db.Where("user_id = ? AND id <> ?", uid, currentID).Delete(&models.Session{})

	h.loginLimiter.reset(ip)

	out := &ChangeEmailOutput{}
	out.Body.OK = true
	out.Body.Email = newEmail
	out.Body.VerificationSent = verificationSent
	return out, nil
}
