package api

import (
	"net/http"
	"strings"

	"github.com/Jungley8/led/internal/models"
)

func (h *Handler) listVPS(w http.ResponseWriter, r *http.Request) {
	var list []models.VPS
	h.db.Order("created_at DESC").Find(&list)
	writeJSON(w, http.StatusOK, list)
}

type vpsDTO struct {
	Name     string `json:"name"`
	IP       string `json:"ip"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	SSHKeyID uint   `json:"sshKeyId"`
}

func (h *Handler) createVPS(w http.ResponseWriter, r *http.Request) {
	var d vpsDTO
	if err := readJSON(r, &d); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	d.Name = strings.TrimSpace(d.Name)
	d.IP = strings.TrimSpace(d.IP)
	if d.Name == "" || d.IP == "" {
		writeErr(w, http.StatusBadRequest, "name and ip are required")
		return
	}
	if d.Port <= 0 {
		d.Port = 22
	}
	if d.User == "" {
		d.User = "root"
	}
	var key models.SSHKey
	if err := h.db.First(&key, d.SSHKeyID).Error; err != nil {
		writeErr(w, http.StatusBadRequest, "invalid ssh key")
		return
	}

	vps := models.VPS{
		OwnerID:   models.SingleUserID,
		Name:      d.Name,
		IP:        d.IP,
		Port:      d.Port,
		User:      d.User,
		SSHKeyID:  d.SSHKeyID,
		Status:    "unknown",
		FailCount: 0,
	}

	if err := h.db.Create(&vps).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to create vps")
		return
	}
	writeJSON(w, http.StatusCreated, vps)
}

func (h *Handler) updateVPS(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var vps models.VPS
	if h.db.First(&vps, id).Error != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}

	var d vpsDTO
	if err := readJSON(r, &d); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}

	if d.Name != "" {
		vps.Name = strings.TrimSpace(d.Name)
	}
	if d.IP != "" {
		vps.IP = strings.TrimSpace(d.IP)
	}
	if d.Port > 0 {
		vps.Port = d.Port
	}
	if d.User != "" {
		vps.User = d.User
	}
	if d.SSHKeyID > 0 {
		var key models.SSHKey
		if h.db.First(&key, d.SSHKeyID).Error != nil {
			writeErr(w, http.StatusBadRequest, "invalid ssh key")
			return
		}
		vps.SSHKeyID = d.SSHKeyID
	}

	if err := h.db.Save(&vps).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to update vps")
		return
	}
	writeJSON(w, http.StatusOK, vps)
}

func (h *Handler) deleteVPS(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	h.db.Delete(&models.VPS{}, id)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
