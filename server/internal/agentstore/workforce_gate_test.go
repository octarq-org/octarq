package agentstore

import (
	"context"
	"errors"
	"testing"

	"github.com/octarq-org/octarq/server/internal/models"
)

// seedAgentAndTask creates one agent and one todo ticket assigned to it.
func seedAgentAndTask(t *testing.T, store *WorkforceStore, orgID uint) (models.WorkforceAgent, models.WorkforceTask) {
	t.Helper()
	agent, err := store.CreateAgent(context.Background(), orgID, models.WorkforceAgent{
		Name: "Ops Bot", Role: "Operator", Department: "Operations", Model: "GPT-4o",
	})
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	task, err := store.CreateTask(context.Background(), orgID, models.WorkforceTask{
		Title: "Rotate keys", Summary: "Rotate stale keys.", AssigneeAgentID: agent.ID,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	return *agent, *task
}

// TestWorkforceStore_DecideApproval covers both outcomes of the gate and the
// refusal of a replayed decision. A decided ticket must never flip twice.
func TestWorkforceStore_DecideApproval(t *testing.T) {
	t.Parallel()
	store, _ := newTestWorkforceStore(t)
	ctx := context.Background()
	orgID := uint(301)

	_, task := seedAgentAndTask(t, store, orgID)

	// An ungated ticket cannot be decided.
	if _, err := store.DecideApproval(ctx, orgID, task.ID, true); !errors.Is(err, ErrNotAwaitingApproval) {
		t.Fatalf("expected ErrNotAwaitingApproval for a todo ticket, got %v", err)
	}
	if _, err := store.SetTaskStatus(ctx, orgID, task.ID, "awaiting_approval"); err != nil {
		t.Fatalf("gate ticket: %v", err)
	}

	approved, err := store.DecideApproval(ctx, orgID, task.ID, true)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if approved.Status != "in_progress" {
		t.Errorf("expected in_progress after approval, got %s", approved.Status)
	}
	if _, err := store.DecideApproval(ctx, orgID, task.ID, true); !errors.Is(err, ErrNotAwaitingApproval) {
		t.Fatalf("expected ErrNotAwaitingApproval for a replay, got %v", err)
	}

	// A rejected ticket returns to the queue for the agent to revise.
	second, err := store.CreateTask(ctx, orgID, models.WorkforceTask{
		Title: "Rotate DNS keys", Summary: "Awaiting sign-off.", AssigneeAgentID: task.AssigneeAgentID,
	})
	if err != nil {
		t.Fatalf("CreateTask second: %v", err)
	}
	if _, err := store.SetTaskStatus(ctx, orgID, second.ID, "awaiting_approval"); err != nil {
		t.Fatalf("gate second: %v", err)
	}
	rejected, err := store.DecideApproval(ctx, orgID, second.ID, false)
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if rejected.Status != "todo" {
		t.Errorf("expected todo after rejection, got %s", rejected.Status)
	}

	// An unknown ticket is a 404, not a silent no-op.
	if _, err := store.DecideApproval(ctx, orgID, "AGT-missing", true); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("expected ErrTaskNotFound, got %v", err)
	}

	// Another workspace cannot decide this ticket.
	if _, err := store.DecideApproval(ctx, orgID+1, task.ID, true); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("expected ErrTaskNotFound across tenants, got %v", err)
	}
}

