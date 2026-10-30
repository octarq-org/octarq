package agentstore

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func seedBillingAgent(t *testing.T, store *WorkforceStore, orgID uint, limit float64) models.WorkforceAgent {
	t.Helper()
	agent, err := store.CreateAgent(context.Background(), orgID, models.WorkforceAgent{
		Name:        "Billing Bot",
		Role:        "Operator",
		Department:  "Operations",
		Model:       "claude-opus-4-8",
		BudgetLimit: limit,
	})
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	return *agent
}

func closeEnough(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

// TestWorkforceBilling_PriceCRUD pins the price source of truth: one row per
// model per workspace, an upsert replaces the values, and a delete of a
// missing price is a distinct answer from a delete that succeeded.
func TestWorkforceBilling_PriceCRUD(t *testing.T) {
	t.Parallel()
	store, _ := newTestWorkforceStore(t)
	ctx := context.Background()
	orgID := uint(501)

	price, err := store.SetModelPrice(ctx, orgID, "claude-opus-4-8", 3, 15)
	if err != nil {
		t.Fatalf("SetModelPrice: %v", err)
	}
	if price.Model != "claude-opus-4-8" || price.InputPerMTok != 3 || price.OutputPerMTok != 15 {
		t.Fatalf("unexpected price row: %+v", price)
	}

	updated, err := store.SetModelPrice(ctx, orgID, "claude-opus-4-8", 5, 25)
	if err != nil {
		t.Fatalf("SetModelPrice upsert: %v", err)
	}
	if updated.InputPerMTok != 5 || updated.OutputPerMTok != 25 {
		t.Fatalf("upsert did not replace the values: %+v", updated)
	}

	prices, err := store.ListModelPrices(ctx, orgID)
	if err != nil {
		t.Fatalf("ListModelPrices: %v", err)
	}
	if len(prices) != 1 || prices[0].InputPerMTok != 5 {
		t.Fatalf("expected one replaced row, got %+v", prices)
	}

	// Another workspace never sees the price.
	if other, err := store.ListModelPrices(ctx, 502); err != nil || len(other) != 0 {
		t.Fatalf("prices leaked across workspaces: %+v (err %v)", other, err)
	}

	if err := store.DeleteModelPrice(ctx, orgID, "claude-opus-4-8"); err != nil {
		t.Fatalf("DeleteModelPrice: %v", err)
	}
	if err := store.DeleteModelPrice(ctx, orgID, "claude-opus-4-8"); !errors.Is(err, ErrModelPriceNotFound) {
		t.Fatalf("expected ErrModelPriceNotFound on the second delete, got %v", err)
	}

	for _, bad := range []struct {
		model string
		in    float64
		out   float64
	}{
		{"", 1, 1},
		{"   ", 1, 1},
		{"m", -1, 1},
		{"m", 1, math.NaN()},
		{"m", math.Inf(1), 1},
	} {
		if _, err := store.SetModelPrice(ctx, orgID, bad.model, bad.in, bad.out); !errors.Is(err, ErrInvalidPrice) {
			t.Fatalf("SetModelPrice(%q, %v, %v) = %v, want ErrInvalidPrice", bad.model, bad.in, bad.out, err)
		}
	}
}

// TestWorkforceBilling_AuthorizeTurn covers every refusal of the pre-turn
// gate, plus the two cases that must pass.
func TestWorkforceBilling_AuthorizeTurn(t *testing.T) {
	t.Parallel()
	store, db := newTestWorkforceStore(t)
	ctx := context.Background()
	orgID := uint(511)

	agent := seedBillingAgent(t, store, orgID, 10)
	if _, err := store.SetModelPrice(ctx, orgID, "claude-opus-4-8", 3, 15); err != nil {
		t.Fatalf("SetModelPrice: %v", err)
	}

	if _, err := store.AuthorizeTurn(ctx, orgID, "agent-missing", "claude-opus-4-8"); !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("unknown agent = %v, want ErrAgentNotFound", err)
	}
	if _, err := store.AuthorizeTurn(ctx, orgID, agent.ID, "unpriced-model"); !errors.Is(err, ErrModelPriceMissing) {
		t.Fatalf("unpriced model = %v, want ErrModelPriceMissing", err)
	}
	if _, err := store.AuthorizeTurn(ctx, orgID, agent.ID, "claude-opus-4-8"); err != nil {
		t.Fatalf("under budget = %v, want nil", err)
	}

	if _, err := store.PauseAgent(ctx, orgID, agent.ID); err != nil {
		t.Fatalf("PauseAgent: %v", err)
	}
	if _, err := store.AuthorizeTurn(ctx, orgID, agent.ID, "claude-opus-4-8"); !errors.Is(err, ErrAgentPaused) {
		t.Fatalf("paused agent = %v, want ErrAgentPaused", err)
	}
	if _, err := store.ResumeAgent(ctx, orgID, agent.ID); err != nil {
		t.Fatalf("ResumeAgent: %v", err)
	}

	// At the limit with auto-halt on (the default) the turn is refused.
	if err := db.Model(&models.WorkforceAgent{}).
		Where("id = ? AND owner_id = ?", agent.ID, orgID).
		Update("budget_used", 10.0).Error; err != nil {
		t.Fatalf("set budget used: %v", err)
	}
	if _, err := store.AuthorizeTurn(ctx, orgID, agent.ID, "claude-opus-4-8"); !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("exhausted budget = %v, want ErrBudgetExceeded", err)
	}

	// Auto-halt off makes the budget advisory.
	if err := store.SetAutoHalt(ctx, orgID, false); err != nil {
		t.Fatalf("SetAutoHalt: %v", err)
	}
	if _, err := store.AuthorizeTurn(ctx, orgID, agent.ID, "claude-opus-4-8"); err != nil {
		t.Fatalf("advisory budget = %v, want nil", err)
	}
}

