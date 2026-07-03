package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/Jungley8/led/internal/models"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
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

	if !h.auth.Check(body.Username, body.Password) {
		h.loginLimiter.recordFailure(ip)
		writeErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	orgID := h.bootstrapOrgID()
	uid := h.bootstrapUserID(body.Username, orgID)

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
	h.auth.SetSession(w, uid, orgID)
	go h.createSessionRecord(r, uid)
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
	if !h.auth.Check(body.Username, body.Password) {
		h.loginLimiter.recordFailure(ip)
		writeErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	orgID := h.bootstrapOrgID()
	uid := h.bootstrapUserID(body.Username, orgID)

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
	h.auth.SetSession(w, uid, orgID)
	go h.createSessionRecord(r, uid)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "username": body.Username})
}

// logoutAll invalidates every outstanding session for the caller by bumping the
// user's SessionEpoch, then clears the current cookie.
// POST /api/auth/logout-all
func (h *Handler) logoutAll(w http.ResponseWriter, r *http.Request) {
	uid := h.auth.UserID(r)
	if uid == 0 {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := h.db.Model(&models.User{}).Where("id = ?", uid).
		UpdateColumn("session_epoch", gorm.Expr("session_epoch + 1")).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to revoke sessions")
		return
	}
	h.db.Where("user_id = ?", uid).Delete(&models.Session{})
	h.auth.Clear(w)
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

// createSessionRecord inserts a new Session row for a login event.
// Called asynchronously (go h.createSessionRecord) to avoid blocking the
// login response.
func (h *Handler) createSessionRecord(r *http.Request, uid uint) {
	sess := models.Session{
		UserID:     uid,
		IP:         reporterIP(r),
		UserAgent:  r.Header.Get("User-Agent"),
		LastSeenAt: time.Now(),
	}
	h.db.Create(&sess)
}

// touchSession bumps LastSeenAt for the most-recent session of this user,
// but at most once per minute to avoid write spam. Designed to be called
// asynchronously (go h.touchSession(uid)).
func (h *Handler) touchSession(uid uint) {
	if uid == 0 {
		return
	}
	now := time.Now()
	var sess models.Session
	if h.db.Where("user_id = ?", uid).Order("last_seen_at DESC").First(&sess).Error == nil {
		if now.Sub(sess.LastSeenAt) > time.Minute {
			h.db.Model(&sess).Update("last_seen_at", now)
		}
	}
}

// GET /api/auth/sessions — list sessions for the current user, newest first.
func (h *Handler) listSessions(w http.ResponseWriter, r *http.Request) {
	uid := h.auth.UserID(r)
	if uid == 0 {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var sessions []models.Session
	h.db.Where("user_id = ?", uid).Order("last_seen_at DESC").Limit(20).Find(&sessions)
	writeJSON(w, http.StatusOK, sessions)
}

// DELETE /api/auth/sessions/{id} — revoke a session record.
// Because cookies are stateless, revoking any session bumps the epoch
// (invalidating all cookies) and re-issues a fresh cookie for the caller.
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
	// Bump epoch to revoke all outstanding cookies, then re-issue for caller.
	if err := h.db.Model(&models.User{}).Where("id = ?", uid).
		UpdateColumn("session_epoch", gorm.Expr("session_epoch + 1")).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to revoke session")
		return
	}
	// Delete all remaining session records too.
	h.db.Where("user_id = ?", uid).Delete(&models.Session{})
	// Re-issue a fresh session cookie for the current operator.
	orgID := h.auth.OrgID(r)
	h.auth.SetSession(w, uid, orgID)
	// Create a new session record for this re-login.
	go h.createSessionRecord(r, uid)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "reissued": true})
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
	h.auth.Clear(w)
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