// TestWorkforceStore_SetTaskStatusRejectsClosedStates pins the terminal states:
// done is final, and a gated ticket leaves only through DecideApproval.
func TestWorkforceStore_SetTaskStatusRejectsClosedStates(t *testing.T) {
	t.Parallel()
	store, _ := newTestWorkforceStore(t)
	ctx := context.Background()
	orgID := uint(302)

	_, task := seedAgentAndTask(t, store, orgID)

	if _, err := store.SetTaskStatus(ctx, orgID, task.ID, "todo"); !errors.Is(err, ErrInvalidTaskTransition) {
		t.Fatalf("todo -> todo must be refused, got %v", err)
	}
	if _, err := store.SetTaskStatus(ctx, orgID, task.ID, "awaiting_approval"); err != nil {
		t.Fatalf("todo -> awaiting_approval: %v", err)
	}
	for _, next := range []string{"todo", "in_progress", "done"} {
		if _, err := store.SetTaskStatus(ctx, orgID, task.ID, next); !errors.Is(err, ErrInvalidTaskTransition) {
			t.Errorf("awaiting_approval -> %s must be refused, got %v", next, err)
		}
	}
	if _, err := store.SetTaskStatus(ctx, orgID, "AGT-missing", "in_progress"); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("expected ErrTaskNotFound, got %v", err)
	}

	// done is terminal.
	if _, err := store.DecideApproval(ctx, orgID, task.ID, true); err != nil {
		t.Fatalf("approve gate: %v", err)
	}
	if _, err := store.SetTaskStatus(ctx, orgID, task.ID, "done"); err != nil {
		t.Fatalf("in_progress -> done: %v", err)
	}
	for _, next := range []string{"todo", "in_progress", "awaiting_approval"} {
		if _, err := store.SetTaskStatus(ctx, orgID, task.ID, next); !errors.Is(err, ErrInvalidTaskTransition) {
			t.Errorf("done -> %s must be refused, got %v", next, err)
		}
	}
}

func TestWorkforceStore_RefusesMissingRows(t *testing.T) {
	t.Parallel()
	store, _ := newTestWorkforceStore(t)
	ctx := context.Background()
	orgID := uint(303)

	if _, err := store.UpdateAgent(ctx, orgID, "agent-missing", map[string]any{"name": "x"}); !errors.Is(err, ErrAgentNotFound) {
		t.Errorf("UpdateAgent: expected ErrAgentNotFound, got %v", err)
	}
	if _, err := store.UpdateTask(ctx, orgID, "AGT-missing", map[string]any{"title": "x"}); !errors.Is(err, ErrTaskNotFound) {
		t.Errorf("UpdateTask: expected ErrTaskNotFound, got %v", err)
	}
	if _, err := store.AppendTaskStep(ctx, orgID, "AGT-missing", "step", ""); !errors.Is(err, ErrTaskNotFound) {
		t.Errorf("AppendTaskStep: expected ErrTaskNotFound, got %v", err)
	}
	if _, err := store.CreateTask(ctx, orgID, models.WorkforceTask{Title: "T", Summary: "S"}); !errors.Is(err, ErrAssigneeRequired) {
		t.Errorf("CreateTask: expected ErrAssigneeRequired, got %v", err)
	}
	if err := store.DeleteAgent(ctx, orgID, "agent-missing"); !errors.Is(err, ErrAgentNotFound) {
		t.Errorf("DeleteAgent: expected ErrAgentNotFound, got %v", err)
	}
}

// TestWorkforceStore_DeleteAgentInUse proves an agent that owns a ticket is
// protected: deleting it would leave assignee references that do not resolve.
func TestWorkforceStore_DeleteAgentInUse(t *testing.T) {
	t.Parallel()
	store, _ := newTestWorkforceStore(t)
	ctx := context.Background()
	orgID := uint(304)

	agent, task := seedAgentAndTask(t, store, orgID)

	if err := store.DeleteAgent(ctx, orgID, agent.ID); !errors.Is(err, ErrAgentInUse) {
		t.Fatalf("expected ErrAgentInUse, got %v", err)
	}
	if _, err := store.GetAgent(ctx, orgID, agent.ID); err != nil {
		t.Fatalf("agent must survive the refused delete: %v", err)
	}

	// Once the ticket is gone the agent can be removed.
	if err := store.db.Delete(&models.WorkforceTask{}, "id = ? AND owner_id = ?", task.ID, orgID).Error; err != nil {
		t.Fatalf("delete task: %v", err)
	}
	if err := store.DeleteAgent(ctx, orgID, agent.ID); err != nil {
		t.Fatalf("DeleteAgent after freeing the ticket: %v", err)
	}
}

