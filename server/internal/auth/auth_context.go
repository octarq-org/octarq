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

var BcryptCost = bcrypt.DefaultCost

type contextKey string

const (
	userIDKey    contextKey = "user_id"
	sessionIDKey contextKey = "session_id"
	tokenIDKey   contextKey = "token_id"
)

func WithOrgID(ctx context.Context, orgID uint) context.Context {
	return plugin.WithOrgID(ctx, orgID)
}

func WithUserID(ctx context.Context, uid uint) context.Context {
	ctx = context.WithValue(ctx, userIDKey, uid)
	return tenantsql.WithUserID(ctx, uid)
}

func UserIDFromContext(ctx context.Context) uint {
	if v, ok := ctx.Value(userIDKey).(uint); ok && v != 0 {
		return v
	}
	return tenantsql.UserIDFromContext(ctx)
}

func WithTokenID(ctx context.Context, id uint) context.Context {
	return context.WithValue(ctx, tokenIDKey, id)
}

func TokenIDFromContext(ctx context.Context) uint {
	if v, ok := ctx.Value(tokenIDKey).(uint); ok {
		return v
	}
	return 0
}

func WithTokenRole(ctx context.Context, role string) context.Context {
	return plugin.WithTokenRole(ctx, role)
}

func TokenRoleFromContext(ctx context.Context) string {
	return plugin.RoleFromContext(ctx)
}

func WithTokenAgent(ctx context.Context, isAgent bool, allowedTools any) context.Context {
	return plugin.WithTokenAgent(ctx, isAgent, allowedTools)
}

func IsAgentFromContext(ctx context.Context) bool {
	return plugin.IsAgentFromContext(ctx)
}

func AllowedToolsFromContext(ctx context.Context) []string {
	return plugin.AllowedToolsFromContext(ctx)
}

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
	addr := r.RemoteAddr
	if i := strings.LastIndex(addr, ":"); i > 0 {
		return addr[:i]
	}
	return addr
}

func (m *Manager) Check(user, pass string) bool {
	userOK := subtle.ConstantTimeCompare([]byte(user), []byte(m.cfg.AdminUser)) == 1
	passOK := subtle.ConstantTimeCompare([]byte(pass), []byte(m.cfg.AdminPassword)) == 1
	return userOK && passOK
}

func (m *Manager) CheckAdminPassword(pass string) bool {
	return subtle.ConstantTimeCompare([]byte(pass), []byte(m.cfg.AdminPassword)) == 1
}

func (m *Manager) IsConfigAdmin(email string) bool {
	return email != "" && email == m.cfg.AdminUser
}
