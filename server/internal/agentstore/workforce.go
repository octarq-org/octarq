// Package agentstore persists the workforce fleet, autonomous tasks, and governance settings.
package agentstore

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/octarq-org/octarq/server/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	// ErrAgentNotFound is returned when an agent is not found in the workspace.
	ErrAgentNotFound = errors.New("agentstore: workforce agent not found")
	// ErrTaskNotFound is returned when a task is not found in the workspace.
	ErrTaskNotFound = errors.New("agentstore: workforce task not found")
)

// WorkforceOverview represents aggregated metrics for the AI Workforce dashboard.
type WorkforceOverview struct {
	ActiveAgents     int     `json:"activeAgents"`
	RunningTasks     int     `json:"runningTasks"`
	QueuedTasks      int     `json:"queuedTasks"`
	PendingApprovals int     `json:"pendingApprovals"`
	MonthlySpend     float64 `json:"monthlySpend"`
	MonthlyLimit     float64 `json:"monthlyLimit"`
	AutoHalt         bool    `json:"autoHalt"`
}

// WorkforceStore manages persistence for workforce agents, tasks, and governance.
type WorkforceStore struct {
	db *gorm.DB
}

// NewWorkforceStore creates a new WorkforceStore instance.
func NewWorkforceStore(db *gorm.DB) *WorkforceStore {
	return &WorkforceStore{db: db}
}