func TestWorkforceStore_ListTasksAssigneeFilter(t *testing.T) {
	t.Parallel()
	store, _ := newTestWorkforceStore(t)
	ctx := context.Background()
	orgID := uint(305)

	agent, _ := seedAgentAndTask(t, store, orgID)
	other, err := store.CreateAgent(ctx, orgID, models.WorkforceAgent{
		Name: "Audit Bot", Role: "Auditor", Department: "Security", Model: "Claude",
	})
	if err != nil {
		t.Fatalf("CreateAgent other: %v", err)
	}
	if _, err := store.CreateTask(ctx, orgID, models.WorkforceTask{
		Title: "Audit", Summary: "Audit trail.", AssigneeAgentID: other.ID,
	}); err != nil {
		t.Fatalf("CreateTask other: %v", err)
	}

	byName, err := store.ListTasks(ctx, orgID, "", agent.Name)
	if err != nil {
		t.Fatalf("ListTasks by name: %v", err)
	}
	if len(byName) != 1 || byName[0].AssigneeAgentID != agent.ID {
		t.Fatalf("expected only the first agent's ticket, got %+v", byName)
	}
	byID, err := store.ListTasks(ctx, orgID, "", other.ID)
	if err != nil {
		t.Fatalf("ListTasks by id: %v", err)
	}
	if len(byID) != 1 || byID[0].AssigneeAgentID != other.ID {
		t.Fatalf("expected only the second agent's ticket, got %+v", byID)
	}
}

func TestWorkforceStore_AutoHaltAcceptsOnlyFalseAsOff(t *testing.T) {
	t.Parallel()
	store, db := newTestWorkforceStore(t)
	ctx := context.Background()
	orgID := uint(306)

	// Without a row the guard defaults to on.
	if on, err := store.GetAutoHalt(ctx, orgID); err != nil || !on {
		t.Fatalf("expected default true, got %v / %v", on, err)
	}

	// A stored value that is not "false" still means on: the guard fails safe.
	if err := db.Create(&models.WorkspaceSetting{OrgID: orgID, Key: "workforce.auto_halt", Value: "unreadable"}).Error; err != nil {
		t.Fatalf("seed setting: %v", err)
	}
	if on, err := store.GetAutoHalt(ctx, orgID); err != nil || !on {
		t.Fatalf("a non-false value must keep the guard on, got %v / %v", on, err)
	}

	// The comparison is case-insensitive so the stored "FALSE" is read as off.
	if err := db.Model(&models.WorkspaceSetting{}).
		Where("org_id = ? AND key = ?", orgID, "workforce.auto_halt").
		Update("value", "FALSE").Error; err != nil {
		t.Fatalf("update setting: %v", err)
	}
	if on, err := store.GetAutoHalt(ctx, orgID); err != nil || on {
		t.Fatalf("expected stored FALSE to read as off, got %v / %v", on, err)
	}
}

// TestWorkforceStore_ProviderBreakdownCoversEveryFamily pins that spend is
// grouped by the provider that bills for each model label, including labels
// the mapping does not recognise.
func TestWorkforceStore_ProviderBreakdownCoversEveryFamily(t *testing.T) {
	t.Parallel()
	store, _ := newTestWorkforceStore(t)
	ctx := context.Background()
	orgID := uint(307)

	for i, model := range []string{"Claude 3.5 Sonnet", "GPT-4o", "DeepSeek-V3", "Gemini 2.5 Pro", "Llama 3", "Mistral Large"} {
		if _, err := store.CreateAgent(ctx, orgID, models.WorkforceAgent{
			Name: "Agent " + model, Role: "Operator", Department: "Ops", Model: model,
			BudgetLimit: 10, BudgetUsed: float64(i + 1),
		}); err != nil {
			t.Fatalf("CreateAgent %q: %v", model, err)
		}
	}

	overview, err := store.GetOverview(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOverview: %v", err)
	}
	got := map[string]bool{}
	for _, p := range overview.SpendByProvider {
		got[p.Provider] = true
	}
	for _, want := range []string{"Anthropic", "OpenAI", "DeepSeek", "Google", "Meta", "Other"} {
		if !got[want] {
			t.Errorf("expected a %s row in the provider breakdown, got %v", want, got)
		}
	}
}

