package app

import (
	"context"
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/server/internal/agentstore"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// newWorkforceSPI builds the ledger a plugin sees, over an in-memory database.
func newWorkforceSPI(t *testing.T) (plugin.Workforce, *agentstore.WorkforceStore) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open in-memory sqlite: %v", err)
	}
	if err := db.AutoMigrate(models.AllModels()...); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	store := agentstore.NewWorkforceStore(db)
	return newWorkforceService(store), store
}

// seedAgent creates one agent and prices its model.
func seedAgent(t *testing.T, store *agentstore.WorkforceStore, orgID uint, status string) string {
	t.Helper()
	ctx := context.Background()
	agent, err := store.CreateAgent(ctx, orgID, models.WorkforceAgent{
		Name: "writer", Role: "writer", Department: "content",
		Model: "test-model", Status: status, Directives: "write",
	})
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	if _, err := store.SetModelPrice(ctx, orgID, "test-model", 3, 15); err != nil {
		t.Fatalf("SetModelPrice: %v", err)
	}
	return agent.ID
}

// TestWorkforceSPI_AuthorizesAPricedActiveAgent pins the happy path: a plugin
// asks the ledger, and a priced active agent is allowed to run.
func TestWorkforceSPI_AuthorizesAPricedActiveAgent(t *testing.T) {
	w, store := newWorkforceSPI(t)
	id := seedAgent(t, store, 7, "active")

	if err := w.AuthorizeTurn(context.Background(), 7, id, "test-model"); err != nil {
		t.Fatalf("AuthorizeTurn = %v, want nil", err)
	}
}

// TestWorkforceSPI_RefusesAPausedAgent pins that the pause switch reaches the
// plugin through the SPI, not only the dashboard.
func TestWorkforceSPI_RefusesAPausedAgent(t *testing.T) {
	w, store := newWorkforceSPI(t)
	id := seedAgent(t, store, 7, "paused")

	err := w.AuthorizeTurn(context.Background(), 7, id, "test-model")
	if !errors.Is(err, plugin.ErrWorkforceAgentPaused) {
		t.Fatalf("AuthorizeTurn = %v, want ErrWorkforceAgentPaused", err)
	}
}

// TestWorkforceSPI_RefusesAnUnpricedModel pins fail-closed pricing: a model the
// workspace never priced cannot be bounded, so the turn does not start.
func TestWorkforceSPI_RefusesAnUnpricedModel(t *testing.T) {
	w, store := newWorkforceSPI(t)
	id := seedAgent(t, store, 7, "active")

	err := w.AuthorizeTurn(context.Background(), 7, id, "never-priced")
	if !errors.Is(err, plugin.ErrWorkforceModelUnpriced) {
		t.Fatalf("AuthorizeTurn = %v, want ErrWorkforceModelUnpriced", err)
	}
	if _, err := w.TurnCost(context.Background(), 7, "never-priced"); !errors.Is(err, plugin.ErrWorkforceModelUnpriced) {
		t.Fatalf("TurnCost = %v, want ErrWorkforceModelUnpriced", err)
	}
}

// TestWorkforceSPI_TurnCostMatchesTheLedger pins that the price a plugin uses
// for its own ceiling is the price the ledger charges.
func TestWorkforceSPI_TurnCostMatchesTheLedger(t *testing.T) {
	w, store := newWorkforceSPI(t)
	id := seedAgent(t, store, 7, "active")
	ctx := context.Background()

	cost, err := w.TurnCost(ctx, 7, "test-model")
	if err != nil {
		t.Fatalf("TurnCost: %v", err)
	}
	// 1M input tokens at 3 USD per million and 1M output at 15.
	if got := cost(1_000_000, 1_000_000, 0); got != 18 {
		t.Fatalf("cost(1M, 1M) = %v, want 18", got)
	}
	record, err := w.RecordSpend(ctx, 7, plugin.TurnSpend{
		AgentID: id, Model: "test-model", InputTokens: 1_000_000, OutputTokens: 1_000_000,
		Source: models.WorkforceSpendSourceRuntime, TurnID: "turn-1",
	})
	if err != nil {
		t.Fatalf("RecordSpend: %v", err)
	}
	if record.CostUSD != 18 {
		t.Fatalf("ledger cost = %v, want the price the plugin was quoted (18)", record.CostUSD)
	}
}

