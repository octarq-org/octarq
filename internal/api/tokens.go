package api

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/Jungley8/led/internal/models"
)

// tokenAlphabet length-independent random token body. We use URL-safe base64
// (without padding) so the token is copy/paste friendly.
func newRawToken() string {
	b := make([]byte, 24) // 24 bytes -> 32 url-safe chars
	rand.Read(b)
	return "led_" + base64.RawURLEncoding.EncodeToString(b)
}

// tokenPrefix is the short, non-secret identifier shown in the list.
func tokenPrefix(raw string) string {
	if len(raw) <= 8 {
		return raw
	}
	return raw[:8]
}

func (h *Handler) listTokens(w http.ResponseWriter, r *http.Request) {
	var toks []models.Token
	h.orgDB(r).Order("created_at DESC").Find(&toks)
	writeJSON(w, http.StatusOK, toks)
}

func (h *Handler) createToken(w http.ResponseWriter, r *http.Request) {
	var d struct {
		Name string `json:"name"`
		Note string `json:"note"`
	}
	if err := readJSON(r, &d); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	d.Name = strings.TrimSpace(d.Name)
	if d.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	raw := newRawToken()
	tok := models.Token{
		OrgID:  h.orgID(r),
		Name:   d.Name,
		Hash:   models.HashToken(raw),
		Prefix: tokenPrefix(raw),
		Note:   d.Note,
	}
	if err := h.db.Create(&tok).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "create token")
		return
	}
	h.audit(r, "token.create", "token", tok.ID, map[string]any{"name": tok.Name, "prefix": tok.Prefix})
	// The raw token is returned ONLY here; it is never stored or shown again.
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":        tok.ID,
		"name":      tok.Name,
		"note":      tok.Note,
		"prefix":    tok.Prefix,
		"createdAt": tok.CreatedAt,
		"token":     raw,
	})
}

func (h *Handler) deleteToken(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	if res := h.db.Where("id = ? AND owner_id = ?", id, h.orgID(r)).Delete(&models.Token{}); res.RowsAffected == 0 {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	h.audit(r, "token.delete", "token", id, nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
