package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/octarq-org/octarq/server/internal/agentstore"
	"github.com/octarq-org/octarq/server/internal/models"
)

// WorkforceModelPriceDTO is one priced model of the workspace.
type WorkforceModelPriceDTO struct {
	Model         string    `json:"model"`
	InputPerMTok  float64   `json:"inputPerMTok"`
	OutputPerMTok float64   `json:"outputPerMTok"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// WorkforceSpendEventDTO is one billed turn in the spend ledger.
type WorkforceSpendEventDTO struct {
	ID              uint      `json:"id"`
	AgentID         string    `json:"agentId"`
	Model           string    `json:"model"`
	InputTokens     int       `json:"inputTokens"`
	OutputTokens    int       `json:"outputTokens"`
	ReasoningTokens int       `json:"reasoningTokens"`
	CostUSD         float64   `json:"costUsd"`
	Source          string    `json:"source"`
	SessionID       string    `json:"sessionId,omitempty"`
	TurnID          string    `json:"turnId,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
}

func toModelPriceDTO(p models.WorkforceModelPrice) WorkforceModelPriceDTO {
	return WorkforceModelPriceDTO{
		Model:         p.Model,
		InputPerMTok:  p.InputPerMTok,
		OutputPerMTok: p.OutputPerMTok,
		CreatedAt:     p.CreatedAt,
		UpdatedAt:     p.UpdatedAt,
	}
}

func toSpendEventDTO(e models.WorkforceSpendEvent) WorkforceSpendEventDTO {
	dto := WorkforceSpendEventDTO{
		ID:              e.ID,
		AgentID:         e.AgentID,
		Model:           e.Model,
		InputTokens:     e.InputTokens,
		OutputTokens:    e.OutputTokens,
		ReasoningTokens: e.ReasoningTokens,
		CostUSD:         e.CostUSD,
		Source:          e.Source,
		SessionID:       e.SessionID,
		CreatedAt:       e.CreatedAt,
	}
	if e.TurnID != nil {
		dto.TurnID = *e.TurnID
	}
	return dto
}

// ListModelPricesInput input for listing model prices.
type ListModelPricesInput struct {
	Ctx huma.Context `json:"-"`
}

