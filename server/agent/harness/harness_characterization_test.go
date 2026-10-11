package harness_test

import (
	"context"
	"testing"

	"github.com/octarq-org/octarq/server/agent/harness"
)

// Compile-time interface checks:
var (
	_ harness.Tracer = (*harness.NopTracer)(nil)
	_ harness.Guard  = (*harness.NopGuard)(nil)
)

func TestHarnessCharacterization_ProfilesAndLimits(t *testing.T) {
	cases := []struct {
		profile       harness.Profile
		destructive   bool
		expectedSteps int
	}{
		{harness.ProfileNormal, false, harness.MaxStepsNormal},
		{harness.ProfileNormal, true, harness.MaxStepsNormal},
		{harness.ProfileReactor, false, harness.MaxStepsReactor},
		{harness.ProfileReactor, true, harness.MaxStepsReactor},
		{harness.ProfileReactorReadOnly, false, harness.MaxStepsReadOnly},
		{harness.ProfileReactorReadOnly, true, harness.MaxStepsReactor},
		{harness.Profile(999), false, harness.MaxStepsNormal},
	}

	for _, tc := range cases {
		steps := tc.profile.MaxSteps(tc.destructive)
		if steps != tc.expectedSteps {
			t.Errorf("profile %d (destructive=%v) expected %d steps, got %d",
				tc.profile, tc.destructive, tc.expectedSteps, steps)
		}
	}

	// Verify constant values
	if harness.MaxStepsNormal != 8 {
		t.Errorf("expected MaxStepsNormal=8, got %d", harness.MaxStepsNormal)
	}
	if harness.MaxStepsReactor != 3 {
		t.Errorf("expected MaxStepsReactor=3, got %d", harness.MaxStepsReactor)
	}
	if harness.MaxStepsReadOnly != 5 {
		t.Errorf("expected MaxStepsReadOnly=5, got %d", harness.MaxStepsReadOnly)
	}
}

func TestHarnessCharacterization_TurnStatusEnum(t *testing.T) {
	if harness.TurnStatusRunning != 0 {
		t.Errorf("TurnStatusRunning should be 0, got %d", harness.TurnStatusRunning)
	}
	if harness.TurnStatusAwaitingApproval != 1 {
		t.Errorf("TurnStatusAwaitingApproval should be 1, got %d", harness.TurnStatusAwaitingApproval)
	}
	if harness.TurnStatusDone != 2 {
		t.Errorf("TurnStatusDone should be 2, got %d", harness.TurnStatusDone)
	}
	if harness.TurnStatusFailed != 3 {
		t.Errorf("TurnStatusFailed should be 3, got %d", harness.TurnStatusFailed)
	}
}

func TestHarnessCharacterization_NopTracerAndGuard(t *testing.T) {
	var tr harness.Tracer = &harness.NopTracer{}
	ctx := context.Background()

	// All calls must be safe and no-op
	tr.StepStart(ctx, "step-1")
	tr.StepEnd(ctx, "step-1", nil)
	tr.TurnEnd(ctx, "turn-1", harness.TurnStatusDone)

	var gd harness.Guard = harness.NopGuard{}
	if err := gd.Allow(ctx, 1, "any_tool"); err != nil {
		t.Errorf("NopGuard should always allow, got %v", err)
	}
}

func TestHarnessCharacterization_NewRunnerInstantiation(t *testing.T) {
	r := harness.NewRunner(nil, nil,
		harness.WithGuard(harness.NopGuard{}),
		harness.WithTracer(&harness.NopTracer{}),
		harness.WithProfile(harness.ProfileNormal),
		harness.WithModel("gemini-2.5-pro"),
		harness.WithSystem("You are an assistant"),
	)
	if r == nil {
		t.Fatal("NewRunner returned nil")
	}
	// harness.Runner conformance is statically guaranteed by NewRunner's
	// signature; only the dynamic StreamRunner capability needs asserting.
	if _, ok := r.(harness.StreamRunner); !ok {
		t.Error("r must implement harness.StreamRunner")
	}
}
