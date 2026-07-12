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
		links     []models.Link
		domains   []models.Domain
		mailboxes []models.Mailbox
		emails    []models.Email
		tokens    []models.Token
		smtp      []models.SMTPSender
		channels  []models.NotificationChannel
		providers []models.ProviderAccount
	)
	h.db.Where("owner_id = ?", org).Find(&links)
	h.db.Where("owner_id = ?", org).Find(&domains)
	h.db.Where("owner_id = ?", org).Find(&mailboxes)
	mailboxIDs := h.db.Model(&models.Mailbox{}).Select("id").Where("owner_id = ?", org)
	h.db.Where("mailbox_id IN (?)", mailboxIDs).Find(&emails)
	h.db.Where("owner_id = ?", org).Find(&tokens)
	h.db.Where("owner_id = ?", org).Find(&smtp)
	h.db.Where("owner_id = ?", org).Find(&channels)
	h.db.Where("owner_id = ?", org).Find(&providers)

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

	out := &ExportAccountOutput{}
	out.ContentType = "application/json; charset=utf-8"
	out.ContentDisposition = fmt.Sprintf("attachment; filename=\"octarq-export-org%d-%s.json\"", org, time.Now().UTC().Format("20060102"))
	out.Body = map[string]any{
		"exportedAt":           time.Now().UTC().Format(time.RFC3339),
		"orgId":                org,
		"links":                links,
		"domains":              domains,
		"mailboxes":            mailboxes,
		"emails":               emails,
		"apiTokens":            tokens,    // hashes excluded (json:"-")
		"smtpSenders":          smtp,      // passwords excluded (json:"-")
		"providerAccounts":     providers, // credentials excluded (json:"-")
		"notificationChannels": chOut,     // configs redacted
		"note":                 "Secret material (token hashes, encrypted credentials, SMTP passwords, channel configs) is intentionally excluded.",
	}
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

	// Emails are scoped via their mailbox, so delete them before the mailboxes.
	mailboxIDs := h.db.Model(&models.Mailbox{}).Select("id").Where("owner_id = ?", org)
	h.db.Where("mailbox_id IN (?)", mailboxIDs).Delete(&models.Email{})

	// Links carry their click events by link_id.
	linkIDs := h.db.Model(&models.Link{}).Select("id").Where("owner_id = ?", org)
	h.db.Where("link_id IN (?)", linkIDs).Delete(&models.LinkEvent{})

	for _, m := range []any{
		&models.Link{}, &models.Mailbox{}, &models.Domain{}, &models.Token{},
		&models.SMTPSender{}, &models.NotificationChannel{}, &models.ProviderAccount{},
	} {
		h.db.Where("owner_id = ?", org).Delete(m)
	}

	h.audit(r, "account.purge", "org", org, nil)
	out := &PurgeAccountOutput{}
	out.Body.OK = true
	return out, nil
}