// TestWorkforceSPI_RefusesAReplayedTurn pins the double-charge guard through
// the SPI: a retry of the same turn is refused, not billed again.
func TestWorkforceSPI_RefusesAReplayedTurn(t *testing.T) {
	w, store := newWorkforceSPI(t)
	id := seedAgent(t, store, 7, "active")
	ctx := context.Background()
	spend := plugin.TurnSpend{
		AgentID: id, Model: "test-model", InputTokens: 1000, OutputTokens: 500,
		Source: models.WorkforceSpendSourceRuntime, TurnID: "turn-1",
	}

	if _, err := w.RecordSpend(ctx, 7, spend); err != nil {
		t.Fatalf("RecordSpend: %v", err)
	}
	if _, err := w.RecordSpend(ctx, 7, spend); !errors.Is(err, plugin.ErrWorkforceSpendReplayed) {
		t.Fatalf("second RecordSpend = %v, want ErrWorkforceSpendReplayed", err)
	}
}

// TestWorkforceSPI_ApprovalIsACompareAndSwap pins that a decision resolves the
// gate once. A replayed decision must not move the task again.
func TestWorkforceSPI_ApprovalIsACompareAndSwap(t *testing.T) {
	w, store := newWorkforceSPI(t)
	id := seedAgent(t, store, 7, "active")
	ctx := context.Background()
	task, err := store.CreateTask(ctx, 7, models.WorkforceTask{
		Title: "publish", AssigneeAgentID: id, Status: "awaiting_approval", Summary: "draft ready",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	if err := w.DecideApproval(ctx, 7, task.ID, true); err != nil {
		t.Fatalf("DecideApproval = %v, want nil", err)
	}
	if err := w.DecideApproval(ctx, 7, task.ID, true); !errors.Is(err, plugin.ErrWorkforceApprovalNotPending) {
		t.Fatalf("replayed DecideApproval = %v, want ErrWorkforceApprovalNotPending", err)
	}
	if err := w.DecideApproval(ctx, 7, "AGT-missing", true); !errors.Is(err, plugin.ErrWorkforceTaskNotFound) {
		t.Fatalf("unknown task = %v, want ErrWorkforceTaskNotFound", err)
	}
}

// TestWorkforceSPI_RefusesAnAgentOfAnotherWorkspace pins the tenant boundary:
// the ledger never answers for a workspace the caller did not name.
func TestWorkforceSPI_RefusesAnAgentOfAnotherWorkspace(t *testing.T) {
	w, store := newWorkforceSPI(t)
	id := seedAgent(t, store, 7, "active")

	err := w.AuthorizeTurn(context.Background(), 8, id, "test-model")
	if !errors.Is(err, plugin.ErrWorkforceAgentNotFound) {
		t.Fatalf("AuthorizeTurn = %v, want ErrWorkforceAgentNotFound", err)
	}
}

// TestWorkforceSPI_WithoutAStoreRefusesEveryCall pins the host with no ledger:
// a turn nobody can bill must not start on a zero-cost answer.
func TestWorkforceSPI_WithoutAStoreRefusesEveryCall(t *testing.T) {
	w := newWorkforceService(nil)
	ctx := context.Background()

	if err := w.AuthorizeTurn(ctx, 1, "a", "m"); !errors.Is(err, plugin.ErrWorkforceUnavailable) {
		t.Fatalf("AuthorizeTurn = %v, want ErrWorkforceUnavailable", err)
	}
	if _, err := w.TurnCost(ctx, 1, "m"); !errors.Is(err, plugin.ErrWorkforceUnavailable) {
		t.Fatalf("TurnCost = %v, want ErrWorkforceUnavailable", err)
	}
	if _, err := w.RecordSpend(ctx, 1, plugin.TurnSpend{}); !errors.Is(err, plugin.ErrWorkforceUnavailable) {
		t.Fatalf("RecordSpend = %v, want ErrWorkforceUnavailable", err)
	}
	if err := w.DecideApproval(ctx, 1, "t", true); !errors.Is(err, plugin.ErrWorkforceUnavailable) {
		t.Fatalf("DecideApproval = %v, want ErrWorkforceUnavailable", err)
	}
}

// TestNoopHostWorkforceRefuses pins the same rule for the host a plugin gets
// when the context carries none.
func TestNoopHostWorkforceRefuses(t *testing.T) {
	host := &plugin.TestHost{}
	w := host.Workforce()
	if err := w.AuthorizeTurn(context.Background(), 1, "a", "m"); !errors.Is(err, plugin.ErrWorkforceUnavailable) {
		t.Fatalf("AuthorizeTurn = %v, want ErrWorkforceUnavailable", err)
	}
}
