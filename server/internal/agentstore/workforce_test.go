package agentstore

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/server/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newTestWorkforceStore(t *testing.T) (*WorkforceStore, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open in-memory sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&models.WorkforceAgent{},
		&models.WorkforceTask{},
		&models.WorkforceTaskStep{},
		&models.WorkspaceSetting{},
	); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	return NewWorkforceStore(db), db
}

func TestWorkforceStore_AutoSeedsDefaultAgentsAndTasks(t *testing.T) {
	t.Parallel()
	store, _ := newTestWorkforceStore(t)
	ctx := context.Background()
	orgID := uint(10)

	// ListAgents triggers auto-seeding
	agents, err := store.ListAgents(ctx, orgID)
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(agents) != 4 {
		t.Fatalf("expected 4 seeded agents, got %d", len(agents))
	}

	agentIDs := map[string]bool{}
	for _, a := range agents {
		agentIDs[a.ID] = true
		if a.OrgID != orgID {
			t.Errorf("expected OrgID %d, got %d", orgID, a.OrgID)
		}
	}
	expectedIDs := []string{"lead-orchestrator", "devops-engineer", "security-auditor", "mail-deliverability"}
	for _, id := range expectedIDs {
		if !agentIDs[id] {
			t.Errorf("missing expected agent %q", id)
		}
	}

	// ListTasks returns seeded tasks
	tasks, err := store.ListTasks(ctx, orgID, "all", "")
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) != 4 {
		t.Fatalf("expected 4 seeded tasks, got %d", len(tasks))
	}
}

func TestWorkforceStore_TenantIsolation(t *testing.T) {
	t.Parallel()
	store, _ := newTestWorkforceStore(t)
	ctx := context.Background()
	org1 := uint(1)
	org2 := uint(2)

	// Trigger seeding in org1
	_, err := store.ListAgents(ctx, org1)
	if err != nil {
		t.Fatalf("ListAgents org1: %v", err)
	}

	// Create custom agent in org1
	customAgent := models.WorkforceAgent{
		ID:           "org1-custom-bot",
		Name:         "Org1 Bot",
		Role:         "Tester",
		Department:   "QA",
		Model:        "Claude 3.5 Sonnet",
		Status:       "active",
		Capabilities: models.StringList{"test"},
		Directives:   "Run tests.",
	}
	_, err = store.CreateAgent(ctx, org1, customAgent)
	if err != nil {
		t.Fatalf("CreateAgent org1: %v", err)
	}

	// Org2 should not see customAgent
	_, err = store.GetAgent(ctx, org2, "org1-custom-bot")
	if !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("expected ErrAgentNotFound for org2, got %v", err)
	}

	// Deleting in org2 fails
	err = store.DeleteAgent(ctx, org2, "org1-custom-bot")
	if !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("expected ErrAgentNotFound when deleting across tenant, got %v", err)
	}

	// Creating a task in org1
	task1, err := store.CreateTask(ctx, org1, models.WorkforceTask{
		ID:              "ORG1-TASK",
		Title:           "Org 1 Secret Task",
		AssigneeAgentID: "lead-orchestrator",
		AssigneeName:    "Lead Orchestrator",
		AssigneeRole:    "Chief Operational Assistant",
		Summary:         "Confidential",
	})
	if err != nil {
		t.Fatalf("CreateTask org1: %v", err)
	}

	// Org2 should not see task1
	_, _, err = store.GetTask(ctx, org2, task1.ID)
	if !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("expected ErrTaskNotFound for org2, got %v", err)
	}
}

func TestWorkforceStore_AgentLifecycleAndMutations(t *testing.T) {
	t.Parallel()
	store, _ := newTestWorkforceStore(t)
	ctx := context.Background()
	orgID := uint(42)

	// Seed agents
	_, err := store.ListAgents(ctx, orgID)
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}

	// Pause agent
	agent, err := store.PauseAgent(ctx, orgID, "lead-orchestrator")
	if err != nil {
		t.Fatalf("PauseAgent: %v", err)
	}
	if agent.Status != "paused" {
		t.Errorf("expected status paused, got %s", agent.Status)
	}

	// Resume agent
	agent, err = store.ResumeAgent(ctx, orgID, "lead-orchestrator")
	if err != nil {
		t.Fatalf("ResumeAgent: %v", err)
	}
	if agent.Status != "active" {
		t.Errorf("expected status active, got %s", agent.Status)
	}

	// Trigger heartbeat
	agent, err = store.TriggerHeartbeat(ctx, orgID, "lead-orchestrator")
	if err != nil {
		t.Fatalf("TriggerHeartbeat: %v", err)
	}
	if agent.Status != "executing" {
		t.Errorf("expected status executing, got %s", agent.Status)
	}
	if agent.LastHeartbeatAt == nil {
		t.Errorf("expected non-nil LastHeartbeatAt")
	}

	// Update directives
	newDirectives := "Updated operational directives for testing."
	agent, err = store.UpdateAgent(ctx, orgID, "lead-orchestrator", map[string]any{
		"directives": newDirectives,
	})
	if err != nil {
		t.Fatalf("UpdateAgent: %v", err)
	}
	if agent.Directives != newDirectives {
		t.Errorf("expected updated directives, got %s", agent.Directives)
	}

	// Delete agent
	err = store.DeleteAgent(ctx, orgID, "lead-orchestrator")
	if err != nil {
		t.Fatalf("DeleteAgent: %v", err)
	}
	_, err = store.GetAgent(ctx, orgID, "lead-orchestrator")
	if !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("expected ErrAgentNotFound after delete, got %v", err)
	}
}

