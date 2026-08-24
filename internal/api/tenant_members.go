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
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/authz"
	"github.com/octarq-org/octarq/internal/eventbus"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugin"
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
		return nil, huma.Error500InternalServerError("failed to query members")
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

	email := strings.TrimSpace(input.Body.Email)
	role := strings.TrimSpace(input.Body.Role)
	if email == "" {
		return nil, huma.Error400BadRequest("email is required")
	}
	if role != "owner" && role != "admin" && role != "member" {
		role = "member"
	}
	// Only an owner may grant or revoke the owner role — an admin can't mint
	// owners (or promote itself) and thereby take over the workspace.
	if authz.Role(role) == authz.RoleOwner && !authz.AtLeast(authz.Role(callerRole), authz.RoleOwner) {
		return nil, huma.Error403Forbidden("forbidden: only an owner can grant the owner role")
	}

	var user models.User
	var isNew bool
	var rawInviteToken string
	if err := h.db.Where("email = ?", email).First(&user).Error; err != nil {
		isNew = true
		tokenBytes := make([]byte, 24)
		if _, err := rand.Read(tokenBytes); err != nil {
			return nil, huma.Error500InternalServerError("failed to generate invite token")
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
			return nil, huma.Error500InternalServerError("failed to create user")
		}
	}

	var existing models.OrgMember
	memErr := h.db.Where("org_id = ? AND user_id = ?", orgID, user.ID).First(&existing).Error
	if memErr == nil {
		// Re-grading an existing owner (demote, or re-affirm) is an owner-only act,
		// so an admin can't strip the owner's role out from under them.
		if authz.Role(existing.Role) == authz.RoleOwner && !authz.AtLeast(authz.Role(callerRole), authz.RoleOwner) {
			return nil, huma.Error403Forbidden("forbidden: only an owner can change an owner's role")
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
					_ = h.auth.Cache().Delete(ctx, "session:"+s.Token)
				}
				if err := tx.Where("user_id = ? AND org_id = ?", user.ID, orgID).Delete(&models.Session{}).Error; err != nil {
					return err
				}
				return nil
			}); err != nil {
				return nil, huma.Error500InternalServerError("failed to update member role")
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
			return nil, huma.Error500InternalServerError("failed to add member")
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

type UpdateOrgMemberInputBody struct {
	Role string `json:"role"`
}

type UpdateOrgMemberInput struct {
	Ctx    huma.Context `hidden:"true"`
	UserID uint         `path:"userId"`
	Body   UpdateOrgMemberInputBody
}

func (i *UpdateOrgMemberInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type UpdateOrgMemberOutputBody struct {
	OK bool `json:"ok"`
}

type UpdateOrgMemberOutput struct {
	Body UpdateOrgMemberOutputBody
}

// updateOrgMember re-grades an existing member.
// PATCH /api/org/members/{userId}  {"role": "admin"}
func (h *Handler) updateOrgMember(ctx context.Context, input *UpdateOrgMemberInput) (*UpdateOrgMemberOutput, error) {
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

	role := authz.Role(strings.TrimSpace(input.Body.Role))
	if role != authz.RoleOwner && role != authz.RoleAdmin && role != authz.RoleMember {
		return nil, huma.Error400BadRequest("role must be one of member, admin, owner")
	}

	var target models.OrgMember
	if err := h.db.Where("org_id = ? AND user_id = ?", orgID, input.UserID).First(&target).Error; err != nil {
		return nil, huma.Error404NotFound("not a member of this organization")
	}
	if target.Role == string(role) {
		out := &UpdateOrgMemberOutput{}
		out.Body.OK = true
		return out, nil
	}

	// The same two owner rules addOrgMember enforces: an admin can neither mint
	// an owner (which would be self-promotion by proxy) nor re-grade the one
	// already there.
	if role == authz.RoleOwner && !authz.AtLeast(authz.Role(callerRole), authz.RoleOwner) {
		return nil, huma.Error403Forbidden("forbidden: only an owner can grant the owner role")
	}
	if authz.Role(target.Role) == authz.RoleOwner && !authz.AtLeast(authz.Role(callerRole), authz.RoleOwner) {
		return nil, huma.Error403Forbidden("forbidden: only an owner can change an owner's role")
	}
	// Demoting the last owner strands the workspace: nobody left who can grant
	// the role back, including the person who just gave it up. removeOrgMember
	// refuses the same shape of mistake.
	if authz.Role(target.Role) == authz.RoleOwner {
		if err := h.requireAnotherOwner(orgID, input.UserID); err != nil {
			return nil, err
		}
	}

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.OrgMember{}).
			Where("org_id = ? AND user_id = ?", orgID, input.UserID).
			Update("role", string(role)).Error; err != nil {
			return err
		}
		var sessions []models.Session
		if err := tx.Where("user_id = ? AND org_id = ?", input.UserID, orgID).Find(&sessions).Error; err != nil {
			return err
		}
		ctx := context.Background()
		for _, s := range sessions {
			_ = h.auth.Cache().Delete(ctx, "session:"+s.Token)
		}
		if err := tx.Where("user_id = ? AND org_id = ?", input.UserID, orgID).Delete(&models.Session{}).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, huma.Error500InternalServerError("failed to update member role")
	}

	h.audit(r, "member.role", "user", input.UserID, map[string]any{
		"from":    target.Role,
		"to":      string(role),
		"oldRole": target.Role,
		"newRole": string(role),
		"actor":   h.auth.UserID(r),
		"target":  input.UserID,
	})
	eventbus.Publish(orgID, "member.role", map[string]any{"userId": input.UserID, "role": string(role)})

	out := &UpdateOrgMemberOutput{}
	out.Body.OK = true
	return out, nil
}