// TestWorkforceBilling_RolloverReleasesTheBudget pins the monthly semantics: a
// counter from an earlier month never blocks this month.
func TestWorkforceBilling_RolloverReleasesTheBudget(t *testing.T) {
	t.Parallel()
	store, db := newTestWorkforceStore(t)
	ctx := context.Background()
	orgID := uint(521)

	agent := seedBillingAgent(t, store, orgID, 10)
	if _, err := store.SetModelPrice(ctx, orgID, "claude-opus-4-8", 3, 15); err != nil {
		t.Fatalf("SetModelPrice: %v", err)
	}
	lastMonth := monthStart(time.Now().UTC()).AddDate(0, -1, 0)
	if err := db.Model(&models.WorkforceAgent{}).
		Where("id = ? AND owner_id = ?", agent.ID, orgID).
		Updates(map[string]any{"budget_used": 10.0, "budget_period_start": lastMonth}).Error; err != nil {
		t.Fatalf("seed previous period: %v", err)
	}

	if _, err := store.AuthorizeTurn(ctx, orgID, agent.ID, "claude-opus-4-8"); err != nil {
		t.Fatalf("previous-month spend blocked the turn: %v", err)
	}

	// Every reader projects the stale counter to zero, so no view shows last
	// month's number as this month's spend.
	listed, err := store.ListAgents(ctx, orgID)
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(listed) != 1 || listed[0].BudgetUsed != 0 {
		t.Fatalf("ListAgents kept the stale counter: %+v", listed)
	}
	overview, err := store.GetOverview(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOverview: %v", err)
	}
	if overview.MonthlySpend != 0 {
		t.Fatalf("overview kept the stale counter: %v", overview.MonthlySpend)
	}

	event, updated, err := store.RecordSpend(ctx, orgID, SpendInput{
		AgentID: agent.ID, Model: "claude-opus-4-8",
		InputTokens: 1_000_000, OutputTokens: 0,
		Source: models.WorkforceSpendSourceRuntime, TurnID: "turn-rollover",
	})
	if err != nil {
		t.Fatalf("RecordSpend: %v", err)
	}
	if !closeEnough(event.CostUSD, 3) {
		t.Fatalf("cost = %v, want 3", event.CostUSD)
	}
	if !closeEnough(updated.BudgetUsed, 3) {
		t.Fatalf("BudgetUsed = %v, want 3 (the counter must roll over, not add to last month)", updated.BudgetUsed)
	}
	if !updated.BudgetPeriodStart.Equal(monthStart(time.Now().UTC())) {
		t.Fatalf("BudgetPeriodStart = %v, want %v", updated.BudgetPeriodStart, monthStart(time.Now().UTC()))
	}
}