// TestWorkforceStore_UpdateTaskFieldOnly pins that non-status attributes change
// while the ticket stays where the state machine put it.
func TestWorkforceStore_UpdateTaskFieldOnly(t *testing.T) {
	t.Parallel()
	store, _ := newTestWorkforceStore(t)
	ctx := context.Background()
	orgID := uint(308)

	_, task := seedAgentAndTask(t, store, orgID)

	updated, err := store.UpdateTask(ctx, orgID, task.ID, map[string]any{
		"title":    "Rotate keys (revised)",
		"summary":  "Include the secondary zone.",
		"priority": "high",
		"context":  "Raised by the audit.",
	})
	if err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	if updated.Title != "Rotate keys (revised)" || updated.Priority != "high" || updated.Context != "Raised by the audit." {
		t.Errorf("expected the new field values, got %+v", updated)
	}
	if updated.Status != "todo" {
		t.Errorf("a field update must not move the ticket, got %s", updated.Status)
	}
	if updated.UpdatedAt.Before(task.UpdatedAt) {
		t.Errorf("expected updated_at to advance, got %v after %v", updated.UpdatedAt, task.UpdatedAt)
	}
}

// TestWorkforceStore_UnconfiguredRefuses proves a store without a database
// answers with an error instead of panicking or pretending to succeed.
func TestWorkforceStore_UnconfiguredRefuses(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewWorkforceStore(nil)

	if _, err := store.ListAgents(ctx, 1); err == nil {
		t.Error("ListAgents must refuse an unconfigured store")
	}
	if _, err := store.GetAgent(ctx, 1, "id"); err == nil {
		t.Error("GetAgent must refuse an unconfigured store")
	}
	if _, err := store.CreateAgent(ctx, 1, models.WorkforceAgent{Name: "B"}); err == nil {
		t.Error("CreateAgent must refuse an unconfigured store")
	}
	if _, err := store.UpdateAgent(ctx, 1, "id", map[string]any{"name": "x"}); err == nil {
		t.Error("UpdateAgent must refuse an unconfigured store")
	}
	if err := store.DeleteAgent(ctx, 1, "id"); err == nil {
		t.Error("DeleteAgent must refuse an unconfigured store")
	}
	if _, err := store.ListTasks(ctx, 1, "", ""); err == nil {
		t.Error("ListTasks must refuse an unconfigured store")
	}
	if _, _, err := store.GetTask(ctx, 1, "id"); err == nil {
		t.Error("GetTask must refuse an unconfigured store")
	}
	if _, err := store.CreateTask(ctx, 1, models.WorkforceTask{AssigneeAgentID: "a"}); err == nil {
		t.Error("CreateTask must refuse an unconfigured store")
	}
	if _, err := store.UpdateTask(ctx, 1, "id", map[string]any{"title": "x"}); err == nil {
		t.Error("UpdateTask must refuse an unconfigured store")
	}
	if _, err := store.SetTaskStatus(ctx, 1, "id", "done"); err == nil {
		t.Error("SetTaskStatus must refuse an unconfigured store")
	}
	if _, err := store.AppendTaskStep(ctx, 1, "id", "step", ""); err == nil {
		t.Error("AppendTaskStep must refuse an unconfigured store")
	}
	if _, err := store.GetOverview(ctx, 1); err == nil {
		t.Error("GetOverview must refuse an unconfigured store")
	}
	if _, err := store.DecideApproval(ctx, 1, "id", true); err == nil {
		t.Error("DecideApproval must refuse an unconfigured store")
	}
	if on, err := store.GetAutoHalt(ctx, 1); err == nil || !on {
		t.Error("GetAutoHalt must report the safe default with an error")
	}
	if err := store.SetAutoHalt(ctx, 1, false); err == nil {
		t.Error("SetAutoHalt must refuse an unconfigured store")
	}
}
