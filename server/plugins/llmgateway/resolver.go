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
	return p.providerFrom(row)
}

func (p *Plugin) providerFrom(row *Provider) (llmprovider.Provider, error) {
	key := ""
	if row.APIKeyEncr != "" {
		vault := p.vault()
		if vault == nil {
			return nil, fmt.Errorf("%w: secret storage is unavailable", ErrInvalidProvider)
		}
		plain, err := vault.Decrypt(row.APIKeyEncr)
		if err != nil {
			return nil, fmt.Errorf("%w: unseal API key: %v", ErrInvalidProvider, err)
		}
		key = string(plain)
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

func (p *Plugin) vault() plugin.CryptoVault {
	if p.pctx == nil || p.pctx.Host == nil {
		return nil
	}
	return p.pctx.Host.Crypto()
}

func (p *Plugin) seal(plain string) (string, error) {
	vault := p.vault()
	if vault == nil {
		return "", fmt.Errorf("secret storage is unavailable")
	}
	return vault.Encrypt([]byte(plain))
}