// TestWorkforceBilling_RecordSpendAccrues covers pricing, the ledger row, the
// accrued counter, and the overview that reads it.
func TestWorkforceBilling_RecordSpendAccrues(t *testing.T) {
	t.Parallel()
	store, db := newTestWorkforceStore(t)
	ctx := context.Background()
	orgID := uint(531)

	agent := seedBillingAgent(t, store, orgID, 10)
	if _, err := store.SetModelPrice(ctx, orgID, "claude-opus-4-8", 3, 15); err != nil {
		t.Fatalf("SetModelPrice: %v", err)
	}

	event, updated, err := store.RecordSpend(ctx, orgID, SpendInput{
		AgentID:         agent.ID,
		Model:           "claude-opus-4-8",
		InputTokens:     1_000_000,
		OutputTokens:    200_000,
		ReasoningTokens: 50_000,
		Source:          models.WorkforceSpendSourceRuntime,
		SessionID:       "sess-1",
		TurnID:          "turn-accrue-1",
	})
	if err != nil {
		t.Fatalf("RecordSpend: %v", err)
	}
	// 1M input at $3/M plus 200k output at $15/M. Reasoning tokens are part of
	// output tokens and are not billed twice.
	if !closeEnough(event.CostUSD, 6) {
		t.Fatalf("cost = %v, want 6", event.CostUSD)
	}
	if !closeEnough(updated.BudgetUsed, 6) {
		t.Fatalf("BudgetUsed = %v, want 6", updated.BudgetUsed)
	}
	if updated.Status != "active" {
		t.Fatalf("agent paused below the limit: %s", updated.Status)
	}

	var stored models.WorkforceSpendEvent
	if err := db.Where("owner_id = ? AND agent_id = ?", orgID, agent.ID).First(&stored).Error; err != nil {
		t.Fatalf("ledger row missing: %v", err)
	}
	if stored.Source != models.WorkforceSpendSourceRuntime || stored.SessionID != "sess-1" {
		t.Fatalf("ledger provenance wrong: %+v", stored)
	}

	// A second turn adds to the counter instead of replacing it.
	if _, second, err := store.RecordSpend(ctx, orgID, SpendInput{
		AgentID:         agent.ID,
		Model:           "claude-opus-4-8",
		InputTokens:     1_000_000,
		OutputTokens:    200_000,
		ReasoningTokens: 50_000,
		Source:          models.WorkforceSpendSourceRuntime,
		SessionID:       "sess-2",
		TurnID:          "turn-accrue-2",
	}); err != nil {
		t.Fatalf("second RecordSpend: %v", err)
	} else if !closeEnough(second.BudgetUsed, 12) {
		t.Fatalf("BudgetUsed after the second turn = %v, want 12", second.BudgetUsed)
	}

	// The ledger is the audit trail of the counter.
	var ledgerSum float64
	if err := db.Model(&models.WorkforceSpendEvent{}).
		Where("owner_id = ? AND agent_id = ?", orgID, agent.ID).
		Select("COALESCE(SUM(cost_usd), 0)").Scan(&ledgerSum).Error; err != nil {
		t.Fatalf("sum ledger: %v", err)
	}
	fresh, err := store.GetAgent(ctx, orgID, agent.ID)
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if !closeEnough(ledgerSum, fresh.BudgetUsed) {
		t.Fatalf("ledger sum %v != BudgetUsed %v", ledgerSum, fresh.BudgetUsed)
	}

	overview, err := store.GetOverview(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOverview: %v", err)
	}
	if !closeEnough(overview.MonthlySpend, 12) {
		t.Fatalf("overview spend = %v, want 12", overview.MonthlySpend)
	}
}

// TestWorkforceBilling_AutoPause pins the halt itself: crossing the limit with
// auto-halt on pauses the agent in the same write, and auto-halt off leaves it
// active.
func TestWorkforceBilling_AutoPause(t *testing.T) {
	t.Parallel()
	store, _ := newTestWorkforceStore(t)
	ctx := context.Background()
	orgID := uint(541)

	agent := seedBillingAgent(t, store, orgID, 1)
	if _, err := store.SetModelPrice(ctx, orgID, "m", 1, 0); err != nil {
		t.Fatalf("SetModelPrice: %v", err)
	}

	_, updated, err := store.RecordSpend(ctx, orgID, SpendInput{
		AgentID: agent.ID, Model: "m", InputTokens: 2_000_000,
		Source: models.WorkforceSpendSourceRuntime, TurnID: "turn-limit",
	})
	if err != nil {
		t.Fatalf("RecordSpend: %v", err)
	}
	if updated.Status != "paused" {
		t.Fatalf("agent status = %q, want paused after crossing the limit", updated.Status)
	}
	if _, err := store.AuthorizeTurn(ctx, orgID, agent.ID, "m"); !errors.Is(err, ErrAgentPaused) {
		t.Fatalf("paused agent ran a turn: %v", err)
	}

	// Auto-halt off: the spend is still recorded, the agent keeps running.
	store2, _ := newTestWorkforceStore(t)
	orgID2 := uint(542)
	agent2 := seedBillingAgent(t, store2, orgID2, 1)
	if err := store2.SetAutoHalt(ctx, orgID2, false); err != nil {
		t.Fatalf("SetAutoHalt: %v", err)
	}
	if _, err := store2.SetModelPrice(ctx, orgID2, "m", 1, 0); err != nil {
		t.Fatalf("SetModelPrice: %v", err)
	}
	_, updated2, err := store2.RecordSpend(ctx, orgID2, SpendInput{
		AgentID: agent2.ID, Model: "m", InputTokens: 2_000_000,
		Source: models.WorkforceSpendSourceRuntime, TurnID: "turn-limit-off",
	})
	if err != nil {
		t.Fatalf("RecordSpend advisory: %v", err)
	}
	if updated2.Status == "paused" {
		t.Fatal("auto-halt off paused the agent")
	}
	if !closeEnough(updated2.BudgetUsed, 2) {
		t.Fatalf("BudgetUsed = %v, want 2", updated2.BudgetUsed)
	}
}

