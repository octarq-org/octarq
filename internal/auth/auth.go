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
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/octarq-org/octarq/config"
	"github.com/octarq-org/octarq/internal/cache"
	"github.com/octarq-org/octarq/internal/crypto"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugin"
	"gorm.io/gorm"
)

type contextKey string

const (
	userIDKey    contextKey = "user_id"
	sessionIDKey contextKey = "session_id"
	tokenIDKey   contextKey = "token_id"
	tokenRoleKey contextKey = "token_role"
)

// WithOrgID returns a new context containing the organization ID.
func WithOrgID(ctx context.Context, orgID uint) context.Context {
	return plugin.WithOrgID(ctx, orgID)
}

// WithUserID returns a new context containing the authenticated user ID, using
// the same key UserID reads. Mirrors WithOrgID.
func WithUserID(ctx context.Context, uid uint) context.Context {
	return context.WithValue(ctx, userIDKey, uid)
}

// WithTokenID returns a new context containing the API bearer token ID.
func WithTokenID(ctx context.Context, id uint) context.Context {
	return context.WithValue(ctx, tokenIDKey, id)
}

// TokenIDFromContext extracts the API bearer token ID from ctx, returning 0 if absent.
func TokenIDFromContext(ctx context.Context) uint {
	if v, ok := ctx.Value(tokenIDKey).(uint); ok {
		return v
	}
	return 0
}

// TokenID extracts the API bearer token ID from the request context.
func (m *Manager) TokenID(r *http.Request) uint {
	return TokenIDFromContext(r.Context())
}

// WithTokenRole returns a new context carrying the bearer token's role ceiling.
// An empty role means an unrestricted (pre-scoping) token — see models.Token.Role.
func WithTokenRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, tokenRoleKey, role)
}

// TokenRoleFromContext returns the bearer token's role ceiling, or "" when the
// request is not token-authenticated OR the token predates scoping. Callers must
// therefore check TokenIDFromContext first to tell those two cases apart — "" on
// its own does not mean "no token".
func TokenRoleFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(tokenRoleKey).(string); ok {
		return v
	}
	return ""
}

// WithTokenIdentity stamps a validated bearer token onto ctx and kicks off the
// best-effort LastUsedAt touch. Every bearer path funnels through here so a new
// token field cannot be wired into one path and forgotten in the others — which
// is exactly how mcpAuth ended up recording no token id at all.
func (m *Manager) WithTokenIdentity(ctx context.Context, tok models.Token) context.Context {
	ctx = WithTokenID(ctx, tok.ID)
	ctx = WithTokenRole(ctx, tok.Role)
	m.touchToken(tok.ID)
	return ctx
}

// touchToken records LastUsedAt asynchronously; never blocks a request.
func (m *Manager) touchToken(id uint) {
	db := m.db
	if db == nil {
		return
	}
	go func() {
		now := time.Now()
		db.Model(&models.Token{}).Where("id = ?", id).Update("last_used_at", &now)
	}()
}

