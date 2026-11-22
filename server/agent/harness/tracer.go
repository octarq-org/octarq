package harness

import (
	"context"
	"sync"
)

type Tracer interface {
	Event(ctx context.Context, ev TraceEvent)
}

type TraceKind string

const (
	TraceKindTurnStart TraceKind = "turn_start"
	TraceKindStep      TraceKind = "step"
	TraceKindTurnEnd   TraceKind = "turn_end"
)

const (
	TraceStatusOK      = "ok"
	TraceStatusError   = "error"
	TraceStatusDenied  = "denied"
	TraceStatusBlocked = "blocked"
)

type TraceEvent struct {
	Kind             TraceKind
	Step             int
	Tool             string
	Status           string
	ErrorCode        string
	DurationMS       int64
	InputTokens      int
	OutputTokens     int
	ReasoningTokens  int
	CacheReadTokens  int
	CacheWriteTokens int
}

type NopTracer struct{}

func (NopTracer) Event(context.Context, TraceEvent) {}

type RecordingTracer struct {
	mu     sync.Mutex
	events []TraceEvent
}

func NewRecordingTracer() *RecordingTracer {
	return &RecordingTracer{}
}

func (t *RecordingTracer) Event(_ context.Context, ev TraceEvent) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.events = append(t.events, ev)
}

func (t *RecordingTracer) Events() []TraceEvent {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]TraceEvent, len(t.events))
	copy(out, t.events)
	return out
}
