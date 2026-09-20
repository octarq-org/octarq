// Package notify exposes a single write tool that delivers a message to the
// calling workspace's configured notification channels. It is the core
// replacement for the removed community telegram/webhook plugins' MCP tools:
// an agent can ping a human over whatever channel the workspace already set up
// in Settings → Alerts, without a second configuration surface.
package notify

import (
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

// Plugin holds the tenant-scoped DB handle used to read the workspace's enabled
// notification channels.
type Plugin struct {
	db *gorm.DB
}

var (
	_ plugin.Plugin    = (*Plugin)(nil)
	_ plugin.Describer = (*Plugin)(nil)
)

// New creates a new notify Plugin.
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
	if ctx != nil && ctx.DB != nil {
		p.db = ctx.DB
	}
	p.registerRoutes(ctx)
}