// TestWorkforceBilling_RecordSpendValidation covers the fail-closed inputs.
func TestWorkforceBilling_RecordSpendValidation(t *testing.T) {
	t.Parallel()
	store, _ := newTestWorkforceStore(t)
	ctx := context.Background()
	orgID := uint(551)

	agent := seedBillingAgent(t, store, orgID, 10)
	if _, err := store.SetModelPrice(ctx, orgID, "m", 1, 1); err != nil {
		t.Fatalf("SetModelPrice: %v", err)
	}

	cases := []struct {
		name string
		in   SpendInput
		want error
	}{
		{"unknown agent", SpendInput{AgentID: "agent-missing", Model: "m", Source: models.WorkforceSpendSourceRuntime, TurnID: "turn-unknown"}, ErrAgentNotFound},
		{"unpriced model", SpendInput{AgentID: agent.ID, Model: "nope", Source: models.WorkforceSpendSourceRuntime, TurnID: "turn-unpriced"}, ErrModelPriceMissing},
		{"no model", SpendInput{AgentID: agent.ID, Source: models.WorkforceSpendSourceRuntime, TurnID: "turn-no-model"}, ErrInvalidSpend},
		{"over-long model", SpendInput{AgentID: agent.ID, Model: strings.Repeat("m", 65), Source: models.WorkforceSpendSourceRuntime, TurnID: "turn-long-model"}, ErrInvalidSpend},
		{"bad source", SpendInput{AgentID: agent.ID, Model: "m", Source: "webhook", TurnID: "turn-bad-source"}, ErrInvalidSpend},
		{"runtime turn without an id", SpendInput{AgentID: agent.ID, Model: "m", Source: models.WorkforceSpendSourceRuntime}, ErrInvalidSpend},
		{"over-long turn id", SpendInput{AgentID: agent.ID, Model: "m", Source: models.WorkforceSpendSourceRuntime, TurnID: strings.Repeat("t", 65)}, ErrInvalidSpend},
		{"negative input", SpendInput{AgentID: agent.ID, Model: "m", InputTokens: -1, Source: models.WorkforceSpendSourceRuntime, TurnID: "turn-neg-in"}, ErrInvalidSpend},
		{"negative output", SpendInput{AgentID: agent.ID, Model: "m", OutputTokens: -1, Source: models.WorkforceSpendSourceRuntime, TurnID: "turn-neg-out"}, ErrInvalidSpend},
		{"negative reasoning", SpendInput{AgentID: agent.ID, Model: "m", ReasoningTokens: -1, Source: models.WorkforceSpendSourceRuntime, TurnID: "turn-neg-reasoning"}, ErrInvalidSpend},
	}
	for _, tc := range cases {
		if _, _, err := store.RecordSpend(ctx, orgID, tc.in); !errors.Is(err, tc.want) {
			t.Errorf("%s = %v, want %v", tc.name, err, tc.want)
		}
	}
}

// TestWorkforceBilling_TenantIsolation pins that neither the price table nor
// the ledger crosses a workspace boundary.
func TestWorkforceBilling_TenantIsolation(t *testing.T) {
	t.Parallel()
	store, _ := newTestWorkforceStore(t)
	ctx := context.Background()
	org1 := uint(561)
	org2 := uint(562)

	agent := seedBillingAgent(t, store, org1, 10)
	if _, err := store.SetModelPrice(ctx, org1, "m", 1, 1); err != nil {
		t.Fatalf("SetModelPrice org1: %v", err)
	}

	// Org 2 has no price for the model, so it cannot price a turn at all.
	if _, _, err := store.RecordSpend(ctx, org2, SpendInput{
		AgentID: agent.ID, Model: "m", InputTokens: 1,
		Source: models.WorkforceSpendSourceRuntime, TurnID: "turn-cross-org",
	}); !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("cross-tenant spend = %v, want ErrAgentNotFound", err)
	}
	if _, err := store.AuthorizeTurn(ctx, org2, agent.ID, "m"); !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("cross-tenant authorize = %v, want ErrAgentNotFound", err)
	}
	if _, err := store.ListSpendEvents(ctx, org2, agent.ID, 10); !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("cross-tenant ledger = %v, want ErrAgentNotFound", err)
	}

	// Org 2 has its own agent but no price. Org 1's price must not price an
	// org 2 turn.
	agent2 := seedBillingAgent(t, store, org2, 10)
	if _, err := store.AuthorizeTurn(ctx, org2, agent2.ID, "m"); !errors.Is(err, ErrModelPriceMissing) {
		t.Fatalf("authorize priced from another workspace: %v", err)
	}
	if _, _, err := store.RecordSpend(ctx, org2, SpendInput{
		AgentID: agent2.ID, Model: "m", InputTokens: 1,
		Source: models.WorkforceSpendSourceRuntime, TurnID: "turn-cross-org-2",
	}); !errors.Is(err, ErrModelPriceMissing) {
		t.Fatalf("spend priced from another workspace: %v", err)
	}

	if _, _, err := store.RecordSpend(ctx, org1, SpendInput{
		AgentID: agent.ID, Model: "m", InputTokens: 1_000_000,
		Source: models.WorkforceSpendSourceRuntime, TurnID: "turn-org1",
	}); err != nil {
		t.Fatalf("RecordSpend org1: %v", err)
	}
	events, err := store.ListSpendEvents(ctx, org1, agent.ID, 10)
	if err != nil {
		t.Fatalf("ListSpendEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one ledger row, got %d", len(events))
	}
}

