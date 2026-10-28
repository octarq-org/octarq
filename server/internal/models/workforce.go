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
	// BudgetUsed is the current spend accrued for the billing cycle.
	BudgetUsed float64 `gorm:"type:decimal(10,2);not null;default:0.00" json:"budgetUsed"`
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