// ensureSeeded idempotently populates default agents and tasks for a new workspace.
func (s *WorkforceStore) ensureSeeded(ctx context.Context, orgID uint) error {
	if s == nil || s.db == nil {
		return errors.New("agentstore: workforce store is not configured")
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var agentCount int64
		if err := tx.Model(&models.WorkforceAgent{}).Where("owner_id = ?", orgID).Count(&agentCount).Error; err != nil {
			return err
		}

		now := time.Now().UTC()
		if agentCount == 0 {
			defaultAgents := []models.WorkforceAgent{
				{
					ID:           "lead-orchestrator",
					OrgID:        orgID,
					Name:         "Lead Orchestrator",
					Role:         "Chief Operational Assistant",
					Department:   "Executive & Routing",
					Model:        "GPT-4o (OpenAI)",
					Status:       "active",
					Heartbeat:    "Every 15m",
					BudgetLimit:  20.00,
					BudgetUsed:   5.80,
					Capabilities: models.StringList{"all-tools", "delegate", "briefing"},
					Directives:   "Act as executive chief-of-staff. Coordinate workspace tasks, summarize metrics, and route issues.",
					CreatedAt:    now,
					UpdatedAt:    now,
				},
				{
					ID:           "devops-engineer",
					OrgID:        orgID,
					Name:         "DevOps Specialist",
					Role:         "Infrastructure & Network Engineer",
					Department:   "Operations",
					Model:        "Claude 3.5 Sonnet",
					Status:       "executing",
					Heartbeat:    "Every 10m",
					BudgetLimit:  15.00,
					BudgetUsed:   4.50,
					Capabilities: models.StringList{"dns", "infra", "domains", "metrics"},
					Directives:   "Continuously verify nameserver latency, detect DNS record drift, and check TLS certificate expiry.",
					CreatedAt:    now,
					UpdatedAt:    now,
				},
				{
					ID:           "security-auditor",
					OrgID:        orgID,
					Name:         "Security & Compliance Auditor",
					Role:         "Trust & Safety Inspector",
					Department:   "Security",
					Model:        "GPT-4o",
					Status:       "idle",
					Heartbeat:    "Every 1h",
					BudgetLimit:  10.00,
					BudgetUsed:   2.10,
					Capabilities: models.StringList{"tokens", "security", "rbac", "audit"},
					Directives:   "Audit API token permissions, enforce 2FA compliance, and inspect dead-letter security anomalies.",
					CreatedAt:    now,
					UpdatedAt:    now,
				},
				{
					ID:           "mail-deliverability",
					OrgID:        orgID,
					Name:         "Mail Deliverability Agent",
					Role:         "Inbound & Relay Specialist",
					Department:   "Communications",
					Model:        "DeepSeek-V3",
					Status:       "active",
					Heartbeat:    "Every 30m",
					BudgetLimit:  5.00,
					BudgetUsed:   1.80,
					Capabilities: models.StringList{"mail", "mailstorage", "webhooks"},
					Directives:   "Analyze bounce spikes, classify inbound spam, and trigger external S3/R2 storage offloading.",
					CreatedAt:    now,
					UpdatedAt:    now,
				},
			}

			for _, agent := range defaultAgents {
				if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&agent).Error; err != nil {
					return err
				}
			}
		}

		var taskCount int64
		if err := tx.Model(&models.WorkforceTask{}).Where("owner_id = ?", orgID).Count(&taskCount).Error; err != nil {
			return err
		}

		if taskCount == 0 {
			defaultTasks := []struct {
				task  models.WorkforceTask
				steps []models.WorkforceTaskStep
			}{
				{
					task: models.WorkforceTask{
						ID:              "AGT-101",
						OrgID:           orgID,
						Title:           "Diagnose DNS resolution latency on managed nameservers",
						AssigneeAgentID: "devops-engineer",
						AssigneeName:    "DevOps Specialist",
						AssigneeRole:    "Infrastructure & Network Engineer",
						Priority:        "urgent",
						Status:          "in_progress",
						Summary:         "Identified intermittent DNS resolution timeout on NS2 secondary node; testing route failover.",
						Context:         "Investigate ping spikes and fallback routing for edge nameservers.",
						LastHeartbeat:   "2m ago",
						CreatedAt:       now,
						UpdatedAt:       now,
					},
					steps: []models.WorkforceTaskStep{
						{OrgID: orgID, TaskID: "AGT-101", StepNumber: 1, StepText: "Checked primary ns1.octarq.io latency (12ms average across 8 regions)", Status: "completed", CreatedAt: now},
						{OrgID: orgID, TaskID: "AGT-101", StepNumber: 2, StepText: "Detected packet drop on ns2.octarq.io Frankfurt edge node", Status: "completed", CreatedAt: now},
						{OrgID: orgID, TaskID: "AGT-101", StepNumber: 3, StepText: "Executing automated fallback route propagation via Anycast health check", Status: "running", CreatedAt: now},
					},
				},
				{
					task: models.WorkforceTask{
						ID:              "AGT-102",
						OrgID:           orgID,
						Title:           "Audit 2FA keys and purge expired API access tokens",
						AssigneeAgentID: "security-auditor",
						AssigneeName:    "Security & Compliance Auditor",
						AssigneeRole:    "Trust & Safety Inspector",
						Priority:        "high",
						Status:          "awaiting_approval",
						Summary:         "Detected 2 inactive API tokens exceeding 90-day idle policy. Prepared revocation batch.",
						Context:         "Routine quarterly credential hygiene scan.",
						LastHeartbeat:   "14m ago",
						CreatedAt:       now,
						UpdatedAt:       now,
					},
					steps: []models.WorkforceTaskStep{
						{OrgID: orgID, TaskID: "AGT-102", StepNumber: 1, StepText: "Scanned all workspace tokens against last-used timestamps", Status: "completed", CreatedAt: now},
						{OrgID: orgID, TaskID: "AGT-102", StepNumber: 2, StepText: "Identified 2 unrotated service tokens with admin scope", Status: "completed", CreatedAt: now},
						{OrgID: orgID, TaskID: "AGT-102", StepNumber: 3, StepText: "Awaiting administrator approval to revoke identified tokens", Status: "awaiting_approval", CreatedAt: now},
					},
				},
				{
					task: models.WorkforceTask{
						ID:              "AGT-103",
						OrgID:           orgID,
						Title:           "Inspect dead-letter queue and reclassify unhandled webhooks",
						AssigneeAgentID: "devops-engineer",
						AssigneeName:    "DevOps Specialist",
						AssigneeRole:    "Infrastructure & Network Engineer",
						Priority:        "normal",
						Status:          "todo",
						Summary:         "3 failed inbound payloads queued for inspection; replaying delivery against updated parser.",
						Context:         "Process unprocessed webhook relay DLQ events.",
						LastHeartbeat:   "45m ago",
						CreatedAt:       now,
						UpdatedAt:       now,
					},
					steps: []models.WorkforceTaskStep{
						{OrgID: orgID, TaskID: "AGT-103", StepNumber: 1, StepText: "Captured 3 dead-letter event payloads from relay queue", Status: "completed", CreatedAt: now},
					},
				},
				{
					task: models.WorkforceTask{
						ID:              "AGT-104",
						OrgID:           orgID,
						Title:           "Review 24-hour inbound email deliverability & bounce trends",
						AssigneeAgentID: "mail-deliverability",
						AssigneeName:    "Mail Deliverability Agent",
						AssigneeRole:    "Inbound & Relay Specialist",
						Priority:        "low",
						Status:          "done",
						Summary:         "Reputation score 99.4%; 0 spam traps triggered; bounce rate well within normal baseline (0.08%).",
						Context:         "Daily morning deliverability diagnostic.",
						LastHeartbeat:   "1h ago",
						CreatedAt:       now,
						UpdatedAt:       now,
					},
					steps: []models.WorkforceTaskStep{
						{OrgID: orgID, TaskID: "AGT-104", StepNumber: 1, StepText: "Aggregated bounce and delivery rates from inbound logs", Status: "completed", CreatedAt: now},
						{OrgID: orgID, TaskID: "AGT-104", StepNumber: 2, StepText: "Verified SPF/DKIM alignment scores across active sending domains", Status: "completed", CreatedAt: now},
						{OrgID: orgID, TaskID: "AGT-104", StepNumber: 3, StepText: "Generated deliverability health scorecard with 99.4% reputation", Status: "completed", CreatedAt: now},
					},
				},
			}

			for _, dt := range defaultTasks {
				if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&dt.task).Error; err != nil {
					return err
				}
				for _, step := range dt.steps {
					if err := tx.Create(&step).Error; err != nil {
						return err
					}
				}
			}
		}

		return nil
	})
}

