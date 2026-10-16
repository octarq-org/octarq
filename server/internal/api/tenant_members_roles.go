package api

import (
	"context"
	"log"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/server/internal/authz"
	"github.com/octarq-org/octarq/server/internal/eventbus"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

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

	role := authz.Role(strings.TrimSpace(input.Body.Role))
	if role != authz.RoleOwner && role != authz.RoleAdmin && role != authz.RoleMember {
		return nil, huma.Error400BadRequest("Role must be one of: member, admin, owner")
	}

	var target models.OrgMember
	if err := h.db.Where("org_id = ? AND user_id = ?", orgID, input.UserID).First(&target).Error; err != nil {
		return nil, huma.Error404NotFound("Member not found in this workspace; verify the member identifier and try again")
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
		return nil, huma.Error403Forbidden("Workspace owner role required to grant owner permissions")
	}
	if authz.Role(target.Role) == authz.RoleOwner && !authz.AtLeast(authz.Role(callerRole), authz.RoleOwner) {
		return nil, huma.Error403Forbidden("Workspace owner role required to modify an owner's role")
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
			if err := h.auth.Cache().Delete(ctx, "session:"+s.Token); err != nil {
				log.Printf("warning: failed to delete session cache for %s: %v", s.Token, err)
			}
		}
		if err := tx.Where("user_id = ? AND org_id = ?", input.UserID, orgID).Delete(&models.Session{}).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, huma.Error500InternalServerError("Could not update member role; retry the request")
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
		return huma.Error400BadRequest("Cannot demote the last workspace owner; transfer ownership first")
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

	callerUID := h.auth.UserID(r)
	if input.UserID == callerUID {
		return nil, huma.Error400BadRequest("Cannot remove your own account from the workspace; ask another administrator to remove you")
	}

	var target models.OrgMember
	if err := h.db.Where("org_id = ? AND user_id = ?", orgID, input.UserID).First(&target).Error; err != nil {
		return nil, huma.Error404NotFound("Member not found in this workspace; verify the member identifier and try again")
	}
	// Only an owner may remove an owner.
	if target.Role == "owner" && callerRole != "owner" {
		return nil, huma.Error403Forbidden("Workspace owner role required to remove an owner")
	}
	// Never strand the workspace ownerless — refuse to remove the last owner.
	if target.Role == "owner" {
		var owners int64
		h.db.Model(&models.OrgMember{}).Where("org_id = ? AND role = ?", orgID, "owner").Count(&owners)
		if owners <= 1 {
			return nil, huma.Error400BadRequest("Cannot remove the last workspace owner; transfer ownership first")
		}
	}

	if err := h.db.Where("org_id = ? AND user_id = ?", orgID, input.UserID).
		Delete(&models.OrgMember{}).Error; err != nil {
		return nil, huma.Error500InternalServerError("Could not remove member from workspace; retry the request")
	}
	revoked := h.auth.RevokeUserOrgSessions(input.UserID, orgID)
	tokensRevoked := h.auth.RevokeUserOrgTokens(input.UserID, orgID)
	plugin.NotifyMemberRemoved(orgID, input.UserID)
	h.audit(r, "member.remove", "user", input.UserID, map[string]any{
		"actor":           h.auth.UserID(r),
		"target":          input.UserID,
		"oldRole":         target.Role,
		"newRole":         "none",
		"role":            target.Role,
		"sessionsRevoked": revoked,
		"tokensRevoked":   tokensRevoked,
	})
	eventbus.Publish(orgID, "member.remove", map[string]any{"userId": input.UserID, "role": target.Role})
	out := &RemoveOrgMemberOutput{}
	out.Body.OK = true
	return out, nil
}
