package app

import (
	"context"
	"errors"

	"github.com/octarq-org/octarq/server/internal/agentstore"
	"github.com/octarq-org/octarq/server/plugin"
)

// workforceService exposes the agent ledger to plugins through
// plugin.Workforce.
//
// The store lives in internal/agentstore, which a plugin cannot import, and no
// plugin may reach the ledger tables directly. This adapter is the only path,
// so every agent a plugin runs bills the same ledger the dashboard shows.
type workforceService struct {
	store *agentstore.WorkforceStore
}

var _ plugin.Workforce = (*workforceService)(nil)

func newWorkforceService(store *agentstore.WorkforceStore) plugin.Workforce {
	return &workforceService{store: store}
}

func (w *workforceService) AuthorizeTurn(ctx context.Context, orgID uint, agentID, model string) error {
	if w == nil || w.store == nil {
		return plugin.ErrWorkforceUnavailable
	}
	_, err := w.store.AuthorizeTurn(ctx, orgID, agentID, model)
	return workforceError(err)
}

func (w *workforceService) TurnCost(ctx context.Context, orgID uint, model string) (func(input, output, reasoning int) float64, error) {
	if w == nil || w.store == nil {
		return nil, plugin.ErrWorkforceUnavailable
	}
	cost, err := w.store.TurnCostFunc(ctx, orgID, model)
	if err != nil {
		return nil, workforceError(err)
	}
	return cost, nil
}

func (w *workforceService) RecordSpend(ctx context.Context, orgID uint, spend plugin.TurnSpend) (*plugin.SpendRecord, error) {
	if w == nil || w.store == nil {
		return nil, plugin.ErrWorkforceUnavailable
	}
	event, agent, err := w.store.RecordSpend(ctx, orgID, agentstore.SpendInput{
		AgentID:         spend.AgentID,
		Model:           spend.Model,
		InputTokens:     spend.InputTokens,
		OutputTokens:    spend.OutputTokens,
		ReasoningTokens: spend.ReasoningTokens,
		Source:          spend.Source,
		SessionID:       spend.SessionID,
		TurnID:          spend.TurnID,
	})
	if err != nil {
		return nil, workforceError(err)
	}
	record := &plugin.SpendRecord{CostUSD: event.CostUSD}
	if agent != nil {
		record.BudgetUsed = agent.BudgetUsed
		record.BudgetLimit = agent.BudgetLimit
	}
	return record, nil
}

func (w *workforceService) DecideApproval(ctx context.Context, orgID uint, taskID string, approve bool) error {
	if w == nil || w.store == nil {
		return plugin.ErrWorkforceUnavailable
	}
	_, err := w.store.DecideApproval(ctx, orgID, taskID, approve)
	return workforceError(err)
}

// workforceError maps the ledger's errors onto the SPI's sentinels, so a
// plugin can branch on an outcome without importing the store.
func workforceError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, agentstore.ErrAgentNotFound):
		return plugin.ErrWorkforceAgentNotFound
	case errors.Is(err, agentstore.ErrAgentPaused):
		return plugin.ErrWorkforceAgentPaused
	case errors.Is(err, agentstore.ErrBudgetExceeded):
		return plugin.ErrWorkforceBudgetExhausted
	case errors.Is(err, agentstore.ErrModelPriceMissing):
		return plugin.ErrWorkforceModelUnpriced
	case errors.Is(err, agentstore.ErrInvalidSpend):
		return plugin.ErrWorkforceInvalidSpend
	case errors.Is(err, agentstore.ErrSpendReplayed):
		return plugin.ErrWorkforceSpendReplayed
	case errors.Is(err, agentstore.ErrTaskNotFound):
		return plugin.ErrWorkforceTaskNotFound
	case errors.Is(err, agentstore.ErrNotAwaitingApproval):
		return plugin.ErrWorkforceApprovalNotPending
	default:
		return err
	}
}
