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

// Data portability (GDPR/CCPA): an operator can export everything their org
// holds as one JSON file, or destroy it. Secret material (token hashes, the
// AES-GCM provider credentials, SMTP passwords) is excluded — those carry a
// json:"-" tag — and notification-channel configs are redacted, since an export
// file shouldn't hand back live bot tokens / webhook URLs.

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

// purgeAccount permanently deletes all data the active org owns. Owner only, and
// guarded by a typed confirmation. DELETE /api/account/data
// Body: {"confirm": "DELETE MY DATA"}
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

	// 1. Call plugin purge services first. Plugins that don't provide a purge
	// service are skipped (nothing to erase). If any plugin purge fails, the
	// operation is aborted before any core data is deleted so that tenant
	// data is not partially orphaned.
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

	// 2. Revoke sessions for all members of this org before purging org_members
	var members []models.OrgMember
	h.db.Where("org_id = ?", org).Find(&members)
	if h.auth != nil {
		for _, m := range members {
			h.auth.RevokeUserOrgSessions(m.UserID, org)
		}
	}

	// 3. Delete core tables inside a single transaction
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
		if err := tx.Where("owner_id = ?", org).Delete(&models.AbuseReport{}).Error; err != nil {
			return err
		}
		// Per-workspace rows that plugins keep in the shared `settings` table,
		// namespaced "org_<id>." — downstream billing, ai and finance all store the
		// workspace's payment and LLM credentials there (the key templates live in
		// downstream packages). Nothing else in this transaction reaches them:
		// they are neither WorkspaceSetting nor owned by any plugin table, so a
		// purged workspace left its Stripe secret, webhook secret and LLM keys
		// behind, and a recycled org id would inherit them.
		//
		// LIKE with an escaped prefix, not a wildcard on user input: the id is a
		// uint, but the pattern is still built explicitly so this cannot widen if
		// the key shape ever changes.
		if err := tx.Where("key LIKE ?", fmt.Sprintf("org_%d.%%", org)).
			Delete(&models.Setting{}).Error; err != nil {
			return err
		}

		// Retire the org's active slug before deleting the Org row so that all
		// addresses ever used by this org (current and rename history) are
		// consistently retired to prevent takeover/squatting. OrgID is intentionally
		// retained as the deleted org's ID (not zeroed) so CheckOrgSlugAvailable
		// evaluates history.OrgID == targetOrgID correctly and treats the slug as
		// SlugTaken for any other org or new registration.
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

		// Session, OrgMember, and Org are deleted last
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

	// Deliberately NOT h.audit: that writes an org-scoped AuditLog row, and writing
	// one here would recreate a row — carrying the actor's ID and IP — for the org
	// we were just asked to erase. It is also fire-and-forget, so the write races
	// the delete and the residue appears only sometimes.
	//
	// The operator still gets a durable record, in the log stream, which is not
	// tenant data and survives the erasure it describes.
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

// deleteAccount permanently deletes the caller's own User row and everything
// user-scoped that references it — memberships, linked identities, per-user
// settings, sessions, and API tokens — in one transaction. Self-service only:
// the user ID comes from the authenticated session, never from a parameter, so
// there is no way to address somebody else's account.
//
// Refused while the user still belongs to any org. Leaving workspaces is the
// org members' management flow's job, and silently evicting someone from active
// workspaces here would be destructive. The guard is "any OrgMember row", which
// also covers the sole-owner case (an owner is a member like anyone else, and
// the last-owner / owner-demotion guards in tenant_menu already refuse to leave
// an org ownerless). DELETE /api/account/user
// Body: {"confirm": "DELETE MY ACCOUNT"}
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

	// Self-service deletion must not silently evict the user from active
	// workspaces — they leave orgs through member management first. Any OrgMember
	// row at all (including being a sole owner) blocks the delete.
	var memberships int64
	h.db.Model(&models.OrgMember{}).Where("user_id = ?", uid).Count(&memberships)
	if memberships != 0 {
		return nil, huma.Error409Conflict("Leave all organizations before deleting your account")
	}

	// Evict every session from the cache first — the cache key is the token
	// hash, so a cached row would keep serving after its DB row is gone. Same
	// ordering purgeAccount uses before it drops org-scoped sessions.
	var sessions []models.Session
	h.db.Where("user_id = ?", uid).Find(&sessions)
	for _, s := range sessions {
		if err := h.auth.Cache().Delete(r.Context(), "session:"+s.Token); err != nil {
			slog.Warn("failed to evict session cache", "token", s.Token, "err", err)
		}
	}

	// Delete user-scoped rows in one transaction. OrgMember is swept even though
	// the guard above already guarantees it is empty, so the delete is atomic
	// either way.
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

	// Unlike purgeAccount, the org survives this deletion, so h.audit is safe
	// and appropriate here: it writes the record to the org the session last
	// held, leaving the workspace a durable trace of who left and when.
	h.audit(r, "account.delete", "user", uid, map[string]any{"userId": uid})
	slog.Info("account deleted",
		"user_id", uid,
		"request_id", r.Header.Get("X-Request-Id"),
	)

	out := &DeleteAccountOutput{}
	out.Body.OK = true
	return out, nil
}
