package api

import (
	"context"
	"errors"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/server/agent/harness"
	"github.com/octarq-org/octarq/server/internal/agentstore"
	"github.com/octarq-org/octarq/server/internal/authz"
	"github.com/octarq-org/octarq/server/internal/models"
)

// The harness admin API: an operator publishes the agent harness of their
// workspace, lists the published versions and rolls back to an older one.
//
// Publishing is a workspace-admin action: the harness decides what the
// workspace's assistant is told and which tools it may call.

// AgentHarnessVersion is one published harness version.
type AgentHarnessVersion struct {
	ID          uint       `json:"id"`
	Name        string     `json:"name"`
	Version     int        `json:"version"`
	Fingerprint string     `json:"fingerprint"`
	Status      string     `json:"status"`
	CreatedBy   uint       `json:"createdBy"`
	CreatedAt   time.Time  `json:"createdAt"`
	PublishedAt *time.Time `json:"publishedAt"`
}

// AgentHarnessSpecDocument is a version together with the spec it carries.
type AgentHarnessSpecDocument struct {
	AgentHarnessVersion
	Spec harness.HarnessSpec `json:"spec"`
}

// ListAgentHarnessSpecsInput is the request body of GET /api/agent-harness/specs.
type ListAgentHarnessSpecsInput struct {
	Ctx huma.Context `json:"-"`
}

