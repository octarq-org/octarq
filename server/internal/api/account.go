package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/server/internal/authz"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

type PurgeAccountInputBody struct {
	Confirm string `json:"confirm"`
}

type PurgeAccountInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body PurgeAccountInputBody
}

func (i *PurgeAccountInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type PurgeAccountOutputBody struct {
	OK bool `json:"ok"`
}

type PurgeAccountOutput struct {
	Body PurgeAccountOutputBody
}

func (h *Handler) purgeAccount(ctx context.Context, input *PurgeAccountInput) (*PurgeAccountOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}
	if err := h.requireRole(r, authz.RoleOwner); err != nil {
		return nil, err
	}
	if input.Body.Confirm != "DELETE MY DATA" {
		return nil, huma.Error400BadRequest(`Confirmation required: send {"confirm":"DELETE MY DATA"}`)
	}
	org, err := h.requireOrg(r)
	if err != nil {
		return nil, err
	}

	var purgeErrs []string
	for _, p := range h.plugins {
		if fn, ok := plugin.LookupServiceAs[plugin.PurgeFunc](h.LookupService, plugin.PurgeServiceName(p.Name())); ok {
			if err := fn(org); err != nil {
				purgeErrs = append(purgeErrs, fmt.Sprintf("%s: %v", p.Name(), err))
			}
		}
	}
	if len(purgeErrs) > 0 {
		return nil, huma.Error500InternalServerError("Could not purge plugin data: " + strings.Join(purgeErrs, "; ") + "; retry the request or contact support")
	}

	var members []models.OrgMember
	h.db.Where("org_id = ?", org).Find(&members)
	if h.auth != nil {
		for _, m := range members {
			h.auth.RevokeUserOrgSessions(m.UserID, org)
		}
	}

	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("owner_id = ?", org).Delete(&models.Token{}).Error; err != nil {
			return err
		}
		if err := tx.Where("owner_id = ?", org).Delete(&models.NotificationChannel{}).Error; err != nil {
			return err
		}
		if err := tx.Where("org_id = ?", org).Delete(&models.WorkspaceSetting{}).Error; err != nil {
			return err
		}
		if err := tx.Where("org_id = ?", org).Delete(&models.PluginSetting{}).Error; err != nil {
			return err
		}
		if err := tx.Where("org_id = ?", org).Delete(&models.AuditLog{}).Error; err != nil {
			return err
		}
		if err := tx.Where("owner_id = ?", org).Delete(&models.Webhook{}).Error; err != nil {
			return err
		}
		if err := tx.Where("key LIKE ?", fmt.Sprintf("org_%d.%%", org)).
			Delete(&models.Setting{}).Error; err != nil {
			return err
		}

		var orgRow models.Org
		if err := tx.First(&orgRow, org).Error; err == nil {
			if orgRow.Slug != "" {
				history := models.OrgSlugHistory{
					Slug:      orgRow.Slug,
					OrgID:     org,
					RetiredAt: time.Now(),
				}
				if err := tx.Save(&history).Error; err != nil {
					return err
				}
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		if err := tx.Where("org_id = ?", org).Delete(&models.Session{}).Error; err != nil {
			return err
		}
		if err := tx.Where("org_id = ?", org).Delete(&models.OrgMember{}).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ?", org).Delete(&models.Org{}).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, huma.Error500InternalServerError("Could not purge organization data; retry the request")
	}

	slog.Info("account purged",
		"org", org,
		"actor_user_id", h.auth.UserID(r),
		"request_id", r.Header.Get("X-Request-Id"),
	)

	out := &PurgeAccountOutput{}
	out.Body.OK = true
	return out, nil
}

type DeleteAccountInputBody struct {
	Confirm string `json:"confirm"`
}

type DeleteAccountInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body DeleteAccountInputBody
}

func (i *DeleteAccountInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type DeleteAccountOutputBody struct {
	OK bool `json:"ok"`
}

type DeleteAccountOutput struct {
	Body DeleteAccountOutputBody
}

func (h *Handler) deleteAccount(ctx context.Context, input *DeleteAccountInput) (*DeleteAccountOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}
	uid := h.auth.UserID(r)

	var user models.User
	if err := h.db.First(&user, uid).Error; err != nil {
		return nil, huma.Error404NotFound("User account not found; sign in again or check your account status")
	}
	if input.Body.Confirm != "DELETE MY ACCOUNT" {
		return nil, huma.Error400BadRequest(`Confirmation required: send {"confirm":"DELETE MY ACCOUNT"}`)
	}

	var memberships int64
	h.db.Model(&models.OrgMember{}).Where("user_id = ?", uid).Count(&memberships)
	if memberships != 0 {
		return nil, huma.Error409Conflict("Leave all organizations before deleting your account")
	}

	var sessions []models.Session
	h.db.Where("user_id = ?", uid).Find(&sessions)
	for _, s := range sessions {
		if err := h.auth.Cache().Delete(r.Context(), "session:"+s.Token); err != nil {
			slog.Warn("failed to evict session cache", "token", s.Token, "err", err)
		}
	}

	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", uid).Delete(&models.Session{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", uid).Delete(&models.OrgMember{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", uid).Delete(&models.UserIdentity{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", uid).Delete(&models.UserSetting{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", uid).Delete(&models.Token{}).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ?", uid).Delete(&models.User{}).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, huma.Error500InternalServerError("Could not delete account; retry the request")
	}

	h.audit(r, "account.delete", "user", uid, map[string]any{"userId": uid})
	slog.Info("account deleted",
		"user_id", uid,
		"request_id", r.Header.Get("X-Request-Id"),
	)

	out := &DeleteAccountOutput{}
	out.Body.OK = true
	return out, nil
}
