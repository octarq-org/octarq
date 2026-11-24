package plugin

import (
	"context"
	"encoding/json"
	"errors"
)

const (
	ServiceSpendLedger = "spend.ledger"
	ServiceHarnessSpec = "harness.spec"
)

type HarnessSpecProvider interface {
	HarnessSpec(ctx context.Context, orgID uint) (json.RawMessage, error)
}

type SpendLedger interface {
	AuthorizeTurn(ctx context.Context, orgID uint, accountID, model string) error
	TurnCost(ctx context.Context, orgID uint, model string) (func(input, output, cacheRead, cacheWrite int) float64, error)
	RecordSpend(ctx context.Context, orgID uint, spend TurnSpend) (*SpendRecord, error)
	IsVirtualKey(rawToken string) bool
	ResolveVirtualKey(ctx context.Context, rawToken string) (*SpendAccountAuth, error)
}

type SpendAccountAuth struct {
	OrgID         uint
	AccountID     string
	Status        string
	BudgetLimit   float64
	BudgetUsed    float64
	AllowedModels []string
}

type TurnSpend struct {
	AccountID        string
	Model            string
	InputTokens      int
	OutputTokens     int
	ReasoningTokens  int
	CacheReadTokens  int
	CacheWriteTokens int
	Source           string
	SessionID        string
	TurnID           string
}

type SpendRecord struct {
	CostUSD     float64
	BudgetUsed  float64
	BudgetLimit float64
}

var (
	ErrSpendUnavailable     = errors.New("spend: the host has no spend ledger")
	ErrSpendAccountNotFound = errors.New("spend: account not found")
	ErrSpendAccountPaused   = errors.New("spend: account is paused")
	ErrSpendBudgetExhausted = errors.New("spend: account budget is exhausted")
	ErrSpendModelUnpriced   = errors.New("spend: the model has no price")
	ErrSpendInvalid         = errors.New("spend: invalid spend event")
	ErrSpendReplayed        = errors.New("spend: the turn was already billed")
)
