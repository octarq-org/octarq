// Package agentstore persists the workforce fleet, autonomous tasks, and governance settings.
package agentstore

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
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
	// ErrNotAwaitingApproval is returned when a decision targets a task that is
	// not currently gated. A concurrent or replayed decision lands here.
	ErrNotAwaitingApproval = errors.New("agentstore: task is not awaiting approval")
	// ErrInvalidTaskTransition is returned for a status change the ticket state
	// machine does not allow, including any change made through UpdateTask.
	ErrInvalidTaskTransition = errors.New("agentstore: task transition not allowed")
	// ErrTaskChanged is returned when a compare-and-swap loses a race: the task
	// moved to another status while the caller was deciding.
	ErrTaskChanged = errors.New("agentstore: task changed during the update")
	// ErrAgentInUse is returned when deleting an agent that still owns tasks.
	ErrAgentInUse = errors.New("agentstore: agent still owns tasks")
	// ErrAssigneeRequired is returned when a task is created without a real
	// assignee from the same workspace.
	ErrAssigneeRequired = errors.New("agentstore: assignee agent is required")
)

// taskTransitions is the ticket state machine. A gated task leaves
// awaiting_approval only through DecideApproval, and done is terminal, so a
// status update can never bypass a recorded decision or reopen finished work.
var taskTransitions = map[string]map[string]bool{
	"todo":              {"in_progress": true, "awaiting_approval": true},
	"in_progress":       {"todo": true, "done": true, "awaiting_approval": true},
	"awaiting_approval": {},
	"done":              {},
}

func canTransition(from, to string) bool {
	return taskTransitions[from][to]
}

// WorkforceSpendAgent is one agent's spend share in the dashboard breakdown.
type WorkforceSpendAgent struct {
	Name  string  `json:"name"`
	Model string  `json:"model"`
	Spend float64 `json:"spend"`
	Pct   int     `json:"pct"`
}

// WorkforceSpendProvider is one model provider's spend share in the dashboard breakdown.
type WorkforceSpendProvider struct {
	Provider string  `json:"provider"`
	Spend    float64 `json:"spend"`
	Pct      int     `json:"pct"`
}

// WorkforceOverview represents aggregated metrics for the AI Workforce dashboard.
type WorkforceOverview struct {
	ActiveAgents     int                      `json:"activeAgents"`
	RunningTasks     int                      `json:"runningTasks"`
	QueuedTasks      int                      `json:"queuedTasks"`
	PendingApprovals int                      `json:"pendingApprovals"`
	MonthlySpend     float64                  `json:"monthlySpend"`
	MonthlyLimit     float64                  `json:"monthlyLimit"`
	AutoHalt         bool                     `json:"autoHalt"`
	SpendByAgent     []WorkforceSpendAgent    `json:"spendByAgent"`
	SpendByProvider  []WorkforceSpendProvider `json:"spendByProvider"`
}

// randomSuffix returns n hex characters from a cryptographically random source.
// Entity IDs must not be derived from the clock: two creates in the same
// millisecond would collide on the composite primary key and fail the insert.
func randomSuffix(n int) (string, error) {
	buf := make([]byte, (n+1)/2)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf)[:n], nil
}

// WorkforceStore manages persistence for workforce agents, tasks, and governance.
type WorkforceStore struct {
	db *gorm.DB
}

// NewWorkforceStore creates a new WorkforceStore instance.
func NewWorkforceStore(db *gorm.DB) *WorkforceStore {
	return &WorkforceStore{db: db}
}

// ListAgents retrieves all agents for the workspace. A new workspace starts
// empty; there is no fabricated demo fleet. Every row returned here was
// created through CreateAgent and is real.
func (s *WorkforceStore) ListAgents(ctx context.Context, orgID uint) ([]models.WorkforceAgent, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("agentstore: workforce store is not configured")
	}

	var agents []models.WorkforceAgent
	if err := s.db.WithContext(ctx).
		Where("owner_id = ?", orgID).
		Order("created_at ASC").
		Find(&agents).Error; err != nil {
		return nil, err
	}
	for i := range agents {
		normalizeBudget(&agents[i], time.Now().UTC())
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
	normalizeBudget(&agent, time.Now().UTC())

	return &agent, nil
}