// TestWorkforceBilling_ListSpendEvents pins the newest-first order, the limit,
// and the 404-style answer for an unknown agent.
func TestWorkforceBilling_ListSpendEvents(t *testing.T) {
	t.Parallel()
	store, _ := newTestWorkforceStore(t)
	ctx := context.Background()
	orgID := uint(571)

	agent := seedBillingAgent(t, store, orgID, 100)
	if _, err := store.SetModelPrice(ctx, orgID, "m", 1, 0); err != nil {
		t.Fatalf("SetModelPrice: %v", err)
	}
	for i := 0; i < 3; i++ {
		if _, _, err := store.RecordSpend(ctx, orgID, SpendInput{
			AgentID: agent.ID, Model: "m", InputTokens: 1_000_000,
			Source: models.WorkforceSpendSourceRuntime, TurnID: fmt.Sprintf("turn-list-%d", i),
		}); err != nil {
			t.Fatalf("RecordSpend %d: %v", i, err)
		}
	}

	events, err := store.ListSpendEvents(ctx, orgID, agent.ID, 2)
	if err != nil {
		t.Fatalf("ListSpendEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("limit not applied: got %d rows", len(events))
	}
	if events[0].ID <= events[1].ID {
		t.Fatalf("rows are not newest first: %d, %d", events[0].ID, events[1].ID)
	}

	if _, err := store.ListSpendEvents(ctx, orgID, "agent-missing", 10); !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("unknown agent = %v, want ErrAgentNotFound", err)
	}
}

// TestWorkforceBilling_ZeroPriceModelIsBillable pins that a self-hosted model
// with a zero price is priced (cost 0), not mistaken for unpriced.
func TestWorkforceBilling_ZeroPriceModelIsBillable(t *testing.T) {
	t.Parallel()
	store, _ := newTestWorkforceStore(t)
	ctx := context.Background()
	orgID := uint(581)

	agent := seedBillingAgent(t, store, orgID, 10)
	if _, err := store.SetModelPrice(ctx, orgID, "local-llama", 0, 0); err != nil {
		t.Fatalf("SetModelPrice: %v", err)
	}
	event, updated, err := store.RecordSpend(ctx, orgID, SpendInput{
		AgentID: agent.ID, Model: "local-llama", InputTokens: 1000, OutputTokens: 1000,
		Source: models.WorkforceSpendSourceRuntime, TurnID: "turn-zero-price",
	})
	if err != nil {
		t.Fatalf("RecordSpend: %v", err)
	}
	if event.CostUSD != 0 || updated.BudgetUsed != 0 {
		t.Fatalf("zero-price model billed %v / %v", event.CostUSD, updated.BudgetUsed)
	}
}

// TestWorkforceBilling_StoreNotConfigured pins the nil guard every public
// method shares.
func TestWorkforceBilling_StoreNotConfigured(t *testing.T) {
	t.Parallel()
	store := NewWorkforceStore(nil)
	ctx := context.Background()
	if _, err := store.ListModelPrices(ctx, 1); err == nil {
		t.Error("ListModelPrices on a nil store did not fail")
	}
	if _, err := store.SetModelPrice(ctx, 1, "m", 1, 1); err == nil {
		t.Error("SetModelPrice on a nil store did not fail")
	}
	if err := store.DeleteModelPrice(ctx, 1, "m"); err == nil {
		t.Error("DeleteModelPrice on a nil store did not fail")
	}
	if _, err := store.AuthorizeTurn(ctx, 1, "a", "m"); err == nil {
		t.Error("AuthorizeTurn on a nil store did not fail")
	}
	if _, _, err := store.RecordSpend(ctx, 1, SpendInput{}); err == nil {
		t.Error("RecordSpend on a nil store did not fail")
	}
	if _, err := store.ListSpendEvents(ctx, 1, "a", 10); err == nil {
		t.Error("ListSpendEvents on a nil store did not fail")
	}
}

