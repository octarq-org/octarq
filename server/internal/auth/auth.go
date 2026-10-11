// Package auth implements session authentication using a DB-backed session
// token stored in a signed cookie. Each login creates a Session row; the
// cookie carries the row's random Token. Deleting the row revokes access
// immediately — no epoch math or cookie re-signing needed.
//
// API bearer-token authentication (Authorization: Bearer oct_…) is also
// supported and does not use the sessions table.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/octarq-org/octarq/server/config"
	"github.com/octarq-org/octarq/server/internal/cache"
	"github.com/octarq-org/octarq/server/internal/crypto"
	"github.com/octarq-org/octarq/server/internal/csrf"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/origin"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

const (
	cookieName    = "octarq_session"
	sessionTTL    = 7 * 24 * time.Hour
	touchInterval = time.Minute
)

// Manager issues and validates session cookies.
type Manager struct {
	cfg    *config.Config
	cipher *crypto.Cipher
	db     *gorm.DB // nil in stateless/test mode
	cache  cache.Cache
}

// trustProxy gates whether proxy-supplied headers are honoured: the client-IP
// headers when deriving the client IP for rate limiting, and X-Forwarded-Proto
// when deciding whether a request arrived over TLS (which sets the session
// cookie's Secure attribute). Set once from config in New.
var trustProxy atomic.Bool

// New creates a new session Manager.
func New(cfg *config.Config, c *crypto.Cipher) *Manager {
	trustProxy.Store(cfg.TrustProxy)
	return &Manager{cfg: cfg, cipher: c, cache: cache.New("")}
}

// WithDB attaches a database so sessions and API bearer tokens can be
// validated against persistent state.
func (m *Manager) WithDB(db *gorm.DB) *Manager {
	m.db = db
	return m
}

// WithCache attaches an optional Cache layer (Redis or Noop fallback) for session retrieval.
func (m *Manager) WithCache(c cache.Cache) *Manager {
	m.cache = c
	return m
}

// Cache returns the attached cache instance.
func (m *Manager) Cache() cache.Cache {
	return m.cache
}

// generateToken returns a random 64-char hex string suitable for use as a
// session token. Panics fail-closed if the OS random source fails.
func generateToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("auth: crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// SetSessionFromRequest creates or refreshes a Session row (capturing IP and User-Agent)
// and sets the session cookie. Logins from the same browser (same IP + User-Agent)
// reuse the existing non-expired session instead of accumulating duplicates.
func (m *Manager) SetSessionFromRequest(r *http.Request, w http.ResponseWriter, uid, orgID uint) {
	ip := reporterIP(r)
	ua := r.Header.Get("User-Agent")
	token := generateToken()

	if m.db != nil {
		now := time.Now()
		expires := now.Add(sessionTTL)

		// Look for an existing non-expired session with the same fingerprint.
		var existing models.Session
		err := m.db.Where("user_id = ? AND ip = ? AND user_agent = ? AND expires_at > ?", uid, ip, ua, now).
			First(&existing).Error
		if err == nil {
			if raw := cookieToken(r); raw != "" && models.HashToken(raw) == existing.Token {
				if updateErr := m.db.Model(&existing).Updates(map[string]any{
					"org_id":       orgID,
					"last_seen_at": now,
					"expires_at":   expires,
				}).Error; updateErr != nil {
					slog.Error("auth: failed to refresh existing session", "user_id", uid, "error", updateErr)
				}
				if delErr := m.cache.Delete(context.Background(), "session:"+existing.Token); delErr != nil {
					slog.Warn("auth: cache eviction failed", "error", delErr)
				}
				m.setCookie(w, r, raw)
				return
			}
			if updateErr := m.db.Model(&existing).Updates(map[string]any{
				"org_id":       orgID,
				"token":        models.HashToken(token),
				"last_seen_at": now,
				"expires_at":   expires,
			}).Error; updateErr != nil {
				slog.Error("auth: failed to rotate existing session", "user_id", uid, "error", updateErr)
			}
			if delErr := m.cache.Delete(context.Background(), "session:"+existing.Token); delErr != nil {
				slog.Warn("auth: cache eviction failed", "error", delErr)
			}
			m.setCookie(w, r, token)
			return
		}

		sess := models.Session{
			UserID:     uid,
			OrgID:      orgID,
			Token:      models.HashToken(token),
			IP:         ip,
			UserAgent:  ua,
			LastSeenAt: now,
			ExpiresAt:  expires,
		}
		if createErr := m.db.Create(&sess).Error; createErr != nil {
			slog.Error("auth: failed to create session", "user_id", uid, "error", createErr)
			return
		}
	}
	m.setCookie(w, r, token)
}

// SetSession creates a minimal session row (no IP/UA) and sets the cookie.
// Prefer SetSessionFromRequest, which also records IP and User-Agent; this form
// exists for callers that have nothing to record (the OAuth callback, tests).
// r is still required: the cookie's Secure attribute is derived from it.
func (m *Manager) SetSession(w http.ResponseWriter, r *http.Request, uid, orgID uint) {
	token := generateToken()
	if m.db != nil {
		now := time.Now()
		sess := models.Session{
			UserID:     uid,
			OrgID:      orgID,
			Token:      models.HashToken(token),
			LastSeenAt: now,
			ExpiresAt:  now.Add(sessionTTL),
		}
		if createErr := m.db.Create(&sess).Error; createErr != nil {
			slog.Error("auth: failed to create session", "user_id", uid, "error", createErr)
			return
		}
	}
	m.setCookie(w, r, token)
}

// setCookie writes the session cookie, marking it Secure exactly when the
// request that is establishing the session arrived over TLS.
func (m *Manager) setCookie(w http.ResponseWriter, r *http.Request, token string) {
	secure := origin.Secure(r, trustProxy.Load())
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})

	// The readable half of the double submit, derived from the same token and
	// marked Secure the same way — the pair has to travel together or writes
	// start failing. See package internal/csrf.
	http.SetCookie(w, csrf.NewCookie(m.cfg.SecretKey, token, secure))
}

