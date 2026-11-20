package plugin

import (
	"context"
	"encoding/json"
	"errors"
)

const (
	ServiceWorkforceLedger = "workforce.ledger"
	ServiceHarnessSpec     = "workforce.spec"
)

type HarnessSpecProvider interface {
	HarnessSpec(ctx context.Context, orgID uint) (json.RawMessage, error)
}

type Workforce interface {
	AuthorizeTurn(ctx context.Context, orgID uint, agentID, model string) error

	TurnCost(ctx context.Context, orgID uint, model string) (func(input, output, reasoning int) float64, error)

	RecordSpend(ctx context.Context, orgID uint, spend TurnSpend) (*SpendRecord, error)

	DecideApproval(ctx context.Context, orgID uint, taskID string, approve bool) error
}

type TurnSpend struct {
	AgentID         string
	Model           string
	InputTokens     int
	OutputTokens    int
	ReasoningTokens int
	Source          string
	SessionID       string
	TurnID          string
}

type SpendRecord struct {
	CostUSD     float64
	BudgetUsed  float64
	BudgetLimit float64
}

var (
	ErrWorkforceUnavailable        = errors.New("workforce: the host has no agent ledger")
	ErrWorkforceAgentNotFound      = errors.New("workforce: agent not found")
	ErrWorkforceAgentPaused        = errors.New("workforce: agent is paused")
	ErrWorkforceBudgetExhausted    = errors.New("workforce: agent budget is exhausted")
	ErrWorkforceModelUnpriced      = errors.New("workforce: the model has no price")
	ErrWorkforceInvalidSpend       = errors.New("workforce: invalid spend event")
	ErrWorkforceSpendReplayed      = errors.New("workforce: the turn was already billed")
	ErrWorkforceTaskNotFound       = errors.New("workforce: task not found")
	ErrWorkforceApprovalNotPending = errors.New("workforce: the task is not awaiting approval")
)
