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

// ProviderDTO is the client view of a provider. The API key is never returned;
// HasKey says whether one is stored.
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

// PriceDTO is one model price per one million tokens.
type PriceDTO struct {
	Model     string    `json:"model"`
	Input     float64   `json:"input"`
	Output    float64   `json:"output"`
	Reasoning float64   `json:"reasoning"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func toProviderDTO(row Provider) ProviderDTO {
	return ProviderDTO{
		ID: row.ID, Name: row.Name, Backend: row.Backend, BaseURL: row.BaseURL,
		Model: row.Model, CheapModel: row.CheapModel, IsDefault: row.IsDefault,
		HasKey: row.APIKeyEncr != "", CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
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
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/llm-gateway/backends",
		Summary: "List LLM Backends", Tags: []string{"LLM Gateway"}}, p.listBackends)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/llm-gateway/prices",
		Summary: "List Model Prices", Tags: []string{"LLM Gateway"}}, p.listPrices)
	huma.Register(api, huma.Operation{Method: "PUT", Path: "/api/llm-gateway/prices",
		Summary: "Set Model Price", Tags: []string{"LLM Gateway"}}, p.setPrice)
	huma.Register(api, huma.Operation{Method: "DELETE", Path: "/api/llm-gateway/prices",
		Summary: "Delete Model Price", Tags: []string{"LLM Gateway"}}, p.deletePrice)
}

// caller authenticates an admin of a workspace and returns the workspace ID.
// Every gateway route is admin-only: the credentials it holds spend money.
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

// --- providers ---

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
	}, p.seal)
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
	}, p.seal)
	if err != nil {
		return nil, mapError(err)
	}
	out := &ProviderOutput{}
	out.Body.Provider = toProviderDTO(*row)
	return out, nil
}

type OKOutput struct {
	Body struct {
		OK bool `json:"ok"`
	}
}

func (p *Plugin) deleteProvider(ctx context.Context, input *ProviderIDInput) (*OKOutput, error) {
	orgID, err := p.caller(input.Ctx)
	if err != nil {
		return nil, err
	}
	if err := p.store.DeleteProvider(ctx, orgID, input.ID); err != nil {
		return nil, mapError(err)
	}
	out := &OKOutput{}
	out.Body.OK = true
	return out, nil
}

func (p *Plugin) setDefaultProvider(ctx context.Context, input *ProviderIDInput) (*OKOutput, error) {
	orgID, err := p.caller(input.Ctx)
	if err != nil {
		return nil, err
	}
	if err := p.store.SetDefault(ctx, orgID, input.ID); err != nil {
		return nil, mapError(err)
	}
	out := &OKOutput{}
	out.Body.OK = true
	return out, nil
}

type TestProviderOutput struct {
	Body struct {
		OK    bool   `json:"ok"`
		Model string `json:"model"`
	}
}

// testProvider resolves the stored row the way a real turn does and sends one
// tiny completion. It is the only way to prove the key, base URL, and model
// agree without starting a chat.
func (p *Plugin) testProvider(ctx context.Context, input *ProviderIDInput) (*TestProviderOutput, error) {
	orgID, err := p.caller(input.Ctx)
	if err != nil {
		return nil, err
	}
	row, err := p.store.GetProvider(ctx, orgID, input.ID)
	if err != nil {
		return nil, mapError(err)
	}
	prov, err := p.providerFrom(row)
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

// listBackends names the backends a provider row may use. The admin form reads
// it so an unsupported name cannot be typed in.
func (p *Plugin) listBackends(ctx context.Context, input *ListPricesInput) (*ListBackendsOutput, error) {
	if _, err := p.caller(input.Ctx); err != nil {
		return nil, err
	}
	out := &ListBackendsOutput{}
	out.Body.Backends = llmprovider.Names()
	return out, nil
}

// --- prices ---
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
	orgID, err := p.caller(input.Ctx)
	if err != nil {
		return nil, err
	}
	rows, err := p.store.ListPrices(ctx, orgID)
	if err != nil {
		return nil, mapError(err)
	}
	out := &ListPricesOutput{}
	out.Body.Prices = make([]PriceDTO, 0, len(rows))
	for _, row := range rows {
		out.Body.Prices = append(out.Body.Prices, PriceDTO{
			Model: row.Model, Input: row.Input, Output: row.Output,
			Reasoning: row.Reasoning, UpdatedAt: row.UpdatedAt,
		})
	}
	return out, nil
}

type SetPriceInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body PriceDTO
}

func (i *SetPriceInput) Resolve(c huma.Context) []error { i.Ctx = c; return nil }

func (p *Plugin) setPrice(ctx context.Context, input *SetPriceInput) (*ListPricesOutput, error) {
	orgID, err := p.caller(input.Ctx)
	if err != nil {
		return nil, err
	}
	if err := p.store.SetPrice(ctx, orgID, ModelPrice{
		Model: input.Body.Model, Input: input.Body.Input,
		Output: input.Body.Output, Reasoning: input.Body.Reasoning,
	}); err != nil {
		return nil, mapError(err)
	}
	return p.listPrices(ctx, &ListPricesInput{})
}

type DeletePriceInput struct {
	Model string       `query:"model" required:"true"`
	Ctx   huma.Context `hidden:"true"`
}

func (i *DeletePriceInput) Resolve(c huma.Context) []error { i.Ctx = c; return nil }

func (p *Plugin) deletePrice(ctx context.Context, input *DeletePriceInput) (*OKOutput, error) {
	orgID, err := p.caller(input.Ctx)
	if err != nil {
		return nil, err
	}
	if err := p.store.DeletePrice(ctx, orgID, input.Model); err != nil {
		return nil, mapError(err)
	}
	out := &OKOutput{}
	out.Body.OK = true
	return out, nil
}

// validateBackend refuses a backend name no factory is registered for, so a
// typo fails at save time instead of at the first turn.
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