// sessionByToken looks up a non-expired Session row by the raw cookie token.
// Only the SHA-256 hash is ever stored or cached, so a DB/cache read cannot
// reveal a usable session token. Returns nil when not found or expired.
func (m *Manager) sessionByToken(token string) *models.Session {
	if m.db == nil || token == "" {
		return nil
	}
	hashed := models.HashToken(token)
	var s models.Session
	ctx := context.Background()
	if m.cache.Get(ctx, "session:"+hashed, &s) {
		// Verify if it is expired in case Redis TTL hasn't kicked in
		if s.ExpiresAt.After(time.Now()) {
			return &s
		}
		if delErr := m.cache.Delete(ctx, "session:"+hashed); delErr != nil {
			slog.Warn("auth: cache delete expired session failed", "error", delErr)
		}
	}

	if err := m.db.Where("token = ? AND expires_at > ?", hashed, time.Now()).First(&s).Error; err != nil {
		return nil
	}

	ttl := time.Until(s.ExpiresAt)
	if ttl > 0 {
		if setErr := m.cache.Set(ctx, "session:"+hashed, &s, ttl); setErr != nil {
			slog.Warn("auth: cache set session failed", "error", setErr)
		}
	}
	return &s
}

// cookieToken reads the raw session token from the request cookie.
func cookieToken(r *http.Request) string {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

// UserID extracts the user ID from the session, checking the request context
// first (set by Require middleware) before hitting the DB.
func (m *Manager) UserID(r *http.Request) uint {
	if id, ok := r.Context().Value(userIDKey).(uint); ok {
		return id
	}
	s := m.sessionByToken(cookieToken(r))
	if s == nil {
		return 0
	}
	return s.UserID
}

// OrgID extracts the org ID from the session.
func (m *Manager) OrgID(r *http.Request) uint {
	if id := plugin.OrgIDFromContext(r.Context()); id != 0 {
		return id
	}
	s := m.sessionByToken(cookieToken(r))
	if s == nil {
		return 0
	}
	return s.OrgID
}

// SessionID extracts the Session row ID from the request context (set by
// Require middleware). Returns 0 if not in context.
func (m *Manager) SessionID(r *http.Request) uint {
	id, _ := r.Context().Value(sessionIDKey).(uint)
	return id
}

// Authed reports whether the request carries a valid, unexpired session.
func (m *Manager) Authed(r *http.Request) bool {
	return m.sessionByToken(cookieToken(r)) != nil
}

// Clear deletes the session row and clears the cookie (single-device logout).
func (m *Manager) Clear(r *http.Request, w http.ResponseWriter) {
	token := cookieToken(r)
	if token != "" {
		hashed := models.HashToken(token)
		if delErr := m.cache.Delete(context.Background(), "session:"+hashed); delErr != nil {
			slog.Warn("auth: cache delete failed on clear", "error", delErr)
		}
		if m.db != nil {
			if delErr := m.db.Where("token = ?", hashed).Delete(&models.Session{}).Error; delErr != nil {
				slog.Error("auth: db session delete failed on clear", "error", delErr)
			}
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: "", Path: "/", HttpOnly: true, MaxAge: -1,
	})
	http.SetCookie(w, csrf.ClearCookie())
}

// RevokeUserOrgSessions deletes every session that binds the given user to the
// given org, evicting each from the cache first so a cached row can't keep
// serving after the DB row is gone. Returns the number of sessions revoked.
func (m *Manager) RevokeUserOrgSessions(userID, orgID uint) int {
	if m.db == nil || userID == 0 || orgID == 0 {
		return 0
	}
	var sessions []models.Session
	if err := m.db.Where("user_id = ? AND org_id = ?", userID, orgID).Find(&sessions).Error; err != nil {
		return 0
	}
	if len(sessions) == 0 {
		return 0
	}
	ctx := context.Background()
	ids := make([]uint, 0, len(sessions))
	for _, s := range sessions {
		if delErr := m.cache.Delete(ctx, "session:"+s.Token); delErr != nil {
			slog.Warn("auth: cache delete failed on revoke", "error", delErr)
		}
		ids = append(ids, s.ID)
	}
	if delErr := m.db.Where("id IN ?", ids).Delete(&models.Session{}).Error; delErr != nil {
		slog.Error("auth: db revoke sessions failed", "error", delErr)
	}
	return len(ids)
}

// TouchSession updates LastSeenAt for the session associated with r, but
// only if it has not been touched within touchInterval to limit write
// amplification. Safe to call asynchronously.
func (m *Manager) TouchSession(r *http.Request) {
	if m.db == nil {
		return
	}
	token := cookieToken(r)
	if token == "" {
		return
	}
	hashed := models.HashToken(token)
	now := time.Now()
	var s models.Session
	if m.db.Where("token = ?", hashed).First(&s).Error != nil {
		return
	}
	if now.Sub(s.LastSeenAt) >= touchInterval {
		if upErr := m.db.Model(&s).Update("last_seen_at", now).Error; upErr != nil {
			slog.Error("auth: touch session update failed", "error", upErr)
		}
		if delErr := m.cache.Delete(context.Background(), "session:"+hashed); delErr != nil {
			slog.Warn("auth: cache delete failed on touch session", "error", delErr)
		}
	}
}

// identify resolves the request's credentials — session cookie first, then API
// bearer token — and returns a context carrying them. Require and
// AuthenticateRequest share it so the two paths cannot drift apart.
func (m *Manager) identify(r *http.Request) (context.Context, bool) {
	// 1. Stateful session cookie.
	if token := cookieToken(r); token != "" {
		if s := m.sessionByToken(token); s != nil {
			ctx := context.WithValue(r.Context(), userIDKey, s.UserID)
			ctx = plugin.WithOrgID(ctx, s.OrgID)
			return context.WithValue(ctx, sessionIDKey, s.ID), true
		}
	}

	// 2. Bearer token (API access, no session row).
	if tok, ok := m.TokenByRequest(r); ok {
		ctx := context.WithValue(r.Context(), userIDKey, tok.UserID)
		ctx = plugin.WithOrgID(ctx, tok.OrgID)
		ctx = context.WithValue(ctx, sessionIDKey, uint(0))
		return m.WithTokenIdentity(ctx, tok), true
	}

	return r.Context(), false
}

// Require is middleware that rejects unauthenticated requests and injects
// UserID, OrgID, and SessionID into the request context.
func (m *Manager) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, ok := m.identify(r)
		if !ok {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// AuthenticateRequest validates the session cookie or bearer token and returns
// a new request with credentials in the context. On failure, returns false.
func (m *Manager) AuthenticateRequest(r *http.Request) (*http.Request, bool) {
	ctx, ok := m.identify(r)
	if !ok {
		return r, false
	}
	return r.WithContext(ctx), true
}
