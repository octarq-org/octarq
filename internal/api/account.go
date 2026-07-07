package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/octarq-org/led/internal/models"
)

// Data portability (GDPR/CCPA): an operator can export everything their org
// holds as one JSON file, or destroy it. Secret material (token hashes, the
// AES-GCM provider credentials, SMTP passwords) is excluded — those carry a
// json:"-" tag — and notification-channel configs are redacted, since an export
// file shouldn't hand back live bot tokens / webhook URLs.

// exportAccount returns a JSON bundle of every record the active org owns.
// Owner/admin only. GET /api/account/export
func (h *Handler) exportAccount(w http.ResponseWriter, r *http.Request) {
	if role := h.callerOrgRole(r); role != "owner" && role != "admin" {
		writeErr(w, http.StatusForbidden, "forbidden: only owner/admin can export org data")
		return
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

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename=\"led-export-org%d-%s.json\"", org, time.Now().UTC().Format("20060102")))
	writeJSON(w, http.StatusOK, map[string]any{
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
	})
}

// purgeAccount permanently deletes all data the active org owns. Owner only, and
// guarded by a typed confirmation. DELETE /api/account/data
// Body: {"confirm": "DELETE MY DATA"}
func (h *Handler) purgeAccount(w http.ResponseWriter, r *http.Request) {
	if role := h.callerOrgRole(r); role != "owner" {
		writeErr(w, http.StatusForbidden, "forbidden: only an owner can destroy org data")
		return
	}
	var body struct {
		Confirm string `json:"confirm"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Confirm != "DELETE MY DATA" {
		writeErr(w, http.StatusBadRequest, `confirmation required: send {"confirm":"DELETE MY DATA"}`)
		return
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
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
