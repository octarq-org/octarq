package api

import (
	"context"
	"log/slog"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/server/internal/auth"
	"github.com/octarq-org/octarq/server/internal/models"
	"golang.org/x/crypto/bcrypt"
)

type ChangePasswordInputBody struct {
	CurrentPassword string `json:"currentPassword" doc:"The password the account signs in with today"`
	NewPassword     string `json:"newPassword" doc:"The replacement, at least 8 characters"`
}

type ChangePasswordInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body ChangePasswordInputBody
}

func (i *ChangePasswordInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ChangePasswordOutputBody struct {
	OK bool `json:"ok"`
}

type ChangePasswordOutput struct {
	Body ChangePasswordOutputBody
}

func (h *Handler) changePassword(ctx context.Context, input *ChangePasswordInput) (*ChangePasswordOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}

	newPassword := input.Body.NewPassword
	if len(newPassword) < 8 {
		return nil, huma.Error400BadRequest("Password must be at least 8 characters; choose a longer password")
	}

	uid := h.auth.UserID(r)
	var user models.User
	if err := h.db.First(&user, uid).Error; err != nil {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}

	if user.PasswordHash == "" {
		return nil, huma.Error400BadRequest("Account has no stored password; use the password reset flow or sign in through your identity provider")
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.Body.CurrentPassword)) != nil {
		return nil, huma.Error400BadRequest("Current password is incorrect; check your password and try again")
	}

	pwHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), auth.BcryptCost)
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

	currentID := h.auth.SessionID(r)
	var sessions []models.Session
	h.db.Where("user_id = ? AND id <> ?", uid, currentID).Find(&sessions)
	reqCtx := r.Context()
	for _, s := range sessions {
		if err := h.auth.Cache().Delete(reqCtx, "session:"+s.Token); err != nil {
			slog.Warn("failed to delete session from cache", "token", s.Token, "err", err)
		}
	}
	h.db.Where("user_id = ? AND id <> ?", uid, currentID).Delete(&models.Session{})

	h.audit(r, "user.change_password", "user", user.ID, map[string]any{"sessionsRevoked": len(sessions)})

	out := &ChangePasswordOutput{}
	out.Body.OK = true
	return out, nil
}
