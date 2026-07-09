package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/octarq-org/octarq/internal/models"
)

// webhookSecretPlaintext returns the usable signing secret for a stored webhook.
// Secrets are AES-GCM encrypted at rest; older rows may still hold plaintext, so
// a failed decrypt falls back to the raw value for backward compatibility.
func (h *Handler) webhookSecretPlaintext(stored string) string {
	if stored == "" {
		return ""
	}
	if b, err := h.cipher.Decrypt(stored); err == nil {
		return string(b)
	}
	return stored // legacy plaintext row
}

// encryptWebhookSecret seals a plaintext signing secret for storage.
func (h *Handler) encryptWebhookSecret(plaintext string) (string, error) {
	return h.cipher.Encrypt([]byte(plaintext))
}

// decryptedForResponse returns a copy of the hook with its secret decrypted, so
// the dashboard (behind auth) can display/copy the signing secret while the
// value stays encrypted at rest.
func (h *Handler) decryptedForResponse(hook models.Webhook) models.Webhook {
	hook.Secret = h.webhookSecretPlaintext(hook.Secret)
	return hook
}

func (h *Handler) listWebhooks(w http.ResponseWriter, r *http.Request) {
	var hooks []models.Webhook
	h.orgDB(r).Order("created_at DESC").Find(&hooks)
	for i := range hooks {
		hooks[i] = h.decryptedForResponse(hooks[i])
	}
	writeJSON(w, http.StatusOK, hooks)
}

func (h *Handler) createWebhook(w http.ResponseWriter, r *http.Request) {
	var d struct {
		Name    string `json:"name"`
		URL     string `json:"url"`
		Secret  string `json:"secret"` // optional, will auto-generate if empty
		Events  string `json:"events"` // comma-separated, default "*"
		Enabled *bool  `json:"enabled"`
	}
	if err := readJSON(r, &d); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	d.Name = strings.TrimSpace(d.Name)
	d.URL = strings.TrimSpace(d.URL)
	if d.Name == "" || d.URL == "" {
		writeErr(w, http.StatusBadRequest, "name and url are required")
		return
	}

	secret := strings.TrimSpace(d.Secret)
	if secret == "" {
		// Generate random 16-byte hex secret (32 chars)
		b := make([]byte, 16)
		_, _ = rand.Read(b)
		secret = hex.EncodeToString(b)
	}

	events := strings.TrimSpace(d.Events)
	if events == "" {
		events = "*"
	}

	enabled := true
	if d.Enabled != nil {
		enabled = *d.Enabled
	}

	encSecret, err := h.encryptWebhookSecret(secret)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to secure secret")
		return
	}

	hook := models.Webhook{
		OrgID:   h.orgID(r),
		Name:    d.Name,
		URL:     d.URL,
		Secret:  encSecret,
		Events:  events,
		Enabled: enabled,
	}

	if err := h.db.Create(&hook).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to save")
		return
	}

	h.audit(r, "webhook.create", "webhook", hook.ID, map[string]any{"name": hook.Name, "url": hook.URL})
	hook.Secret = secret // return the plaintext secret to the creator
	writeJSON(w, http.StatusCreated, hook)
}

func (h *Handler) updateWebhook(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}

	var hook models.Webhook
	if h.db.Where("id = ? AND owner_id = ?", id, h.orgID(r)).First(&hook).Error != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}

	var d struct {
		Name    string `json:"name"`
		URL     string `json:"url"`
		Secret  string `json:"secret"`
		Events  string `json:"events"`
		Enabled *bool  `json:"enabled"`
	}
	if err := readJSON(r, &d); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}

	hook.Name = strings.TrimSpace(d.Name)
	hook.URL = strings.TrimSpace(d.URL)
	if d.Secret != "" {
		enc, err := h.encryptWebhookSecret(strings.TrimSpace(d.Secret))
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "failed to secure secret")
			return
		}
		hook.Secret = enc
	}
	if d.Events != "" {
		hook.Events = strings.TrimSpace(d.Events)
	}
	if d.Enabled != nil {
		hook.Enabled = *d.Enabled
	}

	if hook.Name == "" || hook.URL == "" {
		writeErr(w, http.StatusBadRequest, "name and url are required")
		return
	}

	h.db.Save(&hook)
	meta := map[string]any{
		"name":    hook.Name,
		"url":     hook.URL,
		"events":  hook.Events,
		"enabled": hook.Enabled,
	}
	if d.Secret != "" {
		meta["secret"] = "[REDACTED]"
	}
	h.audit(r, "webhook.update", "webhook", hook.ID, meta)
	writeJSON(w, http.StatusOK, h.decryptedForResponse(hook))
}

func (h *Handler) deleteWebhook(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}

	if res := h.db.Where("id = ? AND owner_id = ?", id, h.orgID(r)).Delete(&models.Webhook{}); res.RowsAffected == 0 {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}

	h.audit(r, "webhook.delete", "webhook", id, nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