// ListAgents retrieves all agents for the workspace, seeding defaults if none exist.
func (s *WorkforceStore) ListAgents(ctx context.Context, orgID uint) ([]models.WorkforceAgent, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("agentstore: workforce store is not configured")
	}

	if err := s.ensureSeeded(ctx, orgID); err != nil {
		return nil, err
	}

	var agents []models.WorkforceAgent
	if err := s.db.WithContext(ctx).
		Where("owner_id = ?", orgID).
		Order("created_at ASC").
		Find(&agents).Error; err != nil {
		return nil, err
	}

	return agents, nil
}

// GetAgent retrieves a specific agent within a workspace.
func (s *WorkforceStore) GetAgent(ctx context.Context, orgID uint, id string) (*models.WorkforceAgent, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("agentstore: workforce store is not configured")
	}

	var agent models.WorkforceAgent
	err := s.db.WithContext(ctx).
		Where("id = ? AND owner_id = ?", id, orgID).
		First(&agent).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAgentNotFound
		}
		return nil, err
	}

	return &agent, nil
}

// CreateAgent adds a new agent to the workspace.
func (s *WorkforceStore) CreateAgent(ctx context.Context, orgID uint, agent models.WorkforceAgent) (*models.WorkforceAgent, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("agentstore: workforce store is not configured")
	}

	if agent.ID == "" {
		agent.ID = fmt.Sprintf("agent-%d", time.Now().UnixNano()%1000000)
	}
	agent.OrgID = orgID
	if agent.Status == "" {
		agent.Status = "active"
	}
	if agent.Heartbeat == "" {
		agent.Heartbeat = "Every 15m"
	}
	now := time.Now().UTC()
	agent.CreatedAt = now
	agent.UpdatedAt = now

	if err := s.db.WithContext(ctx).Create(&agent).Error; err != nil {
		return nil, err
	}

	return &agent, nil
}

// UpdateAgent applies partial updates to an existing agent.
func (s *WorkforceStore) UpdateAgent(ctx context.Context, orgID uint, id string, updates map[string]any) (*models.WorkforceAgent, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("agentstore: workforce store is not configured")
	}

	agent, err := s.GetAgent(ctx, orgID, id)
	if err != nil {
		return nil, err
	}

	updates["updated_at"] = time.Now().UTC()
	if err := s.db.WithContext(ctx).
		Model(agent).
		Where("id = ? AND owner_id = ?", id, orgID).
		Updates(updates).Error; err != nil {
		return nil, err
	}

	return s.GetAgent(ctx, orgID, id)
}

// PauseAgent sets agent status to paused.
func (s *WorkforceStore) PauseAgent(ctx context.Context, orgID uint, id string) (*models.WorkforceAgent, error) {
	return s.UpdateAgent(ctx, orgID, id, map[string]any{"status": "paused"})
}

// ResumeAgent sets agent status to active.
func (s *WorkforceStore) ResumeAgent(ctx context.Context, orgID uint, id string) (*models.WorkforceAgent, error) {
	return s.UpdateAgent(ctx, orgID, id, map[string]any{"status": "active"})
}

