package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/server/agent/harness"
	"github.com/octarq-org/octarq/server/internal/agentstore"
	"github.com/octarq-org/octarq/server/internal/authz"
	"github.com/octarq-org/octarq/server/internal/models"
)

// The harness admin API: an operator stages the agent harness of their
// workspace, reads the staged versions, activates one of them and keeps the
// record of what the workspace learned along the way.
//
// Staging and activating are separate on purpose. A version is not live until
// a human says so, and that human is the only gate an imported lab proposal
// has to pass.
//
// Publishing is a workspace-admin action: the harness decides what the
// workspace's assistant is told and which tools it may call.

// AgentHarnessVersion is one version of the workspace's harness.
type AgentHarnessVersion struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	Version     int    `json:"version"`
	Fingerprint string `json:"fingerprint"`
	// Status is draft / published / archived / discarded.
	Status string `json:"status"`
	// CreatedBy names the account that staged the version.
	CreatedBy string    `json:"createdBy"`
	CreatedAt time.Time `json:"createdAt"`
	// PublishedAt is null for a version that was never live.
	PublishedAt *time.Time `json:"publishedAt"`
	// Evidence is the JSON document that justifies the version, or null.
	Evidence json.RawMessage `json:"evidence"`
}

// AgentHarnessSpecDocument is a version together with the spec it carries.
type AgentHarnessSpecDocument struct {
	AgentHarnessVersion
	Spec harness.HarnessSpec `json:"spec"`
}

// AgentHarnessDraftBody is the body of a staging request.
type AgentHarnessDraftBody struct {
	Spec harness.HarnessSpec `json:"spec"`
}

// AgentHarnessProposal is a harness proposal produced elsewhere — by the
// offline lab, or by any other tool that ran the candidates and kept the
// evidence. The spec is validated exactly like a hand-written one.
type AgentHarnessProposal struct {
	// Name labels the proposal.
	Name string `json:"name"`
	// Spec is the proposed harness.
	Spec harness.HarnessSpec `json:"spec"`
	// Evidence is the free-form JSON that justifies the proposal.
	Evidence json.RawMessage `json:"evidence,omitempty"`
}

// AgentHarnessLessonBody is the body of a lesson creation request.
type AgentHarnessLessonBody struct {
	Scope    string          `json:"scope"`
	Note     string          `json:"note"`
	Evidence json.RawMessage `json:"evidence,omitempty"`
}

