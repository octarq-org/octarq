package api

import (
	"context"
	"fmt"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/models"
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
	if role := h.callerOrgRole(r); role != "owner" && role != "admin" {
		return nil, huma.Error403Forbidden("forbidden: only owner/admin can export org data")
	}
	org := h.orgID(r)

	var (
		tokens   []models.Token
		channels []models.NotificationChannel
	)
	h.db.Where("owner_id = ?", org).Find(&tokens)
	h.db.Where("owner_id = ?", org).Find(&channels)

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

	bodyMap := map[string]any{
		"exportedAt":           time.Now().UTC().Format(time.RFC3339),
		"orgId":                org,
		"apiTokens":            tokens, // hashes excluded (json:"-")
		"notificationChannels": chOut,  // configs redacted
		"note":                 "Secret material (token hashes, encrypted credentials, SMTP passwords, channel configs) is intentionally excluded.",
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
	if role := h.callerOrgRole(r); role != "owner" {
		return nil, huma.Error403Forbidden("forbidden: only an owner can destroy org data")
	}
	if input.Body.Confirm != "DELETE MY DATA" {
		return nil, huma.Error400BadRequest(`confirmation required: send {"confirm":"DELETE MY DATA"}`)
	}
	org := h.orgID(r)

	for _, p := range h.plugins {
		svcName := p.Name() + ".purge"
		if v, ok := h.LookupService(svcName); ok {
			if fn, ok := v.(func(orgID uint) error); ok {
				_ = fn(org)
			}
		}
	}

	for _, m := range []any{
		&models.Token{}, &models.NotificationChannel{},
	} {
		h.db.Where("owner_id = ?", org).Delete(m)
	}

	h.audit(r, "account.purge", "org", org, nil)
	out := &PurgeAccountOutput{}
	out.Body.OK = true
	return out, nil
}
