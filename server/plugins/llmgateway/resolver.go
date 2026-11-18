package llmgateway

import (
	"context"
	"fmt"
	"strings"

	"github.com/octarq-org/octarq/server/llmprovider"
	"github.com/octarq-org/octarq/server/plugin"
)

// pricingService exposes the gateway's price table to features and ledgers
// through plugin.ServiceModelPricing. It is a thin adapter so a consumer never
// imports this package.
type pricingService struct {
	plugin *Plugin
}

var _ plugin.ModelPricing = pricingService{}

func (s pricingService) Price(ctx context.Context, orgID uint, model string) (plugin.ModelPrice, error) {
	if s.plugin == nil || s.plugin.store == nil {
		return plugin.ModelPrice{}, plugin.ErrPricingUnavailable
	}
	return s.plugin.store.Price(ctx, orgID, model)
}

// gatewayService exposes the workspace backend to features that call a model
// outside the core chat, through plugin.ServiceLLMProvider.
type gatewayService struct {
	plugin *Plugin
}

var _ plugin.LLMGateway = gatewayService{}

func (s gatewayService) Provider(ctx context.Context, orgID uint) (llmprovider.Provider, error) {
	if s.plugin == nil {
		return nil, plugin.ErrLLMGatewayUnavailable
	}
	return s.plugin.Resolve(ctx, orgID)
}

// Resolve returns the provider that serves orgID, with its key unsealed.
//
// It never falls back to instance environment variables: a workspace without a
// configured provider reports ErrNotConfigured, and the caller must refuse the
// turn rather than spend the operator's credentials.
func (p *Plugin) Resolve(ctx context.Context, orgID uint) (llmprovider.Provider, error) {
	if p == nil || p.store == nil {
		return nil, ErrNotConfigured
	}
	row, err := p.store.ActiveProvider(ctx, orgID)
	if err != nil {
		return nil, err
	}
	return p.providerFrom(ctx, row)
}

func (p *Plugin) providerFrom(ctx context.Context, row *Provider) (llmprovider.Provider, error) {
	key, err := p.resolveAPIKey(ctx, row)
	if err != nil {
		return nil, err
	}
	prov, err := llmprovider.New(llmprovider.Options{
		Provider:   strings.ToLower(strings.TrimSpace(row.Backend)),
		APIKey:     key,
		BaseURL:    row.BaseURL,
		Model:      row.Model,
		CheapModel: row.CheapModel,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidProvider, err)
	}
	return prov, nil
}

func (p *Plugin) vaultService() plugin.VaultService {
	if p == nil || p.pctx == nil {
		return nil
	}
	vs, ok := plugin.LookupAs[plugin.VaultService](p.pctx, plugin.ServiceVault)
	if !ok {
		return nil
	}
	return vs
}

func (p *Plugin) resolveAPIKey(ctx context.Context, row *Provider) (string, error) {
	if row == nil || row.VaultItemID == 0 {
		return "", nil
	}
	vs := p.vaultService()
	if vs == nil {
		return "", fmt.Errorf("%w: secret storage is unavailable", ErrInvalidProvider)
	}
	sec, err := vs.GetSecret(ctx, row.OrgID, row.VaultItemID)
	if err != nil {
		return "", fmt.Errorf("%w: unseal API key: %v", ErrInvalidProvider, err)
	}
	if k, ok := sec.Payload["apiKey"].(string); ok {
		return strings.TrimSpace(k), nil
	}
	return "", nil
}
