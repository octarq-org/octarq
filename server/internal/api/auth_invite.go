package api

import (
	"context"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/server/internal/auth"
	"github.com/octarq-org/octarq/server/internal/eventbus"
	"github.com/octarq-org/octarq/server/internal/models"
	"golang.org/x/crypto/bcrypt"
)

type AcceptInviteInputBody struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

type AcceptInviteInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body AcceptInviteInputBody
}

func (i *AcceptInviteInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type AcceptInviteOutputBody struct {
	OK bool `json:"ok"`
}

type AcceptInviteOutput struct {
	Body AcceptInviteOutputBody
}

// POST /api/auth/invite/accept
func (h *Handler) acceptInvite(ctx context.Context, input *AcceptInviteInput) (*AcceptInviteOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, w := humago.Unwrap(input.Ctx)
	ip := reporterIP(r)
	// Same budget as the password-reset endpoints: this is an unauthenticated
	// "completion" endpoint, so every request counts (there is no "failed"
	// attempt to budget on), and sharing the budget keeps a single limiter.
	if !h.recoveryLimiter.allow(ip) {
		return nil, huma.Error429TooManyRequests("too many attempts")
	}
	h.recoveryLimiter.recordFailure(ip)

	token := strings.TrimSpace(input.Body.Token)
	if token == "" {
		return nil, huma.Error400BadRequest("token is required")
	}
	password := input.Body.Password
	if len(password) < 8 {
		return nil, huma.Error400BadRequest("password must be at least 8 characters")
	}

	var user models.User
	if err := h.db.Where("invite_token_hash = ?", hashToken(token)).First(&user).Error; err != nil {
		return nil, huma.Error400BadRequest("invalid token")
	}

	if user.InviteExpiresAt == nil || user.InviteExpiresAt.Before(time.Now()) {
		return nil, huma.Error400BadRequest("invite token has expired")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), auth.BcryptCost)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to hash password")
	}

	user.PasswordHash = string(hash)
	user.InviteTokenHash = ""
	user.InviteExpiresAt = nil
	// Redeeming a valid, unexpired invite token proves ownership of the account's
	// address: the token is delivered only to that mailbox (see sendInviteEmail),
	// so nothing more than holding it is needed to mark the email verified. This
	// keeps invited teammates from bouncing off the login gate on instances that
	// require verification — the very users the admin just brought in.
	user.EmailVerified = true

	if err := h.db.Save(&user).Error; err != nil {
		return nil, huma.Error500InternalServerError("failed to save user settings")
	}

	h.audit(r, "user.activate", "user", user.ID, map[string]any{"email": user.Email})
	// The invite is now redeemed: the account both joined its workspace(s) and
	// set its password in this one step.
	memberOrgs := h.memberOrgIDs(user.ID)
	for _, oid := range memberOrgs {
		eventbus.Publish(oid, "member.join", map[string]any{"userId": user.ID, "email": user.Email})
		eventbus.Publish(oid, "auth.password_changed", map[string]any{"userId": user.ID, "email": user.Email})
	}

	primaryOrgID := uint(0)
	if len(memberOrgs) > 0 {
		primaryOrgID = memberOrgs[0]
	}
	if w != nil && h.auth != nil {
		h.auth.SetSessionFromRequest(r, w, user.ID, primaryOrgID)
	}

	out := &AcceptInviteOutput{}
	out.Body.OK = true
	return out, nil
}
