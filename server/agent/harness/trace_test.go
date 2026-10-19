package harness

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/plugin"
)

// The trace is the evidence layer: without it a turn is an opaque stream of
// text. These tests pin the events the runner emits and the recorder that
// collects them.

func TestRecordingTracer_CollectsEvents(t *testing.T) {
	tr := NewRecordingTracer()

	tr.Event(context.Background(), TraceEvent{Kind: TraceKindTurnStart})
	tr.Event(context.Background(), TraceEvent{Kind: TraceKindStep, Tool: "read_links", Status: TraceStatusOK})

	events := tr.Events()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Kind != TraceKindTurnStart {
		t.Errorf("first event kind = %q, want %q", events[0].Kind, TraceKindTurnStart)
	}
	if events[1].Tool != "read_links" {
		t.Errorf("second event tool = %q, want read_links", events[1].Tool)
	}
}

func TestRecordingTracer_EventsAreACopy(t *testing.T) {
	tr := NewRecordingTracer()
	tr.Event(context.Background(), TraceEvent{Kind: TraceKindTurnStart, Tool: "a"})

	events := tr.Events()
	events[0].Tool = "mutated"

	if tr.Events()[0].Tool != "a" {
		t.Error("Events() handed out the internal slice; callers can corrupt the trace")
	}
}

func TestRunner_EmitsTurnAndStepEvents(t *testing.T) {
	comp := &fakeCompleter{
		responses: []string{
			toolCallBlock("read_links", "{}"),
			"final answer",
		},
	}
	exec := &fakeExecutor{outputs: map[string]string{"read_links": "ok"}}
	tr := NewRecordingTracer()

	runner := NewRunner(comp, exec, WithTracer(tr))
	sess := &Session{ID: "s1", OrgID: 1, Channel: "test"}
	turn := &Turn{Input: "go"}

	if err := runner.Run(context.Background(), sess, turn); err != nil {
		t.Fatalf("Run: %v", err)
	}

	events := tr.Events()
	if len(events) < 3 {
		t.Fatalf("expected turn start, a step and turn end, got %d events: %+v", len(events), events)
	}
	if events[0].Kind != TraceKindTurnStart {
		t.Errorf("first event = %q, want %q", events[0].Kind, TraceKindTurnStart)
	}

	var step *TraceEvent
	for i := range events {
		if events[i].Kind == TraceKindStep {
			step = &events[i]
			break
		}
	}
	if step == nil {
		t.Fatalf("no step event emitted: %+v", events)
	}
	if step.Tool != "read_links" {
		t.Errorf("step tool = %q, want read_links", step.Tool)
	}
	if step.Status != TraceStatusOK {
		t.Errorf("step status = %q, want %q", step.Status, TraceStatusOK)
	}

	last := events[len(events)-1]
	if last.Kind != TraceKindTurnEnd {
		t.Errorf("last event = %q, want %q", last.Kind, TraceKindTurnEnd)
	}
	if last.Status != TurnStatusDone.String() {
		t.Errorf("turn status = %q, want %q", last.Status, TurnStatusDone.String())
	}
}

func TestRunner_StepEventCarriesTheErrorCode(t *testing.T) {
	comp := &fakeCompleter{
		responses: []string{
			toolCallBlock("write_thing", "{}"),
			"final answer",
		},
	}
	exec := &fakeExecutor{
		outputs: map[string]string{},
		errs: map[string]error{
			"write_thing": plugin.NewAgentError(409, "SLUG_TAKEN", "slug is taken", "pick another", false),
		},
	}
	tr := NewRecordingTracer()

	runner := NewRunner(comp, exec, WithTracer(tr))
	sess := &Session{ID: "s1", OrgID: 1, Channel: "test"}
	turn := &Turn{Input: "go"}

	if err := runner.Run(context.Background(), sess, turn); err != nil {
		t.Fatalf("Run: %v", err)
	}

	for _, ev := range tr.Events() {
		if ev.Kind != TraceKindStep {
			continue
		}
		if ev.Status != TraceStatusError {
			t.Errorf("step status = %q, want %q", ev.Status, TraceStatusError)
		}
		if ev.ErrorCode != "SLUG_TAKEN" {
			t.Errorf("step error code = %q, want SLUG_TAKEN", ev.ErrorCode)
		}
		return
	}
	t.Fatalf("no step event emitted")
}

