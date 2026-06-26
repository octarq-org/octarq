package api

import (
	"net/http"

	"github.com/Jungley8/led/internal/models"
)

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if !h.auth.Check(body.Username, body.Password) {
		writeErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	orgID := h.bootstrapOrgID()
	h.auth.SetSession(w, 1, orgID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "username": body.Username})
}

// bootstrapOrgID returns the ID of the first org (creating one if none exists).
// This ensures the admin session always has a valid org scope.
func (h *Handler) bootstrapOrgID() uint {
	var org models.Org
	if err := h.db.First(&org).Error; err == nil {
		return org.ID
	}
	slug := safeSlug(h.cfg.AdminUser)
	if slug == "" {
		slug = "default"
	}
	org = models.Org{Name: h.cfg.AdminUser, Slug: slug}
	h.db.Create(&org)
	return org.ID
}

// safeSlug lowercases s and replaces any non-alphanumeric character with "-".
func safeSlug(s string) string {
	b := []byte(s)
	for i, c := range b {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			continue
		}
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		} else {
			b[i] = '-'
		}
	}
	return string(b)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	h.auth.Clear(w)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	if !h.auth.Authed(r) {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"username": h.cfg.AdminUser})
}
