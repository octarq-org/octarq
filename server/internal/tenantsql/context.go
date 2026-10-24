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

// WithOrgID returns a new context containing the organization ID for tenant scopes.
func WithOrgID(ctx context.Context, orgID uint) context.Context {
	return context.WithValue(ctx, OrgIDKey, orgID)
}

// OrgIDFromContext extracts the organization ID from context (0 if unset).
func OrgIDFromContext(ctx context.Context) uint {
	if ctx == nil {
		return 0
	}
	if id, ok := ctx.Value(OrgIDKey).(uint); ok {
		return id
	}
	return 0
}

// WithUserID returns a new context containing the authenticated user ID.
func WithUserID(ctx context.Context, uid uint) context.Context {
	return context.WithValue(ctx, UserIDKey, uid)
}

// UserIDFromContext extracts the authenticated user ID from ctx, returning 0 if absent.
func UserIDFromContext(ctx context.Context) uint {
	if ctx == nil {
		return 0
	}
	if v, ok := ctx.Value(UserIDKey).(uint); ok {
		return v
	}
	return 0
}

// WithScopeKey binds an active sovereignty scope_key (e.g. "work:default" or "personal:private")
// to the context. Setting a "personal:*" scope key also grants personal scope access in AllowedScopesFromContext
// unless explicit allowed scopes were already locked down.
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

// ScopeKeyFromContext returns the active scope_key from context, defaulting to "work:default".
func ScopeKeyFromContext(ctx context.Context) string {
	if ctx == nil {
		return DefaultWorkScope
	}
	if v, ok := ctx.Value(ScopeKeyCtxKey).(string); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return DefaultWorkScope
}

// WithAllowedScopes sets the explicit list of scope patterns (e.g. "work:*", "personal:*")
// that the caller context is authorized to access.
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

// AllowedScopesFromContext returns the allowed scope patterns for the caller context.
// Un-elevated workspace/team requests default strictly to ["work:*"].
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

// AllowsPersonalScope reports whether the context grants access to "personal:*" records.
func AllowsPersonalScope(ctx context.Context) bool {
	for _, pat := range AllowedScopesFromContext(ctx) {
		lower := strings.ToLower(strings.TrimSpace(pat))
		if lower == "*" || lower == ScopeWildcardPerson || strings.HasPrefix(lower, "personal:") {
			return true
		}
	}
	return false
}

// MatchScopePattern checks if a concrete scopeKey (e.g. "work:default", "personal:private")
// matches an allowed pattern (e.g. "work:*", "personal:*", or exact match).
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

// ScopeAllowed reports whether the given scopeKey is permitted by AllowedScopesFromContext(ctx).
func ScopeAllowed(ctx context.Context, scopeKey string) bool {
	for _, pat := range AllowedScopesFromContext(ctx) {
		if MatchScopePattern(pat, scopeKey) {
			return true
		}
	}
	return false
}
