package plugin

import (
	"github.com/octarq-org/octarq/server/internal/tenantsql"
	"gorm.io/gorm"
)

// TenantColumn describes a column in a tenant view schema.
type TenantColumn = tenantsql.TenantColumn

// TenantView defines a tenant-isolated SQL view.
type TenantView = tenantsql.TenantView

// TenantDB provides a type-safe, fail-closed database handle strictly scoped to a specific tenant (OrgID).
type TenantDB = tenantsql.TenantDB

// NewTenantDB constructs a new TenantDB scoped to orgID.
func NewTenantDB(db *gorm.DB, orgID uint) (*TenantDB, error) {
	return tenantsql.NewTenantDB(db, orgID)
}

// RegisterTenantView is a type-safe helper to register a TenantView onto a Context.
func RegisterTenantView(ctx *Context, view TenantView) {
	if ctx != nil && ctx.RegisterTenantView != nil {
		ctx.RegisterTenantView(view)
	}
}
