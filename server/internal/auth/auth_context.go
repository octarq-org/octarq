package auth

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/octarq-org/octarq/server/internal/tenantsql"
	"github.com/octarq-org/octarq/server/plugin"
	"golang.org/x/crypto/bcrypt"
)

// BcryptCost is the bcrypt hashing cost used across the application.
// Defaults to bcrypt.DefaultCost (10), but can be set to bcrypt.MinCost (4) in tests.
var BcryptCost = bcrypt.DefaultCost

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
	ctx = context.WithValue(ctx, userIDKey, uid)
	return tenantsql.WithUserID(ctx, uid)
}

// UserIDFromContext extracts the authenticated user ID from ctx, returning 0 if absent.
func UserIDFromContext(ctx context.Context) uint {
	if v, ok := ctx.Value(userIDKey).(uint); ok && v != 0 {
		return v
	}
	return tenantsql.UserIDFromContext(ctx)
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

// reporterIP extracts the best-effort client IP from the request. Proxy
// headers are honoured only when trustProxy is set, otherwise a client could
// spoof X-Forwarded-For to evade the login rate limiter.
func reporterIP(r *http.Request) string {
	if trustProxy.Load() {
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

// Check verifies admin credentials using a constant-time comparison, so a
// timing side channel cannot reveal prefix information about them (the
// comparison still reveals each string's length).
func (m *Manager) Check(user, pass string) bool {
	userOK := subtle.ConstantTimeCompare([]byte(user), []byte(m.cfg.AdminUser)) == 1
	passOK := subtle.ConstantTimeCompare([]byte(pass), []byte(m.cfg.AdminPassword)) == 1
	return userOK && passOK
}

// CheckAdminPassword verifies the instance admin password using constant-time comparison.
func (m *Manager) CheckAdminPassword(pass string) bool {
	return subtle.ConstantTimeCompare([]byte(pass), []byte(m.cfg.AdminPassword)) == 1
}

// IsConfigAdmin reports whether email is the instance's bootstrap admin — the
// account whose password lives in the instance config rather than in a row.
// Callers that need to know whether octarq holds a checkable password for an
// account (re-authentication before a sensitive change) cannot tell from an
// empty PasswordHash alone: this user has one too, and it is not "no password".
func (m *Manager) IsConfigAdmin(email string) bool {
	return email != "" && email == m.cfg.AdminUser
}
