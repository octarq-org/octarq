package harness

import "context"

// Tracer observes step and turn lifecycle events for trajectory recording,
// cost accounting, and offline evaluation.
//
// P3 前 API 易变.
type Tracer interface {
	StepStart(ctx context.Context, stepID string)
	StepEnd(ctx context.Context, stepID string, err error)
	TurnEnd(ctx context.Context, turnID string, status TurnStatus)
}

// NopTracer silently discards all events.
type NopTracer struct{}

func (NopTracer) StepStart(context.Context, string)           {}
func (NopTracer) StepEnd(context.Context, string, error)      {}
func (NopTracer) TurnEnd(context.Context, string, TurnStatus) {}
