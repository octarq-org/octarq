package api

import (
	"net/http"
	"strings"

	"github.com/Jungley8/led/internal/models"
)

type providerAccountDTO struct {
	Name   string         `json:"name"`
	Type   string         `json:"type"`
	Config map[string]any `json:"config"`
}

func (h *Handler) listProviderAccounts(w http.ResponseWriter, r *http.Request) {
	var accounts []models.ProviderAccount
	h.orgDB(r).Order("created_at DESC").Find(&accounts)
	writeJSON(w, http.StatusOK, accounts)
}

func (h *Handler) createProviderAccount(w http.ResponseWriter, r *http.Request) {
	var d providerAccountDTO
	if err := readJSON(r, &d); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	d.Name = strings.TrimSpace(d.Name)
	d.Type = strings.TrimSpace(d.Type)
	if d.Name == "" || d.Type == "" {
		writeErr(w, http.StatusBadRequest, "name and type are required")
		return
	}
	enc, err := h.encryptConfig(d.Config)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "encrypt config")
		return
	}
	acc := models.ProviderAccount{
		OrgID:  h.orgID(r),
		Name:   d.Name,
		Type:   d.Type,
		Config: enc,
	}
	if err := h.db.Create(&acc).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(r, "provider.create", "provider", acc.ID, map[string]any{"name": acc.Name, "type": acc.Type})
	writeJSON(w, http.StatusCreated, acc)
}

func (h *Handler) updateProviderAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var acc models.ProviderAccount
	if h.db.Where("id = ? AND owner_id = ?", id, h.orgID(r)).First(&acc).Error != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	var d providerAccountDTO
	if err := readJSON(r, &d); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if strings.TrimSpace(d.Name) != "" {
		acc.Name = strings.TrimSpace(d.Name)
	}
	if len(d.Config) > 0 {
		enc, err := h.encryptConfig(d.Config)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "encrypt config")
			return
		}
		acc.Config = enc
	}
	h.db.Save(&acc)
	h.audit(r, "provider.update", "provider", acc.ID, nil)
	writeJSON(w, http.StatusOK, acc)
}

func (h *Handler) deleteProviderAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}

	// Check if any domain is using this account
	var count int64
	h.db.Model(&models.Domain{}).Where("provider_account_id = ?", id).Count(&count)
	if count > 0 {
		writeErr(w, http.StatusConflict, "cannot delete provider account because it is used by one or more domains")
		return
	}

	if res := h.db.Where("id = ? AND owner_id = ?", id, h.orgID(r)).Delete(&models.ProviderAccount{}); res.RowsAffected == 0 {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	h.audit(r, "provider.delete", "provider", id, nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
