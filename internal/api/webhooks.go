package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/Jungley8/led/internal/models"
)

func (h *Handler) listWebhooks(w http.ResponseWriter, r *http.Request) {
	var hooks []models.Webhook
	h.orgDB(r).Order("created_at DESC").Find(&hooks)
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

	hook := models.Webhook{
		OrgID:   h.orgID(r),
		Name:    d.Name,
		URL:     d.URL,
		Secret:  secret,
		Events:  events,
		Enabled: enabled,
	}

	if err := h.db.Create(&hook).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to save")
		return
	}

	h.audit(r, "webhook.create", "webhook", hook.ID, map[string]any{"name": hook.Name, "url": hook.URL})
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
		hook.Secret = strings.TrimSpace(d.Secret)
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
	writeJSON(w, http.StatusOK, hook)
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