func TestRunner_GuardRejectionIsTracedAsDenied(t *testing.T) {
	comp := &fakeCompleter{
		responses: []string{
			toolCallBlock("write_thing", "{}"),
			"final answer",
		},
	}
	exec := &fakeExecutor{outputs: map[string]string{}}
	tr := NewRecordingTracer()

	runner := NewRunner(comp, exec, WithTracer(tr), WithGuard(&denyGuard{denied: "write_thing"}))
	sess := &Session{ID: "s1", OrgID: 1, Channel: "test"}
	turn := &Turn{Input: "go"}

	if err := runner.Run(context.Background(), sess, turn); err != nil {
		t.Fatalf("Run: %v", err)
	}

	for _, ev := range tr.Events() {
		if ev.Kind == TraceKindStep {
			if ev.Status != TraceStatusDenied {
				t.Errorf("step status = %q, want %q", ev.Status, TraceStatusDenied)
			}
			return
		}
	}
	t.Fatalf("no step event emitted")
}

func TestRunner_TurnEndEventCarriesTokenTotals(t *testing.T) {
	comp := &fakeCompleter{responses: []string{"final answer"}}
	exec := &fakeExecutor{outputs: map[string]string{}}
	tr := NewRecordingTracer()

	runner := NewRunner(comp, exec, WithTracer(tr))
	sess := &Session{ID: "s1", OrgID: 1, Channel: "test"}
	turn := &Turn{Input: "go"}
	turn.InputTokens = 11
	turn.OutputTokens = 7
	turn.ReasoningTokens = 3

	if err := runner.Run(context.Background(), sess, turn); err != nil {
		t.Fatalf("Run: %v", err)
	}

	events := tr.Events()
	last := events[len(events)-1]
	if last.InputTokens != 11 || last.OutputTokens != 7 || last.ReasoningTokens != 3 {
		t.Errorf("turn end token totals = %d/%d/%d, want 11/7/3",
			last.InputTokens, last.OutputTokens, last.ReasoningTokens)
	}
}

func TestRunner_ApprovalRequiredIsTraced(t *testing.T) {
	comp := &fakeCompleter{
		responses: []string{toolCallBlock("write_thing", "{}")},
	}
	exec := &fakeExecutor{outputs: map[string]string{}}
	tr := NewRecordingTracer()

	runner := NewRunner(comp, exec, WithTracer(tr), WithGuard(&approvalGuard{tool: "write_thing"}))
	sess := &Session{ID: "s1", OrgID: 1, Channel: "test"}
	turn := &Turn{Input: "go"}

	err := runner.Run(context.Background(), sess, turn)
	if !errors.Is(err, ErrApprovalRequired) {
		t.Fatalf("expected ErrApprovalRequired, got %v", err)
	}

	events := tr.Events()
	last := events[len(events)-1]
	if last.Kind != TraceKindTurnEnd {
		t.Fatalf("last event = %q, want %q", last.Kind, TraceKindTurnEnd)
	}
	if last.Status != TurnStatusAwaitingApproval.String() {
		t.Errorf("turn status = %q, want %q", last.Status, TurnStatusAwaitingApproval.String())
	}
}

func TestRunner_StepEventCarriesDuration(t *testing.T) {
	comp := &fakeCompleter{
		responses: []string{
			toolCallBlock("read_links", "{}"),
			"final answer",
		},
	}
	exec := &fakeExecutor{outputs: map[string]string{"read_links": "ok"}}
	tr := NewRecordingTracer()

	runner := NewRunner(comp, exec, WithTracer(tr))
	sess := &Session{ID: "s1", OrgID: 1, Channel: "test"}
	turn := &Turn{Input: "go"}

	start := time.Now()
	if err := runner.Run(context.Background(), sess, turn); err != nil {
		t.Fatalf("Run: %v", err)
	}
	_ = start

	for _, ev := range tr.Events() {
		if ev.Kind == TraceKindStep && ev.Tool == "read_links" {
			if ev.DurationMS < 0 {
				t.Errorf("duration = %d, want a non-negative value", ev.DurationMS)
			}
			return
		}
	}
	t.Fatalf("no step event emitted")
}
