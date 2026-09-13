package api

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/models"
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

// changePassword replaces the caller's own password, given the current one.
// POST /api/auth/password
//
// This is the authenticated counterpart to the emailed reset flow: the same
// 8-character floor, and the same session cleanup, except the caller's own
// session survives so changing a password doesn't sign you out of the page you
// changed it on. Every OTHER session dies — that is the point of the endpoint
// as much as the new hash is, since "someone else is logged in as me" is the
// reason people change a password in a hurry.
//
// What actually revokes access is deleting the session rows and their cache
// entries — there is no epoch column to bump, and resetPassword neither
// bumps one nor needs to: deletion is the whole mechanism. Doing only the
// thing that works beats doing two things and leaving the next reader unsure
// which one matters.
func (h *Handler) changePassword(ctx context.Context, input *ChangePasswordInput) (*ChangePasswordOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	newPassword := input.Body.NewPassword
	if len(newPassword) < 8 {
		return nil, huma.Error400BadRequest("password must be at least 8 characters")
	}

	uid := h.auth.UserID(r)
	var user models.User
	if err := h.db.First(&user, uid).Error; err != nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	// An empty hash means this account has no password of its own to replace:
	// it signs in through an OAuth provider, or it is the env-configured
	// bootstrap admin whose credentials live in OCTARQ_ADMIN_PASSWORD. Writing a
	// hash for either would be worse than refusing — the bootstrap admin would
	// still authenticate with the env password (h.auth.Check runs first), so the
	// change would appear to work and change nothing.
	if user.PasswordHash == "" {
		return nil, huma.Error400BadRequest("this account signs in without a stored password; use the password reset flow or your identity provider")
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.Body.CurrentPassword)) != nil {
		return nil, huma.Error400BadRequest("current password is incorrect")
	}

	pwHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to hash password")
	}
	// Clear any outstanding reset token along with the password. Changing your
	// password from inside the app is the natural response to "I got a reset
	// email I didn't ask for", and it has to actually end that link: the token
	// stays valid for its full hour otherwise, and redeeming it overwrites the
	// password just chosen and deletes every session — so the defensive action
	// hands the account over instead of protecting it.
	if err := h.db.Model(&user).Updates(map[string]any{
		"password_hash":      string(pwHash),
		"reset_token_hash":   "",
		"reset_token_expiry": nil,
	}).Error; err != nil {
		return nil, huma.Error500InternalServerError("failed to update password")
	}

	// Revoke every session except the one making this request.
	currentID := h.auth.SessionID(r)
	var sessions []models.Session
	h.db.Where("user_id = ? AND id <> ?", uid, currentID).Find(&sessions)
	reqCtx := r.Context()
	for _, s := range sessions {
		_ = h.auth.Cache().Delete(reqCtx, "session:"+s.Token)
	}
	h.db.Where("user_id = ? AND id <> ?", uid, currentID).Delete(&models.Session{})

	h.audit(r, "user.change_password", "user", user.ID, map[string]any{"sessionsRevoked": len(sessions)})

	out := &ChangePasswordOutput{}
	out.Body.OK = true
	return out, nil
}
