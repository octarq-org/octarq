package plugin

import (
	"context"
	"encoding/json"
	"errors"
)

// Well-known service names a workforce plugin registers during Mount.
//
// Workforce is a business feature, not core plumbing: the open-source core
// ships the contract and a host that refuses every call, and the plugin that
// owns agents, tasks, approvals, the ledger, and the published harness supply
// the implementation. Core never imports that plugin.
const (
	// ServiceWorkforceLedger is where the ledger implementing Workforce is
	// registered.
	ServiceWorkforceLedger = "workforce.ledger"
	// ServiceHarnessSpec is where the published-harness store is registered.
	ServiceHarnessSpec = "workforce.spec"
)

// HarnessSpecProvider supplies the workspace's published agent harness as its
// canonical JSON document.
//
// A nil document means the workspace published none: the caller uses the
// built-in default. An error means a published document exists but could not be
// read — the caller must fail closed rather than run a different harness.
type HarnessSpecProvider interface {
	HarnessSpec(ctx context.Context, orgID uint) (json.RawMessage, error)
}

// Workforce is the host's agent ledger: whether an agent may run, what a turn
// cost, and whether an operator approved a task.
//
// A plugin that runs agents reports to this ledger instead of keeping its own
// counters and its own approval table. Two ledgers disagree the first time one
// of them misses a turn, and the operator's monthly budget is the number that
// has to be right.
type Workforce interface {
	// AuthorizeTurn decides whether agentID may run one turn on model. It
	// refuses an unknown agent, a paused agent, an unpriced model, and an
	// exhausted budget. A model without a price is refused because a cost
	// ceiling over it could never trip.
	AuthorizeTurn(ctx context.Context, orgID uint, agentID, model string) error

	// TurnCost returns a function that prices the tokens a turn spends on
	// model, in US dollars, for the caller's own budget check. It is the same
	// price RecordSpend charges, so a turn that stops at its ceiling stops at
	// the number the ledger would have billed.
	TurnCost(ctx context.Context, orgID uint, model string) (func(input, output, reasoning int) float64, error)

	// RecordSpend prices one reported turn, appends it to the ledger and
	// accrues the agent's monthly budget. A turn is recorded even when it
	// failed halfway: the provider bills the tokens it generated.
	RecordSpend(ctx context.Context, orgID uint, spend TurnSpend) (*SpendRecord, error)

	// DecideApproval resolves a task's approval gate. Approving resumes the
	// work; rejecting returns the task to the queue.
	//
	// The change is a compare-and-swap on the task's current status, so a
	// replayed or concurrent decision fails with ErrWorkforceApprovalNotPending
	// instead of resolving the same task twice.
	DecideApproval(ctx context.Context, orgID uint, taskID string, approve bool) error
}

// TurnSpend is one reported agent turn. The caller reports usage; the host
// prices it.
type TurnSpend struct {
	AgentID         string
	Model           string
	InputTokens     int
	OutputTokens    int
	ReasoningTokens int
	// Source is where the usage came from: "chat" or "runtime". A runtime turn
	// must carry TurnID, which is what stops a retry from billing twice.
	Source    string
	SessionID string
	// TurnID is the producer's identifier for a runtime turn. A second report
	// with the same identifier is refused with ErrWorkforceSpendReplayed.
	TurnID string
}

// SpendRecord is what the ledger wrote for one turn.
type SpendRecord struct {
	// CostUSD is the frozen cost of the turn.
	CostUSD float64
	// BudgetUsed is the agent's monthly total after this turn.
	BudgetUsed float64
	// BudgetLimit is the agent's monthly ceiling.
	BudgetLimit float64
}

var (
	// ErrWorkforceUnavailable is returned when the host has no ledger behind it.
	ErrWorkforceUnavailable = errors.New("workforce: the host has no agent ledger")
	// ErrWorkforceAgentNotFound is returned for an agent this workspace does not have.
	ErrWorkforceAgentNotFound = errors.New("workforce: agent not found")
	// ErrWorkforceAgentPaused is returned when a turn targets a paused agent.
	ErrWorkforceAgentPaused = errors.New("workforce: agent is paused")
	// ErrWorkforceBudgetExhausted is returned when the agent has spent its
	// monthly limit and auto-halt is on.
	ErrWorkforceBudgetExhausted = errors.New("workforce: agent budget is exhausted")
	// ErrWorkforceModelUnpriced is returned for a model the workspace never priced.
	ErrWorkforceModelUnpriced = errors.New("workforce: the model has no price")
	// ErrWorkforceInvalidSpend is returned for a spend event the ledger refuses:
	// a missing or over-long model, negative token counts, or an unknown source.
	ErrWorkforceInvalidSpend = errors.New("workforce: invalid spend event")
	// ErrWorkforceSpendReplayed is returned when a turn is reported a second
	// time. The first write owns the charge.
	ErrWorkforceSpendReplayed = errors.New("workforce: the turn was already billed")
	// ErrWorkforceTaskNotFound is returned for a task this workspace does not have.
	ErrWorkforceTaskNotFound = errors.New("workforce: task not found")
	// ErrWorkforceApprovalNotPending is returned when a decision targets a task
	// that is not gated. A replayed or concurrent decision lands here.
	ErrWorkforceApprovalNotPending = errors.New("workforce: the task is not awaiting approval")
)
