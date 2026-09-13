package tenantquery

import (
	"github.com/octarq-org/octarq/internal/tenantsql"
	"github.com/octarq-org/octarq/plugin"
	"gorm.io/gorm"
)

// Plugin provides the tenant SQL view describe and query tools.
type Plugin struct {
	db       *gorm.DB
	registry *tenantsql.Registry
}

var (
	_ plugin.Plugin    = (*Plugin)(nil)
	_ plugin.Describer = (*Plugin)(nil)
)

// New creates a new tenantquery Plugin.
func New() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Name() string {
	return "tenantquery"
}

func (p *Plugin) Describe() plugin.Info {
	return plugin.Info{
		Title:            "Tenant Query",
		Description:      "Read-only tenant SQL views and query engine for AI and analytics.",
		Category:         plugin.CategoryUtilities,
		Core:             true,
		EnabledByDefault: true,
	}
}

func (p *Plugin) Models() []any {
	return nil
}

func (p *Plugin) Mount(mux plugin.Mux, ctx *plugin.Context) {
	if ctx != nil && ctx.DB != nil {
		p.db = ctx.DB
	}
	p.registry = tenantsql.DefaultRegistry()
	p.registerRoutes(ctx)
}