// TestWorkforceBilling_SubCentTurnsAccrue pins the ledger and the counter to
// the same scale. A billed turn can cost less than one cent, so a counter that
// rounds each accrual disagrees with the ledger it is reconciled against.
func TestWorkforceBilling_SubCentTurnsAccrue(t *testing.T) {
	t.Parallel()
	store, db := newTestWorkforceStore(t)
	ctx := context.Background()
	orgID := uint(571)

	agent := seedBillingAgent(t, store, orgID, 10)
	// $3/M input and $15/M output: 1,000 in and 100 out is $0.0045 a turn.
	if _, err := store.SetModelPrice(ctx, orgID, "m", 3, 15); err != nil {
		t.Fatalf("SetModelPrice: %v", err)
	}
	const turns = 100
	for i := 0; i < turns; i++ {
		if _, _, err := store.RecordSpend(ctx, orgID, SpendInput{
			AgentID: agent.ID, Model: "m", InputTokens: 1_000, OutputTokens: 100,
			Source: models.WorkforceSpendSourceRuntime, TurnID: fmt.Sprintf("turn-subcent-%d", i),
		}); err != nil {
			t.Fatalf("RecordSpend %d: %v", i, err)
		}
	}

	var ledgerSum float64
	if err := db.Model(&models.WorkforceSpendEvent{}).
		Where("owner_id = ? AND agent_id = ?", orgID, agent.ID).
		Select("COALESCE(SUM(cost_usd), 0)").Scan(&ledgerSum).Error; err != nil {
		t.Fatalf("sum ledger: %v", err)
	}
	fresh, err := store.GetAgent(ctx, orgID, agent.ID)
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if !closeEnough(ledgerSum, 0.45) {
		t.Fatalf("ledger sum = %v, want 0.45", ledgerSum)
	}
	if !closeEnough(fresh.BudgetUsed, ledgerSum) {
		t.Fatalf("BudgetUsed %v != ledger sum %v: the counter lost sub-cent spend", fresh.BudgetUsed, ledgerSum)
	}
}

// TestWorkforceBilling_BudgetCounterColumnKeepsLedgerScale pins the DDL of the
// counter column. SQLite stores every number as a REAL, so the accrual tests
// cannot see a column that rounds sub-cent spend away; Postgres enforces the
// declared scale. Two decimals would make a $0.0045 turn accrue $0.00.
func TestWorkforceBilling_BudgetCounterColumnKeepsLedgerScale(t *testing.T) {
	t.Parallel()

	db, err := gorm.Open(
		postgres.New(postgres.Config{DSN: "host=127.0.0.1 port=1 user=test dbname=test sslmode=disable"}),
		&gorm.Config{DisableAutomaticPing: true},
	)
	if err != nil {
		t.Fatalf("open postgres dialector: %v", err)
	}
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(&models.WorkforceAgent{}); err != nil {
		t.Fatalf("parse model: %v", err)
	}
	field := stmt.Schema.LookUpField("BudgetUsed")
	if field == nil {
		t.Fatal("BudgetUsed field missing from the agent schema")
	}
	ddl := fmt.Sprint(db.Migrator().FullDataTypeOf(field).SQL)
	if !strings.Contains(ddl, "decimal(12,6)") {
		t.Fatalf("budget_used DDL = %q, want decimal(12,6): the ledger keeps six decimals", ddl)
	}
}

// TestWorkforceBilling_RecordSpendTrimsTheModelLabel pins the stored label to
// the spelling the price row uses, so a turn cannot ledger a label that never
// matches a price.
func TestWorkforceBilling_RecordSpendTrimsTheModelLabel(t *testing.T) {
	t.Parallel()
	store, _ := newTestWorkforceStore(t)
	ctx := context.Background()
	orgID := uint(581)

	agent := seedBillingAgent(t, store, orgID, 10)
	if _, err := store.SetModelPrice(ctx, orgID, "m", 1, 1); err != nil {
		t.Fatalf("SetModelPrice: %v", err)
	}
	event, _, err := store.RecordSpend(ctx, orgID, SpendInput{
		AgentID: agent.ID, Model: "  m  ", InputTokens: 10,
		Source: models.WorkforceSpendSourceRuntime, TurnID: "turn-trim",
	})
	if err != nil {
		t.Fatalf("RecordSpend: %v", err)
	}
	if event.Model != "m" {
		t.Fatalf("stored model label = %q, want %q", event.Model, "m")
	}
}

