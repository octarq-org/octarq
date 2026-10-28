package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/server/internal/agentstore"
	"github.com/octarq-org/octarq/server/internal/authz"
	"github.com/octarq-org/octarq/server/internal/models"
)

// WorkforceAgentDTO represents the API response for an AI workforce agent.
type WorkforceAgentDTO struct {
	ID              string     `json:"id"`
	OrgID           uint       `json:"orgId"`
	Name            string     `json:"name"`
	Role            string     `json:"role"`
	Department      string     `json:"department"`
	Model           string     `json:"model"`
	Status          string     `json:"status"`
	Heartbeat       string     `json:"heartbeat"`
	BudgetLimit     float64    `json:"budgetLimit"`
	BudgetUsed      float64    `json:"budgetUsed"`
	Capabilities    []string   `json:"capabilities"`
	Directives      string     `json:"directives"`
	LastHeartbeatAt *time.Time `json:"lastHeartbeatAt,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

// WorkforceTaskDTO represents the API response for an autonomous task ticket.
type WorkforceTaskDTO struct {
	ID              string     `json:"id"`
	OrgID           uint       `json:"orgId"`
	Title           string     `json:"title"`
	AssigneeAgentID string     `json:"assigneeAgentId"`
	Assignee        string     `json:"assignee"`
	AssigneeRole    string     `json:"assigneeRole"`
	Priority        string     `json:"priority"`
	Status          string     `json:"status"`
	Summary         string     `json:"summary"`
	Context         string     `json:"context,omitempty"`
	LastHeartbeat   string     `json:"lastHeartbeat"`
	LastHeartbeatAt *time.Time `json:"lastHeartbeatAt,omitempty"`
	ExecutionSteps  []string   `json:"executionSteps,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

// WorkforceTaskStepDTO represents an execution step log in an agent run.
type WorkforceTaskStepDTO struct {
	ID         uint      `json:"id"`
	TaskID     string    `json:"taskId"`
	StepNumber int       `json:"stepNumber"`
	StepText   string    `json:"stepText"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"createdAt"`
}

// WorkforceOverviewDTO represents executive summary metrics.
type WorkforceOverviewDTO struct {
	ActiveAgents     int     `json:"activeAgents"`
	RunningTasks     int     `json:"runningTasks"`
	QueuedTasks      int     `json:"queuedTasks"`
	PendingApprovals int     `json:"pendingApprovals"`
	MonthlySpend     float64 `json:"monthlySpend"`
	MonthlyLimit     float64 `json:"monthlyLimit"`
	AutoHalt         bool    `json:"autoHalt"`
}

func toAgentDTO(a models.WorkforceAgent) WorkforceAgentDTO {
	caps := []string(a.Capabilities)
	if caps == nil {
		caps = []string{}
	}
	return WorkforceAgentDTO{
		ID:              a.ID,
		OrgID:           a.OrgID,
		Name:            a.Name,
		Role:            a.Role,
		Department:      a.Department,
		Model:           a.Model,
		Status:          a.Status,
		Heartbeat:       a.Heartbeat,
		BudgetLimit:     a.BudgetLimit,
		BudgetUsed:      a.BudgetUsed,
		Capabilities:    caps,
		Directives:      a.Directives,
		LastHeartbeatAt: a.LastHeartbeatAt,
		CreatedAt:       a.CreatedAt,
		UpdatedAt:       a.UpdatedAt,
	}
}

func toTaskDTO(t models.WorkforceTask, steps []models.WorkforceTaskStep) WorkforceTaskDTO {
	stepTexts := make([]string, 0, len(steps))
	for _, s := range steps {
		stepTexts = append(stepTexts, s.StepText)
	}
	return WorkforceTaskDTO{
		ID:              t.ID,
		OrgID:           t.OrgID,
		Title:           t.Title,
		AssigneeAgentID: t.AssigneeAgentID,
		Assignee:        t.AssigneeName,
		AssigneeRole:    t.AssigneeRole,
		Priority:        t.Priority,
		Status:          t.Status,
		Summary:         t.Summary,
		Context:         t.Context,
		LastHeartbeat:   t.LastHeartbeat,
		LastHeartbeatAt: t.LastHeartbeatAt,
		ExecutionSteps:  stepTexts,
		CreatedAt:       t.CreatedAt,
		UpdatedAt:       t.UpdatedAt,
	}
}

func (h *Handler) workforceCaller(ctx huma.Context) (*http.Request, uint, *agentstore.WorkforceStore, error) {
	if ctx == nil {
		return nil, 0, nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, 0, nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}
	if err := h.requireRole(r, authz.RoleAdmin); err != nil {
		return nil, 0, nil, err
	}
	orgID, err := h.requireOrg(r)
	if err != nil {
		return nil, 0, nil, err
	}
	store := h.getWorkforceStore()
	if store == nil {
		return nil, 0, nil, huma.Error500InternalServerError("The workforce store is not configured; retry the request.")
	}
	return r, orgID, store, nil
}

// ListWorkforceAgentsInput input for listing agents.
type ListWorkforceAgentsInput struct {
	Ctx huma.Context `json:"-"`
}

func (i *ListWorkforceAgentsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

// ListWorkforceAgentsOutput output for listing agents.
type ListWorkforceAgentsOutput struct {
	Body struct {
		Items []WorkforceAgentDTO `json:"items"`
	}
}

func (h *Handler) listWorkforceAgents(ctx context.Context, input *ListWorkforceAgentsInput) (*ListWorkforceAgentsOutput, error) {
	r, orgID, store, err := h.workforceCaller(input.Ctx)
	if err != nil {
		return nil, err
	}

	agents, err := store.ListAgents(r.Context(), orgID)
	if err != nil {
		return nil, huma.Error500InternalServerError("Could not read workforce agents; retry the request")
	}

	out := &ListWorkforceAgentsOutput{}
	out.Body.Items = make([]WorkforceAgentDTO, 0, len(agents))
	for _, a := range agents {
		out.Body.Items = append(out.Body.Items, toAgentDTO(a))
	}
	return out, nil
}

// CreateWorkforceAgentInput input for deploying an agent.
type CreateWorkforceAgentInput struct {
	Ctx  huma.Context `json:"-"`
	Body struct {
		Name         string   `json:"name"`
		Role         string   `json:"role"`
		Department   string   `json:"department,omitempty"`
		Model        string   `json:"model"`
		Directives   string   `json:"directives,omitempty"`
		Heartbeat    string   `json:"heartbeat,omitempty"`
		BudgetLimit  float64  `json:"budgetLimit,omitempty"`
		Capabilities []string `json:"capabilities,omitempty"`
	}
}

func (i *CreateWorkforceAgentInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

// CreateWorkforceAgentOutput output for deploying an agent.
type CreateWorkforceAgentOutput struct {
	Body WorkforceAgentDTO
}

func (h *Handler) createWorkforceAgent(ctx context.Context, input *CreateWorkforceAgentInput) (*CreateWorkforceAgentOutput, error) {
	r, orgID, store, err := h.workforceCaller(input.Ctx)
	if err != nil {
		return nil, err
	}

	if input.Body.Name == "" || input.Body.Role == "" || input.Body.Model == "" {
		return nil, huma.Error400BadRequest("Agent name, role, and model are required.")
	}

	agent := models.WorkforceAgent{
		Name:         input.Body.Name,
		Role:         input.Body.Role,
		Department:   input.Body.Department,
		Model:        input.Body.Model,
		Directives:   input.Body.Directives,
		Heartbeat:    input.Body.Heartbeat,
		BudgetLimit:  input.Body.BudgetLimit,
		Capabilities: models.StringList(input.Body.Capabilities),
	}
	if agent.Department == "" {
		agent.Department = "Operations"
	}
	if agent.BudgetLimit <= 0 {
		agent.BudgetLimit = 10.0
	}

	created, err := store.CreateAgent(r.Context(), orgID, agent)
	if err != nil {
		return nil, huma.Error500InternalServerError("Could not deploy workforce agent; retry the request")
	}

	return &CreateWorkforceAgentOutput{Body: toAgentDTO(*created)}, nil
}

// GetWorkforceAgentInput input for retrieving agent details.
type GetWorkforceAgentInput struct {
	Ctx huma.Context `json:"-"`
	ID  string       `path:"id"`
}

func (i *GetWorkforceAgentInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

// GetWorkforceAgentOutput output for agent details.
type GetWorkforceAgentOutput struct {
	Body WorkforceAgentDTO
}

func (h *Handler) getWorkforceAgent(ctx context.Context, input *GetWorkforceAgentInput) (*GetWorkforceAgentOutput, error) {
	r, orgID, store, err := h.workforceCaller(input.Ctx)
	if err != nil {
		return nil, err
	}

	agent, err := store.GetAgent(r.Context(), orgID, input.ID)
	if err != nil {
		if errors.Is(err, agentstore.ErrAgentNotFound) {
			return nil, huma.Error404NotFound("No such workforce agent found in this workspace.")
		}
		return nil, huma.Error500InternalServerError("Could not read workforce agent; retry the request")
	}

	return &GetWorkforceAgentOutput{Body: toAgentDTO(*agent)}, nil
}

// UpdateWorkforceAgentInput input for updating agent attributes.
type UpdateWorkforceAgentInput struct {
	Ctx  huma.Context `json:"-"`
	ID   string       `path:"id"`
	Body struct {
		Name         *string  `json:"name,omitempty"`
		Role         *string  `json:"role,omitempty"`
		Department   *string  `json:"department,omitempty"`
		Model        *string  `json:"model,omitempty"`
		Directives   *string  `json:"directives,omitempty"`
		Heartbeat    *string  `json:"heartbeat,omitempty"`
		BudgetLimit  *float64 `json:"budgetLimit,omitempty"`
		Status       *string  `json:"status,omitempty"`
		Capabilities []string `json:"capabilities,omitempty"`
	}
}

func (i *UpdateWorkforceAgentInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

// UpdateWorkforceAgentOutput output for updated agent.
type UpdateWorkforceAgentOutput struct {
	Body WorkforceAgentDTO
}

func (h *Handler) updateWorkforceAgent(ctx context.Context, input *UpdateWorkforceAgentInput) (*UpdateWorkforceAgentOutput, error) {
	r, orgID, store, err := h.workforceCaller(input.Ctx)
	if err != nil {
		return nil, err
	}

	updates := make(map[string]any)
	if input.Body.Name != nil {
		updates["name"] = *input.Body.Name
	}
	if input.Body.Role != nil {
		updates["role"] = *input.Body.Role
	}
	if input.Body.Department != nil {
		updates["department"] = *input.Body.Department
	}
	if input.Body.Model != nil {
		updates["model"] = *input.Body.Model
	}
	if input.Body.Directives != nil {
		updates["directives"] = *input.Body.Directives
	}
	if input.Body.Heartbeat != nil {
		updates["heartbeat"] = *input.Body.Heartbeat
	}
	if input.Body.BudgetLimit != nil {
		updates["budget_limit"] = *input.Body.BudgetLimit
	}
	if input.Body.Status != nil {
		updates["status"] = *input.Body.Status
	}
	if input.Body.Capabilities != nil {
		updates["capabilities"] = models.StringList(input.Body.Capabilities)
	}

	agent, err := store.UpdateAgent(r.Context(), orgID, input.ID, updates)
	if err != nil {
		if errors.Is(err, agentstore.ErrAgentNotFound) {
			return nil, huma.Error404NotFound("No such workforce agent found in this workspace.")
		}
		return nil, huma.Error500InternalServerError("Could not update workforce agent; retry the request")
	}

	return &UpdateWorkforceAgentOutput{Body: toAgentDTO(*agent)}, nil
}

// PauseWorkforceAgentInput input for pausing agent.
type PauseWorkforceAgentInput struct {
	Ctx huma.Context `json:"-"`
	ID  string       `path:"id"`
}

func (i *PauseWorkforceAgentInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

// PauseWorkforceAgentOutput output for paused agent.
type PauseWorkforceAgentOutput struct {
	Body WorkforceAgentDTO
}

func (h *Handler) pauseWorkforceAgent(ctx context.Context, input *PauseWorkforceAgentInput) (*PauseWorkforceAgentOutput, error) {
	r, orgID, store, err := h.workforceCaller(input.Ctx)
	if err != nil {
		return nil, err
	}

	agent, err := store.PauseAgent(r.Context(), orgID, input.ID)
	if err != nil {
		if errors.Is(err, agentstore.ErrAgentNotFound) {
			return nil, huma.Error404NotFound("No such workforce agent found in this workspace.")
		}
		return nil, huma.Error500InternalServerError("Could not pause workforce agent; retry the request")
	}

	return &PauseWorkforceAgentOutput{Body: toAgentDTO(*agent)}, nil
}

// ResumeWorkforceAgentInput input for resuming agent.
type ResumeWorkforceAgentInput struct {
	Ctx huma.Context `json:"-"`
	ID  string       `path:"id"`
}

func (i *ResumeWorkforceAgentInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

// ResumeWorkforceAgentOutput output for resumed agent.
type ResumeWorkforceAgentOutput struct {
	Body WorkforceAgentDTO
}

func (h *Handler) resumeWorkforceAgent(ctx context.Context, input *ResumeWorkforceAgentInput) (*ResumeWorkforceAgentOutput, error) {
	r, orgID, store, err := h.workforceCaller(input.Ctx)
	if err != nil {
		return nil, err
	}

	agent, err := store.ResumeAgent(r.Context(), orgID, input.ID)
	if err != nil {
		if errors.Is(err, agentstore.ErrAgentNotFound) {
			return nil, huma.Error404NotFound("No such workforce agent found in this workspace.")
		}
		return nil, huma.Error500InternalServerError("Could not resume workforce agent; retry the request")
	}

	return &ResumeWorkforceAgentOutput{Body: toAgentDTO(*agent)}, nil
}

// TriggerWorkforceHeartbeatInput input for triggering heartbeat.
type TriggerWorkforceHeartbeatInput struct {
	Ctx huma.Context `json:"-"`
	ID  string       `path:"id"`
}

func (i *TriggerWorkforceHeartbeatInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

// TriggerWorkforceHeartbeatOutput output for triggered heartbeat.
type TriggerWorkforceHeartbeatOutput struct {
	Body WorkforceAgentDTO
}

func (h *Handler) triggerWorkforceHeartbeat(ctx context.Context, input *TriggerWorkforceHeartbeatInput) (*TriggerWorkforceHeartbeatOutput, error) {
	r, orgID, store, err := h.workforceCaller(input.Ctx)
	if err != nil {
		return nil, err
	}

	agent, err := store.TriggerHeartbeat(r.Context(), orgID, input.ID)
	if err != nil {
		if errors.Is(err, agentstore.ErrAgentNotFound) {
			return nil, huma.Error404NotFound("No such workforce agent found in this workspace.")
		}
		return nil, huma.Error500InternalServerError("Could not trigger workforce agent heartbeat; retry the request")
	}

	return &TriggerWorkforceHeartbeatOutput{Body: toAgentDTO(*agent)}, nil
}

// DeleteWorkforceAgentInput input for deleting agent.
type DeleteWorkforceAgentInput struct {
	Ctx huma.Context `json:"-"`
	ID  string       `path:"id"`
}

func (i *DeleteWorkforceAgentInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

// DeleteWorkforceAgentOutput output for deleted agent.
type DeleteWorkforceAgentOutput struct {
	Body struct {
		Ok bool `json:"ok"`
	}
}

func (h *Handler) deleteWorkforceAgent(ctx context.Context, input *DeleteWorkforceAgentInput) (*DeleteWorkforceAgentOutput, error) {
	r, orgID, store, err := h.workforceCaller(input.Ctx)
	if err != nil {
		return nil, err
	}

	err = store.DeleteAgent(r.Context(), orgID, input.ID)
	if err != nil {
		if errors.Is(err, agentstore.ErrAgentNotFound) {
			return nil, huma.Error404NotFound("No such workforce agent found in this workspace.")
		}
		return nil, huma.Error500InternalServerError("Could not delete workforce agent; retry the request")
	}

	out := &DeleteWorkforceAgentOutput{}
	out.Body.Ok = true
	return out, nil
}

// ListWorkforceTasksInput input for listing tasks.
type ListWorkforceTasksInput struct {
	Ctx      huma.Context `json:"-"`
	Status   string       `query:"status,omitempty"`
	Assignee string       `query:"assignee,omitempty"`
}

func (i *ListWorkforceTasksInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

// ListWorkforceTasksOutput output for listing tasks.
type ListWorkforceTasksOutput struct {
	Body struct {
		Items []WorkforceTaskDTO `json:"items"`
	}
}

func (h *Handler) listWorkforceTasks(ctx context.Context, input *ListWorkforceTasksInput) (*ListWorkforceTasksOutput, error) {
	r, orgID, store, err := h.workforceCaller(input.Ctx)
	if err != nil {
		return nil, err
	}

	tasks, err := store.ListTasks(r.Context(), orgID, input.Status, input.Assignee)
	if err != nil {
		return nil, huma.Error500InternalServerError("Could not read workforce tasks; retry the request")
	}

	out := &ListWorkforceTasksOutput{}
	out.Body.Items = make([]WorkforceTaskDTO, 0, len(tasks))
	for _, t := range tasks {
		out.Body.Items = append(out.Body.Items, toTaskDTO(t, nil))
	}
	return out, nil
}

// CreateWorkforceTaskInput input for creating a task ticket.
type CreateWorkforceTaskInput struct {
	Ctx  huma.Context `json:"-"`
	Body struct {
		Title           string `json:"title"`
		AssigneeAgentID string `json:"assigneeAgentId,omitempty"`
		Assignee        string `json:"assignee,omitempty"`
		AssigneeRole    string `json:"assigneeRole,omitempty"`
		Priority        string `json:"priority,omitempty"`
		Summary         string `json:"summary"`
		Context         string `json:"context,omitempty"`
	}
}

func (i *CreateWorkforceTaskInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

// CreateWorkforceTaskOutput output for created task ticket.
type CreateWorkforceTaskOutput struct {
	Body WorkforceTaskDTO
}

func (h *Handler) createWorkforceTask(ctx context.Context, input *CreateWorkforceTaskInput) (*CreateWorkforceTaskOutput, error) {
	r, orgID, store, err := h.workforceCaller(input.Ctx)
	if err != nil {
		return nil, err
	}

	if input.Body.Title == "" || input.Body.Summary == "" {
		return nil, huma.Error400BadRequest("Task title and summary are required.")
	}

	task := models.WorkforceTask{
		Title:           input.Body.Title,
		AssigneeAgentID: input.Body.AssigneeAgentID,
		AssigneeName:    input.Body.Assignee,
		AssigneeRole:    input.Body.AssigneeRole,
		Priority:        input.Body.Priority,
		Summary:         input.Body.Summary,
		Context:         input.Body.Context,
	}
	if task.AssigneeAgentID == "" {
		task.AssigneeAgentID = "lead-orchestrator"
		task.AssigneeName = "Lead Orchestrator"
		task.AssigneeRole = "Chief Operational Assistant"
	}

	created, err := store.CreateTask(r.Context(), orgID, task)
	if err != nil {
		return nil, huma.Error500InternalServerError("Could not create workforce task; retry the request")
	}

	return &CreateWorkforceTaskOutput{Body: toTaskDTO(*created, nil)}, nil
}

// GetWorkforceTaskInput input for retrieving task details.
type GetWorkforceTaskInput struct {
	Ctx huma.Context `json:"-"`
	ID  string       `path:"id"`
}

func (i *GetWorkforceTaskInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

// GetWorkforceTaskOutput output for task details with execution steps.
type GetWorkforceTaskOutput struct {
	Body WorkforceTaskDTO
}

func (h *Handler) getWorkforceTask(ctx context.Context, input *GetWorkforceTaskInput) (*GetWorkforceTaskOutput, error) {
	r, orgID, store, err := h.workforceCaller(input.Ctx)
	if err != nil {
		return nil, err
	}

	task, steps, err := store.GetTask(r.Context(), orgID, input.ID)
	if err != nil {
		if errors.Is(err, agentstore.ErrTaskNotFound) {
			return nil, huma.Error404NotFound("No such workforce task found in this workspace.")
		}
		return nil, huma.Error500InternalServerError("Could not read workforce task; retry the request")
	}

	return &GetWorkforceTaskOutput{Body: toTaskDTO(*task, steps)}, nil
}

// UpdateWorkforceTaskInput input for updating task status.
type UpdateWorkforceTaskInput struct {
	Ctx  huma.Context `json:"-"`
	ID   string       `path:"id"`
	Body struct {
		Title    *string `json:"title,omitempty"`
		Status   *string `json:"status,omitempty"`
		Priority *string `json:"priority,omitempty"`
		Summary  *string `json:"summary,omitempty"`
		Context  *string `json:"context,omitempty"`
	}
}

func (i *UpdateWorkforceTaskInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

// UpdateWorkforceTaskOutput output for updated task.
type UpdateWorkforceTaskOutput struct {
	Body WorkforceTaskDTO
}

func (h *Handler) updateWorkforceTask(ctx context.Context, input *UpdateWorkforceTaskInput) (*UpdateWorkforceTaskOutput, error) {
	r, orgID, store, err := h.workforceCaller(input.Ctx)
	if err != nil {
		return nil, err
	}

	updates := make(map[string]any)
	if input.Body.Title != nil {
		updates["title"] = *input.Body.Title
	}
	if input.Body.Status != nil {
		updates["status"] = *input.Body.Status
	}
	if input.Body.Priority != nil {
		updates["priority"] = *input.Body.Priority
	}
	if input.Body.Summary != nil {
		updates["summary"] = *input.Body.Summary
	}
	if input.Body.Context != nil {
		updates["context"] = *input.Body.Context
	}

	task, err := store.UpdateTask(r.Context(), orgID, input.ID, updates)
	if err != nil {
		if errors.Is(err, agentstore.ErrTaskNotFound) {
			return nil, huma.Error404NotFound("No such workforce task found in this workspace.")
		}
		return nil, huma.Error500InternalServerError("Could not update workforce task; retry the request")
	}

	return &UpdateWorkforceTaskOutput{Body: toTaskDTO(*task, nil)}, nil
}

// AppendWorkforceTaskStepInput input for appending execution step.
type AppendWorkforceTaskStepInput struct {
	Ctx  huma.Context `json:"-"`
	ID   string       `path:"id"`
	Body struct {
		StepText string `json:"stepText"`
		Status   string `json:"status,omitempty"`
	}
}

func (i *AppendWorkforceTaskStepInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

// AppendWorkforceTaskStepOutput output for appended step.
type AppendWorkforceTaskStepOutput struct {
	Body WorkforceTaskStepDTO
}

func (h *Handler) appendWorkforceTaskStep(ctx context.Context, input *AppendWorkforceTaskStepInput) (*AppendWorkforceTaskStepOutput, error) {
	r, orgID, store, err := h.workforceCaller(input.Ctx)
	if err != nil {
		return nil, err
	}

	if input.Body.StepText == "" {
		return nil, huma.Error400BadRequest("Step log text is required.")
	}

	step, err := store.AppendTaskStep(r.Context(), orgID, input.ID, input.Body.StepText, input.Body.Status)
	if err != nil {
		if errors.Is(err, agentstore.ErrTaskNotFound) {
			return nil, huma.Error404NotFound("No such workforce task found in this workspace.")
		}
		return nil, huma.Error500InternalServerError("Could not append task step log; retry the request")
	}

	out := &AppendWorkforceTaskStepOutput{
		Body: WorkforceTaskStepDTO{
			ID:         step.ID,
			TaskID:     step.TaskID,
			StepNumber: step.StepNumber,
			StepText:   step.StepText,
			Status:     step.Status,
			CreatedAt:  step.CreatedAt,
		},
	}
	return out, nil
}

// GetWorkforceOverviewInput input for executive overview.
type GetWorkforceOverviewInput struct {
	Ctx huma.Context `json:"-"`
}

func (i *GetWorkforceOverviewInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

// GetWorkforceOverviewOutput output for executive overview.
type GetWorkforceOverviewOutput struct {
	Body WorkforceOverviewDTO
}

func (h *Handler) getWorkforceOverview(ctx context.Context, input *GetWorkforceOverviewInput) (*GetWorkforceOverviewOutput, error) {
	r, orgID, store, err := h.workforceCaller(input.Ctx)
	if err != nil {
		return nil, err
	}

	overview, err := store.GetOverview(r.Context(), orgID)
	if err != nil {
		return nil, huma.Error500InternalServerError("Could not calculate workforce overview; retry the request")
	}

	out := &GetWorkforceOverviewOutput{
		Body: WorkforceOverviewDTO{
			ActiveAgents:     overview.ActiveAgents,
			RunningTasks:     overview.RunningTasks,
			QueuedTasks:      overview.QueuedTasks,
			PendingApprovals: overview.PendingApprovals,
			MonthlySpend:     overview.MonthlySpend,
			MonthlyLimit:     overview.MonthlyLimit,
			AutoHalt:         overview.AutoHalt,
		},
	}
	return out, nil
}

// ToggleWorkforceAutoHaltInput input for toggling auto-halt setting.
type ToggleWorkforceAutoHaltInput struct {
	Ctx  huma.Context `json:"-"`
	Body struct {
		AutoHalt bool `json:"autoHalt"`
	}
}

func (i *ToggleWorkforceAutoHaltInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

// ToggleWorkforceAutoHaltOutput output for toggled auto-halt setting.
type ToggleWorkforceAutoHaltOutput struct {
	Body struct {
		AutoHalt bool `json:"autoHalt"`
	}
}

func (h *Handler) toggleWorkforceAutoHalt(ctx context.Context, input *ToggleWorkforceAutoHaltInput) (*ToggleWorkforceAutoHaltOutput, error) {
	r, orgID, store, err := h.workforceCaller(input.Ctx)
	if err != nil {
		return nil, err
	}

	if err := store.SetAutoHalt(r.Context(), orgID, input.Body.AutoHalt); err != nil {
		return nil, huma.Error500InternalServerError("Could not save auto-halt setting; retry the request")
	}

	out := &ToggleWorkforceAutoHaltOutput{}
	out.Body.AutoHalt = input.Body.AutoHalt
	return out, nil
}