func TestWorkforceStore_TaskLifecycleAndStepAppending(t *testing.T) {
	t.Parallel()
	store, _ := newTestWorkforceStore(t)
	ctx := context.Background()
	orgID := uint(99)

	// Seed tasks
	_, err := store.ListTasks(ctx, orgID, "all", "")
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}

	// Get AGT-101 and its steps
	task, steps, err := store.GetTask(ctx, orgID, "AGT-101")
	if err != nil {
		t.Fatalf("GetTask AGT-101: %v", err)
	}
	if task.ID != "AGT-101" {
		t.Errorf("expected AGT-101, got %s", task.ID)
	}
	if len(steps) != 3 {
		t.Errorf("expected 3 steps for AGT-101, got %d", len(steps))
	}

	// Append step
	step, err := store.AppendTaskStep(ctx, orgID, "AGT-101", "Step 4: Latency stabilized to 15ms", "completed")
	if err != nil {
		t.Fatalf("AppendTaskStep: %v", err)
	}
	if step.StepNumber != 4 {
		t.Errorf("expected step number 4, got %d", step.StepNumber)
	}

	// Update task status to done
	updatedTask, err := store.UpdateTask(ctx, orgID, "AGT-101", map[string]any{
		"status": "done",
	})
	if err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	if updatedTask.Status != "done" {
		t.Errorf("expected status done, got %s", updatedTask.Status)
	}

	// Filter tasks by status=done
	doneTasks, err := store.ListTasks(ctx, orgID, "done", "")
	if err != nil {
		t.Fatalf("ListTasks status=done: %v", err)
	}
	// AGT-104 was seeded as done, now AGT-101 is also done -> 2 tasks
	if len(doneTasks) != 2 {
		t.Errorf("expected 2 done tasks, got %d", len(doneTasks))
	}
}

func TestWorkforceStore_OverviewAndAutoHalt(t *testing.T) {
	t.Parallel()
	store, _ := newTestWorkforceStore(t)
	ctx := context.Background()
	orgID := uint(55)

	overview, err := store.GetOverview(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOverview: %v", err)
	}

	// Initial seeded defaults:
	// Agents: 4 (lead-orchestrator: active, devops-engineer: executing, security-auditor: idle, mail-deliverability: active)
	// None paused -> 4 active
	if overview.ActiveAgents != 4 {
		t.Errorf("expected 4 active agents, got %d", overview.ActiveAgents)
	}

	// Spend: 5.80 + 4.50 + 2.10 + 1.80 = 14.20
	if math.Abs(overview.MonthlySpend-14.20) > 0.001 {
		t.Errorf("expected spend 14.20, got %f", overview.MonthlySpend)
	}

	// Limit: 20.00 + 15.00 + 10.00 + 5.00 = 50.00
	if math.Abs(overview.MonthlyLimit-50.00) > 0.001 {
		t.Errorf("expected limit 50.00, got %f", overview.MonthlyLimit)
	}

	// Default autoHalt should be true
	if !overview.AutoHalt {
		t.Errorf("expected autoHalt default true, got false")
	}

	// Toggle autoHalt to false
	err = store.SetAutoHalt(ctx, orgID, false)
	if err != nil {
		t.Fatalf("SetAutoHalt false: %v", err)
	}

	autoHalt, err := store.GetAutoHalt(ctx, orgID)
	if err != nil {
		t.Fatalf("GetAutoHalt: %v", err)
	}
	if autoHalt {
		t.Errorf("expected autoHalt to be false")
	}

	overview, err = store.GetOverview(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOverview after toggle: %v", err)
	}
	if overview.AutoHalt {
		t.Errorf("expected overview.AutoHalt to be false")
	}
}

func TestWorkforceStore_ConcurrentAccessRaceSafe(t *testing.T) {
	t.Parallel()
	store, _ := newTestWorkforceStore(t)
	ctx := context.Background()
	orgID := uint(77)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, _ = store.ListAgents(ctx, orgID)
			_, _ = store.ListTasks(ctx, orgID, "all", "")
			_, _ = store.GetOverview(ctx, orgID)
		}(i)
	}
	wg.Wait()
}