// TestWorkforceBilling_ProviderSpendFollowsTheLedger pins the provider
// breakdown to the model that was billed. An agent's display model does not
// select the price or the route, so attributing its spend to that label would
// charge a provider that never ran the turn.
func TestWorkforceBilling_ProviderSpendFollowsTheLedger(t *testing.T) {
	t.Parallel()
	store, _ := newTestWorkforceStore(t)
	ctx := context.Background()
	orgID := uint(591)

	// The agent is labelled as an Anthropic model but its turn is billed on an
	// OpenAI model, which is what the request named.
	agent := seedBillingAgent(t, store, orgID, 10)
	if _, err := store.SetModelPrice(ctx, orgID, "gpt-4o", 5, 15); err != nil {
		t.Fatalf("SetModelPrice: %v", err)
	}
	if _, _, err := store.RecordSpend(ctx, orgID, SpendInput{
		AgentID: agent.ID, Model: "gpt-4o", InputTokens: 1_000_000,
		Source: models.WorkforceSpendSourceRuntime, TurnID: "turn-provider",
	}); err != nil {
		t.Fatalf("RecordSpend: %v", err)
	}

	overview, err := store.GetOverview(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOverview: %v", err)
	}
	if len(overview.SpendByProvider) != 1 || overview.SpendByProvider[0].Provider != "OpenAI" {
		t.Fatalf("provider breakdown = %+v, want only OpenAI", overview.SpendByProvider)
	}
	if !closeEnough(overview.SpendByProvider[0].Spend, 5) {
		t.Fatalf("provider spend = %v, want 5", overview.SpendByProvider[0].Spend)
	}
}

// TestWorkforceBilling_PriceBounds pins the price range to what the column can
// hold. A price the column cannot store must be a client error, and a price
// that rounds to zero must not silently make a model free.
func TestWorkforceBilling_PriceBounds(t *testing.T) {
	t.Parallel()
	store, _ := newTestWorkforceStore(t)
	ctx := context.Background()
	orgID := uint(601)

	cases := []struct {
		name string
		in   float64
	}{
		{"above the column range", 1e12},
		{"below the column scale", 0.00004},
		{"infinite", math.Inf(1)},
		{"not a number", math.NaN()},
	}
	for _, tc := range cases {
		if _, err := store.SetModelPrice(ctx, orgID, "m", tc.in, 1); !errors.Is(err, ErrInvalidPrice) {
			t.Errorf("%s: err = %v, want ErrInvalidPrice", tc.name, err)
		}
	}

	// The smallest representable non-zero price is accepted and kept.
	price, err := store.SetModelPrice(ctx, orgID, "m", 0.0001, 0)
	if err != nil {
		t.Fatalf("SetModelPrice at the smallest step: %v", err)
	}
	if price.InputPerMTok != 0.0001 {
		t.Fatalf("stored input price = %v, want 0.0001", price.InputPerMTok)
	}
}

