package models

import (
	"time"
)

// WorkforceAgent represents an AI employee / autonomous agent within a workspace.
// Aligned with Paperclip agent specifications.
type WorkforceAgent struct {
	// ID is the unique agent identifier within the workspace (e.g. "lead-orchestrator", "agent-123").
	ID string `gorm:"primaryKey;size:64" json:"id"`
	// OrgID scopes the agent to one workspace.
	OrgID uint `gorm:"primaryKey;column:owner_id;index:idx_workforce_agents_org;not null" json:"orgId"`
	// Name is the display title of the agent.
	Name string `gorm:"size:128;not null" json:"name"`
	// Role is the operational responsibility.
	Role string `gorm:"size:128;not null" json:"role"`
	// Department is the operational grouping (e.g. Operations, Security, Communications).
	Department string `gorm:"size:128;not null" json:"department"`
	// Model is the LLM backing the agent (e.g. "GPT-4o", "Claude 3.5 Sonnet").
	Model string `gorm:"size:64;not null" json:"model"`
	// Status is one of active / executing / idle / paused.
	Status string `gorm:"size:32;not null;default:'active';index" json:"status"`
	// Heartbeat describes the scheduled wake cadence (e.g. "Every 15m").
	Heartbeat string `gorm:"size:64;not null;default:'Every 15m'" json:"heartbeat"`
	// BudgetLimit is the maximum spend allocated for the billing cycle.
	BudgetLimit float64 `gorm:"type:decimal(10,2);not null;default:10.00" json:"budgetLimit"`
	// BudgetUsed is the current spend accrued for the billing period.
	BudgetUsed float64 `gorm:"type:decimal(10,2);not null;default:0.00" json:"budgetUsed"`
	// BudgetPeriodStart is the first instant of the billing period BudgetUsed
	// covers. The first spend event of a new calendar month resets the counter,
	// so a budget limit is always a monthly limit.
	BudgetPeriodStart time.Time `gorm:"column:budget_period_start" json:"budgetPeriodStart,omitempty"`
	// Capabilities is the set of tool domains the agent can invoke.
	Capabilities StringList `gorm:"type:text;not null" json:"capabilities"`
	// Directives is the core behavioral prompt and operational constraints.
	Directives string `gorm:"type:text;not null" json:"directives"`
	// LastHeartbeatAt records when the agent last checked for work.
	LastHeartbeatAt *time.Time `json:"lastHeartbeatAt,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

func (WorkforceAgent) TableName() string {
	return "workforce_agents"
}

// WorkforceTask represents an issue or goal dispatched to an autonomous AI agent.
type WorkforceTask struct {
	// ID is the unique task ticket identifier within the workspace (e.g. "AGT-101").
	ID string `gorm:"primaryKey;size:64" json:"id"`
	// OrgID scopes the task to one workspace.
	OrgID uint `gorm:"primaryKey;column:owner_id;index:idx_workforce_tasks_org;not null" json:"orgId"`
	// Title is the concise description of the task objective.
	Title string `gorm:"size:256;not null" json:"title"`
	// AssigneeAgentID references WorkforceAgent.ID.
	AssigneeAgentID string `gorm:"size:64;index;not null;column:assignee_agent_id" json:"assigneeAgentId"`
	// AssigneeName caches the display name of the assigned agent.
	AssigneeName string `gorm:"size:128;not null;column:assignee_name" json:"assignee"`
	// AssigneeRole caches the role of the assigned agent.
	AssigneeRole string `gorm:"size:128;not null;column:assignee_role" json:"assigneeRole"`
	// Priority is urgent / high / normal / low.
	Priority string `gorm:"size:32;not null;default:'normal';index" json:"priority"`
	// Status is todo / in_progress / awaiting_approval / done.
	Status string `gorm:"size:32;not null;default:'todo';index" json:"status"`
	// Summary is the latest progress or completion report.
	Summary string `gorm:"type:text;not null" json:"summary"`
	// Context is background information and initial input.
	Context string `gorm:"type:text" json:"context,omitempty"`
	// LastHeartbeat is the human-readable timestamp of last activity.
	LastHeartbeat string `gorm:"size:64;not null;default:'Just now'" json:"lastHeartbeat"`
	// LastHeartbeatAt records when the task was last worked on.
	LastHeartbeatAt *time.Time `json:"lastHeartbeatAt,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

func (WorkforceTask) TableName() string {
	return "workforce_tasks"
}

// WorkforceTaskStep represents an execution tool call or step in an agent's run trajectory.
type WorkforceTaskStep struct {
	ID         uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	OrgID      uint      `gorm:"index:idx_task_steps_org;not null;column:owner_id" json:"orgId"`
	TaskID     string    `gorm:"size:64;index:idx_task_steps_task;not null" json:"taskId"`
	StepNumber int       `gorm:"not null" json:"stepNumber"`
	StepText   string    `gorm:"type:text;not null" json:"stepText"`
	Status     string    `gorm:"size:32;not null;default:'completed'" json:"status"` // completed, running, failed, awaiting_approval
	CreatedAt  time.Time `json:"createdAt"`
}

func (WorkforceTaskStep) TableName() string {
	return "workforce_task_steps"
}

// WorkforceModelPrice is one workspace's price for one model. Prices are per
// million tokens in US dollars. A model without a row is unpriced: the
// billing path refuses to attribute a turn to it instead of inventing a price.
type WorkforceModelPrice struct {
	OrgID uint `gorm:"primaryKey;column:owner_id;not null" json:"orgId"`
	// Model is the provider model identifier, e.g. "claude-opus-4-8".
	Model string `gorm:"primaryKey;size:64;not null" json:"model"`
	// InputPerMTok is the price of one million input (prompt) tokens.
	InputPerMTok float64 `gorm:"column:input_per_mtok;type:decimal(12,4);not null" json:"inputPerMTok"`
	// OutputPerMTok is the price of one million output (completion) tokens.
	OutputPerMTok float64   `gorm:"column:output_per_mtok;type:decimal(12,4);not null" json:"outputPerMTok"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

func (WorkforceModelPrice) TableName() string {
	return "workforce_model_prices"
}

// WorkforceSpendEvent is one billed agent turn in the spend ledger. The ledger
// is the audit trail of how BudgetUsed was accrued; the price is frozen into
// cost_usd at write time, so a later price change does not rewrite history.
type WorkforceSpendEvent struct {
	ID uint `gorm:"primaryKey;autoIncrement" json:"id"`
	// OrgID scopes the event to one workspace.
	OrgID uint `gorm:"index:idx_workforce_spend_org_agent;not null;column:owner_id" json:"orgId"`
	// AgentID references WorkforceAgent.ID.
	AgentID string `gorm:"size:64;index:idx_workforce_spend_org_agent;not null" json:"agentId"`
	// Model is the provider model identifier that was billed.
	Model string `gorm:"size:64;not null" json:"model"`
	// InputTokens is the prompt token count of the turn.
	InputTokens int `gorm:"not null" json:"inputTokens"`
	// OutputTokens is the completion token count of the turn. Every supported
	// provider already includes reasoning tokens here.
	OutputTokens int `gorm:"not null" json:"outputTokens"`
	// ReasoningTokens records the reasoning subset of OutputTokens for audit.
	// It is not billed a second time.
	ReasoningTokens int `gorm:"not null" json:"reasoningTokens"`
	// CostUSD is the frozen cost of the turn in US dollars.
	CostUSD float64 `gorm:"type:decimal(12,6);not null" json:"costUsd"`
	// Source is where the usage was reported from: chat or runtime.
	Source string `gorm:"size:32;not null" json:"source"`
	// SessionID links a chat turn to its session.
	SessionID string    `gorm:"size:64" json:"sessionId,omitempty"`
	CreatedAt time.Time `gorm:"index" json:"createdAt"`
}

func (WorkforceSpendEvent) TableName() string {
	return "workforce_spend_events"
}

// Closed vocabularies. The API validates every inbound status and priority
// against these sets: an unknown value would seed a row that no dashboard query
// or state machine ever matches.
var (
	// WorkforceAgentStatuses are the states an agent can be set to directly.
	// "executing" is written only by a heartbeat, never by a caller.
	WorkforceAgentStatuses = []string{"active", "idle", "paused"}
	// WorkforceTaskStatuses follow the ticket state machine.
	WorkforceTaskStatuses = []string{"todo", "in_progress", "awaiting_approval", "done"}
	// WorkforceTaskPriorities are the supported ticket priorities.
	WorkforceTaskPriorities = []string{"urgent", "high", "normal", "low"}
	// WorkforceTaskStepStatuses are the supported execution-log states.
	WorkforceTaskStepStatuses = []string{"completed", "running", "failed", "awaiting_approval"}
	// WorkforceSpendSources are the supported provenance labels of a spend
	// event. "chat" is written by the agent chat stream; "runtime" is written
	// by an external agent process that reports its own usage.
	WorkforceSpendSources = []string{"chat", "runtime"}
)

// Named spend sources. The API and the store use these constants so a label
// typo cannot create a provenance value no report matches.
const (
	WorkforceSpendSourceChat    = "chat"
	WorkforceSpendSourceRuntime = "runtime"
)

// InStatusSet reports whether value belongs to the given closed vocabulary.
func InStatusSet(value string, set []string) bool {
	for _, v := range set {
		if v == value {
			return true
		}
	}
	return false
}
