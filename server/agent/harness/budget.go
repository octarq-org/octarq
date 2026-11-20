package harness

import (
	"errors"
	"fmt"
	"math"
	"time"
)

var ErrBudgetExceeded = errors.New("harness: turn budget exceeded")

var ErrBudgetUnpriceable = errors.New("harness: cost ceiling set but the turn cannot be priced")

type Budget struct {
	MaxTokens   int
	MaxCostUSD  float64
	MaxWallTime time.Duration
	Cost        func(input, output, reasoning int) float64
}

func turnTokens(t *Turn) int {
	return t.InputTokens + t.OutputTokens
}

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
		if used := t.Elapsed + time.Since(started); used >= b.MaxWallTime {
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

func validBudgetNumber(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0
}

func WithBudget(b Budget) RunnerOption {
	return func(r *defaultRunner) { r.budget = b }
}

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
