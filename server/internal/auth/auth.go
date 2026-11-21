package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
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

type Manager struct {
	cfg    *config.Config
	cipher *crypto.Cipher
	db     *gorm.DB
	cache  cache.Cache
}

var trustProxy atomic.Bool

func New(cfg *config.Config, c *crypto.Cipher) *Manager {
	trustProxy.Store(cfg.TrustProxy)
	return &Manager{cfg: cfg, cipher: c, cache: cache.New("")}
}

func (m *Manager) WithDB(db *gorm.DB) *Manager {
	m.db = db
	return m
}

func (m *Manager) WithCache(c cache.Cache) *Manager {
	m.cache = c
	return m
}

func (m *Manager) Cache() cache.Cache {
	return m.cache
}

func generateToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("auth: crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(b)
}

func (m *Manager) SetSessionFromRequest(r *http.Request, w http.ResponseWriter, uid, orgID uint) {
	ip := reporterIP(r)
	ua := r.Header.Get("User-Agent")
	token := generateToken()

	if m.db != nil {
		now := time.Now()
		expires := now.Add(sessionTTL)

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

	http.SetCookie(w, csrf.NewCookie(m.cfg.SecretKey, token, secure))
}

func (m *Manager) sessionByToken(token string) *models.Session {
	if m.db == nil || token == "" {
		return nil
	}
	hashed := models.HashToken(token)
	var s models.Session
	ctx := context.Background()
	if m.cache.Get(ctx, "session:"+hashed, &s) {
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

func cookieToken(r *http.Request) string {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

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

func (m *Manager) PersonalOrgID(r *http.Request) uint {
	if m == nil || m.db == nil || r == nil {
		return 0
	}
	return PersonalOrgID(m.db, m.UserID(r))
}

func PersonalOrgID(db *gorm.DB, userID uint) uint {
	id, err := personalOrgID(db, userID)
	if err != nil {
		return 0
	}
	return id
}

func personalOrgID(db *gorm.DB, userID uint) (uint, error) {
	if db == nil || userID == 0 {
		return 0, nil
	}
	var member models.OrgMember
	err := db.Table("org_members").
		Joins("JOIN orgs ON orgs.id = org_members.org_id").
		Where("org_members.user_id = ? AND orgs.kind = ?", userID, models.OrgKindPersonal).
		Order("org_members.org_id").
		First(&member).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return member.OrgID, nil
}

func (m *Manager) SessionID(r *http.Request) uint {
	id, _ := r.Context().Value(sessionIDKey).(uint)
	return id
}

func (m *Manager) Authed(r *http.Request) bool {
	return m.sessionByToken(cookieToken(r)) != nil
}

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

func (m *Manager) identify(r *http.Request) (context.Context, bool) {
	if token := cookieToken(r); token != "" {
		if s := m.sessionByToken(token); s != nil {
			ctx := WithUserID(r.Context(), s.UserID)
			ctx = plugin.WithOrgID(ctx, s.OrgID)
			return context.WithValue(ctx, sessionIDKey, s.ID), true
		}
	}

	if tok, ok := m.TokenByRequest(r); ok {
		ctx := WithUserID(r.Context(), tok.UserID)
		ctx = plugin.WithOrgID(ctx, tok.OrgID)
		ctx = context.WithValue(ctx, sessionIDKey, uint(0))
		return m.WithTokenIdentity(ctx, tok), true
	}

	return r.Context(), false
}

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

func (m *Manager) AuthenticateRequest(r *http.Request) (*http.Request, bool) {
	ctx, ok := m.identify(r)
	if !ok {
		return r, false
	}
	return r.WithContext(ctx), true
}
