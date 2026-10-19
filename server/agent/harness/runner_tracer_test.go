package harness

import (
	"context"
	"testing"
)

type recordingTracer struct {
	events []TraceEvent
}

func (r *recordingTracer) Event(_ context.Context, ev TraceEvent) {
	r.events = append(r.events, ev)
}

func TestNopTracer(t *testing.T) {
	t.Parallel()
	var tr NopTracer
	tr.Event(context.Background(), TraceEvent{Kind: TraceKindTurnStart})
}

func TestProfile_MaxSteps(t *testing.T) {
	t.Parallel()
	if got := ProfileNormal.MaxSteps(false); got != MaxStepsNormal {
		t.Errorf("ProfileNormal false = %d, want %d", got, MaxStepsNormal)
	}
	if got := ProfileNormal.MaxSteps(true); got != MaxStepsNormal {
		t.Errorf("ProfileNormal true = %d, want %d", got, MaxStepsNormal)
	}
	if got := ProfileReactor.MaxSteps(false); got != MaxStepsReactor {
		t.Errorf("ProfileReactor false = %d, want %d", got, MaxStepsReactor)
	}
	if got := ProfileReactor.MaxSteps(true); got != MaxStepsReactor {
		t.Errorf("ProfileReactor true = %d, want %d", got, MaxStepsReactor)
	}
	if got := ProfileReactorReadOnly.MaxSteps(false); got != MaxStepsReadOnly {
		t.Errorf("ProfileReactorReadOnly false = %d, want %d", got, MaxStepsReadOnly)
	}
	if got := ProfileReactorReadOnly.MaxSteps(true); got != MaxStepsReactor {
		t.Errorf("ProfileReactorReadOnly true = %d, want %d", got, MaxStepsReactor)
	}
	if got := Profile(999).MaxSteps(false); got != MaxStepsNormal {
		t.Errorf("Profile(999) = %d, want %d", got, MaxStepsNormal)
	}
}

func TestRunner_WithTracerAndOptions(t *testing.T) {
	t.Parallel()
	tr := &recordingTracer{}
	comp := &fakeCompleter{
		responses: []string{
			"Thinking before call\n" + toolCallBlock("lookup", `{"id":"1"}`) + "\nThinking after call",
			"done final answer",
		},
	}
	exec := &fakeExecutor{
		outputs: map[string]string{"lookup": "result-1"},
		errs:    map[string]error{},
	}

	runner := NewRunner(comp, exec,
		WithTracer(tr),
		WithModel("test-model-4"),
		WithSystem("You are a helpful assistant"),
	)
	sess := &Session{ID: "sess-tr", OrgID: 10}
	turn := &Turn{Input: "hi"}

	if err := runner.Run(context.Background(), sess, turn); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	var starts, steps, ends int
	for _, ev := range tr.events {
		switch ev.Kind {
		case TraceKindTurnStart:
			starts++
		case TraceKindStep:
			steps++
		case TraceKindTurnEnd:
			ends++
		}
	}
	if starts != 1 || steps != 1 || ends != 1 {
		t.Errorf("tracer mismatch: starts=%d, steps=%d, ends=%d (%+v)", starts, steps, ends, tr.events)
	}
	if turn.Status != TurnStatusDone {
		t.Errorf("turn status = %d, want %d", turn.Status, TurnStatusDone)
	}
}
