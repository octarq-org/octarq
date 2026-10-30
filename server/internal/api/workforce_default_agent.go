package api

import (
	"context"
	"errors"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/octarq-org/octarq/server/internal/agentstore"
)

// DefaultAgentDTO is the workspace's default agent for chat attribution.
type DefaultAgentDTO struct {
	// AgentID is empty when the workspace bills no default.
	AgentID string `json:"agentId"`
}

// GetDefaultAgentInput input for reading the default agent.
type GetDefaultAgentInput struct {
	Ctx huma.Context `json:"-"`
}

func (i *GetDefaultAgentInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

// DefaultAgentOutput output for the default agent.
type DefaultAgentOutput struct {
	Body DefaultAgentDTO
}

// getDefaultAgent serves any member: a member needs to know where their turns
// are billed before they type.
func (h *Handler) getDefaultAgent(ctx context.Context, input *GetDefaultAgentInput) (*DefaultAgentOutput, error) {
	r, orgID, store, err := h.workforceMemberCaller(input.Ctx)
	if err != nil {
		return nil, err
	}

	agentID, err := store.GetDefaultAgent(r.Context(), orgID)
	if err != nil {
		return nil, huma.Error500InternalServerError("Could not read the default agent; retry the request.")
	}
	return &DefaultAgentOutput{Body: DefaultAgentDTO{AgentID: agentID}}, nil
}

// SetDefaultAgentInput input for setting or clearing the default agent.
type SetDefaultAgentInput struct {
	Ctx  huma.Context `json:"-"`
	Body struct {
		// AgentID is a pointer so an absent field is refused: decoding it into
		// "" would clear the default instead of reporting a bad request.
		AgentID *string `json:"agentId"`
	}
}

func (i *SetDefaultAgentInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

// setDefaultAgent is an admin action: it decides whose budget every member's
// chat turn spends.
func (h *Handler) setDefaultAgent(ctx context.Context, input *SetDefaultAgentInput) (*DefaultAgentOutput, error) {
	r, orgID, store, err := h.workforceCaller(input.Ctx)
	if err != nil {
		return nil, err
	}

	if input.Body.AgentID == nil {
		return nil, huma.Error400BadRequest("agentId is required. Send an empty string to clear the default.")
	}
	agentID := strings.TrimSpace(*input.Body.AgentID)
	if err := store.SetDefaultAgent(r.Context(), orgID, agentID); err != nil {
		if errors.Is(err, agentstore.ErrAgentNotFound) {
			return nil, huma.Error404NotFound("No such workforce agent found in this workspace.")
		}
		return nil, huma.Error500InternalServerError("Could not save the default agent; retry the request.")
	}
	h.auditAs(r, orgID, h.auth.UserID(r), "workforce.default_agent.set", "workspace_setting", 0, map[string]any{"agentId": agentID})

	return &DefaultAgentOutput{Body: DefaultAgentDTO{AgentID: agentID}}, nil
}
