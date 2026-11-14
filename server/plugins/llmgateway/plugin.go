package llmgateway

import (
	"context"
	"embed"
	"io/fs"

	"github.com/octarq-org/octarq/server/llmprovider"
	"github.com/octarq-org/octarq/server/plugin"
)

//go:embed docs
var docs embed.FS

// Plugin is the core model gateway.
type Plugin struct {
	store *Store
	pctx  *plugin.Context
}

var (
	_ plugin.Plugin     = (*Plugin)(nil)
	_ plugin.Describer  = (*Plugin)(nil)
	_ plugin.HelpDocsFS = (*Plugin)(nil)
)

// New builds the gateway plugin.
func New() *Plugin { return &Plugin{} }

// Name is the stable identifier used by the feature gate and the help index.
func (p *Plugin) Name() string { return "llmgateway" }

// Describe marks the gateway as core plumbing: every AI feature resolves its
// backend through it, so a workspace toggle could only break those features.
func (p *Plugin) Describe() plugin.Info {
	return plugin.Info{
		Title:            "LLM Gateway",
		Description:      "Configure the model backends of the workspace and the price of each model.",
		Category:         plugin.CategoryAI,
		Icon:             "brain",
		Core:             true,
		EnabledByDefault: true,
	}
}

// Models are migrated with the rest of the schema before any route is served.
func (p *Plugin) Models() []any {
	return []any{&Provider{}, &ModelPrice{}}
}

// Mount wires the store, the price service, the AI resolver, and the routes.
func (p *Plugin) Mount(_ plugin.Mux, ctx *plugin.Context) {
	if ctx == nil {
		return
	}
	p.pctx = ctx
	if ctx.DB != nil {
		p.store = NewStore(ctx.DB)
	}
	ctx.Provide(plugin.ServiceModelPricing, pricingService{plugin: p})
	ctx.Provide(plugin.ServiceLLMProvider, gatewayService{plugin: p})
	if ctx.SetLLMResolverForOrg != nil {
		// The seam carries no request context: it is registered once at Mount and
		// called per request. The lookup is one indexed read, so a background
		// context keeps the contract simple without risking a hang.
		ctx.SetLLMResolverForOrg(func(orgID uint) (llmprovider.Provider, error) {
			return p.Resolve(context.Background(), orgID)
		})
	}
	p.registerRoutes(ctx)
}

// HelpDocsFS hands the embedded pages to the help aggregator.
func (p *Plugin) HelpDocsFS() fs.FS { return docs }
