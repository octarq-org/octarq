package plugin

import (
	"github.com/octarq-org/octarq/server/internal/tenantsql"
	"gorm.io/gorm"
)

type TenantColumn = tenantsql.TenantColumn

type TenantView = tenantsql.TenantView

type TenantDB = tenantsql.TenantDB

type Scope = tenantsql.Scope

func NewTenantDB(db *gorm.DB, orgID uint) (*TenantDB, error) {
	return tenantsql.NewTenantDB(db, orgID)
}

func RegisterTenantView(ctx *Context, view TenantView) {
	if ctx != nil && ctx.RegisterTenantView != nil {
		ctx.RegisterTenantView(view)
	}
}
