package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/server/internal/eventbus"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
)

// sendInviteEmail best-effort delivers the invite accept link to the invited
// address through the instance's system sender, reporting whether it went out.
// It never returns an error: a missing sender or a send failure is logged and
// swallowed so the invite itself still succeeds. The caller surfaces the false
// case as emailSent, because the link is no longer recoverable any other way.
func (h *Handler) sendInviteEmail(to, acceptURL string) bool {
	if fn, ok := plugin.LookupServiceAs[plugin.SystemMailSender](h.LookupService, plugin.ServiceMailSendSystem); ok {
		text := fmt.Sprintf("You've been invited to join a workspace on octarq.\n\n"+
			"Accept your invite and set a password here:\n%s\n\n"+
			"This link expires in 24 hours.", acceptURL)
		if err := fn(to, "You've been invited to octarq", "", text); err != nil {
			log.Printf("invite email to %s failed: %v", to, err)
			return false
		}
		return true
	}
	log.Printf("invite email skipped for %s: mail plugin not mounted", to)
	return false
}

type ResendOrgMemberInviteInput struct {
	UserID uint64       `path:"userId"`
	Ctx    huma.Context `hidden:"true"`
}

func (i *ResendOrgMemberInviteInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ResendOrgMemberInviteOutput struct {
	Body struct {
		OK        bool `json:"ok"`
		EmailSent bool `json:"emailSent"`
	}
}

// resendOrgMemberInvite re-generates an invite token and delivers a new invite
// email to a pending workspace member who hasn't accepted yet.
// POST /api/org/members/{userId}/resend
func (h *Handler) resendOrgMemberInvite(ctx context.Context, input *ResendOrgMemberInviteInput) (*ResendOrgMemberInviteOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	orgID, err := h.requireOrg(r)
	if err != nil {
		return nil, err
	}
	callerRole := string(h.effectiveRole(r))
	if callerRole != "owner" && callerRole != "admin" {
		return nil, huma.Error403Forbidden("forbidden: only owner/admin can manage members")
	}

	// Rate limit check: 30 seconds cooldown per user ID
	if last, ok := h.resendInviteLimits.Load(input.UserID); ok {
		if t, isTime := last.(time.Time); isTime && time.Since(t) < 30*time.Second {
			return nil, huma.Error429TooManyRequests("please wait before resending invitation")
		}
	}

	var mem models.OrgMember
	if err := h.db.Where("org_id = ? AND user_id = ?", orgID, input.UserID).First(&mem).Error; err != nil {
		return nil, huma.Error404NotFound("not a member of this organization")
	}

	var user models.User
	if err := h.db.Where("id = ?", input.UserID).First(&user).Error; err != nil {
		return nil, huma.Error404NotFound("user not found")
	}
	if user.InviteTokenHash == "" {
		return nil, huma.Error400BadRequest("member has already accepted invitation")
	}

	tokenBytes := make([]byte, 24)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, huma.Error500InternalServerError("failed to generate invite token")
	}
	rawInviteToken := hex.EncodeToString(tokenBytes)
	expiresAt := time.Now().Add(24 * time.Hour)

	if err := h.db.Model(&user).Updates(map[string]any{
		"invite_token_hash": hashToken(rawInviteToken),
		"invite_expires_at": &expiresAt,
	}).Error; err != nil {
		return nil, huma.Error500InternalServerError("failed to update invitation")
	}

	h.resendInviteLimits.Store(input.UserID, time.Now())

	acceptURL := "/admin/invite/accept?token=" + rawInviteToken
	if base := h.origin(r); base != "" {
		acceptURL = base + acceptURL
	}
	sent := h.sendInviteEmail(user.Email, acceptURL)

	h.audit(r, "member.invite_resend", "user", uint(input.UserID), map[string]any{
		"actor":     h.auth.UserID(r),
		"target":    input.UserID,
		"email":     user.Email,
		"emailSent": sent,
	})
	eventbus.Publish(orgID, "member.invite_resend", map[string]any{
		"userId":    input.UserID,
		"email":     user.Email,
		"emailSent": sent,
	})

	out := &ResendOrgMemberInviteOutput{}
	out.Body.OK = true
	out.Body.EmailSent = sent
	return out, nil
}
