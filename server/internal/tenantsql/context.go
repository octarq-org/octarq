package tenantsql

import (
	"context"
	"strings"
)

type ContextKey string

const (
	OrgIDKey         ContextKey = "org_id"
	UserIDKey        ContextKey = "user_id"
	ScopeKeyCtxKey   ContextKey = "scope_key"
	AllowedScopesKey ContextKey = "allowed_scopes"

	DefaultWorkScope     = "work:default"
	PersonalPrivateScope = "personal:private"
	ScopeWildcardWork    = "work:*"
	ScopeWildcardPerson  = "personal:*"
)

func WithOrgID(ctx context.Context, orgID uint) context.Context {
	return context.WithValue(ctx, OrgIDKey, orgID)
}

func OrgIDFromContext(ctx context.Context) uint {
	if ctx == nil {
		return 0
	}
	if id, ok := ctx.Value(OrgIDKey).(uint); ok {
		return id
	}
	return 0
}

func WithUserID(ctx context.Context, uid uint) context.Context {
	return context.WithValue(ctx, UserIDKey, uid)
}

func UserIDFromContext(ctx context.Context) uint {
	if ctx == nil {
		return 0
	}
	if v, ok := ctx.Value(UserIDKey).(uint); ok {
		return v
	}
	return 0
}

func WithScopeKey(ctx context.Context, scopeKey string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	trimmed := strings.TrimSpace(scopeKey)
	if trimmed == "" {
		trimmed = DefaultWorkScope
	}
	ctx = context.WithValue(ctx, ScopeKeyCtxKey, trimmed)
	if _, hasExplicit := ctx.Value(AllowedScopesKey).([]string); !hasExplicit {
		if strings.HasPrefix(strings.ToLower(trimmed), "personal:") {
			ctx = context.WithValue(ctx, AllowedScopesKey, []string{ScopeWildcardWork, ScopeWildcardPerson})
		}
	}
	return ctx
}

func ScopeKeyFromContext(ctx context.Context) string {
	if ctx == nil {
		return DefaultWorkScope
	}
	if v, ok := ctx.Value(ScopeKeyCtxKey).(string); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return DefaultWorkScope
}

func WithAllowedScopes(ctx context.Context, scopes ...string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	cleaned := make([]string, 0, len(scopes))
	for _, s := range scopes {
		if t := strings.TrimSpace(s); t != "" {
			cleaned = append(cleaned, t)
		}
	}
	if len(cleaned) == 0 {
		cleaned = []string{ScopeWildcardWork}
	}
	return context.WithValue(ctx, AllowedScopesKey, cleaned)
}

func AllowedScopesFromContext(ctx context.Context) []string {
	if ctx == nil {
		return []string{ScopeWildcardWork}
	}
	if v, ok := ctx.Value(AllowedScopesKey).([]string); ok && len(v) > 0 {
		out := make([]string, len(v))
		copy(out, v)
		return out
	}
	if sk, ok := ctx.Value(ScopeKeyCtxKey).(string); ok && strings.HasPrefix(strings.ToLower(strings.TrimSpace(sk)), "personal:") {
		return []string{ScopeWildcardWork, ScopeWildcardPerson}
	}
	return []string{ScopeWildcardWork}
}

func AllowsPersonalScope(ctx context.Context) bool {
	for _, pat := range AllowedScopesFromContext(ctx) {
		lower := strings.ToLower(strings.TrimSpace(pat))
		if lower == "*" || lower == ScopeWildcardPerson || strings.HasPrefix(lower, "personal:") {
			return true
		}
	}
	return false
}

func MatchScopePattern(pattern, scopeKey string) bool {
	p := strings.ToLower(strings.TrimSpace(pattern))
	s := strings.ToLower(strings.TrimSpace(scopeKey))
	if s == "" {
		s = DefaultWorkScope
	}
	if p == "*" || p == s {
		return true
	}
	if strings.HasSuffix(p, "*") {
		prefix := strings.TrimSuffix(p, "*")
		return strings.HasPrefix(s, prefix)
	}
	return false
}

func ScopeAllowed(ctx context.Context, scopeKey string) bool {
	for _, pat := range AllowedScopesFromContext(ctx) {
		if MatchScopePattern(pat, scopeKey) {
			return true
		}
	}
	return false
}
