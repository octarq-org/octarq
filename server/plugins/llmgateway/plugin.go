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

type Plugin struct {
	store *Store
	pctx  *plugin.Context
}

var (
	_ plugin.Plugin     = (*Plugin)(nil)
	_ plugin.Describer  = (*Plugin)(nil)
	_ plugin.HelpDocsFS = (*Plugin)(nil)
	_ plugin.Validator  = (*Plugin)(nil)
)

func New() *Plugin { return &Plugin{} }

func (p *Plugin) Name() string { return "llmgateway" }

func (p *Plugin) Describe() plugin.Info {
	return plugin.Info{
		Title:            "LLM Gateway",
		Description:      "Configure the model backends of the workspace. Model prices come from the bundled pricing dataset.",
		Category:         plugin.CategoryAI,
		Icon:             "brain",
		Core:             true,
		EnabledByDefault: true,
	}
}

func (p *Plugin) Models() []any {
	return []any{&Provider{}}
}

func (p *Plugin) Validate(ctx context.Context, host plugin.Host) error {
	if _, err := pricingCatalog(); err != nil {
		return err
	}
	return nil
}

func (p *Plugin) Mount(mux plugin.Mux, ctx *plugin.Context) {
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
		ctx.SetLLMResolverForOrg(func(orgID uint) (llmprovider.Provider, error) {
			return p.Resolve(context.Background(), orgID)
		})
	}
	p.registerRoutes(ctx)
	p.registerAgentRoutes(mux, ctx)
}

func (p *Plugin) HelpDocsFS() fs.FS { return docs }