// TestWorkforceBilling_TurnIDMakesIngestIdempotent pins the contract a
// retrying runtime depends on: the same turn identifier bills once, and the
// retry answers with the original charge.
func TestWorkforceBilling_TurnIDMakesIngestIdempotent(t *testing.T) {
	t.Parallel()
	store, _ := newTestWorkforceStore(t)
	ctx := context.Background()
	orgID := uint(611)

	agent := seedBillingAgent(t, store, orgID, 10)
	other := seedBillingAgent(t, store, orgID, 10)
	if _, err := store.SetModelPrice(ctx, orgID, "m", 1, 0); err != nil {
		t.Fatalf("SetModelPrice: %v", err)
	}

	input := SpendInput{
		AgentID: agent.ID, Model: "m", InputTokens: 1_000_000,
		Source: models.WorkforceSpendSourceRuntime, TurnID: "turn-1",
	}
	first, _, err := store.RecordSpend(ctx, orgID, input)
	if err != nil {
		t.Fatalf("RecordSpend: %v", err)
	}

	if _, _, err := store.RecordSpend(ctx, orgID, input); !errors.Is(err, ErrSpendReplayed) {
		t.Fatalf("replayed turn = %v, want ErrSpendReplayed", err)
	}

	events, err := store.ListSpendEvents(ctx, orgID, agent.ID, 10)
	if err != nil {
		t.Fatalf("ListSpendEvents: %v", err)
	}
	if len(events) != 1 || events[0].ID != first.ID {
		t.Fatalf("replay wrote a ledger row: %+v", events)
	}
	after, err := store.GetAgent(ctx, orgID, agent.ID)
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if !closeEnough(after.BudgetUsed, 1) {
		t.Fatalf("BudgetUsed after replay = %v, want 1", after.BudgetUsed)
	}

	// The replay reader answers with the row the first report wrote.
	got, owner, err := store.SpendByTurn(ctx, orgID, agent.ID, "turn-1")
	if err != nil {
		t.Fatalf("SpendByTurn: %v", err)
	}
	if got.ID != first.ID || !closeEnough(got.CostUSD, 1) {
		t.Fatalf("SpendByTurn = %+v, want event %d costing 1", got, first.ID)
	}
	if owner == nil || !closeEnough(owner.BudgetUsed, 1) {
		t.Fatalf("SpendByTurn agent counter = %+v, want 1", owner)
	}
	if _, _, err := store.SpendByTurn(ctx, orgID, agent.ID, "turn-unknown"); !errors.Is(err, ErrSpendTurnNotFound) {
		t.Fatalf("SpendByTurn on an unknown turn = %v, want ErrSpendTurnNotFound", err)
	}

	// The identifier is scoped to the agent: the same id on another agent is a
	// new charge.
	if _, _, err := store.RecordSpend(ctx, orgID, SpendInput{
		AgentID: other.ID, Model: "m", InputTokens: 1_000_000,
		Source: models.WorkforceSpendSourceRuntime, TurnID: "turn-1",
	}); err != nil {
		t.Fatalf("second agent, same turn id: %v", err)
	}
	if otherAfter, err := store.GetAgent(ctx, orgID, other.ID); err != nil || !closeEnough(otherAfter.BudgetUsed, 1) {
		t.Fatalf("second agent counter = %+v (err %v), want 1", otherAfter, err)
	}

	// A chat turn carries no identifier, and the unique index lets every one of
	// them through.
	for i := 0; i < 2; i++ {
		if _, _, err := store.RecordSpend(ctx, orgID, SpendInput{
			AgentID: agent.ID, Model: "m", InputTokens: 1_000_000,
			Source: models.WorkforceSpendSourceChat, SessionID: "sess-chat",
		}); err != nil {
			t.Fatalf("chat turn %d: %v", i, err)
		}
	}
	events, err = store.ListSpendEvents(ctx, orgID, agent.ID, 10)
	if err != nil {
		t.Fatalf("ListSpendEvents: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("ledger rows = %d, want 3", len(events))
	}

	// A price removed after the first report does not erase the charge. The
	// retry still answers with the row; a new turn is refused.
	if err := store.DeleteModelPrice(ctx, orgID, "m"); err != nil {
		t.Fatalf("DeleteModelPrice: %v", err)
	}
	if _, _, err := store.RecordSpend(ctx, orgID, input); !errors.Is(err, ErrSpendReplayed) {
		t.Fatalf("replay after the price was removed = %v, want ErrSpendReplayed", err)
	}
	if _, _, err := store.SpendByTurn(ctx, orgID, agent.ID, "turn-1"); err != nil {
		t.Fatalf("SpendByTurn after the price was removed: %v", err)
	}
	if _, _, err := store.RecordSpend(ctx, orgID, SpendInput{
		AgentID: agent.ID, Model: "m", InputTokens: 1,
		Source: models.WorkforceSpendSourceRuntime, TurnID: "turn-2",
	}); !errors.Is(err, ErrModelPriceMissing) {
		t.Fatalf("new turn on an unpriced model = %v, want ErrModelPriceMissing", err)
	}
}

// TestWorkforceBilling_TurnIndexIsUnique pins the DDL that makes ingest
// idempotent on a server that enforces constraints. A plain index would let two
// concurrent reports of one turn each take a ledger row and a charge.
func TestWorkforceBilling_TurnIndexIsUnique(t *testing.T) {
	t.Parallel()

	db, err := gorm.Open(
		postgres.New(postgres.Config{DSN: "host=127.0.0.1 port=1 user=test dbname=test sslmode=disable"}),
		&gorm.Config{DisableAutomaticPing: true},
	)
	if err != nil {
		t.Fatalf("open postgres dialector: %v", err)
	}
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(&models.WorkforceSpendEvent{}); err != nil {
		t.Fatalf("parse model: %v", err)
	}
	var idx *schema.Index
	for _, candidate := range stmt.Schema.ParseIndexes() {
		if candidate.Name == "idx_workforce_spend_turn" {
			idx = candidate
			break
		}
	}
	if idx == nil {
		t.Fatal("idx_workforce_spend_turn is not declared on the ledger")
	}
	if idx.Class != "UNIQUE" {
		t.Fatalf("idx_workforce_spend_turn is not unique: a replayed turn could be billed twice")
	}
	var columns []string
	for _, f := range idx.Fields {
		columns = append(columns, f.DBName)
	}
	want := []string{"owner_id", "agent_id", "external_id"}
	if len(columns) != len(want) {
		t.Fatalf("index columns = %v, want %v", columns, want)
	}
	for i := range want {
		if columns[i] != want[i] {
			t.Fatalf("index columns = %v, want %v (a replay must match the same workspace, agent, and turn)", columns, want)
		}
	}
}
