package harness

import (
	"context"
	"sync"
)

// The trace is the evidence layer of the harness: without it a turn is an
// opaque stream of text and nothing can be learned from it — not by an
// operator reading the audit log, and not by a later harness experiment.
//
// A Tracer observes the lifecycle of a turn. Events are structured, not
// stringly-typed, so a consumer can aggregate them (token totals, per-tool
// failure rates, step counts) without parsing prose.
//
// P3 前 API 易变.
type Tracer interface {
	Event(ctx context.Context, ev TraceEvent)
}

// TraceKind classifies a trace event.
type TraceKind string

const (
	// TraceKindTurnStart is emitted once, before the first model call.
	TraceKindTurnStart TraceKind = "turn_start"
	// TraceKindStep is emitted once per tool invocation.
	TraceKindStep TraceKind = "step"
	// TraceKindTurnEnd is emitted once, when the turn reaches a terminal state.
	TraceKindTurnEnd TraceKind = "turn_end"
)

// Step outcomes.
const (
	TraceStatusOK      = "ok"
	TraceStatusError   = "error"
	TraceStatusDenied  = "denied"
	TraceStatusBlocked = "blocked" // the step was refused before execution
)

// TraceEvent is one observation of the agent loop.
//
// It deliberately carries no tool arguments and no tool output: those belong
// to the session, not to the trace. What the trace keeps is the shape of what
// happened — which tool, whether it worked, how long it took, what it cost.
type TraceEvent struct {
	Kind TraceKind
	// Step is the zero-based index of the agent step the event belongs to.
	Step int
	// Tool is the tool name for a step event.
	Tool string
	// Status is the outcome of a step, or the terminal turn status.
	Status string
	// ErrorCode is the stable business error code of a failed step.
	ErrorCode string
	// DurationMS is how long the tool call took.
	DurationMS int64
	// Token totals, carried by the turn_end event.
	InputTokens     int
	OutputTokens    int
	ReasoningTokens int
}

// NopTracer silently discards all events.
type NopTracer struct{}

// Event discards the event.
func (NopTracer) Event(context.Context, TraceEvent) {}

// RecordingTracer collects trace events in memory. The caller decides what
// to do with them — persist them, print them, compare them.
type RecordingTracer struct {
	mu     sync.Mutex
	events []TraceEvent
}

// NewRecordingTracer constructs an empty RecordingTracer.
func NewRecordingTracer() *RecordingTracer {
	return &RecordingTracer{}
}

// Event records the event.
func (t *RecordingTracer) Event(_ context.Context, ev TraceEvent) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.events = append(t.events, ev)
}

// Events returns a copy of the recorded events.
func (t *RecordingTracer) Events() []TraceEvent {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]TraceEvent, len(t.events))
	copy(out, t.events)
	return out
}
