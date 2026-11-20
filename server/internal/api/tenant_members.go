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
	err = h.db.Table("org_members").
		Select("users.id as user_id, users.email, org_members.role, users.invite_token_hash, org_members.created_at as joined_at").
		Joins("JOIN users ON users.id = org_members.user_id").
		Where("org_members.org_id = ?", orgID).
		Scan(&rows).Error
	if err != nil {
		return nil, huma.Error500InternalServerError("Could not load workspace members; retry the request")
	}
	for _, row := range rows {
		isPending := row.InviteTokenHash != ""
		item := MemberItem{
			UserID:  row.UserID,
			Email:   row.Email,
			Role:    row.Role,
			Pending: isPending,
		}
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
		if authz.Role(existing.Role) == authz.RoleOwner && !authz.AtLeast(authz.Role(callerRole), authz.RoleOwner) {
			return nil, huma.Error403Forbidden("Workspace owner role required to modify an owner's role")
		}
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
