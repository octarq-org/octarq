package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/octarq-org/led/internal/models"
	"github.com/octarq-org/led/internal/notify"
)

func (h *Handler) listNotificationChannels(w http.ResponseWriter, r *http.Request) {
	var channels []models.NotificationChannel
	h.orgDB(r).Order("created_at DESC").Find(&channels)
	writeJSON(w, http.StatusOK, channels)
}

func (h *Handler) createNotificationChannel(w http.ResponseWriter, r *http.Request) {
	var d models.NotificationChannel
	if err := readJSON(r, &d); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	d.Type = strings.ToLower(strings.TrimSpace(d.Type))
	if d.Name == "" || d.Type == "" {
		writeErr(w, http.StatusBadRequest, "name and type are required")
		return
	}
	d.OrgID = h.orgID(r)
	if err := h.db.Create(&d).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to create")
		return
	}
	h.audit(r, "notification.create", "notification_channel", d.ID, map[string]any{"name": d.Name, "type": d.Type})
	writeJSON(w, http.StatusCreated, d)
}

func (h *Handler) updateNotificationChannel(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var ch models.NotificationChannel
	if h.db.Where("id = ? AND owner_id = ?", id, h.orgID(r)).First(&ch).Error != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	var d struct {
		Name    *string `json:"name"`
		Type    *string `json:"type"`
		Config  *string `json:"config"`
		Enabled *bool   `json:"enabled"`
	}
	if err := readJSON(r, &d); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if d.Name != nil {
		ch.Name = *d.Name
	}
	if d.Type != nil {
		ch.Type = strings.ToLower(strings.TrimSpace(*d.Type))
	}
	if d.Config != nil {
		ch.Config = *d.Config
	}
	if d.Enabled != nil {
		ch.Enabled = *d.Enabled
	h.db.Save(&ch)
	meta := make(map[string]any)
	if d.Name != nil {
		meta["name"] = *d.Name
	}
	if d.Type != nil {
		meta["type"] = *d.Type
	}
	if d.Config != nil {
		meta["config"] = "[REDACTED]"
	}
	if d.Enabled != nil {
		meta["enabled"] = *d.Enabled
	}
	h.audit(r, "notification.update", "notification_channel", ch.ID, meta)
	writeJSON(w, http.StatusOK, ch)
}

func (h *Handler) deleteNotificationChannel(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	if res := h.db.Where("id = ? AND owner_id = ?", id, h.orgID(r)).Delete(&models.NotificationChannel{}); res.RowsAffected == 0 {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	h.audit(r, "notification.delete", "notification_channel", id, nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) testNotificationChannel(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var ch models.NotificationChannel
	if h.db.Where("id = ? AND owner_id = ?", id, h.orgID(r)).First(&ch).Error != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := notify.Send(ctx, ch.Type, ch.Config, "🔔 Test notification from led!"); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
