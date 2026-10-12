package harness

import (
	"context"
	"testing"
)

type recordingTracer struct {
	starts []string
	ends   []string
	turns  []string
}

func (r *recordingTracer) StepStart(_ context.Context, stepID string) {
	r.starts = append(r.starts, stepID)
}

func (r *recordingTracer) StepEnd(_ context.Context, stepID string, _ error) {
	r.ends = append(r.ends, stepID)
}

func (r *recordingTracer) TurnEnd(_ context.Context, turnID string, _ TurnStatus) {
	r.turns = append(r.turns, turnID)
}

func TestNopTracer(t *testing.T) {
	t.Parallel()
	var tr NopTracer
	ctx := context.Background()
	tr.StepStart(ctx, "s1")
	tr.StepEnd(ctx, "s1", nil)
	tr.TurnEnd(ctx, "t1", TurnStatusDone)
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
	if len(tr.starts) != 1 || len(tr.ends) != 1 || len(tr.turns) != 1 {
		t.Errorf("tracer mismatch: starts=%v, ends=%v, turns=%v", tr.starts, tr.ends, tr.turns)
	}
	if turn.Status != TurnStatusDone {
		t.Errorf("turn status = %d, want %d", turn.Status, TurnStatusDone)
	}
}
