package harness

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestStepBudget pins the shared rule: a zero budget means the profile
// decides, a budget within the ceiling is returned unchanged, and a budget
// above the ceiling is refused. The helper never lowers a request.
func TestStepBudget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		profile        Profile
		maxSteps       int
		hasDestructive bool
		want           int
		wantErr        bool
		wantSentinel   bool
	}{
		{name: "zero means the profile decides", profile: ProfileNormal, want: MaxStepsNormal},
		{name: "a budget within the ceiling is kept", profile: ProfileNormal, maxSteps: 4, want: 4},
		{name: "the exact ceiling is kept", profile: ProfileNormal, maxSteps: MaxStepsNormal, want: MaxStepsNormal},
		{name: "above the ceiling is refused", profile: ProfileNormal, maxSteps: MaxStepsNormal + 1, wantErr: true, wantSentinel: true},
		{name: "a worker budget is kept", profile: ProfileWorker, maxSteps: 20, want: 20},
		{name: "above the worker ceiling is refused", profile: ProfileWorker, maxSteps: MaxStepsWorker + 1, wantErr: true, wantSentinel: true},
		{name: "a negative budget is refused", profile: ProfileNormal, maxSteps: -1, wantErr: true},
		{name: "an unknown profile is refused", profile: Profile(99), maxSteps: 1, wantErr: true},
		{
			name:           "the destructive reactor lock applies",
			profile:        ProfileReactorReadOnly,
			maxSteps:       MaxStepsReadOnly + 1,
			hasDestructive: true,
			wantErr:        true,
			wantSentinel:   true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := StepBudget(tc.profile, tc.maxSteps, tc.hasDestructive)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("StepBudget(%v, %d, %v) = %d, want an error", tc.profile, tc.maxSteps, tc.hasDestructive, got)
				}
				if tc.wantSentinel && !errors.Is(err, ErrMaxStepsExceedsProfile) {
					t.Fatalf("error = %v, want ErrMaxStepsExceedsProfile", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("StepBudget(%v, %d, %v): %v", tc.profile, tc.maxSteps, tc.hasDestructive, err)
			}
			if got != tc.want {
				t.Fatalf("StepBudget(%v, %d, %v) = %d, want %d", tc.profile, tc.maxSteps, tc.hasDestructive, got, tc.want)
			}
		})
	}
}

func TestNewRunner_RefusesAStepBudgetAboveTheProfile(t *testing.T) {
	t.Parallel()

	_, err := NewRunner(&fakeCompleter{}, &fakeExecutor{}, WithProfile(ProfileNormal), WithMaxSteps(MaxStepsNormal+1))
	if !errors.Is(err, ErrMaxStepsExceedsProfile) {
		t.Fatalf("NewRunner error = %v, want ErrMaxStepsExceedsProfile", err)
	}
	for _, want := range []string{"8", "9"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q must name both numbers, missing %q", err.Error(), want)
		}
	}

	if _, err := NewRunner(&fakeCompleter{}, &fakeExecutor{}, WithMaxSteps(-1)); err == nil {
		t.Fatal("a negative step budget must be refused")
	}
}

func TestNewRunner_AcceptsAWorkerBudget(t *testing.T) {
	t.Parallel()

	runner, err := NewRunner(&fakeCompleter{}, &fakeExecutor{}, WithProfile(ProfileWorker), WithMaxSteps(20))
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	if got := runner.MaxStepsFor(false); got != 20 {
		t.Fatalf("MaxStepsFor(false) = %d, want 20", got)
	}
	if got := runner.MaxStepsFor(true); got != 20 {
		t.Fatalf("MaxStepsFor(true) = %d, want 20", got)
	}
}

func TestNewRunner_ProfileDecidesWhenNoBudgetIsSet(t *testing.T) {
	t.Parallel()

	for _, p := range []Profile{ProfileNormal, ProfileReactor, ProfileReactorReadOnly, ProfileWorker} {
		runner, err := NewRunner(&fakeCompleter{}, &fakeExecutor{}, WithProfile(p))
		if err != nil {
			t.Fatalf("NewRunner(%v): %v", p, err)
		}
		if got, want := runner.MaxStepsFor(false), p.MaxSteps(false); got != want {
			t.Errorf("MaxStepsFor(false) for %v = %d, want %d", p, got, want)
		}
	}
}

// TestRunner_PromptBudgetMatchesTheEnforcedLoop is the K1 regression test: a
// worker spec that asks for 20 steps must state 20 in the prompt and must
// actually run 20 rounds. Before this change the prompt said 20 and the loop
// stopped at 8.
func TestRunner_PromptBudgetMatchesTheEnforcedLoop(t *testing.T) {
	t.Parallel()

	spec := DefaultHarnessSpec()
	spec.Name = "twenty-step-worker"
	spec.Profile = ProfileWorker
	spec.MaxSteps = 20

	maxSteps, err := spec.EffectiveMaxSteps(false)
	if err != nil {
		t.Fatalf("EffectiveMaxSteps: %v", err)
	}
	spec = spec.WithMaxSteps(maxSteps)

	// The prompt is built from the same value the runner is assembled with.
	prompt := Catalog{}.SystemPrompt(spec)
	if !strings.Contains(prompt, "at most 20 tool-calling steps") {
		t.Fatalf("the prompt does not state the enforced budget:\n%s", prompt)
	}

	responses := make([]string, 25)
	for i := range responses {
		responses[i] = toolCallBlock("noop", "{}")
	}
	comp := &fakeCompleter{responses: responses}
	exec := &fakeExecutor{outputs: map[string]string{"noop": "ok"}}

	runner, err := NewRunner(comp, exec,
		WithProfile(spec.Profile),
		WithSystem(prompt),
		WithMaxSteps(maxSteps),
	)
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	if got := runner.MaxStepsFor(false); got != 20 {
		t.Fatalf("MaxStepsFor(false) = %d, want 20", got)
	}

	session := &Session{ID: "k1", OrgID: 1, Channel: "test"}
	turn := &Turn{Input: "keep calling"}
	err = runner.Run(context.Background(), session, turn)
	if !errors.Is(err, ErrMaxStepsExceeded) {
		t.Fatalf("Run error = %v, want ErrMaxStepsExceeded", err)
	}
	if turn.Rounds != 20 {
		t.Fatalf("turn.Rounds = %d, want 20", turn.Rounds)
	}
	if len(turn.Steps) != 20 {
		t.Fatalf("turn.Steps = %d, want 20", len(turn.Steps))
	}
	if len(exec.calls) != 20 {
		t.Fatalf("executed %d tool calls, want 20", len(exec.calls))
	}
}

// TestMaxStepsFor_ReactorReadOnlyDestructiveLock pins the one profile-owned
// reduction left: a destructive turn under ProfileReactorReadOnly runs at the
// reactor hard cap even when the caller set the read-only ceiling.
func TestMaxStepsFor_ReactorReadOnlyDestructiveLock(t *testing.T) {
	t.Parallel()

	runner, err := NewRunner(&fakeCompleter{}, &fakeExecutor{},
		WithProfile(ProfileReactorReadOnly),
		WithMaxSteps(MaxStepsReadOnly),
	)
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	if got := runner.MaxStepsFor(false); got != MaxStepsReadOnly {
		t.Fatalf("MaxStepsFor(false) = %d, want %d", got, MaxStepsReadOnly)
	}
	if got := runner.MaxStepsFor(true); got != MaxStepsReactor {
		t.Fatalf("MaxStepsFor(true) = %d, want %d", got, MaxStepsReactor)
	}
}
