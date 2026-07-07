package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/octarq-org/led/internal/models"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
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
	ip := reporterIP(r)
	if !h.loginLimiter.allow(ip) {
		writeErr(w, http.StatusTooManyRequests, "too many failed login attempts")
		return
	}

	uid, orgID, ok := h.authenticate(body.Username, body.Password)
	if !ok {
		h.loginLimiter.recordFailure(ip)
		writeErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	// If this operator has TOTP 2FA enabled, defer the session: the client must
	// re-post username+password plus a valid 6-digit code (or recovery code) to
	// /api/auth/2fa/verify. We keep the failed-login limiter armed until that
	// second factor succeeds.
	var user models.User
	if h.db.First(&user, uid).Error == nil && user.TOTPEnabled {
		writeJSON(w, http.StatusOK, map[string]any{"twoFactorRequired": true, "username": body.Username})
		return
	}

	h.loginLimiter.reset(ip)
	h.auth.SetSessionFromRequest(r, w, uid, orgID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "username": body.Username})
}

// verify2FA completes a login that requires a second factor. The client re-sends
// username+password (re-verified here, so the challenge can't be forged) along
// with a TOTP code or a one-time recovery code. On success the session is set.
// POST /api/auth/2fa/verify  {username, password, code}
func (h *Handler) verify2FA(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	ip := reporterIP(r)
	if !h.loginLimiter.allow(ip) {
		writeErr(w, http.StatusTooManyRequests, "too many failed login attempts")
		return
	}
	uid, orgID, ok := h.authenticate(body.Username, body.Password)
	if !ok {
		h.loginLimiter.recordFailure(ip)
		writeErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	var user models.User
	if h.db.First(&user, uid).Error != nil {
		writeErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	// If 2FA isn't actually enabled, treat the password as sufficient.
	if user.TOTPEnabled {
		if !h.verifyTOTPOrRecovery(&user, strings.TrimSpace(body.Code)) {
			h.loginLimiter.recordFailure(ip)
			writeErr(w, http.StatusUnauthorized, "invalid 2FA code")
			return
		}
	}
	h.loginLimiter.reset(ip)
	h.auth.SetSessionFromRequest(r, w, uid, orgID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "username": body.Username})
}

// logoutAll deletes every session row for the caller and clears the cookie.
// POST /api/auth/logout-all
func (h *Handler) logoutAll(w http.ResponseWriter, r *http.Request) {
	uid := h.auth.UserID(r)
	if uid == 0 {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var sessions []models.Session
	h.db.Where("user_id = ?", uid).Find(&sessions)
	ctx := r.Context()
	for _, s := range sessions {
		_ = h.auth.Cache().Delete(ctx, "session:"+s.Token)
	}
	h.db.Where("user_id = ?", uid).Delete(&models.Session{})
	h.auth.Clear(r, w)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// bootstrapUserID finds or creates the user for the admin login.
func (h *Handler) bootstrapUserID(username string, orgID uint) uint {
	var user models.User
	if err := h.db.Where("email = ?", username).First(&user).Error; err != nil {
		user = models.User{Email: username, PasswordHash: ""}
		h.db.Create(&user)
		// Link to the bootstrap org as owner.
		h.db.Create(&models.OrgMember{OrgID: orgID, UserID: user.ID, Role: "owner"})
	}
	return user.ID
}

// GET /api/auth/sessions — list sessions for the current user, newest first.
// The session matching the caller's cookie is flagged isCurrent: true.
func (h *Handler) listSessions(w http.ResponseWriter, r *http.Request) {
	uid := h.auth.UserID(r)
	if uid == 0 {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var sessions []models.Session
	h.db.Where("user_id = ?", uid).Order("last_seen_at DESC").Limit(20).Find(&sessions)

	currID := h.auth.SessionID(r)
	type row struct {
		ID         uint      `json:"id"`
		IP         string    `json:"ip"` // pre-masked; raw value never leaves server
		UserAgent  string    `json:"userAgent"`
		LastSeenAt time.Time `json:"lastSeenAt"`
		CreatedAt  time.Time `json:"createdAt"`
		IsCurrent  bool      `json:"isCurrent"`
		Location   string    `json:"location,omitempty"`
	}
	out := make([]row, len(sessions))
	for i, s := range sessions {
		ipClean := strings.Trim(s.IP, "[]")
		maskedIP := maskIPServer(ipClean)

		var location string
		if ipClean == "::1" || ipClean == "127.0.0.1" {
			location = "Localhost"
		} else if h.geo != nil {
			country, _, city := h.geo.Locate(ipClean)
			if city != "" && country != "" {
				location = city + ", " + country
			} else if country != "" {
				location = country
			} else if city != "" {
				location = city
			}
		}
		out[i] = row{
			ID:         s.ID,
			IP:         maskedIP,
			UserAgent:  s.UserAgent,
			LastSeenAt: s.LastSeenAt,
			CreatedAt:  s.CreatedAt,
			IsCurrent:  s.ID == currID,
			Location:   location,
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// maskIPServer redacts the last octet/group of an IP (GDPR-style).
// Localhost addresses are left as-is since they carry no PII.
func maskIPServer(ip string) string {
	if ip == "::1" || ip == "127.0.0.1" {
		return ip
	}
	// IPv4
	if parts := strings.Split(ip, "."); len(parts) == 4 {
		return parts[0] + "." + parts[1] + "." + parts[2] + ".*"
	}
	// IPv6
	if idx := strings.LastIndex(ip, ":"); idx >= 0 {
		return ip[:idx] + ":*"
	}
	return ip
}

// DELETE /api/auth/sessions/{id} — revoke a specific session row.
// With stateful cookies, just deleting the row is sufficient: the next
// request from that device will find no matching session and get a 401.
// If the caller revokes their OWN session, the cookie is also cleared.
func (h *Handler) revokeSession(w http.ResponseWriter, r *http.Request) {
	uid := h.auth.UserID(r)
	if uid == 0 {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	idStr := r.PathValue("id")
	var sess models.Session
	if err := h.db.Where("id = ? AND user_id = ?", idStr, uid).First(&sess).Error; err != nil {
		writeErr(w, http.StatusNotFound, "session not found")
		return
	}
	h.db.Delete(&sess)
	_ = h.auth.Cache().Delete(r.Context(), "session:"+sess.Token)

	// If the caller just revoked their own session, clear the cookie too.
	isSelf := h.auth.SessionID(r) == sess.ID
	if isSelf {
		h.auth.Clear(r, w)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "self": isSelf})
}

// bootstrapOrgID returns the ID of the admin's own org, creating it if it
// doesn't exist yet. It looks up by slug (derived from AdminUser) rather than
// taking db.First(), so an OAuth user logging in before the admin password
// login cannot accidentally become the admin's org.
func (h *Handler) bootstrapOrgID() uint {
	slug := safeSlug(h.cfg.AdminUser)
	if slug == "" {
		slug = "default"
	}
	var org models.Org
	if h.db.Where("slug = ?", slug).First(&org).Error == nil {
		return org.ID
	}
	org = models.Org{Name: h.cfg.AdminUser, Slug: slug, InboundToken: uuid.NewString()}
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
	h.auth.Clear(r, w)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	if !h.auth.Authed(r) {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	uid := h.auth.UserID(r)
	var user models.User
	if err := h.db.First(&user, uid).Error; err != nil {
		writeErr(w, http.StatusUnauthorized, "user not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"username": user.Email, "orgId": h.orgID(r)})
}

// POST /api/auth/invite/accept
func (h *Handler) acceptInvite(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	token := strings.TrimSpace(body.Token)
	if token == "" {
		writeErr(w, http.StatusBadRequest, "token is required")
		return
	}
	password := body.Password
	if len(password) < 8 {
		writeErr(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	var user models.User
	if err := h.db.Where("invite_token = ?", token).First(&user).Error; err != nil {
		writeErr(w, http.StatusBadRequest, "invalid token")
		return
	}

	if user.InviteExpiresAt == nil || user.InviteExpiresAt.Before(time.Now()) {
		writeErr(w, http.StatusBadRequest, "invite token has expired")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	user.PasswordHash = string(hash)
	user.InviteToken = ""
	user.InviteExpiresAt = nil

	if err := h.db.Save(&user).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to save user settings")
		return
	}

	h.audit(r, "user.activate", "user", user.ID, map[string]any{"email": user.Email})

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// GET /api/auth/config (public) returns whether Google and GitHub logins are enabled.
func (h *Handler) authConfig(w http.ResponseWriter, r *http.Request) {
	googleEnabled := h.oauth != nil && h.getSetting(keyGoogleClientID) != "" && h.getSetting(keyGoogleClientSecret) != ""
	githubEnabled := h.oauth != nil && h.getSetting(keyGitHubClientID) != "" && h.getSetting(keyGitHubClientSecret) != ""
	writeJSON(w, http.StatusOK, map[string]any{
		"googleEnabled":       googleEnabled,
		"githubEnabled":       githubEnabled,
		"registrationEnabled": h.registrationEnabled(),
		"appName":             h.AppName(),
	})
}

// authenticate resolves username+password to a (userID, orgID) pair. It accepts
// two credential sources, in order:
//  1. the instance admin credential (config-backed) → the admin's bootstrap org
//  2. a database user carrying a bcrypt password hash (invited members and
//     self-serve sign-ups) → the first org they belong to
//
// It returns ok=false when neither matches. Callers arm the failed-login limiter.
func (h *Handler) authenticate(username, password string) (uid, orgID uint, ok bool) {
	if h.auth.Check(username, password) {
		orgID = h.bootstrapOrgID()
		return h.bootstrapUserID(username, orgID), orgID, true
	}

	email := strings.ToLower(strings.TrimSpace(username))
	var user models.User
	if h.db.Where("LOWER(email) = ?", email).First(&user).Error != nil {
		return 0, 0, false
	}
	if user.PasswordHash == "" {
		return 0, 0, false
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return 0, 0, false
	}
	var member models.OrgMember
	if h.db.Where("user_id = ?", user.ID).First(&member).Error != nil {
		return 0, 0, false
	}
	return user.ID, member.OrgID, true
}
