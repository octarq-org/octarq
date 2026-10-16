// Package api provides member management handlers.
//
// Invariant: Changing a member's role (promote/demote) or removing a member must
// immediately invalidate all stateful sessions (user_sessions) for that user in
// that org (org_id), preventing stale sessions from continuing to act on previous roles.
package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/server/internal/authz"
	"github.com/octarq-org/octarq/server/internal/eventbus"
	"github.com/octarq-org/octarq/server/internal/models"
	"gorm.io/gorm"
)

type ListOrgMembersInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *ListOrgMembersInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ListOrgMembersOutput struct {
	Body []MemberItem
}

// listOrgMembers lists all members and their roles for the current active organization.
// GET /api/org/members
func (h *Handler) listOrgMembers(ctx context.Context, input *ListOrgMembersInput) (*ListOrgMembersOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}

	orgID, err := h.requireOrg(r)
	if err != nil {
		return nil, err
	}
	items := []MemberItem{}
	type queryResult struct {
		UserID          uint
		Email           string
		Role            string
		InviteTokenHash string
		JoinedAt        time.Time
	}
	var rows []queryResult
	// joined_at is the membership row's own timestamp. Selecting
	// users.created_at here instead — which this did — reported the account's
	// registration date, so one person showed the same "joined" date in every
	// workspace they belong to.
	err = h.db.Table("org_members").
		Select("users.id as user_id, users.email, org_members.role, users.invite_token_hash, org_members.created_at as joined_at").
		Joins("JOIN users ON users.id = org_members.user_id").
		Where("org_members.org_id = ?", orgID).
		Scan(&rows).Error
	if err != nil {
		return nil, huma.Error500InternalServerError("Could not load workspace members; retry the request")
	}
	for _, row := range rows {
		// Pending means an unredeemed invite. An empty password hash alone is
		// NOT pending: the bootstrap instance admin authenticates against the
		// configured env password and never stores a hash.
		isPending := row.InviteTokenHash != ""
		item := MemberItem{
			UserID:  row.UserID,
			Email:   row.Email,
			Role:    row.Role,
			Pending: isPending,
		}
		// Zero means the row predates the column: omit the field rather than
		// serialising the zero time as a join date.
		if !row.JoinedAt.IsZero() {
			t := row.JoinedAt
			item.JoinedAt = &t
		}
		items = append(items, item)
	}
	return &ListOrgMembersOutput{Body: items}, nil
}

type AddOrgMemberInputBody struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

type AddOrgMemberInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body AddOrgMemberInputBody
}