// AgentHarnessLesson is one thing the workspace learned about its agent.
type AgentHarnessLesson struct {
	ID     uint   `json:"id"`
	Scope  string `json:"scope"`
	Note   string `json:"note"`
	Status string `json:"status"`
	// Evidence is the JSON document that backs the lesson, or null.
	Evidence  json.RawMessage `json:"evidence"`
	CreatedBy string          `json:"createdBy"`
	CreatedAt time.Time       `json:"createdAt"`
	// SpecID is the harness version the lesson motivated, if any.
	SpecID uint `json:"specId"`
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

// StageAgentHarnessDraftInput is the request body of POST /api/agent-harness/drafts.
type StageAgentHarnessDraftInput struct {
	Ctx  huma.Context          `json:"-"`
	Body AgentHarnessDraftBody `json:"spec"`
}

// Resolve injects the request context into the input.
func (i *StageAgentHarnessDraftInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

// StageAgentHarnessDraftOutput is the response body of POST /api/agent-harness/drafts.
type StageAgentHarnessDraftOutput struct {
	Body AgentHarnessSpecDocument `json:"-"`
}

// ImportAgentHarnessProposalInput is the request body of POST /api/agent-harness/proposals.
type ImportAgentHarnessProposalInput struct {
	Ctx  huma.Context         `json:"-"`
	Body AgentHarnessProposal `json:"proposal"`
}

// Resolve injects the request context into the input.
func (i *ImportAgentHarnessProposalInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

// ImportAgentHarnessProposalOutput is the response body of POST /api/agent-harness/proposals.
type ImportAgentHarnessProposalOutput struct {
	Body AgentHarnessSpecDocument `json:"-"`
}

// ActivateAgentHarnessSpecInput is the request body of POST /api/agent-harness/specs/{id}/activate.
type ActivateAgentHarnessSpecInput struct {
	Ctx huma.Context `json:"-"`
	ID  uint         `path:"id"`
}

// Resolve injects the request context into the input.
func (i *ActivateAgentHarnessSpecInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

// ActivateAgentHarnessSpecOutput is the response body of POST /api/agent-harness/specs/{id}/activate.
type ActivateAgentHarnessSpecOutput struct {
	Body struct {
		OK bool `json:"ok"`
	} `json:"-"`
}

// DiscardAgentHarnessSpecInput is the request body of POST /api/agent-harness/specs/{id}/discard.
type DiscardAgentHarnessSpecInput struct {
	Ctx huma.Context `json:"-"`
	ID  uint         `path:"id"`
}

// Resolve injects the request context into the input.
func (i *DiscardAgentHarnessSpecInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

// DiscardAgentHarnessSpecOutput is the response body of POST /api/agent-harness/specs/{id}/discard.
type DiscardAgentHarnessSpecOutput struct {
	Body struct {
		OK bool `json:"ok"`
	} `json:"-"`
}

// ListAgentHarnessLessonsInput is the request body of GET /api/agent-harness/lessons.
type ListAgentHarnessLessonsInput struct {
	Ctx huma.Context `json:"-"`
}

// Resolve injects the request context into the input.
func (i *ListAgentHarnessLessonsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

// ListAgentHarnessLessonsOutput is the response body of GET /api/agent-harness/lessons.
type ListAgentHarnessLessonsOutput struct {
	Body struct {
		Lessons []AgentHarnessLesson `json:"lessons"`
	} `json:"-"`
}

// CreateAgentHarnessLessonInput is the request body of POST /api/agent-harness/lessons.
type CreateAgentHarnessLessonInput struct {
	Ctx  huma.Context           `json:"-"`
	Body AgentHarnessLessonBody `json:"lesson"`
}

// Resolve injects the request context into the input.
func (i *CreateAgentHarnessLessonInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

// CreateAgentHarnessLessonOutput is the response body of POST /api/agent-harness/lessons.
type CreateAgentHarnessLessonOutput struct {
	Body AgentHarnessLesson `json:"-"`
}

// DismissAgentHarnessLessonInput is the request body of POST /api/agent-harness/lessons/{id}/dismiss.
type DismissAgentHarnessLessonInput struct {
	Ctx huma.Context `json:"-"`
	ID  uint         `path:"id"`
}

// Resolve injects the request context into the input.
func (i *DismissAgentHarnessLessonInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

// DismissAgentHarnessLessonOutput is the response body of POST /api/agent-harness/lessons/{id}/dismiss.
type DismissAgentHarnessLessonOutput struct {
	Body struct {
		OK bool `json:"ok"`
	} `json:"-"`
}

// rawJSONOrNull turns stored evidence into a JSON value: null when there is
// none, the document when it parses, and the raw text as a string when it
// does not (a caller must never receive invalid JSON).
func rawJSONOrNull(text string) json.RawMessage {
	if text == "" {
		return nil
	}
	if json.Valid([]byte(text)) {
		return json.RawMessage(text)
	}
	quoted, err := json.Marshal(text)
	if err != nil {
		return nil
	}
	return json.RawMessage(quoted)
}

// harnessVersion renders a stored row as the API's version shape. The author
// is resolved to an account name; a row whose account is gone keeps a stable
// placeholder rather than an empty field.
func (h *Handler) harnessVersion(row models.AgentHarnessSpec) AgentHarnessVersion {
	return AgentHarnessVersion{
		ID:          row.ID,
		Name:        row.Name,
		Version:     row.Version,
		Fingerprint: row.Fingerprint,
		Status:      row.Status,
		CreatedBy:   h.actorName(row.CreatedBy),
		CreatedAt:   row.CreatedAt,
		PublishedAt: row.PublishedAt,
		Evidence:    rawJSONOrNull(row.Evidence),
	}
}

// harnessLesson renders a stored lesson as the API's shape.
func (h *Handler) harnessLesson(row models.AgentHarnessLesson) AgentHarnessLesson {
	return AgentHarnessLesson{
		ID:        row.ID,
		Scope:     row.Scope,
		Note:      row.Note,
		Status:    row.Status,
		Evidence:  rawJSONOrNull(row.Evidence),
		CreatedBy: h.actorName(row.CreatedBy),
		CreatedAt: row.CreatedAt,
		SpecID:    row.SpecID,
	}
}

// harnessDocument renders a stored row with the spec it carries.
func (h *Handler) harnessDocument(row models.AgentHarnessSpec) (AgentHarnessSpecDocument, error) {
	spec, err := harness.DecodeSpec([]byte(row.SpecJSON))
	if err != nil {
		// A stored spec that cannot be decoded is a broken row: report it
		// rather than pretending the version carries the default.
		return AgentHarnessSpecDocument{}, huma.Error500InternalServerError("This harness version cannot be read; stage a new one.")
	}
	return AgentHarnessSpecDocument{AgentHarnessVersion: h.harnessVersion(row), Spec: spec}, nil
}

// actorName resolves a user id to the account name an operator recognises.
func (h *Handler) actorName(userID uint) string {
	if userID == 0 {
		return "system"
	}
	if h == nil || h.db == nil {
		return fmt.Sprintf("user #%d", userID)
	}
	var user models.User
	if err := h.db.Select("id", "email").First(&user, userID).Error; err != nil {
		return fmt.Sprintf("user #%d", userID)
	}
	return user.Email
}

// harnessCaller authenticates the caller and resolves the workspace and the
// role the operation needs. Every harness endpoint starts here.
func (h *Handler) harnessCaller(ctx huma.Context) (*http.Request, uint, error) {
	if ctx == nil {
		return nil, 0, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, 0, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}
	if err := h.requireRole(r, authz.RoleAdmin); err != nil {
		return nil, 0, err
	}
	orgID, err := h.requireOrg(r)
	if err != nil {
		return nil, 0, err
	}
	if h.harnessStore == nil {
		return nil, 0, huma.Error500InternalServerError("The agent harness store is not configured; ask an administrator.")
	}
	return r, orgID, nil
}

// listAgentHarnessSpecs lists the versions of the workspace.
func (h *Handler) listAgentHarnessSpecs(ctx context.Context, input *ListAgentHarnessSpecsInput) (*ListAgentHarnessSpecsOutput, error) {
	r, orgID, err := h.harnessCaller(input.Ctx)
	if err != nil {
		return nil, err
	}

	rows, err := h.harnessStore.List(r.Context(), orgID)
	if err != nil {
		return nil, huma.Error500InternalServerError("Could not read the harness versions; retry the request")
	}

	out := &ListAgentHarnessSpecsOutput{}
	out.Body.Versions = make([]AgentHarnessVersion, 0, len(rows))
	for _, row := range rows {
		out.Body.Versions = append(out.Body.Versions, h.harnessVersion(row))
		if row.Status == models.AgentHarnessStatusPublished {
			active := h.harnessVersion(row)
			out.Body.Active = &active
		}
	}
	return out, nil
}

// getAgentHarnessSpec returns one version of the workspace.
func (h *Handler) getAgentHarnessSpec(ctx context.Context, input *GetAgentHarnessSpecInput) (*GetAgentHarnessSpecOutput, error) {
	r, orgID, err := h.harnessCaller(input.Ctx)
	if err != nil {
		return nil, err
	}

	row, err := h.harnessStore.Get(r.Context(), orgID, input.ID)
	if err != nil {
		if errors.Is(err, agentstore.ErrNotFound) {
			return nil, huma.Error404NotFound("No such harness version")
		}
		return nil, huma.Error500InternalServerError("Could not read the harness version; retry the request")
	}
	doc, err := h.harnessDocument(*row)
	if err != nil {
		return nil, err
	}
	return &GetAgentHarnessSpecOutput{Body: doc}, nil
}

// stageAgentHarnessDraft stores a spec as a draft — a version that is not
// live until somebody activates it.
func (h *Handler) stageAgentHarnessDraft(ctx context.Context, input *StageAgentHarnessDraftInput) (*StageAgentHarnessDraftOutput, error) {
	r, orgID, err := h.harnessCaller(input.Ctx)
	if err != nil {
		return nil, err
	}

	spec := input.Body.Spec
	if err := harness.Validate(spec); err != nil {
		return nil, huma.Error400BadRequest("The harness spec is invalid: " + err.Error())
	}

	row, err := h.harnessStore.Stage(r.Context(), orgID, spec, h.auth.UserID(r))
	if err != nil {
		return nil, huma.Error500InternalServerError("Could not stage the harness version; retry the request")
	}

	doc, err := h.harnessDocument(*row)
	if err != nil {
		return nil, err
	}
	h.auditAs(r, orgID, h.auth.UserID(r), "agent.harness.stage", "agent_harness", row.ID, map[string]any{
		"version":     row.Version,
		"fingerprint": row.Fingerprint,
	})
	return &StageAgentHarnessDraftOutput{Body: doc}, nil
}

// importAgentHarnessProposal stores a lab proposal as a draft, keeping the
// evidence that came with it.
func (h *Handler) importAgentHarnessProposal(ctx context.Context, input *ImportAgentHarnessProposalInput) (*ImportAgentHarnessProposalOutput, error) {
	r, orgID, err := h.harnessCaller(input.Ctx)
	if err != nil {
		return nil, err
	}

	spec := input.Body.Spec
	if err := harness.Validate(spec); err != nil {
		return nil, huma.Error400BadRequest("The proposed harness spec is invalid: " + err.Error())
	}

	row, err := h.harnessStore.ImportProposal(r.Context(), orgID, spec, input.Body.Evidence, h.auth.UserID(r))
	if err != nil {
		return nil, huma.Error500InternalServerError("Could not import the harness proposal; retry the request")
	}

	// The proposal is also a lesson: it records what was tried and why.
	if _, err := h.harnessStore.CreateLesson(r.Context(), orgID, "harness", "Proposal imported: "+spec.Name, input.Body.Evidence, row.ID, h.auth.UserID(r)); err != nil {
		// A lesson that cannot be written does not invalidate the import,
		// but it must not disappear silently either.
		h.auditAs(r, orgID, h.auth.UserID(r), "agent.harness.lesson_failed", "agent_harness", row.ID, map[string]any{
			"reason": err.Error(),
		})
	}

	doc, err := h.harnessDocument(*row)
	if err != nil {
		return nil, err
	}
	h.auditAs(r, orgID, h.auth.UserID(r), "agent.harness.import", "agent_harness", row.ID, map[string]any{
		"version":     row.Version,
		"fingerprint": row.Fingerprint,
	})
	return &ImportAgentHarnessProposalOutput{Body: doc}, nil
}

// activateAgentHarnessSpec makes a version the live one.
func (h *Handler) activateAgentHarnessSpec(ctx context.Context, input *ActivateAgentHarnessSpecInput) (*ActivateAgentHarnessSpecOutput, error) {
	r, orgID, err := h.harnessCaller(input.Ctx)
	if err != nil {
		return nil, err
	}

	row, err := h.harnessStore.Activate(r.Context(), orgID, input.ID, h.auth.UserID(r))
	if err != nil {
		switch {
		case errors.Is(err, agentstore.ErrNotFound):
			return nil, huma.Error404NotFound("No such harness version")
		case errors.Is(err, agentstore.ErrInvalidState):
			return nil, huma.Error400BadRequest(err.Error())
		}
		return nil, huma.Error500InternalServerError("Could not activate the harness version; retry the request")
	}

	h.auditAs(r, orgID, h.auth.UserID(r), "agent.harness.activate", "agent_harness", row.ID, map[string]any{
		"version":     row.Version,
		"fingerprint": row.Fingerprint,
	})
	out := &ActivateAgentHarnessSpecOutput{}
	out.Body.OK = true
	return out, nil
}

// discardAgentHarnessSpec throws a draft away.
func (h *Handler) discardAgentHarnessSpec(ctx context.Context, input *DiscardAgentHarnessSpecInput) (*DiscardAgentHarnessSpecOutput, error) {
	r, orgID, err := h.harnessCaller(input.Ctx)
	if err != nil {
		return nil, err
	}

	row, err := h.harnessStore.Discard(r.Context(), orgID, input.ID)
	if err != nil {
		switch {
		case errors.Is(err, agentstore.ErrNotFound):
			return nil, huma.Error404NotFound("No such harness version")
		case errors.Is(err, agentstore.ErrInvalidState):
			return nil, huma.Error400BadRequest(err.Error())
		}
		return nil, huma.Error500InternalServerError("Could not discard the harness version; retry the request")
	}

	h.auditAs(r, orgID, h.auth.UserID(r), "agent.harness.discard", "agent_harness", row.ID, map[string]any{
		"version": row.Version,
	})
	out := &DiscardAgentHarnessSpecOutput{}
	out.Body.OK = true
	return out, nil
}

// listAgentHarnessLessons lists what the workspace learned about its agent.
func (h *Handler) listAgentHarnessLessons(ctx context.Context, input *ListAgentHarnessLessonsInput) (*ListAgentHarnessLessonsOutput, error) {
	r, orgID, err := h.harnessCaller(input.Ctx)
	if err != nil {
		return nil, err
	}

	rows, err := h.harnessStore.ListLessons(r.Context(), orgID)
	if err != nil {
		return nil, huma.Error500InternalServerError("Could not read the lessons; retry the request")
	}

	out := &ListAgentHarnessLessonsOutput{}
	out.Body.Lessons = make([]AgentHarnessLesson, 0, len(rows))
	for _, row := range rows {
		out.Body.Lessons = append(out.Body.Lessons, h.harnessLesson(row))
	}
	return out, nil
}

// createAgentHarnessLesson records a lesson.
func (h *Handler) createAgentHarnessLesson(ctx context.Context, input *CreateAgentHarnessLessonInput) (*CreateAgentHarnessLessonOutput, error) {
	r, orgID, err := h.harnessCaller(input.Ctx)
	if err != nil {
		return nil, err
	}

	if input.Body.Scope == "" || input.Body.Note == "" {
		return nil, huma.Error400BadRequest("A lesson needs a scope and a note.")
	}

	row, err := h.harnessStore.CreateLesson(r.Context(), orgID, input.Body.Scope, input.Body.Note, input.Body.Evidence, 0, h.auth.UserID(r))
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	h.auditAs(r, orgID, h.auth.UserID(r), "agent.harness.lesson", "agent_harness", row.ID, map[string]any{
		"scope": row.Scope,
	})
	return &CreateAgentHarnessLessonOutput{Body: h.harnessLesson(*row)}, nil
}

// dismissAgentHarnessLesson marks a lesson as rejected.
func (h *Handler) dismissAgentHarnessLesson(ctx context.Context, input *DismissAgentHarnessLessonInput) (*DismissAgentHarnessLessonOutput, error) {
	r, orgID, err := h.harnessCaller(input.Ctx)
	if err != nil {
		return nil, err
	}

	row, err := h.harnessStore.DismissLesson(r.Context(), orgID, input.ID)
	if err != nil {
		if errors.Is(err, agentstore.ErrNotFound) {
			return nil, huma.Error404NotFound("No such lesson")
		}
		return nil, huma.Error500InternalServerError("Could not dismiss the lesson; retry the request")
	}

	h.auditAs(r, orgID, h.auth.UserID(r), "agent.harness.lesson_dismissed", "agent_harness", row.ID, nil)
	out := &DismissAgentHarnessLessonOutput{}
	out.Body.OK = true
	return out, nil
}
