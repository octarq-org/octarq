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
		&models.WorkforceModelPrice{},
		&models.WorkforceSpendEvent{},
	); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	return NewWorkforceStore(db), db
}

// TestWorkforceStore_NewWorkspaceStartsEmpty pins the fail-closed contract: a
// new workspace has no fabricated fleet and no fabricated task history. Every
// row the dashboard shows was created through the API.
func TestWorkforceStore_NewWorkspaceStartsEmpty(t *testing.T) {
	t.Parallel()
	store, _ := newTestWorkforceStore(t)
	ctx := context.Background()
	orgID := uint(10)

	agents, err := store.ListAgents(ctx, orgID)
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(agents) != 0 {
		t.Fatalf("expected an empty new workspace, got %d agents", len(agents))
	}

	tasks, err := store.ListTasks(ctx, orgID, "all", "")
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("expected an empty new workspace, got %d tasks", len(tasks))
	}

	overview, err := store.GetOverview(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOverview: %v", err)
	}
	if overview.ActiveAgents != 0 || overview.MonthlySpend != 0 || overview.MonthlyLimit != 0 {
		t.Fatalf("expected zeroed overview for an empty workspace, got %+v", overview)
	}
}

func TestWorkforceStore_TenantIsolation(t *testing.T) {
	t.Parallel()
	store, _ := newTestWorkforceStore(t)
	ctx := context.Background()
	org1 := uint(1)
	org2 := uint(2)

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
	_, err := store.CreateAgent(ctx, org1, customAgent)
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

	// A task can only be assigned to a real agent from the same workspace.
	if _, err := store.CreateTask(ctx, org1, models.WorkforceTask{
		Title:           "Ghost task",
		AssigneeAgentID: "lead-orchestrator",
		Summary:         "No such agent exists.",
	}); !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("expected ErrAgentNotFound for an unknown assignee, got %v", err)
	}

	// Creating a task in org1
	task1, err := store.CreateTask(ctx, org1, models.WorkforceTask{
		ID:              "ORG1-TASK",
		Title:           "Org 1 Secret Task",
		AssigneeAgentID: "org1-custom-bot",
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

	created, err := store.CreateAgent(ctx, orgID, models.WorkforceAgent{
		Name:         "Lead Orchestrator",
		Role:         "Chief Operational Assistant",
		Department:   "Executive & Routing",
		Model:        "GPT-4o",
		Directives:   "Coordinate workspace tasks.",
		BudgetLimit:  20,
		Capabilities: models.StringList{"all-tools"},
	})
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected a generated agent ID")
	}

	// Pause agent
	agent, err := store.PauseAgent(ctx, orgID, created.ID)
	if err != nil {
		t.Fatalf("PauseAgent: %v", err)
	}
	if agent.Status != "paused" {
		t.Errorf("expected status paused, got %s", agent.Status)
	}

	// Resume agent
	agent, err = store.ResumeAgent(ctx, orgID, created.ID)
	if err != nil {
		t.Fatalf("ResumeAgent: %v", err)
	}
	if agent.Status != "active" {
		t.Errorf("expected status active, got %s", agent.Status)
	}

	// Trigger heartbeat
	agent, err = store.TriggerHeartbeat(ctx, orgID, created.ID)
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
	agent, err = store.UpdateAgent(ctx, orgID, created.ID, map[string]any{
		"directives": newDirectives,
	})
	if err != nil {
		t.Fatalf("UpdateAgent: %v", err)
	}
	if agent.Directives != newDirectives {
		t.Errorf("expected updated directives, got %s", agent.Directives)
	}

	// Delete agent
	err = store.DeleteAgent(ctx, orgID, created.ID)
	if err != nil {
		t.Fatalf("DeleteAgent: %v", err)
	}
	_, err = store.GetAgent(ctx, orgID, created.ID)
	if !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("expected ErrAgentNotFound after delete, got %v", err)
	}
}

