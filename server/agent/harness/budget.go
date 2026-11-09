package harness

import (
	"errors"
	"fmt"
	"math"
	"time"
)

// ErrBudgetExceeded is returned when a turn reaches a ceiling the harness spec
// set. The turn is over: the runner does not start another round.
var ErrBudgetExceeded = errors.New("harness: turn budget exceeded")

// ErrBudgetUnpriceable is returned when a budget sets a cost ceiling but the
// turn carries no price. A cost ceiling that can never be reached is worse than
// no ceiling: the operator reads the spec, believes a limit is in force, and
// pays for a loop that was never stopped.
var ErrBudgetUnpriceable = errors.New("harness: cost ceiling set but the turn cannot be priced")

// Budget bounds what one turn may spend. Every zero field means no ceiling, so
// a zero Budget is the unlimited default.
//
// The ceilings are checked before each round, not during one: a round that
// starts under the ceiling runs to completion, and the check refuses the next
// round. A turn can therefore end slightly above its ceiling — by at most the
// cost of the round that was already running. That is the honest reading of
// "at most N", and it is what a caller can reason about; aborting mid-LLM-call
// would leave the provider billing a request the harness abandoned.
type Budget struct {
	// MaxTokens bounds input and output tokens of the turn together.
	MaxTokens int
	// MaxCostUSD bounds the turn's cost in US dollars. Cost is required when
	// this is set.
	MaxCostUSD float64
	// MaxWallTime bounds the turn's wall-clock time.
	MaxWallTime time.Duration
	// Cost prices the tokens a turn has spent so far, in US dollars. A nil Cost
	// over a cost ceiling fails the turn with ErrBudgetUnpriceable.
	//
	// Reasoning tokens are passed separately because a provider may bill them
	// apart from the visible output; a call site that counts them inside output
	// ignores the argument.
	Cost func(input, output, reasoning int) float64
}

// turnTokens is the token count a budget ceiling is measured against.
func turnTokens(t *Turn) int {
	return t.InputTokens + t.OutputTokens
}

// checkBudget reports the first ceiling the turn has reached, or nil.
//
// It runs before a round starts, so it is also where a budget that could never
// be enforced is refused: an unpriceable cost ceiling fails on the first round
// instead of silently never tripping.
func (r *defaultRunner) checkBudget(t *Turn, started time.Time) error {
	b := r.budget
	if b.MaxCostUSD > 0 && b.Cost == nil {
		return fmt.Errorf("%w: set a model price, or remove maxCostUSD from the harness spec", ErrBudgetUnpriceable)
	}
	if b.MaxTokens > 0 {
		if used := turnTokens(t); used >= b.MaxTokens {
			return fmt.Errorf("%w: %d of %d tokens used", ErrBudgetExceeded, used, b.MaxTokens)
		}
	}
	if b.MaxWallTime > 0 {
		if used := time.Since(started); used >= b.MaxWallTime {
			return fmt.Errorf("%w: %s of %s used", ErrBudgetExceeded, used.Round(time.Second), b.MaxWallTime)
		}
	}
	if b.MaxCostUSD > 0 {
		if used := b.Cost(t.InputTokens, t.OutputTokens, t.ReasoningTokens); used >= b.MaxCostUSD {
			return fmt.Errorf("%w: %.4f of %.4f USD used", ErrBudgetExceeded, used, b.MaxCostUSD)
		}
	}
	return nil
}

// validBudgetNumber reports whether a spec value usable as a ceiling is
// non-negative and finite. NaN fails every comparison, so a NaN ceiling would
// accept any spend.
func validBudgetNumber(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0
}

// WithBudget sets the turn budget. The zero Budget — the default — leaves the
// turn unbounded, which is what a workspace that published no ceilings asked
// for.
func WithBudget(b Budget) RunnerOption {
	return func(r *defaultRunner) { r.budget = b }
}

// BudgetFromSpec converts the published spec's ceilings into a Budget. cost
// prices the turn's tokens; pass nil for a model the workspace never priced.
//
// The three fields are copied as they are. A spec that sets a cost ceiling over
// an unpriced model produces a Budget whose Cost is nil, and the runner refuses
// the turn rather than running it uncapped.
func BudgetFromSpec(spec HarnessSpec, cost func(input, output, reasoning int) float64) Budget {
	b := Budget{
		MaxTokens:   spec.MaxTokens,
		MaxCostUSD:  spec.MaxCostUSD,
		MaxWallTime: time.Duration(spec.MaxWallTimeSec) * time.Second,
	}
	if spec.MaxCostUSD > 0 {
		b.Cost = cost
	}
	return b
}
