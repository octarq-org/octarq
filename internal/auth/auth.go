// Package auth implements single-user session authentication using a signed
// cookie. The move to multi-user later replaces the credential check and the
// uid baked into the cookie; the cookie mechanism stays the same.
package auth

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Jungley8/led/config"
	"github.com/Jungley8/led/internal/crypto"
	"github.com/Jungley8/led/internal/models"
	"gorm.io/gorm"
)

type contextKey string

const (
	orgIDKey  contextKey = "org_id"
	userIDKey contextKey = "user_id"
)

const cookieName = "led_session"

// Manager issues and validates session cookies, and authenticates API tokens.
type Manager struct {
	cfg    *config.Config
	cipher *crypto.Cipher
	db     *gorm.DB // optional; enables bearer-token auth when set
}

func New(cfg *config.Config, c *crypto.Cipher) *Manager {
	return &Manager{cfg: cfg, cipher: c}
}

// WithDB attaches a database so API requests can authenticate via bearer token.
func (m *Manager) WithDB(db *gorm.DB) *Manager {
	m.db = db
	return m
}

// Check verifies admin credentials.
func (m *Manager) Check(user, pass string) bool {
	return user == m.cfg.AdminUser && pass == m.cfg.AdminPassword
}

// issue builds a signed token "uid:orgid:epoch|exp|sig". The epoch lets a user
// invalidate every outstanding session at once (see logout-all): a cookie whose
// epoch no longer matches the user's current SessionEpoch is rejected.
func (m *Manager) issue(uid, orgID, epoch uint, ttl time.Duration) string {
	exp := time.Now().Add(ttl).Unix()
	payload := fmt.Sprintf("%d:%d:%d|%d", uid, orgID, epoch, exp)
	return payload + "|" + m.cipher.Sign([]byte(payload))
}

// validate returns (uid, orgID, epoch) if the token is well-formed, signed, and
// unexpired. Legacy cookies without an epoch segment ("uid:orgid") are accepted
// with epoch 0 for backward compatibility.
func (m *Manager) validate(tok string) (uid, orgID, epoch uint, ok bool) {
	parts := strings.Split(tok, "|")
	if len(parts) != 3 {
		return 0, 0, 0, false
	}
	payload := parts[0] + "|" + parts[1]
	if !m.cipher.Verify([]byte(payload), parts[2]) {
		return 0, 0, 0, false
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return 0, 0, 0, false
	}
	// parts[0] is "uid:orgid" (legacy) or "uid:orgid:epoch".
	ids := strings.SplitN(parts[0], ":", 3)
	if len(ids) < 2 {
		return 0, 0, 0, false
	}
	u, err := strconv.ParseUint(ids[0], 10, 64)
	if err != nil {
		return 0, 0, 0, false
	}
	o, err := strconv.ParseUint(ids[1], 10, 64)
	if err != nil {
		return 0, 0, 0, false
	}
	var ep uint64
	if len(ids) == 3 {
		if ep, err = strconv.ParseUint(ids[2], 10, 64); err != nil {
			return 0, 0, 0, false
		}
	}
	return uint(u), uint(o), uint(ep), true
}

// userEpoch loads the user's current SessionEpoch. Returns 0 when no DB is
// attached — the stateless, backward-compatible mode. Pluck leaves epoch at 0
// when the user row is absent, so a synthetic uid (or a since-deleted user)
// validates against an epoch-0 token.
func (m *Manager) userEpoch(uid uint) uint {
	if m.db == nil {
		return 0
	}
	var epoch uint
	m.db.Model(&models.User{}).Where("id = ?", uid).Pluck("session_epoch", &epoch)
	return epoch
}

// SetSession writes the session cookie after a successful login, baking in the
// user's current SessionEpoch.
func (m *Manager) SetSession(w http.ResponseWriter, uid, orgID uint) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    m.issue(uid, orgID, m.userEpoch(uid), 7*24*time.Hour),
		Path:     "/",
		HttpOnly: true,
		Secure:   m.cfg.SecureCookies,
		// SameSite=Lax (not Strict) is required so the cookie survives the
		// top-level redirect back from an OAuth provider; it still blocks
		// cross-site state-changing requests, which is the CSRF protection.
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int((7 * 24 * time.Hour).Seconds()),
	})
}

// UserID extracts the user ID from the session cookie, or 0 if not authed.
func (m *Manager) UserID(r *http.Request) uint {
	if id, ok := r.Context().Value(userIDKey).(uint); ok {
		return id
	}
	c, err := r.Cookie(cookieName)
	if err != nil {
		return 0
	}
	uid, _, _, ok := m.validate(c.Value)
	if !ok {
		return 0
	}
	return uid
}

// OrgID extracts the org ID from the session cookie, or 0 if not authed.
func (m *Manager) OrgID(r *http.Request) uint {
	if id, ok := r.Context().Value(orgIDKey).(uint); ok {
		return id
	}
	c, err := r.Cookie(cookieName)
	if err != nil {
		return 0
	}
	_, orgID, _, ok := m.validate(c.Value)
	if !ok {
		return 0
	}
	return orgID
}

// Clear removes the session cookie (logout).
func (m *Manager) Clear(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: "", Path: "/", HttpOnly: true, MaxAge: -1,
	})
}

// Authed reports whether the request carries a valid session. When a DB is
// attached it also enforces the session epoch, so a logged-out-everywhere
// cookie is treated as unauthenticated.
func (m *Manager) Authed(r *http.Request) bool {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return false
	}
	uid, _, epoch, ok := m.validate(c.Value)
	if !ok {
		return false
	}
	return epoch == m.userEpoch(uid)
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

// tokenAuthed reports whether the request carries a valid API token. On success
// it best-effort records LastUsedAt asynchronously.
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

// Require is middleware that 401s API requests lacking a valid session cookie
// or API bearer token, and injects authenticated UserID and OrgID into the request context.
func (m *Manager) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var uid, orgID uint
		var authed bool

		// 1. Session Cookie
		if c, err := r.Cookie(cookieName); err == nil {
			if u, o, epoch, ok := m.validate(c.Value); ok {
				// Enforce the session epoch so "log out everywhere" takes effect.
				// One DB read per authenticated dashboard request is acceptable.
				if epoch == m.userEpoch(u) {
					uid, orgID, authed = u, o, true
				}
			}
		}

		// 2. Bearer Token
		if !authed && m.db != nil {
			if raw := bearerToken(r); strings.HasPrefix(raw, "led_") {
				hash := models.HashToken(raw)
				var tok models.Token
				if m.db.Where("hash = ?", hash).First(&tok).Error == nil {
					uid, orgID, authed = 0, tok.OrgID, true
					// Update LastUsedAt asynchronously
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
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
