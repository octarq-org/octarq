package notify

import (
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

type Plugin struct {
	db   *gorm.DB
	pctx *plugin.Context
}

var (
	_ plugin.Plugin    = (*Plugin)(nil)
	_ plugin.Describer = (*Plugin)(nil)
)

func New() *Plugin { return &Plugin{} }

func (p *Plugin) Name() string { return "notify" }

func (p *Plugin) Describe() plugin.Info {
	return plugin.Info{
		Title:            "Notify",
		Description:      "Send a message to the workspace's configured notification channels on behalf of an AI agent.",
		Category:         plugin.CategoryUtilities,
		Core:             true,
		EnabledByDefault: true,
	}
}

func (p *Plugin) Models() []any { return nil }

func (p *Plugin) Mount(_ plugin.Mux, ctx *plugin.Context) {
	p.pctx = ctx
	if ctx != nil && ctx.DB != nil {
		p.db = ctx.DB
	}
	p.registerRoutes(ctx)
}