// Resolve injects the request context into the input.
func (i *ListAgentHarnessSpecsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

// ListAgentHarnessSpecsOutput is the response body of GET /api/agent-harness/specs.
type ListAgentHarnessSpecsOutput struct {
	Body struct {
		Active   *AgentHarnessVersion  `json:"active"`
		Versions []AgentHarnessVersion `json:"versions"`
	} `json:"-"`
}

// GetAgentHarnessSpecInput is the request body of GET /api/agent-harness/specs/{id}.
type GetAgentHarnessSpecInput struct {
	Ctx huma.Context `json:"-"`
	ID  uint         `path:"id"`
}

// Resolve injects the request context into the input.
func (i *GetAgentHarnessSpecInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

// GetAgentHarnessSpecOutput is the response body of GET /api/agent-harness/specs/{id}.
type GetAgentHarnessSpecOutput struct {
	Body AgentHarnessSpecDocument `json:"-"`
}

// PublishAgentHarnessSpecInput is the request body of POST /api/agent-harness/specs.
type PublishAgentHarnessSpecInput struct {
	Ctx  huma.Context        `json:"-"`
	Body harness.HarnessSpec `json:"spec"`
}

// Resolve injects the request context into the input.
func (i *PublishAgentHarnessSpecInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

// PublishAgentHarnessSpecOutput is the response body of POST /api/agent-harness/specs.
type PublishAgentHarnessSpecOutput struct {
	Body AgentHarnessSpecDocument `json:"-"`
}

// RollbackAgentHarnessSpecInput is the request body of POST /api/agent-harness/specs/{id}/rollback.
type RollbackAgentHarnessSpecInput struct {
	Ctx huma.Context `json:"-"`
	ID  uint         `path:"id"`
}

// Resolve injects the request context into the input.
func (i *RollbackAgentHarnessSpecInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

// RollbackAgentHarnessSpecOutput is the response body of POST /api/agent-harness/specs/{id}/rollback.
type RollbackAgentHarnessSpecOutput struct {
	Body struct {
		OK bool `json:"ok"`
	} `json:"-"`
}

// harnessVersion renders a stored row as the API's version shape.
func harnessVersion(row models.AgentHarnessSpec) AgentHarnessVersion {
	return AgentHarnessVersion{
		ID:          row.ID,
		Name:        row.Name,
		Version:     row.Version,
		Fingerprint: row.Fingerprint,
		Status:      row.Status,
		CreatedBy:   row.CreatedBy,
		CreatedAt:   row.CreatedAt,
		PublishedAt: row.PublishedAt,
	}
}

// harnessDocument renders a stored row with the spec it carries.
func harnessDocument(row models.AgentHarnessSpec) (AgentHarnessSpecDocument, error) {
	spec, err := harness.DecodeSpec([]byte(row.SpecJSON))
	if err != nil {
		// A stored spec that cannot be decoded is a broken row: report it
		// rather than pretending the version carries the default.
		return AgentHarnessSpecDocument{}, huma.Error500InternalServerError("This harness version cannot be read; publish a new one.")
	}
	return AgentHarnessSpecDocument{AgentHarnessVersion: harnessVersion(row), Spec: spec}, nil
}

// listAgentHarnessSpecs lists the published versions of the workspace.
func (h *Handler) listAgentHarnessSpecs(ctx context.Context, input *ListAgentHarnessSpecsInput) (*ListAgentHarnessSpecsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}
	if err := h.requireRole(r, authz.RoleAdmin); err != nil {
		return nil, err
	}
	orgID, err := h.requireOrg(r)
	if err != nil {
		return nil, err
	}
	if h.harnessStore == nil {
		return nil, huma.Error500InternalServerError("The agent harness store is not configured; ask an administrator.")
	}

	rows, err := h.harnessStore.List(r.Context(), orgID)
	if err != nil {
		return nil, huma.Error500InternalServerError("Could not read the harness versions; retry the request")
	}

	out := &ListAgentHarnessSpecsOutput{}
	out.Body.Versions = make([]AgentHarnessVersion, 0, len(rows))
	for _, row := range rows {
		out.Body.Versions = append(out.Body.Versions, harnessVersion(row))
		if row.Status == models.AgentHarnessStatusPublished {
			active := harnessVersion(row)
			out.Body.Active = &active
		}
	}
	return out, nil
}

// getAgentHarnessSpec returns one version of the workspace.
func (h *Handler) getAgentHarnessSpec(ctx context.Context, input *GetAgentHarnessSpecInput) (*GetAgentHarnessSpecOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}
	if err := h.requireRole(r, authz.RoleAdmin); err != nil {
		return nil, err
	}
	orgID, err := h.requireOrg(r)
	if err != nil {
		return nil, err
	}
	if h.harnessStore == nil {
		return nil, huma.Error500InternalServerError("The agent harness store is not configured; ask an administrator.")
	}

	row, err := h.harnessStore.Get(r.Context(), orgID, input.ID)
	if err != nil {
		if errors.Is(err, agentstore.ErrNotFound) {
			return nil, huma.Error404NotFound("No such harness version")
		}
		return nil, huma.Error500InternalServerError("Could not read the harness version; retry the request")
	}
	doc, err := harnessDocument(*row)
	if err != nil {
		return nil, err
	}
	return &GetAgentHarnessSpecOutput{Body: doc}, nil
}

// publishAgentHarnessSpec validates a spec and publishes it as the active
// version of the workspace.
func (h *Handler) publishAgentHarnessSpec(ctx context.Context, input *PublishAgentHarnessSpecInput) (*PublishAgentHarnessSpecOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}
	if err := h.requireRole(r, authz.RoleAdmin); err != nil {
		return nil, err
	}
	orgID, err := h.requireOrg(r)
	if err != nil {
		return nil, err
	}
	if h.harnessStore == nil {
		return nil, huma.Error500InternalServerError("The agent harness store is not configured; ask an administrator.")
	}

	spec := input.Body
	if err := harness.Validate(spec); err != nil {
		return nil, huma.Error400BadRequest("The harness spec is invalid: " + err.Error())
	}

	row, err := h.harnessStore.Publish(r.Context(), orgID, spec, h.auth.UserID(r))
	if err != nil {
		return nil, huma.Error500InternalServerError("Could not publish the harness spec; retry the request")
	}

	doc, err := harnessDocument(*row)
	if err != nil {
		return nil, err
	}
	h.auditAs(r, orgID, h.auth.UserID(r), "agent.harness.publish", "agent_harness", row.ID, map[string]any{
		"version":     row.Version,
		"fingerprint": row.Fingerprint,
	})
	return &PublishAgentHarnessSpecOutput{Body: doc}, nil
}

// rollbackAgentHarnessSpec makes an archived version the active one.
func (h *Handler) rollbackAgentHarnessSpec(ctx context.Context, input *RollbackAgentHarnessSpecInput) (*RollbackAgentHarnessSpecOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}
	if err := h.requireRole(r, authz.RoleAdmin); err != nil {
		return nil, err
	}
	orgID, err := h.requireOrg(r)
	if err != nil {
		return nil, err
	}
	if h.harnessStore == nil {
		return nil, huma.Error500InternalServerError("The agent harness store is not configured; ask an administrator.")
	}

	row, err := h.harnessStore.Rollback(r.Context(), orgID, input.ID, h.auth.UserID(r))
	if err != nil {
		if errors.Is(err, agentstore.ErrNotFound) {
			return nil, huma.Error404NotFound("No such harness version")
		}
		return nil, huma.Error500InternalServerError("Could not roll back the harness version; retry the request")
	}

	h.auditAs(r, orgID, h.auth.UserID(r), "agent.harness.rollback", "agent_harness", row.ID, map[string]any{
		"version":     row.Version,
		"fingerprint": row.Fingerprint,
	})
	out := &RollbackAgentHarnessSpecOutput{}
	out.Body.OK = true
	return out, nil
}