// requireAnotherOwner reports whether the workspace would still have an owner
// after userID stops being one.
func (h *Handler) requireAnotherOwner(orgID, userID uint) error {
	var owners int64
	h.db.Model(&models.OrgMember{}).
		Where("org_id = ? AND role = ? AND user_id <> ?", orgID, "owner", userID).
		Count(&owners)
	if owners == 0 {
		return huma.Error400BadRequest("cannot demote the last owner of the workspace")
	}
	return nil
}

type RemoveOrgMemberInput struct {
	Ctx    huma.Context `hidden:"true"`
	UserID uint         `path:"userId"`
}

func (i *RemoveOrgMemberInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type RemoveOrgMemberOutputBody struct {
	OK bool `json:"ok"`
}

type RemoveOrgMemberOutput struct {
	Body RemoveOrgMemberOutputBody
}

// removeOrgMember removes a user from the active organization.
// DELETE /api/org/members/{userId}
func (h *Handler) removeOrgMember(ctx context.Context, input *RemoveOrgMemberInput) (*RemoveOrgMemberOutput, error) {
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

	callerUID := h.auth.UserID(r)
	if input.UserID == callerUID {
		return nil, huma.Error400BadRequest("cannot remove yourself from the workspace")
	}

	var target models.OrgMember
	if err := h.db.Where("org_id = ? AND user_id = ?", orgID, input.UserID).First(&target).Error; err != nil {
		return nil, huma.Error404NotFound("not a member of this organization")
	}
	// Only an owner may remove an owner.
	if target.Role == "owner" && callerRole != "owner" {
		return nil, huma.Error403Forbidden("forbidden: only an owner can remove an owner")
	}
	// Never strand the workspace ownerless — refuse to remove the last owner.
	if target.Role == "owner" {
		var owners int64
		h.db.Model(&models.OrgMember{}).Where("org_id = ? AND role = ?", orgID, "owner").Count(&owners)
		if owners <= 1 {
			return nil, huma.Error400BadRequest("cannot remove the last owner of the workspace")
		}
	}

	if err := h.db.Where("org_id = ? AND user_id = ?", orgID, input.UserID).
		Delete(&models.OrgMember{}).Error; err != nil {
		return nil, huma.Error500InternalServerError("failed to remove member")
	}
	revoked := h.auth.RevokeUserOrgSessions(input.UserID, orgID)
	tokensRevoked := h.auth.RevokeUserOrgTokens(input.UserID, orgID)
	plugin.NotifyMemberRemoved(orgID, input.UserID)
	h.audit(r, "member.remove", "user", input.UserID, map[string]any{
		"role":            target.Role,
		"sessionsRevoked": revoked,
		"tokensRevoked":   tokensRevoked,
	})
	eventbus.Publish(orgID, "member.remove", map[string]any{"userId": input.UserID, "role": target.Role})
	out := &RemoveOrgMemberOutput{}
	out.Body.OK = true
	return out, nil
}
