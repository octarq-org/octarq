package tenantsql

import (
	"context"
)

type ContextKey string

const (
	OrgIDKey  ContextKey = "org_id"
	UserIDKey ContextKey = "user_id"
)

// WithOrgID returns a new context containing the organization ID for tenant scopes.
func WithOrgID(ctx context.Context, orgID uint) context.Context {
	return context.WithValue(ctx, OrgIDKey, orgID)
}

// OrgIDFromContext extracts the organization ID from context (0 if unset).
func OrgIDFromContext(ctx context.Context) uint {
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
	if v, ok := ctx.Value(UserIDKey).(uint); ok {
		return v
	}
	return 0
}
