// Package auth implements single-user session authentication using a signed
// cookie. The move to multi-user later replaces the credential check and the
// uid baked into the cookie; the cookie mechanism stays the same.
package auth

import (
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

// issue builds a signed token "uid|exp|sig".
func (m *Manager) issue(uid uint, ttl time.Duration) string {
	exp := time.Now().Add(ttl).Unix()
	payload := fmt.Sprintf("%d|%d", uid, exp)
	return payload + "|" + m.cipher.Sign([]byte(payload))
}

// validate returns the uid if the token is well-formed, signed, and unexpired.
func (m *Manager) validate(tok string) (uint, bool) {
	parts := strings.Split(tok, "|")
	if len(parts) != 3 {
		return 0, false
	}
	payload := parts[0] + "|" + parts[1]
	if !m.cipher.Verify([]byte(payload), parts[2]) {
		return 0, false
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return 0, false
	}
	uid, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		return 0, false
	}
	return uint(uid), true
}

// SetSession writes the session cookie after a successful login.
func (m *Manager) SetSession(w http.ResponseWriter, uid uint) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    m.issue(uid, 7*24*time.Hour),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int((7 * 24 * time.Hour).Seconds()),
	})
}

// Clear removes the session cookie (logout).
func (m *Manager) Clear(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: "", Path: "/", HttpOnly: true, MaxAge: -1,
	})
}

// Authed reports whether the request carries a valid session.
func (m *Manager) Authed(r *http.Request) bool {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return false
	}
	_, ok := m.validate(c.Value)
	return ok
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
// or API bearer token.
func (m *Manager) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !m.APIAuthed(r) {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
