package llmgateway

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/server/llmprovider"
	"github.com/octarq-org/octarq/server/plugin"
)

type ProviderDTO struct {
	ID         uint      `json:"id"`
	Name       string    `json:"name"`
	Backend    string    `json:"backend"`
	BaseURL    string    `json:"baseUrl"`
	Model      string    `json:"model"`
	CheapModel string    `json:"cheapModel"`
	IsDefault  bool      `json:"isDefault"`
	HasKey     bool      `json:"hasKey"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type PriceDTO struct {
	Model       string  `json:"model"`
	DisplayName string  `json:"displayName"`
	Input       float64 `json:"input"`
	Output      float64 `json:"output"`
	Reasoning   float64 `json:"reasoning"`
}

func toProviderDTO(row Provider) ProviderDTO {
	return ProviderDTO{
		ID: row.ID, Name: row.Name, Backend: row.Backend, BaseURL: row.BaseURL,
		Model: row.Model, CheapModel: row.CheapModel, IsDefault: row.IsDefault,
		HasKey: row.VaultItemID > 0, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func (p *Plugin) registerRoutes(ctx *plugin.Context) {
	if ctx == nil || ctx.Huma == nil {
		return
	}
	api := ctx.Huma
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/llm-gateway/providers",
		Summary: "List LLM Providers", Tags: []string{"LLM Gateway"}}, p.listProviders)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/llm-gateway/providers",
		Summary: "Create LLM Provider", Tags: []string{"LLM Gateway"}, DefaultStatus: 201}, p.createProvider)
	huma.Register(api, huma.Operation{Method: "PUT", Path: "/api/llm-gateway/providers/{id}",
		Summary: "Update LLM Provider", Tags: []string{"LLM Gateway"}}, p.updateProvider)
	huma.Register(api, huma.Operation{Method: "DELETE", Path: "/api/llm-gateway/providers/{id}",
		Summary: "Delete LLM Provider", Tags: []string{"LLM Gateway"}}, p.deleteProvider)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/llm-gateway/providers/{id}/default",
		Summary: "Set Default LLM Provider", Tags: []string{"LLM Gateway"}}, p.setDefaultProvider)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/llm-gateway/providers/{id}/test",
		Summary: "Test LLM Provider", Tags: []string{"LLM Gateway"}}, p.testProvider)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/llm-gateway/models",
		Summary: "Discover Provider Models", Tags: []string{"LLM Gateway"}}, p.discoverModels)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/llm-gateway/backends",
		Summary: "List LLM Backends", Tags: []string{"LLM Gateway"}}, p.listBackends)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/llm-gateway/prices",
		Summary: "List Model Prices", Tags: []string{"LLM Gateway"}}, p.listPrices)
}

func (p *Plugin) caller(c huma.Context) (uint, error) {
	if c == nil {
		return 0, huma.Error500InternalServerError("Request context is unavailable. Try again.")
	}
	r, _ := humago.Unwrap(c)
	if p.pctx == nil || p.pctx.Host == nil || p.pctx.Host.Session() == nil {
		return 0, huma.Error500InternalServerError("The host session is unavailable. Try again.")
	}
	s := p.pctx.Host.Session()
	if s.UserID(r) == 0 {
		return 0, huma.Error401Unauthorized("Authentication required. Sign in to continue.")
	}
	if !s.RequireRole(r, "admin") {
		return 0, huma.Error403Forbidden("Workspace permission denied. An admin must configure the gateway.")
	}
	orgID := s.OrgID(r)
	if orgID == 0 {
		return 0, huma.Error400BadRequest("Select a workspace first.")
	}
	if p.store == nil {
		return 0, huma.Error500InternalServerError("The gateway store is not configured. Retry the request.")
	}
	return orgID, nil
}

func mapError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrNotFound):
		return huma.Error404NotFound("LLM provider not found. Check the provider and try again.")
	case errors.Is(err, ErrNotConfigured):
		return huma.Error400BadRequest("No LLM provider is configured for this workspace. Add one first.")
	case errors.Is(err, ErrInvalidProvider):
		return huma.Error400BadRequest(err.Error())
	default:
		return huma.Error500InternalServerError("The request failed. Try again.")
	}
}

type ListProvidersInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *ListProvidersInput) Resolve(c huma.Context) []error { i.Ctx = c; return nil }

type ListProvidersOutput struct {
	Body struct {
		Providers []ProviderDTO `json:"providers"`
	}
}

func (p *Plugin) listProviders(ctx context.Context, input *ListProvidersInput) (*ListProvidersOutput, error) {
	orgID, err := p.caller(input.Ctx)
	if err != nil {
		return nil, err
	}
	rows, err := p.store.ListProviders(ctx, orgID)
	if err != nil {
		return nil, mapError(err)
	}
	out := &ListProvidersOutput{}
	out.Body.Providers = make([]ProviderDTO, 0, len(rows))
	for _, row := range rows {
		out.Body.Providers = append(out.Body.Providers, toProviderDTO(row))
	}
	return out, nil
}

type ProviderBody struct {
	Name       string `json:"name" required:"true" maxLength:"128"`
	Backend    string `json:"backend" required:"true" maxLength:"32"`
	APIKey     string `json:"apiKey,omitempty"`
	BaseURL    string `json:"baseUrl,omitempty" maxLength:"256"`
	Model      string `json:"model,omitempty" maxLength:"64"`
	CheapModel string `json:"cheapModel,omitempty" maxLength:"64"`
}

type CreateProviderInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body ProviderBody
}

func (i *CreateProviderInput) Resolve(c huma.Context) []error { i.Ctx = c; return nil }

type ProviderOutput struct {
	Body struct {
		Provider ProviderDTO `json:"provider"`
	}
}

func (p *Plugin) createProvider(ctx context.Context, input *CreateProviderInput) (*ProviderOutput, error) {
	orgID, err := p.caller(input.Ctx)
	if err != nil {
		return nil, err
	}
	if err := validateBackend(input.Body.Backend); err != nil {
		return nil, mapError(err)
	}
	row, err := p.store.CreateProvider(ctx, orgID, ProviderInput{
		Name: input.Body.Name, Backend: input.Body.Backend, APIKey: input.Body.APIKey,
		BaseURL: input.Body.BaseURL, Model: input.Body.Model, CheapModel: input.Body.CheapModel,
	}, p.vaultService())
	if err != nil {
		return nil, mapError(err)
	}
	out := &ProviderOutput{}
	out.Body.Provider = toProviderDTO(*row)
	return out, nil
}

type ProviderIDInput struct {
	ID  uint         `path:"id"`
	Ctx huma.Context `hidden:"true"`
}

func (i *ProviderIDInput) Resolve(c huma.Context) []error { i.Ctx = c; return nil }

type UpdateProviderInput struct {
	ID   uint         `path:"id"`
	Ctx  huma.Context `hidden:"true"`
	Body ProviderBody
}

func (i *UpdateProviderInput) Resolve(c huma.Context) []error { i.Ctx = c; return nil }

func (p *Plugin) updateProvider(ctx context.Context, input *UpdateProviderInput) (*ProviderOutput, error) {
	orgID, err := p.caller(input.Ctx)
	if err != nil {
		return nil, err
	}
	if err := validateBackend(input.Body.Backend); err != nil {
		return nil, mapError(err)
	}
	row, err := p.store.UpdateProvider(ctx, orgID, input.ID, ProviderInput{
		Name: input.Body.Name, Backend: input.Body.Backend, APIKey: input.Body.APIKey,
		BaseURL: input.Body.BaseURL, Model: input.Body.Model, CheapModel: input.Body.CheapModel,
	}, p.vaultService())
	if err != nil {
		return nil, mapError(err)
	}
	out := &ProviderOutput{}
	out.Body.Provider = toProviderDTO(*row)
	return out, nil
}

type GatewayOKOutput struct {
	Body struct {
		OK bool `json:"ok"`
	}
}

func (p *Plugin) deleteProvider(ctx context.Context, input *ProviderIDInput) (*GatewayOKOutput, error) {
	orgID, err := p.caller(input.Ctx)
	if err != nil {
		return nil, err
	}
	if err := p.store.DeleteProvider(ctx, orgID, input.ID, p.vaultService()); err != nil {
		return nil, mapError(err)
	}
	out := &GatewayOKOutput{}
	out.Body.OK = true
	return out, nil
}

func (p *Plugin) setDefaultProvider(ctx context.Context, input *ProviderIDInput) (*GatewayOKOutput, error) {
	orgID, err := p.caller(input.Ctx)
	if err != nil {
		return nil, err
	}
	if err := p.store.SetDefault(ctx, orgID, input.ID); err != nil {
		return nil, mapError(err)
	}
	out := &GatewayOKOutput{}
	out.Body.OK = true
	return out, nil
}

type TestProviderOutput struct {
	Body struct {
		OK    bool   `json:"ok"`
		Model string `json:"model"`
	}
}

type ListProviderModelsOutput struct {
	Body struct {
		Models []modelEntry `json:"models"`
	}
}

type DiscoverModelsBody struct {
	ProviderID uint   `json:"providerId,omitempty"`
	Backend    string `json:"backend,omitempty" maxLength:"32"`
	BaseURL    string `json:"baseUrl,omitempty" maxLength:"256"`
	APIKey     string `json:"apiKey,omitempty"`
}

type DiscoverModelsInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body DiscoverModelsBody
}

func (i *DiscoverModelsInput) Resolve(c huma.Context) []error { i.Ctx = c; return nil }

func (p *Plugin) discoverModels(ctx context.Context, input *DiscoverModelsInput) (*ListProviderModelsOutput, error) {
	orgID, err := p.caller(input.Ctx)
	if err != nil {
		return nil, err
	}
	ap, err := p.draftProvider(ctx, orgID, input.Body)
	if err != nil {
		return nil, mapError(err)
	}
	models, err := p.discoveredModels(ctx, ap)
	if err != nil {
		return nil, huma.Error502BadGateway(fmt.Sprintf("The provider did not list its models: %v", err))
	}
	out := &ListProviderModelsOutput{}
	out.Body.Models = models
	return out, nil
}

func (p *Plugin) draftProvider(ctx context.Context, orgID uint, in DiscoverModelsBody) (agentProvider, error) {
	if in.ProviderID != 0 {
		row, err := p.store.GetProvider(ctx, orgID, in.ProviderID)
		if err != nil {
			return agentProvider{}, err
		}
		if b := strings.TrimSpace(in.Backend); b != "" {
			row.Backend = b
		}
		if u := strings.TrimSpace(in.BaseURL); u != "" {
			row.BaseURL = u
		}
		return p.discoveryProviderFromRow(ctx, row, in.APIKey)
	}
	if strings.TrimSpace(in.Backend) == "" {
		return agentProvider{}, fmt.Errorf("%w: backend is required", ErrInvalidProvider)
	}
	return p.discoveryProviderFromRow(ctx, &Provider{Backend: in.Backend, BaseURL: in.BaseURL}, in.APIKey)
}

func (p *Plugin) testProvider(ctx context.Context, input *ProviderIDInput) (*TestProviderOutput, error) {
	orgID, err := p.caller(input.Ctx)
	if err != nil {
		return nil, err
	}
	row, err := p.store.GetProvider(ctx, orgID, input.ID)
	if err != nil {
		return nil, mapError(err)
	}
	prov, err := p.providerFrom(ctx, row)
	if err != nil {
		return nil, mapError(err)
	}
	tctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	resp, err := prov.Complete(tctx, llmprovider.Request{
		Model:     row.Model,
		System:    "Reply with the single word OK.",
		Messages:  []llmprovider.Message{{Role: llmprovider.RoleUser, Content: "ping"}},
		MaxTokens: 8,
	})
	if err != nil {
		return nil, huma.Error502BadGateway(fmt.Sprintf("The provider refused the test call: %v", err))
	}
	out := &TestProviderOutput{}
	out.Body.OK = true
	if resp.Model != "" {
		out.Body.Model = resp.Model
	} else {
		out.Body.Model = row.Model
	}
	return out, nil
}

type ListBackendsOutput struct {
	Body struct {
		Backends []string `json:"backends"`
	}
}

func (p *Plugin) listBackends(ctx context.Context, input *ListPricesInput) (*ListBackendsOutput, error) {
	if _, err := p.caller(input.Ctx); err != nil {
		return nil, err
	}
	out := &ListBackendsOutput{}
	out.Body.Backends = llmprovider.Names()
	return out, nil
}

type ListPricesInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *ListPricesInput) Resolve(c huma.Context) []error { i.Ctx = c; return nil }

type ListPricesOutput struct {
	Body struct {
		Prices []PriceDTO `json:"prices"`
	}
}

func (p *Plugin) listPrices(ctx context.Context, input *ListPricesInput) (*ListPricesOutput, error) {
	if _, err := p.caller(input.Ctx); err != nil {
		return nil, err
	}
	return p.prices()
}

func (p *Plugin) prices() (*ListPricesOutput, error) {
	c, err := pricingCatalog()
	if err != nil {
		return nil, mapError(err)
	}
	out := &ListPricesOutput{}
	out.Body.Prices = c.billable()
	return out, nil
}

func validateBackend(backend string) error {
	name := strings.ToLower(strings.TrimSpace(backend))
	if name == "" {
		return fmt.Errorf("%w: backend is required", ErrInvalidProvider)
	}
	for _, known := range llmprovider.Names() {
		if known == name {
			return nil
		}
	}
	return fmt.Errorf("%w: unknown backend %q (have %v)", ErrInvalidProvider, backend, llmprovider.Names())
}