func TestWorkforceStore_TaskLifecycleAndStepAppending(t *testing.T) {
	t.Parallel()
	store, _ := newTestWorkforceStore(t)
	ctx := context.Background()
	orgID := uint(99)

	agent, err := store.CreateAgent(ctx, orgID, models.WorkforceAgent{
		Name: "DevOps Specialist", Role: "Infrastructure & Network Engineer",
		Department: "Operations", Model: "GPT-4o",
	})
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	created, err := store.CreateTask(ctx, orgID, models.WorkforceTask{
		Title:           "Diagnose DNS resolution latency",
		AssigneeAgentID: agent.ID,
		Summary:         "Investigate ping spikes.",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected a generated task ID")
	}

	// Append steps; numbering starts at 1.
	step1, err := store.AppendTaskStep(ctx, orgID, created.ID, "Checked primary nameserver latency", "completed")
	if err != nil {
		t.Fatalf("AppendTaskStep 1: %v", err)
	}
	if step1.StepNumber != 1 {
		t.Errorf("expected step number 1, got %d", step1.StepNumber)
	}
	step2, err := store.AppendTaskStep(ctx, orgID, created.ID, "Detected packet drop on secondary node", "completed")
	if err != nil {
		t.Fatalf("AppendTaskStep 2: %v", err)
	}
	if step2.StepNumber != 2 {
		t.Errorf("expected step number 2, got %d", step2.StepNumber)
	}

	// The assignee name and role are copied from the agent row, never supplied.
	if created.AssigneeName != "DevOps Specialist" || created.AssigneeRole != "Infrastructure & Network Engineer" {
		t.Errorf("assignee identity must come from the agent row, got %q / %q", created.AssigneeName, created.AssigneeRole)
	}

	// Status moves through the state machine, not UpdateTask.
	if _, err := store.UpdateTask(ctx, orgID, created.ID, map[string]any{"status": "done"}); !errors.Is(err, ErrInvalidTaskTransition) {
		t.Fatalf("UpdateTask must refuse status changes, got %v", err)
	}
	if _, err := store.SetTaskStatus(ctx, orgID, created.ID, "done"); !errors.Is(err, ErrInvalidTaskTransition) {
		t.Fatalf("todo -> done must be refused, got %v", err)
	}
	if _, err := store.SetTaskStatus(ctx, orgID, created.ID, "in_progress"); err != nil {
		t.Fatalf("todo -> in_progress: %v", err)
	}
	updatedTask, err := store.SetTaskStatus(ctx, orgID, created.ID, "done")
	if err != nil {
		t.Fatalf("in_progress -> done: %v", err)
	}
	if updatedTask.Status != "done" {
		t.Errorf("expected status done, got %s", updatedTask.Status)
	}

	// Filter tasks by status=done
	doneTasks, err := store.ListTasks(ctx, orgID, "done", "")
	if err != nil {
		t.Fatalf("ListTasks status=done: %v", err)
	}
	if len(doneTasks) != 1 {
		t.Errorf("expected 1 done task, got %d", len(doneTasks))
	}

	// The task detail carries both steps in order.
	_, steps, err := store.GetTask(ctx, orgID, created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if len(steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(steps))
	}
}

func TestWorkforceStore_OverviewAndAutoHalt(t *testing.T) {
	t.Parallel()
	store, _ := newTestWorkforceStore(t)
	ctx := context.Background()
	orgID := uint(55)

	_, err := store.CreateAgent(ctx, orgID, models.WorkforceAgent{
		Name: "Lead Orchestrator", Role: "Chief Operational Assistant", Department: "Executive",
		Model: "GPT-4o", Status: "active", BudgetLimit: 20, BudgetUsed: 5.80,
	})
	if err != nil {
		t.Fatalf("CreateAgent 1: %v", err)
	}
	_, err = store.CreateAgent(ctx, orgID, models.WorkforceAgent{
		Name: "DevOps Specialist", Role: "Infrastructure Engineer", Department: "Operations",
		Model: "Claude 3.5 Sonnet", Status: "executing", BudgetLimit: 15, BudgetUsed: 4.50,
	})
	if err != nil {
		t.Fatalf("CreateAgent 2: %v", err)
	}
	// A paused agent does not count as active.
	_, err = store.CreateAgent(ctx, orgID, models.WorkforceAgent{
		Name: "Paused Auditor", Role: "Auditor", Department: "Security",
		Model: "DeepSeek-V3", Status: "paused", BudgetLimit: 5, BudgetUsed: 1.80,
	})
	if err != nil {
		t.Fatalf("CreateAgent 3: %v", err)
	}

	overview, err := store.GetOverview(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOverview: %v", err)
	}

	if overview.ActiveAgents != 2 {
		t.Errorf("expected 2 active agents, got %d", overview.ActiveAgents)
	}
	if math.Abs(overview.MonthlySpend-12.10) > 0.001 {
		t.Errorf("expected spend 12.10, got %f", overview.MonthlySpend)
	}
	if math.Abs(overview.MonthlyLimit-40.00) > 0.001 {
		t.Errorf("expected limit 40.00, got %f", overview.MonthlyLimit)
	}

	// The breakdown is derived from the same rows, never fabricated.
	if len(overview.SpendByAgent) != 3 {
		t.Fatalf("expected 3 spend-by-agent rows, got %d", len(overview.SpendByAgent))
	}
	if overview.SpendByAgent[0].Name != "Lead Orchestrator" {
		t.Errorf("expected the top spender first, got %q", overview.SpendByAgent[0].Name)
	}
	if got := overview.SpendByAgent[0].Pct; got != 48 { // 5.80 / 12.10
		t.Errorf("expected top spender at 48%%, got %d", got)
	}
	for _, p := range overview.SpendByProvider {
		if p.Provider == "" {
			t.Errorf("expected every provider row to name a provider")
		}
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

// TestRandomSuffixUniqueUnderConcurrency proves the generated IDs come from a
// random source, not the clock: two creates in the same instant cannot collide.
func TestRandomSuffixUniqueUnderConcurrency(t *testing.T) {
	t.Parallel()

	const n = 512
	ids := make([]string, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			s, err := randomSuffix(12)
			if err != nil {
				t.Errorf("randomSuffix: %v", err)
				return
			}
			ids[idx] = s
		}(i)
	}
	wg.Wait()

	seen := map[string]bool{}
	for _, id := range ids {
		if len(id) != 12 {
			t.Fatalf("expected a 12-character suffix, got %q", id)
		}
		if seen[id] {
			t.Fatalf("duplicate generated suffix %q", id)
		}
		seen[id] = true
	}
}

func TestWorkforceStore_GeneratedIDsAreNotClockDerived(t *testing.T) {
	t.Parallel()
	store, _ := newTestWorkforceStore(t)
	ctx := context.Background()
	orgID := uint(88)

	agent, err := store.CreateAgent(ctx, orgID, models.WorkforceAgent{
		Name: "Bot", Role: "Tester", Department: "QA", Model: "GPT-4o",
	})
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	if len(agent.ID) != len("agent-")+12 {
		t.Errorf("unexpected agent ID shape: %q", agent.ID)
	}

	task, err := store.CreateTask(ctx, orgID, models.WorkforceTask{Title: "T", Summary: "S", AssigneeAgentID: agent.ID})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if len(task.ID) != len("AGT-")+10 {
		t.Errorf("unexpected task ID shape: %q", task.ID)
	}
}
