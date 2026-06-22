package api

import "net/http"

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
	h.auth.SetSession(w, 1)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "username": body.Username})
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
