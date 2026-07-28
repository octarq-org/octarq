package api

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/authz"
	"github.com/octarq-org/octarq/internal/models"
	"gorm.io/gorm"
)

// Data portability (GDPR/CCPA): an operator can export everything their org
// holds as one JSON file, or destroy it. Secret material (token hashes, the
// AES-GCM provider credentials, SMTP passwords) is excluded — those carry a
// json:"-" tag — and notification-channel configs are redacted, since an export
// file shouldn't hand back live bot tokens / webhook URLs.

type ExportAccountInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *ExportAccountInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ExportAccountOutput struct {
	ContentDisposition string `header:"Content-Disposition"`
	ContentType        string `header:"Content-Type"`
	Body               map[string]any
}

// exportAccount returns a JSON bundle of every record the active org owns.
// Owner/admin only. GET /api/account/export
func (h *Handler) exportAccount(ctx context.Context, input *ExportAccountInput) (*ExportAccountOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	if err := h.requireRole(r, authz.RoleAdmin); err != nil {
		return nil, err
	}
	org, err := h.requireOrg(r)
	if err != nil {
		return nil, err
	}

	var (
		orgRow            models.Org
		tokens            []models.Token
		channels          []models.NotificationChannel
		workspaceSettings []models.WorkspaceSetting
		pluginSettings    []models.PluginSetting
		auditLogs         []models.AuditLog
		webhooks          []models.Webhook
		abuseReports      []models.AbuseReport
	)
	h.db.Where("id = ?", org).First(&orgRow)
	h.db.Where("owner_id = ?", org).Find(&tokens)
	h.db.Where("owner_id = ?", org).Find(&channels)
	h.db.Where("org_id = ?", org).Find(&workspaceSettings)
	h.db.Where("org_id = ?", org).Find(&pluginSettings)
	h.db.Where("org_id = ?", org).Find(&auditLogs)
	h.db.Where("owner_id = ?", org).Find(&webhooks)
	h.db.Where("owner_id = ?", org).Find(&abuseReports)

	// Redact channel configs (they may hold a bot token / webhook secret).
	type channelOut struct {
		models.NotificationChannel
		Config string `json:"config"`
	}
	chOut := make([]channelOut, len(channels))
	for i, c := range channels {
		c.Config = ""
		chOut[i] = channelOut{NotificationChannel: c, Config: "[redacted]"}
	}

	// Redact webhook secrets.
	type webhookOut struct {
		models.Webhook
		Secret string `json:"secret"`
	}
	whOut := make([]webhookOut, len(webhooks))
	for i, w := range webhooks {
		w.Secret = ""
		whOut[i] = webhookOut{Webhook: w, Secret: "[redacted]"}
	}

	bodyMap := map[string]any{
		"exportedAt":           time.Now().UTC().Format(time.RFC3339),
		"orgId":                org,
		"organization":         orgRow, // InboundToken excluded via json:"-"
		"apiTokens":            tokens, // hashes excluded via json:"-"
		"notificationChannels": chOut,  // configs redacted
		"workspaceSettings":    workspaceSettings,
		"pluginSettings":       pluginSettings,
		"auditLogs":            auditLogs,
		"webhooks":             whOut, // secret redacted
		"abuseReports":         abuseReports,
		"note":                 "Secret material (token hashes, encrypted credentials, SMTP passwords, channel configs, webhook secrets) is intentionally excluded.",
	}

	for _, p := range h.plugins {
		svcName := p.Name() + ".export"
		if v, ok := h.LookupService(svcName); ok {
			if fn, ok := v.(func(orgID uint) map[string]any); ok {
				for k, val := range fn(org) {
					bodyMap[k] = val
				}
			}
		}
	}

	out := &ExportAccountOutput{}
	out.ContentType = "application/json; charset=utf-8"
	out.ContentDisposition = fmt.Sprintf("attachment; filename=\"octarq-export-org%d-%s.json\"", org, time.Now().UTC().Format("20060102"))
	out.Body = bodyMap
	return out, nil
}

type PurgeAccountInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body struct {
		Confirm string `json:"confirm"`
	}
}

func (i *PurgeAccountInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type PurgeAccountOutput struct {
	Body struct {
		OK bool `json:"ok"`
	}
}

// purgeAccount permanently deletes all data the active org owns. Owner only, and
// guarded by a typed confirmation. DELETE /api/account/data
// Body: {"confirm": "DELETE MY DATA"}
func (h *Handler) purgeAccount(ctx context.Context, input *PurgeAccountInput) (*PurgeAccountOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	if err := h.requireRole(r, authz.RoleOwner); err != nil {
		return nil, err
	}
	if input.Body.Confirm != "DELETE MY DATA" {
		return nil, huma.Error400BadRequest(`confirmation required: send {"confirm":"DELETE MY DATA"}`)
	}
	org, err := h.requireOrg(r)
	if err != nil {
		return nil, err
	}

	// 1. Call plugin purge services first
	for _, p := range h.plugins {
		svcName := p.Name() + ".purge"
		if v, ok := h.LookupService(svcName); ok {
			if fn, ok := v.(func(orgID uint) error); ok {
				_ = fn(org)
			}
		}
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
		return nil, huma.Error500InternalServerError("failed to purge account data: " + err.Error())
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