// TriggerHeartbeat sets agent status to executing and updates heartbeat timestamp.
func (s *WorkforceStore) TriggerHeartbeat(ctx context.Context, orgID uint, id string) (*models.WorkforceAgent, error) {
	now := time.Now().UTC()
	return s.UpdateAgent(ctx, orgID, id, map[string]any{
		"status":            "executing",
		"last_heartbeat_at": &now,
	})
}

// DeleteAgent permanently removes an agent from the workspace.
func (s *WorkforceStore) DeleteAgent(ctx context.Context, orgID uint, id string) error {
	if s == nil || s.db == nil {
		return errors.New("agentstore: workforce store is not configured")
	}

	res := s.db.WithContext(ctx).
		Where("id = ? AND owner_id = ?", id, orgID).
		Delete(&models.WorkforceAgent{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrAgentNotFound
	}
	return nil
}

// ListTasks retrieves tasks for the workspace, with optional filtering.
func (s *WorkforceStore) ListTasks(ctx context.Context, orgID uint, status, assignee string) ([]models.WorkforceTask, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("agentstore: workforce store is not configured")
	}

	if err := s.ensureSeeded(ctx, orgID); err != nil {
		return nil, err
	}

	query := s.db.WithContext(ctx).Where("owner_id = ?", orgID)
	if status != "" && status != "all" {
		query = query.Where("status = ?", status)
	}
	if assignee != "" {
		query = query.Where("assignee_agent_id = ? OR assignee_name = ?", assignee, assignee)
	}

	var tasks []models.WorkforceTask
	if err := query.Order("created_at DESC").Find(&tasks).Error; err != nil {
		return nil, err
	}

	return tasks, nil
}

// GetTask retrieves a task and its execution steps.
func (s *WorkforceStore) GetTask(ctx context.Context, orgID uint, id string) (*models.WorkforceTask, []models.WorkforceTaskStep, error) {
	if s == nil || s.db == nil {
		return nil, nil, errors.New("agentstore: workforce store is not configured")
	}

	var task models.WorkforceTask
	err := s.db.WithContext(ctx).
		Where("id = ? AND owner_id = ?", id, orgID).
		First(&task).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrTaskNotFound
		}
		return nil, nil, err
	}

	var steps []models.WorkforceTaskStep
	if err := s.db.WithContext(ctx).
		Where("task_id = ? AND owner_id = ?", id, orgID).
		Order("step_number ASC").
		Find(&steps).Error; err != nil {
		return nil, nil, err
	}

	return &task, steps, nil
}

// CreateTask adds a new task to the workspace.
func (s *WorkforceStore) CreateTask(ctx context.Context, orgID uint, task models.WorkforceTask) (*models.WorkforceTask, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("agentstore: workforce store is not configured")
	}

	if task.ID == "" {
		task.ID = fmt.Sprintf("AGT-%d", time.Now().UnixNano()%100000)
	}
	task.OrgID = orgID
	if task.Priority == "" {
		task.Priority = "normal"
	}
	if task.Status == "" {
		task.Status = "todo"
	}
	if task.LastHeartbeat == "" {
		task.LastHeartbeat = "Just now"
	}
	now := time.Now().UTC()
	task.CreatedAt = now
	task.UpdatedAt = now

	if err := s.db.WithContext(ctx).Create(&task).Error; err != nil {
		return nil, err
	}

	return &task, nil
}

// UpdateTask updates attributes on an existing task.
func (s *WorkforceStore) UpdateTask(ctx context.Context, orgID uint, id string, updates map[string]any) (*models.WorkforceTask, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("agentstore: workforce store is not configured")
	}

	task, _, err := s.GetTask(ctx, orgID, id)
	if err != nil {
		return nil, err
	}

	updates["updated_at"] = time.Now().UTC()
	if err := s.db.WithContext(ctx).
		Model(task).
		Where("id = ? AND owner_id = ?", id, orgID).
		Updates(updates).Error; err != nil {
		return nil, err
	}

	updatedTask, _, err := s.GetTask(ctx, orgID, id)
	return updatedTask, err
}