func (i *AddOrgMemberInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type AddOrgMemberOutput struct {
	Body map[string]any
}

// addOrgMember adds or invites a user to the active organization.
// POST /api/org/members  {"email": "user@example.com", "role": "member"}
func (h *Handler) addOrgMember(ctx context.Context, input *AddOrgMemberInput) (*AddOrgMemberOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}

	orgID, err := h.requireOrg(r)
	if err != nil {
		return nil, err
	}
	callerRole := string(h.effectiveRole(r))
	if callerRole != "owner" && callerRole != "admin" {
		return nil, huma.Error403Forbidden("Workspace owner or administrator role required to manage members")
	}

	email := strings.TrimSpace(input.Body.Email)
	role := strings.TrimSpace(input.Body.Role)
	if email == "" {
		return nil, huma.Error400BadRequest("Member email address is required; provide an email address")
	}
	if role != "owner" && role != "admin" && role != "member" {
		role = "member"
	}
	// Only an owner may grant or revoke the owner role — an admin can't mint
	// owners (or promote itself) and thereby take over the workspace.
	if authz.Role(role) == authz.RoleOwner && !authz.AtLeast(authz.Role(callerRole), authz.RoleOwner) {
		return nil, huma.Error403Forbidden("Workspace owner role required to grant owner permissions")
	}

	var user models.User
	var isNew bool
	var rawInviteToken string
	if err := h.db.Where("email = ?", email).First(&user).Error; err != nil {
		isNew = true
		tokenBytes := make([]byte, 24)
		if _, err := rand.Read(tokenBytes); err != nil {
			return nil, huma.Error500InternalServerError("Could not generate invitation token; retry the request")
		}
		rawInviteToken = hex.EncodeToString(tokenBytes)
		expiresAt := time.Now().Add(24 * time.Hour)

		// The raw invite token is a 192-bit secret delivered only to the invited
		// mailbox (sendInviteEmail) and shown once to the operator right here. Only
		// its SHA-256 hash is persisted; the accept endpoint looks up by hash, so a
		// DB read cannot hand out a live invite.
		user = models.User{
			Email:           email,
			PasswordHash:    "",
			InviteTokenHash: hashToken(rawInviteToken),
			InviteExpiresAt: &expiresAt,
		}
		if err := h.db.Create(&user).Error; err != nil {
			return nil, huma.Error500InternalServerError("Could not create user account; retry the request")
		}
	}

	var existing models.OrgMember
	memErr := h.db.Where("org_id = ? AND user_id = ?", orgID, user.ID).First(&existing).Error
	if memErr == nil {
		// Re-grading an existing owner (demote, or re-affirm) is an owner-only act,
		// so an admin can't strip the owner's role out from under them.
		if authz.Role(existing.Role) == authz.RoleOwner && !authz.AtLeast(authz.Role(callerRole), authz.RoleOwner) {
			return nil, huma.Error403Forbidden("Workspace owner role required to modify an owner's role")
		}
		// Re-inviting the sole owner at a lower role demotes them, and this path
		// had no last-owner guard — the workspace could be left with nobody able
		// to grant the role back. Same rule as PATCH and DELETE.
		if authz.Role(existing.Role) == authz.RoleOwner && authz.Role(role) != authz.RoleOwner {
			if err := h.requireAnotherOwner(orgID, user.ID); err != nil {
				return nil, err
			}
		}
		if existing.Role != role {
			if err := h.db.Transaction(func(tx *gorm.DB) error {
				if err := tx.Model(&models.OrgMember{}).
					Where("org_id = ? AND user_id = ?", orgID, user.ID).
					Update("role", role).Error; err != nil {
					return err
				}
				var sessions []models.Session
				if err := tx.Where("user_id = ? AND org_id = ?", user.ID, orgID).Find(&sessions).Error; err != nil {
					return err
				}
				ctx := context.Background()
				for _, s := range sessions {
					if err := h.auth.Cache().Delete(ctx, "session:"+s.Token); err != nil {
						log.Printf("warning: failed to delete session cache for %s: %v", s.Token, err)
					}
				}
				if err := tx.Where("user_id = ? AND org_id = ?", user.ID, orgID).Delete(&models.Session{}).Error; err != nil {
					return err
				}
				return nil
			}); err != nil {
				return nil, huma.Error500InternalServerError("Could not update member role; retry the request")
			}
			h.audit(r, "member.role", "user", user.ID, map[string]any{
				"from":    existing.Role,
				"to":      role,
				"oldRole": existing.Role,
				"newRole": role,
				"actor":   h.auth.UserID(r),
				"target":  user.ID,
			})
			eventbus.Publish(orgID, "member.role", map[string]any{"userId": user.ID, "role": role})
		}
	} else {
		mem := models.OrgMember{OrgID: orgID, UserID: user.ID, Role: role}
		if err := h.db.Create(&mem).Error; err != nil {
			return nil, huma.Error500InternalServerError("Could not add member to workspace; retry the request")
		}
	}

	h.audit(r, "member.add", "user", user.ID, map[string]any{"email": user.Email, "role": role})
	eventbus.Publish(orgID, "member.invite", map[string]any{"userId": user.ID, "email": user.Email, "role": role, "pending": user.InviteTokenHash != ""})

	if isNew {
		// The invite link goes to the invited mailbox and NOWHERE else. It used
		// to come back in this response too, "so the operator can deliver it
		// out-of-band" — but redeeming it sets a password AND marks the address
		// verified (acceptInvite, auth.go), so the response handed the inviter a
		// working credential for an address they do not control.
		acceptURL := "/admin/invite/accept?token=" + rawInviteToken
		if base := h.origin(r); base != "" {
			acceptURL = base + acceptURL
		}
		sent := h.sendInviteEmail(email, acceptURL)
		return &AddOrgMemberOutput{
			Body: map[string]any{
				"ok":        true,
				"emailSent": sent,
			},
		}, nil
	}
	return &AddOrgMemberOutput{
		Body: map[string]any{"ok": true},
	}, nil
}
