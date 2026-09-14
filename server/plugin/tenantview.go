package plugin

// TenantColumn describes a column in a tenant view schema.
type TenantColumn struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

// TenantView defines a tenant-isolated SQL view.
type TenantView struct {
	Name       string                  `json:"name"` // 强制 "tenant_" 前缀
	Columns    []TenantColumn          `json:"columns"`
	Sensitive  []string                `json:"sensitive"` // 出参二次脱敏列
	Definition func(orgID uint) string `json:"-"`
}

// RegisterTenantView is a type-safe helper to register a TenantView onto a Context.
func RegisterTenantView(ctx *Context, view TenantView) {
	if ctx != nil && ctx.RegisterTenantView != nil {
		ctx.RegisterTenantView(view)
	}
}