// AppendTaskStep appends a step to a task's execution trajectory.
func (s *WorkforceStore) AppendTaskStep(ctx context.Context, orgID uint, taskID string, stepText string, status string) (*models.WorkforceTaskStep, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("agentstore: workforce store is not configured")
	}

	// Verify task exists in tenant
	_, _, err := s.GetTask(ctx, orgID, taskID)
	if err != nil {
		return nil, err
	}

	var maxStep int
	row := s.db.WithContext(ctx).Model(&models.WorkforceTaskStep{}).
		Where("task_id = ? AND owner_id = ?", taskID, orgID).
		Select("COALESCE(MAX(step_number), 0)").Row()
	_ = row.Scan(&maxStep)

	if status == "" {
		status = "completed"
	}

	step := models.WorkforceTaskStep{
		OrgID:      orgID,
		TaskID:     taskID,
		StepNumber: maxStep + 1,
		StepText:   stepText,
		Status:     status,
		CreatedAt:  time.Now().UTC(),
	}

	if err := s.db.WithContext(ctx).Create(&step).Error; err != nil {
		return nil, err
	}

	return &step, nil
}

// GetOverview aggregates workforce metrics for the dashboard.
func (s *WorkforceStore) GetOverview(ctx context.Context, orgID uint) (*WorkforceOverview, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("agentstore: workforce store is not configured")
	}

	if err := s.ensureSeeded(ctx, orgID); err != nil {
		return nil, err
	}

	var activeAgents int64
	if err := s.db.WithContext(ctx).Model(&models.WorkforceAgent{}).
		Where("owner_id = ? AND status != 'paused'", orgID).
		Count(&activeAgents).Error; err != nil {
		return nil, err
	}

	var runningTasks int64
	if err := s.db.WithContext(ctx).Model(&models.WorkforceTask{}).
		Where("owner_id = ? AND status = 'in_progress'", orgID).
		Count(&runningTasks).Error; err != nil {
		return nil, err
	}

	var queuedTasks int64
	if err := s.db.WithContext(ctx).Model(&models.WorkforceTask{}).
		Where("owner_id = ? AND status = 'todo'", orgID).
		Count(&queuedTasks).Error; err != nil {
		return nil, err
	}

	var pendingApprovals int64
	if err := s.db.WithContext(ctx).Model(&models.WorkforceTask{}).
		Where("owner_id = ? AND status = 'awaiting_approval'", orgID).
		Count(&pendingApprovals).Error; err != nil {
		return nil, err
	}

	type SpendResult struct {
		TotalSpend float64
		TotalLimit float64
	}
	var spend SpendResult
	row := s.db.WithContext(ctx).Model(&models.WorkforceAgent{}).
		Where("owner_id = ?", orgID).
		Select("COALESCE(SUM(budget_used), 0) as total_spend, COALESCE(SUM(budget_limit), 0) as total_limit").
		Row()
	_ = row.Scan(&spend.TotalSpend, &spend.TotalLimit)

	autoHalt, err := s.GetAutoHalt(ctx, orgID)
	if err != nil {
		autoHalt = true
	}

	return &WorkforceOverview{
		ActiveAgents:     int(activeAgents),
		RunningTasks:     int(runningTasks),
		QueuedTasks:      int(queuedTasks),
		PendingApprovals: int(pendingApprovals),
		MonthlySpend:     spend.TotalSpend,
		MonthlyLimit:     spend.TotalLimit,
		AutoHalt:         autoHalt,
	}, nil
}

// GetAutoHalt returns the auto-halt setting for a workspace. Defaults to true.
func (s *WorkforceStore) GetAutoHalt(ctx context.Context, orgID uint) (bool, error) {
	if s == nil || s.db == nil {
		return true, errors.New("agentstore: workforce store is not configured")
	}

	var setting models.WorkspaceSetting
	err := s.db.WithContext(ctx).
		Where("org_id = ? AND key = ?", orgID, "workforce.auto_halt").
		First(&setting).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return true, nil
		}
		return true, err
	}

	return strings.ToLower(setting.Value) != "false", nil
}

// SetAutoHalt updates the auto-halt setting for a workspace.
func (s *WorkforceStore) SetAutoHalt(ctx context.Context, orgID uint, autoHalt bool) error {
	if s == nil || s.db == nil {
		return errors.New("agentstore: workforce store is not configured")
	}

	setting := models.WorkspaceSetting{
		OrgID: orgID,
		Key:   "workforce.auto_halt",
		Value: strconv.FormatBool(autoHalt),
	}

	return s.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "org_id"}, {Name: "key"}},
			DoUpdates: clause.AssignmentColumns([]string{"value"}),
		}).
		Create(&setting).Error
}