func (i *ListModelPricesInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

// ListModelPricesOutput output for model prices.
type ListModelPricesOutput struct {
	Body struct {
		Items []WorkforceModelPriceDTO `json:"items"`
	}
}

func (h *Handler) listModelPrices(ctx context.Context, input *ListModelPricesInput) (*ListModelPricesOutput, error) {
	r, orgID, store, err := h.workforceCaller(input.Ctx)
	if err != nil {
		return nil, err
	}

	prices, err := store.ListModelPrices(r.Context(), orgID)
	if err != nil {
		return nil, huma.Error500InternalServerError("Could not read model prices; retry the request")
	}

	out := &ListModelPricesOutput{}
	out.Body.Items = make([]WorkforceModelPriceDTO, 0, len(prices))
	for _, p := range prices {
		out.Body.Items = append(out.Body.Items, toModelPriceDTO(p))
	}
	return out, nil
}

// SetModelPriceInput input for creating or replacing a model price.
type SetModelPriceInput struct {
	Ctx  huma.Context `json:"-"`
	Body struct {
		Model         string  `json:"model"`
		InputPerMTok  float64 `json:"inputPerMTok"`
		OutputPerMTok float64 `json:"outputPerMTok"`
	}
}

func (i *SetModelPriceInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

// SetModelPriceOutput output for the stored price.
type SetModelPriceOutput struct {
	Body WorkforceModelPriceDTO
}

func (h *Handler) setModelPrice(ctx context.Context, input *SetModelPriceInput) (*SetModelPriceOutput, error) {
	r, orgID, store, err := h.workforceCaller(input.Ctx)
	if err != nil {
		return nil, err
	}

	if input.Body.Model == "" {
		return nil, huma.Error400BadRequest("Model is required.")
	}
	if input.Body.InputPerMTok < 0 || input.Body.OutputPerMTok < 0 {
		return nil, huma.Error400BadRequest("Prices cannot be negative.")
	}

	price, err := store.SetModelPrice(r.Context(), orgID, input.Body.Model, input.Body.InputPerMTok, input.Body.OutputPerMTok)
	if err != nil {
		if errors.Is(err, agentstore.ErrInvalidPrice) {
			return nil, huma.Error400BadRequest("Model must have 1 to 64 characters. The price is out of range.")
		}
		return nil, huma.Error500InternalServerError("Could not save model price; retry the request")
	}
	h.auditAs(r, orgID, h.auth.UserID(r), "workforce.price.set", "workforce_model_price", 0, map[string]any{
		"model": price.Model, "inputPerMTok": price.InputPerMTok, "outputPerMTok": price.OutputPerMTok,
	})

	return &SetModelPriceOutput{Body: toModelPriceDTO(*price)}, nil
}

// DeleteModelPriceInput input for deleting a model price.
type DeleteModelPriceInput struct {
	Ctx   huma.Context `json:"-"`
	Model string       `query:"model"`
}

func (i *DeleteModelPriceInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

// DeleteModelPriceOutput output for a deleted price.
type DeleteModelPriceOutput struct {
	Body struct {
		Ok bool `json:"ok"`
	}
}

func (h *Handler) deleteModelPrice(ctx context.Context, input *DeleteModelPriceInput) (*DeleteModelPriceOutput, error) {
	r, orgID, store, err := h.workforceCaller(input.Ctx)
	if err != nil {
		return nil, err
	}

	if input.Model == "" {
		return nil, huma.Error400BadRequest("Model query parameter is required.")
	}
	if err := store.DeleteModelPrice(r.Context(), orgID, input.Model); err != nil {
		if errors.Is(err, agentstore.ErrModelPriceNotFound) {
			return nil, huma.Error404NotFound("No price is configured for this model in this workspace.")
		}
		return nil, huma.Error500InternalServerError("Could not delete model price; retry the request")
	}
	h.auditAs(r, orgID, h.auth.UserID(r), "workforce.price.delete", "workforce_model_price", 0, map[string]any{"model": input.Model})

	out := &DeleteModelPriceOutput{}
	out.Body.Ok = true
	return out, nil
}

// ListAgentSpendInput input for listing an agent's spend ledger.
type ListAgentSpendInput struct {
	Ctx   huma.Context `json:"-"`
	ID    string       `path:"id"`
	Limit int          `query:"limit,omitempty"`
}

func (i *ListAgentSpendInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

// ListAgentSpendOutput output for the spend ledger.
type ListAgentSpendOutput struct {
	Body struct {
		Items []WorkforceSpendEventDTO `json:"items"`
	}
}

func (h *Handler) listAgentSpend(ctx context.Context, input *ListAgentSpendInput) (*ListAgentSpendOutput, error) {
	r, orgID, store, err := h.workforceCaller(input.Ctx)
	if err != nil {
		return nil, err
	}

	if input.Limit < 0 || input.Limit > 500 {
		return nil, huma.Error400BadRequest("Limit must be between 1 and 500.")
	}

	events, err := store.ListSpendEvents(r.Context(), orgID, input.ID, input.Limit)
	if err != nil {
		if errors.Is(err, agentstore.ErrAgentNotFound) {
			return nil, huma.Error404NotFound("No such workforce agent found in this workspace.")
		}
		return nil, huma.Error500InternalServerError("Could not read agent spend; retry the request")
	}

	out := &ListAgentSpendOutput{}
	out.Body.Items = make([]WorkforceSpendEventDTO, 0, len(events))
	for _, e := range events {
		out.Body.Items = append(out.Body.Items, toSpendEventDTO(e))
	}
	return out, nil
}

// RecordAgentSpendInput input for reporting usage of one agent turn.
//
// There is deliberately no cost field: the runner reports tokens, and the
// server prices them. A caller cannot set what it owes.
type RecordAgentSpendInput struct {
	Ctx  huma.Context `json:"-"`
	ID   string       `path:"id"`
	Body struct {
		Model           string `json:"model"`
		InputTokens     int    `json:"inputTokens"`
		OutputTokens    int    `json:"outputTokens"`
		ReasoningTokens int    `json:"reasoningTokens,omitempty"`
		// TurnID is the runner's identifier for this turn. It is required, so a
		// retried report answers with the original charge instead of billing the
		// same work twice.
		TurnID string `json:"turnId"`
	}
}

func (i *RecordAgentSpendInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

// RecordAgentSpendOutput output for a recorded turn.
type RecordAgentSpendOutput struct {
	Body struct {
		Event WorkforceSpendEventDTO `json:"event"`
		Agent WorkforceAgentDTO      `json:"agent"`
	}
}

func (h *Handler) recordAgentSpend(ctx context.Context, input *RecordAgentSpendInput) (*RecordAgentSpendOutput, error) {
	r, orgID, store, err := h.workforceCaller(input.Ctx)
	if err != nil {
		return nil, err
	}

	if input.Body.Model == "" {
		return nil, huma.Error400BadRequest("Model is required.")
	}
	turnID := strings.TrimSpace(input.Body.TurnID)
	if turnID == "" || len(turnID) > 64 {
		return nil, huma.Error400BadRequest("Turn ID must have 1 to 64 characters.")
	}
	if input.Body.InputTokens < 0 || input.Body.OutputTokens < 0 || input.Body.ReasoningTokens < 0 {
		return nil, huma.Error400BadRequest("Token counts cannot be negative.")
	}
	if input.Body.ReasoningTokens > input.Body.OutputTokens {
		return nil, huma.Error400BadRequest("Reasoning tokens cannot exceed output tokens.")
	}

	event, agent, err := store.RecordSpend(r.Context(), orgID, agentstore.SpendInput{
		AgentID:         input.ID,
		Model:           input.Body.Model,
		InputTokens:     input.Body.InputTokens,
		OutputTokens:    input.Body.OutputTokens,
		ReasoningTokens: input.Body.ReasoningTokens,
		Source:          models.WorkforceSpendSourceRuntime,
		TurnID:          turnID,
	})
	replayed := false
	if errors.Is(err, agentstore.ErrSpendReplayed) {
		// The first report owns the charge. Answer with that row so a retrying
		// runner sees the same result instead of an error it would retry again.
		replayed = true
		event, agent, err = store.SpendByTurn(r.Context(), orgID, input.ID, turnID)
		if err == nil {
			input.Ctx.SetStatus(http.StatusOK)
		}
	}
	if err != nil {
		switch {
		case errors.Is(err, agentstore.ErrAgentNotFound):
			return nil, huma.Error404NotFound("No such workforce agent found in this workspace.")
		case errors.Is(err, agentstore.ErrModelPriceMissing):
			return nil, huma.Error400BadRequest("No price is configured for this model. Add a price before you record spend.")
		case errors.Is(err, agentstore.ErrInvalidSpend):
			return nil, huma.Error400BadRequest("Model must have 1 to 64 characters. Token counts cannot be negative.")
		default:
			return nil, huma.Error500InternalServerError("Could not record agent spend; retry the request")
		}
	}
	if !replayed {
		h.auditAs(r, orgID, h.auth.UserID(r), "workforce.spend.record", "workforce_agent", 0, map[string]any{
			"agentId": event.AgentID, "model": event.Model, "costUsd": event.CostUSD, "source": event.Source,
		})
	}

	out := &RecordAgentSpendOutput{}
	out.Body.Event = toSpendEventDTO(*event)
	out.Body.Agent = toAgentDTO(*agent)
	return out, nil
}