// CreateAgent adds a new agent to the workspace.
func (s *WorkforceStore) CreateAgent(ctx context.Context, orgID uint, agent models.WorkforceAgent) (*models.WorkforceAgent, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("agentstore: workforce store is not configured")
	}

	if agent.ID == "" {
		suffix, err := randomSuffix(12)
		if err != nil {
			return nil, err
		}
		agent.ID = "agent-" + suffix
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

// DeleteAgent permanently removes an agent from the workspace. An agent that
// still owns tasks is refused: deleting it would leave assignee references that
// no longer resolve.
func (s *WorkforceStore) DeleteAgent(ctx context.Context, orgID uint, id string) error {
	if s == nil || s.db == nil {
		return errors.New("agentstore: workforce store is not configured")
	}

	if _, err := s.GetAgent(ctx, orgID, id); err != nil {
		return err
	}

	var assigned int64
	if err := s.db.WithContext(ctx).Model(&models.WorkforceTask{}).
		Where("owner_id = ? AND assignee_agent_id = ?", orgID, id).
		Count(&assigned).Error; err != nil {
		return err
	}
	if assigned > 0 {
		return ErrAgentInUse
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

// ListTasks retrieves tasks for the workspace, with optional filtering. A new
// workspace starts empty; no demo tickets are fabricated.
func (s *WorkforceStore) ListTasks(ctx context.Context, orgID uint, status, assignee string) ([]models.WorkforceTask, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("agentstore: workforce store is not configured")
	}

	query := s.db.WithContext(ctx).Where("owner_id = ?", orgID)
	if status != "" {
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

// CreateTask adds a new task to the workspace. The assignee must be a real
// agent from the same workspace: the display name and role are copied from that
// row, never supplied by the caller.
func (s *WorkforceStore) CreateTask(ctx context.Context, orgID uint, task models.WorkforceTask) (*models.WorkforceTask, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("agentstore: workforce store is not configured")
	}
	if task.AssigneeAgentID == "" {
		return nil, ErrAssigneeRequired
	}
	assignee, err := s.GetAgent(ctx, orgID, task.AssigneeAgentID)
	if err != nil {
		return nil, err
	}
	task.AssigneeName = assignee.Name
	task.AssigneeRole = assignee.Role

	if task.ID == "" {
		suffix, err := randomSuffix(10)
		if err != nil {
			return nil, err
		}
		task.ID = "AGT-" + suffix
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

// UpdateTask updates attributes on an existing task. Status is not one of
// them: a status change goes through SetTaskStatus or DecideApproval so the
// state machine and its compare-and-swap always apply.
func (s *WorkforceStore) UpdateTask(ctx context.Context, orgID uint, id string, updates map[string]any) (*models.WorkforceTask, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("agentstore: workforce store is not configured")
	}
	if _, ok := updates["status"]; ok {
		return nil, ErrInvalidTaskTransition
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

// SetTaskStatus applies one step of the ticket state machine as a compare-and-
// swap on the status the caller was shown. A concurrent writer that moved the
// task first makes the update match no rows, which surfaces as ErrTaskChanged
// instead of silently overwriting.
func (s *WorkforceStore) SetTaskStatus(ctx context.Context, orgID uint, id, next string) (*models.WorkforceTask, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("agentstore: workforce store is not configured")
	}

	task, _, err := s.GetTask(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if !canTransition(task.Status, next) {
		return nil, ErrInvalidTaskTransition
	}

	res := s.db.WithContext(ctx).
		Model(&models.WorkforceTask{}).
		Where("id = ? AND owner_id = ? AND status = ?", id, orgID, task.Status).
		Updates(map[string]any{"status": next, "updated_at": time.Now().UTC()})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, ErrTaskChanged
	}

	updated, _, err := s.GetTask(ctx, orgID, id)
	return updated, err
}

// AppendTaskStep appends a step to a task's execution trajectory. The step
// number is allocated inside the insert transaction so two concurrent writers
// cannot claim the same number.
func (s *WorkforceStore) AppendTaskStep(ctx context.Context, orgID uint, taskID string, stepText string, status string) (*models.WorkforceTaskStep, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("agentstore: workforce store is not configured")
	}

	// Verify task exists in tenant
	_, _, err := s.GetTask(ctx, orgID, taskID)
	if err != nil {
		return nil, err
	}

	if status == "" {
		status = "completed"
	}

	step := models.WorkforceTaskStep{
		OrgID:     orgID,
		TaskID:    taskID,
		StepText:  stepText,
		Status:    status,
		CreatedAt: time.Now().UTC(),
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var maxStep int
		if err := tx.Model(&models.WorkforceTaskStep{}).
			Where("task_id = ? AND owner_id = ?", taskID, orgID).
			Select("COALESCE(MAX(step_number), 0)").Row().Scan(&maxStep); err != nil {
			return err
		}
		step.StepNumber = maxStep + 1
		return tx.Create(&step).Error
	})
	if err != nil {
		return nil, err
	}

	return &step, nil
}

// GetOverview aggregates workforce metrics for the dashboard.
func (s *WorkforceStore) GetOverview(ctx context.Context, orgID uint) (*WorkforceOverview, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("agentstore: workforce store is not configured")
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

	// The breakdown is computed from the same agent rows the fleet view shows,
	// so the budget page can never display numbers no agent ever accrued.
	var agents []models.WorkforceAgent
	if err := s.db.WithContext(ctx).Where("owner_id = ?", orgID).Find(&agents).Error; err != nil {
		return nil, err
	}

	var totalSpend, totalLimit float64
	byAgent := make([]WorkforceSpendAgent, 0, len(agents))
	now := time.Now().UTC()
	for _, a := range agents {
		// A counter from an earlier month reads as zero: the dashboard shows
		// this month's spend, the same number the budget gate compares against.
		normalizeBudget(&a, now)
		totalSpend += a.BudgetUsed
		totalLimit += a.BudgetLimit
		byAgent = append(byAgent, WorkforceSpendAgent{Name: a.Name, Model: a.Model, Spend: a.BudgetUsed})
	}

	pctOf := func(v, total float64) int {
		if total <= 0 {
			return 0
		}
		return int(v/total*100 + 0.5)
	}
	for i := range byAgent {
		byAgent[i].Pct = pctOf(byAgent[i].Spend, totalSpend)
	}
	sort.SliceStable(byAgent, func(i, j int) bool { return byAgent[i].Spend > byAgent[j].Spend })

	// Provider attribution comes from the ledger, not from the agent's display
	// label: workforce_agents.model does not select the price or the route, so
	// summing the counter by that label would charge a provider that never ran
	// the turn. The ledger rows carry the model that was actually billed.
	var modelSpend []struct {
		Model string
		Spend float64
	}
	if err := s.db.WithContext(ctx).Model(&models.WorkforceSpendEvent{}).
		Select("model, COALESCE(SUM(cost_usd), 0) AS spend").
		Where("owner_id = ? AND created_at >= ?", orgID, monthStart(now)).
		Group("model").
		Scan(&modelSpend).Error; err != nil {
		return nil, err
	}

	providerSpend := map[string]float64{}
	var providerTotal float64
	for _, row := range modelSpend {
		providerSpend[providerForModel(row.Model)] += row.Spend
		providerTotal += row.Spend
	}

	byProvider := make([]WorkforceSpendProvider, 0, len(providerSpend))
	for name, spend := range providerSpend {
		byProvider = append(byProvider, WorkforceSpendProvider{Provider: name, Spend: spend, Pct: pctOf(spend, providerTotal)})
	}
	sort.SliceStable(byProvider, func(i, j int) bool { return byProvider[i].Spend > byProvider[j].Spend })

	autoHalt, err := s.GetAutoHalt(ctx, orgID)
	if err != nil {
		// "Halt is off" and "governance state is unreadable" are different
		// answers; the caller must not receive metrics that hide the second.
		return nil, err
	}

	return &WorkforceOverview{
		ActiveAgents:     int(activeAgents),
		RunningTasks:     int(runningTasks),
		QueuedTasks:      int(queuedTasks),
		PendingApprovals: int(pendingApprovals),
		MonthlySpend:     totalSpend,
		MonthlyLimit:     totalLimit,
		AutoHalt:         autoHalt,
		SpendByAgent:     byAgent,
		SpendByProvider:  byProvider,
	}, nil
}

// providerForModel maps a model label to the provider that bills for it.
func providerForModel(model string) string {
	m := strings.ToLower(model)
	switch {
	case strings.Contains(m, "claude") || strings.Contains(m, "anthropic"):
		return "Anthropic"
	case strings.Contains(m, "gpt") || strings.Contains(m, "openai") || strings.Contains(m, "o1") || strings.Contains(m, "o3"):
		return "OpenAI"
	case strings.Contains(m, "deepseek"):
		return "DeepSeek"
	case strings.Contains(m, "gemini") || strings.Contains(m, "google"):
		return "Google"
	case strings.Contains(m, "llama") || strings.Contains(m, "meta"):
		return "Meta"
	default:
		return "Other"
	}
}

// DecideApproval resolves a task's approval gate. Approving resumes the work
// (in_progress); rejecting returns the ticket to the queue (todo) so the agent
// can revise it. The transition is a compare-and-swap on the current status, so
// a concurrent or replayed decision fails with ErrNotAwaitingApproval instead
// of silently flipping an already-resolved task.
func (s *WorkforceStore) DecideApproval(ctx context.Context, orgID uint, taskID string, approve bool) (*models.WorkforceTask, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("agentstore: workforce store is not configured")
	}

	next := "in_progress"
	if !approve {
		next = "todo"
	}

	res := s.db.WithContext(ctx).
		Model(&models.WorkforceTask{}).
		Where("id = ? AND owner_id = ? AND status = ?", taskID, orgID, "awaiting_approval").
		Updates(map[string]any{"status": next, "updated_at": time.Now().UTC()})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		// Either the task does not exist or it is not gated. Distinguish the
		// two so a caller gets a truthful 404 or 409.
		if _, _, err := s.GetTask(ctx, orgID, taskID); err != nil {
			return nil, err
		}
		return nil, ErrNotAwaitingApproval
	}

	task, _, err := s.GetTask(ctx, orgID, taskID)
	return task, err
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
