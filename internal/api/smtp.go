package api

import (
	"net/http"
	"strings"

	"github.com/Jungley8/led/internal/models"
)

func (h *Handler) listSMTPSenders(w http.ResponseWriter, r *http.Request) {
	var senders []models.SMTPSender
	h.orgDB(r).Order("name ASC").Find(&senders)
	writeJSON(w, http.StatusOK, senders)
}

func (h *Handler) createSMTPSender(w http.ResponseWriter, r *http.Request) {
	var d struct {
		Name      string `json:"name"`
		Host      string `json:"host"`
		Port      int    `json:"port"`
		User      string `json:"user"`
		Pass      string `json:"pass"`
		FromEmail string `json:"fromEmail"`
	}
	if err := readJSON(r, &d); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	d.Name = strings.TrimSpace(d.Name)
	if d.Name == "" || d.Host == "" || d.Port == 0 || d.User == "" || d.Pass == "" {
		writeErr(w, http.StatusBadRequest, "name, host, port, user and pass are required")
		return
	}

	encPass, err := h.cipher.Encrypt([]byte(d.Pass))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "encrypt failed")
		return
	}

	sender := models.SMTPSender{
		OrgID:     h.orgID(r),
		Name:      d.Name,
		Host:      strings.TrimSpace(d.Host),
		Port:      d.Port,
		User:      strings.TrimSpace(d.User),
		Pass:      encPass,
		FromEmail: strings.TrimSpace(d.FromEmail),
	}

	if err := h.db.Create(&sender).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to save")
		return
	}
	h.audit(r, "smtp.create", "smtp_sender", sender.ID, map[string]any{"name": sender.Name, "host": sender.Host})
	writeJSON(w, http.StatusCreated, sender)
}

func (h *Handler) updateSMTPSender(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}

	var sender models.SMTPSender
	if h.db.Where("id = ? AND owner_id = ?", id, h.orgID(r)).First(&sender).Error != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}

	var d struct {
		Name      string `json:"name"`
		Host      string `json:"host"`
		Port      int    `json:"port"`
		User      string `json:"user"`
		Pass      string `json:"pass"` // optional on update
		FromEmail string `json:"fromEmail"`
	}
	if err := readJSON(r, &d); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}

	sender.Name = strings.TrimSpace(d.Name)
	sender.Host = strings.TrimSpace(d.Host)
	sender.Port = d.Port
	sender.User = strings.TrimSpace(d.User)
	sender.FromEmail = strings.TrimSpace(d.FromEmail)

	if d.Pass != "" {
		enc, err := h.cipher.Encrypt([]byte(d.Pass))
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "encrypt failed")
			return
		}
		sender.Pass = enc
	}

	h.db.Save(&sender)
	meta := map[string]any{
		"name":      sender.Name,
		"host":      sender.Host,
		"port":      sender.Port,
		"user":      sender.User,
		"fromEmail": sender.FromEmail,
	}
	if d.Pass != "" {
		meta["pass"] = "[REDACTED]"
	}
	h.audit(r, "smtp.update", "smtp_sender", sender.ID, meta)
	writeJSON(w, http.StatusOK, sender)
}

func (h *Handler) deleteSMTPSender(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}

	// Optional: we could check if it's the default sender, but we'll let them delete any.
	if res := h.db.Where("id = ? AND owner_id = ?", id, h.orgID(r)).Delete(&models.SMTPSender{}); res.RowsAffected == 0 {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	h.audit(r, "smtp.delete", "smtp_sender", id, nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