// TokenByRequest resolves and validates the request's "oct_" bearer token.
// It does NOT touch LastUsedAt — callers that admit the token use
// withTokenIdentity, which does.
func (m *Manager) TokenByRequest(r *http.Request) (models.Token, bool) {
	var tok models.Token
	if m.db == nil {
		return tok, false
	}
	raw := bearerToken(r)
	if !strings.HasPrefix(raw, "oct_") {
		return tok, false
	}
	if m.db.Where("hash = ?", models.HashToken(raw)).First(&tok).Error != nil {
		return tok, false
	}
	if tok.Expired() {
		return models.Token{}, false
	}
	return tok, true
}

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
			// Refresh the existing session. Only the token hash is stored, so we
			// can't recover the raw token from the row. If the caller already
			// holds this session's cookie (switch-org, refresh), reuse that raw
			// token to preserve continuity; otherwise (fresh login from the same
			// fingerprint) rotate to the freshly generated token.
			//
			// org_id must be written on BOTH refresh paths, not just on the
			// create path below. switch-org is exactly the case that lands here
			// (same browser, same cookie), so omitting it made every switch
			// after the first a silent no-op: the endpoint answered 200, the
			// dashboard remounted showing the new workspace's name, and every
			// subsequent request still resolved to the old org.
			if raw := cookieToken(r); raw != "" && models.HashToken(raw) == existing.Token {
				m.db.Model(&existing).Updates(map[string]any{
					"org_id":       orgID,
					"last_seen_at": now,
					"expires_at":   expires,
				})
				// The cache is keyed by token hash, and this path keeps the
				// token — so without an explicit invalidation a cached row would
				// keep serving the pre-switch org even now that the DB is right.
				_ = m.cache.Delete(context.Background(), "session:"+existing.Token)
				m.setCookie(w, raw)
				return
			}
			m.db.Model(&existing).Updates(map[string]any{
				"org_id":       orgID,
				"token":        models.HashToken(token),
				"last_seen_at": now,
				"expires_at":   expires,
			})
			_ = m.cache.Delete(context.Background(), "session:"+existing.Token)
			m.setCookie(w, token)
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
			Token:      models.HashToken(token),
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
	// Try fetching from cache first
	if m.cache.Get(ctx, "session:"+hashed, &s) {
		// Verify if it is expired in case Redis TTL hasn't kicked in
		if s.ExpiresAt.After(time.Now()) {
			return &s
		}
		// If expired, clean it up
		_ = m.cache.Delete(ctx, "session:"+hashed)
	}

	if err := m.db.Where("token = ? AND expires_at > ?", hashed, time.Now()).First(&s).Error; err != nil {
		return nil
	}

	// Cache the retrieved session
	ttl := time.Until(s.ExpiresAt)
	if ttl > 0 {
		_ = m.cache.Set(ctx, "session:"+hashed, &s, ttl)
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
		_ = m.cache.Delete(context.Background(), "session:"+hashed)
		if m.db != nil {
			m.db.Where("token = ?", hashed).Delete(&models.Session{})
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: "", Path: "/", HttpOnly: true, MaxAge: -1,
	})
}

// RevokeUserOrgSessions deletes every session that binds the given user to the
// given org, evicting each from the cache first so a cached row can't keep
// serving after the DB row is gone. Returns the number of sessions revoked.
//
// Membership is checked ONCE, when the session is minted: a session row carries
// its org and every later request trusts it. So dropping the org_members row is
// not by itself a revocation — without this, a removed member keeps full access
// to the workspace's data until their session expires (up to sessionTTL). Any
// code path that ends a membership must call this.
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
		// Session.Token holds the SHA-256 hash, which is exactly the cache key.
		_ = m.cache.Delete(ctx, "session:"+s.Token)
		ids = append(ids, s.ID)
	}
	m.db.Where("id IN ?", ids).Delete(&models.Session{})
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
		m.db.Model(&s).Update("last_seen_at", now)
		// Evict from cache to force update on next read
		_ = m.cache.Delete(context.Background(), "session:"+hashed)
	}
}

// bearerToken extracts a "oct_" token from the Authorization header.
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
	tok, ok := m.TokenByRequest(r)
	if !ok {
		return false
	}
	m.touchToken(tok.ID)
	return true
}

// APIAuthed reports whether the request is authenticated by either a valid
// session cookie or a valid API bearer token.
func (m *Manager) APIAuthed(r *http.Request) bool {
	return m.Authed(r) || m.tokenAuthed(r)
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

	// 2. Bearer token (API access, no session row). The user id stays 0 — a token
	// authenticates the workspace, not a person — so role gates read the token's
	// role ceiling instead (see callerHoldsRole).
	if tok, ok := m.TokenByRequest(r); ok {
		ctx := context.WithValue(r.Context(), userIDKey, uint(0))
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
