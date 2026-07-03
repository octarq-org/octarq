// Package auth implements session authentication using a DB-backed session
// token stored in a signed cookie. Each login creates a Session row; the
// cookie carries the row's random Token. Deleting the row revokes access
// immediately — no epoch math or cookie re-signing needed.
//
// API bearer-token authentication (Authorization: Bearer led_…) is also
// supported and does not use the sessions table.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/Jungley8/led/config"
	"github.com/Jungley8/led/internal/cache"
	"github.com/Jungley8/led/internal/crypto"
	"github.com/Jungley8/led/internal/models"
	"gorm.io/gorm"
)

type contextKey string

const (
	orgIDKey     contextKey = "org_id"
	userIDKey    contextKey = "user_id"
	sessionIDKey contextKey = "session_id"
)

const (
	cookieName    = "led_session"
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

func New(cfg *config.Config, c *crypto.Cipher) *Manager {
	trustProxy = cfg.TrustProxy
	return &Manager{cfg: cfg, cipher: c, cache: cache.New("")}
}

// trustProxy gates whether proxy-supplied client-IP headers are honoured when
// deriving the client IP for rate limiting. Set once from config in New.
var trustProxy bool

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

// Check verifies admin credentials using constant-time comparison so neither
// the username nor password leaks length/prefix information via timing.
func (m *Manager) Check(user, pass string) bool {
	userOK := subtle.ConstantTimeCompare([]byte(user), []byte(m.cfg.AdminUser)) == 1
	passOK := subtle.ConstantTimeCompare([]byte(pass), []byte(m.cfg.AdminPassword)) == 1
	return userOK && passOK
}

// generateToken returns a random 64-char hex string suitable for use as a
// session token. Panics only if the OS random source fails.
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
			// Refresh the existing session instead of creating a new one.
			m.db.Model(&existing).Updates(map[string]any{
				"last_seen_at": now,
				"expires_at":   expires,
			})
			m.setCookie(w, existing.Token)
			return
		}

		sess := models.Session{
			UserID:     uid,
			OrgID:      orgID,
			Token:      token,
			IP:         ip,
			UserAgent:  ua,
			LastSeenAt: now,
			ExpiresAt:  expires,
		}
		m.db.Create(&sess)
	}
	m.setCookie(w, token)
}

// SetSession creates a minimal session row (no IP/UA) and sets the cookie.
// It accepts the same signature as before so existing call sites (OAuth
// callbacks, tests without a request object) continue to compile.
func (m *Manager) SetSession(w http.ResponseWriter, uid, orgID uint) {
	token := generateToken()
	if m.db != nil {
		now := time.Now()
		sess := models.Session{
			UserID:     uid,
			OrgID:      orgID,
			Token:      token,
			LastSeenAt: now,
			ExpiresAt:  now.Add(sessionTTL),
		}
		m.db.Create(&sess)
	}
	m.setCookie(w, token)
}

func (m *Manager) setCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   m.cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
}

// sessionByToken looks up a non-expired Session row by its token.
// Returns nil when not found or expired. Checks cache first.
func (m *Manager) sessionByToken(token string) *models.Session {
	if m.db == nil || token == "" {
		return nil
	}
	var s models.Session
	ctx := context.Background()
	// Try fetching from cache first
	if m.cache.Get(ctx, "session:"+token, &s) {
		// Verify if it is expired in case Redis TTL hasn't kicked in
		if s.ExpiresAt.After(time.Now()) {
			return &s
		}
		// If expired, clean it up
		_ = m.cache.Delete(ctx, "session:"+token)
	}

	if err := m.db.Where("token = ? AND expires_at > ?", token, time.Now()).First(&s).Error; err != nil {
		return nil
	}

	// Cache the retrieved session
	ttl := time.Until(s.ExpiresAt)
	if ttl > 0 {
		_ = m.cache.Set(ctx, "session:"+token, &s, ttl)
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
	if id, ok := r.Context().Value(orgIDKey).(uint); ok {
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
		_ = m.cache.Delete(context.Background(), "session:"+token)
		if m.db != nil {
			m.db.Where("token = ?", token).Delete(&models.Session{})
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: "", Path: "/", HttpOnly: true, MaxAge: -1,
	})
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
	now := time.Now()
	var s models.Session
	if m.db.Where("token = ?", token).First(&s).Error != nil {
		return
	}
	if now.Sub(s.LastSeenAt) >= touchInterval {
		m.db.Model(&s).Update("last_seen_at", now)
		// Evict from cache to force update on next read
		_ = m.cache.Delete(context.Background(), "session:"+token)
	}
}

// bearerToken extracts a "led_" token from the Authorization header.
func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const pfx = "Bearer "
	if !strings.HasPrefix(h, pfx) {
		return ""
	}
	return strings.TrimSpace(h[len(pfx):])
}

// tokenAuthed reports whether the request carries a valid API bearer token.
// On success it best-effort records LastUsedAt asynchronously.
func (m *Manager) tokenAuthed(r *http.Request) bool {
	if m.db == nil {
		return false
	}
	raw := bearerToken(r)
	if !strings.HasPrefix(raw, "led_") {
		return false
	}
	hash := models.HashToken(raw)
	var tok models.Token
	if m.db.Where("hash = ?", hash).First(&tok).Error != nil {
		return false
	}
	id := tok.ID
	db := m.db
	go func() {
		now := time.Now()
		db.Model(&models.Token{}).Where("id = ?", id).Update("last_used_at", &now)
	}()
	return true
}

// APIAuthed reports whether the request is authenticated by either a valid
// session cookie or a valid API bearer token.
func (m *Manager) APIAuthed(r *http.Request) bool {
	return m.Authed(r) || m.tokenAuthed(r)
}

// Require is middleware that rejects unauthenticated requests and injects
// UserID, OrgID, and SessionID into the request context.
func (m *Manager) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var uid, orgID, sessID uint
		var authed bool

		// 1. Stateful session cookie.
		if token := cookieToken(r); token != "" {
			if s := m.sessionByToken(token); s != nil {
				uid, orgID, sessID, authed = s.UserID, s.OrgID, s.ID, true
			}
		}

		// 2. Bearer token (API access, no session row).
		if !authed && m.db != nil {
			if raw := bearerToken(r); strings.HasPrefix(raw, "led_") {
				hash := models.HashToken(raw)
				var tok models.Token
				if m.db.Where("hash = ?", hash).First(&tok).Error == nil {
					uid, orgID, authed = 0, tok.OrgID, true
					id := tok.ID
					db := m.db
					go func() {
						now := time.Now()
						db.Model(&models.Token{}).Where("id = ?", id).Update("last_used_at", &now)
					}()
				}
			}
		}

		if !authed {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), userIDKey, uid)
		ctx = context.WithValue(ctx, orgIDKey, orgID)
		ctx = context.WithValue(ctx, sessionIDKey, sessID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// reporterIP extracts the best-effort client IP from the request. Proxy
// headers are honoured only when trustProxy is set, otherwise a client could
// spoof X-Forwarded-For to evade the login rate limiter.
func reporterIP(r *http.Request) string {
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.SplitN(xff, ",", 2)
			return strings.TrimSpace(parts[0])
		}
		if xri := r.Header.Get("X-Real-IP"); xri != "" {
			return strings.TrimSpace(xri)
		}
	}
	// RemoteAddr is "host:port" — strip port.
	addr := r.RemoteAddr
	if i := strings.LastIndex(addr, ":"); i > 0 {
		return addr[:i]
	}
	return addr
}
