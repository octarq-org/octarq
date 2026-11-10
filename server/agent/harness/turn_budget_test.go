package harness

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestResumeContinuesTheStepBudget pins that a stored turn keeps the rounds it
// already spent. An approval is an answer to a question, not a new allowance:
// a turn that comes back must not start its step budget over.
func TestResumeContinuesTheStepBudget(t *testing.T) {
	comp := &fakeCompleter{responses: []string{"nothing left to do"}}
	r := NewRunner(comp, &fakeExecutor{}, WithMaxSteps(2))
	s, turn := newBudgetTurn()
	turn.Rounds = 2 // the turn used two rounds before it was stored
	turn.PendingCall = NewPendingCall("delete_link", `{"id":"42"}`)

	err := r.Resume(context.Background(), s, turn)
	if !errors.Is(err, ErrMaxStepsExceeded) {
		t.Fatalf("Resume = %v, want ErrMaxStepsExceeded: the resumed turn started a fresh step budget", err)
	}
}

// TestResumeSpendsTheStoredWallClock pins that a stored turn keeps the time it
// already spent, so a resume cannot restart the wall-clock budget either.
func TestResumeSpendsTheStoredWallClock(t *testing.T) {
	comp := &fakeCompleter{responses: []string{"nothing left to do"}}
	r := NewRunner(comp, &fakeExecutor{}, WithBudget(Budget{MaxWallTime: time.Minute}))
	s, turn := newBudgetTurn()
	turn.Rounds = 1
	turn.Elapsed = time.Hour // the turn was stored after a long run
	turn.PendingCall = NewPendingCall("delete_link", `{"id":"42"}`)

	err := r.Resume(context.Background(), s, turn)
	if !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("Resume = %v, want ErrBudgetExceeded: the resumed turn restarted its wall clock", err)
	}
}

// TestResumeDoesNotCountTheOperatorsWait pins the other half of the rule: the
// time between Run returning and Resume being called belongs to the operator.
// A turn parked for approval overnight must still have its budget when it
// comes back.
func TestResumeDoesNotCountTheOperatorsWait(t *testing.T) {
	comp := &fakeCompleter{responses: []string{"nothing left to do"}}
	r := NewRunner(comp, &fakeExecutor{}, WithBudget(Budget{MaxWallTime: 100 * time.Millisecond}))
	s, turn := newBudgetTurn()
	turn.Rounds = 1
	turn.Elapsed = 10 * time.Millisecond // the turn ran briefly before it stopped
	turn.PendingCall = NewPendingCall("delete_link", `{"id":"42"}`)

	time.Sleep(150 * time.Millisecond) // the operator decides, longer than the budget

	if err := r.Resume(context.Background(), s, turn); err != nil {
		t.Fatalf("Resume = %v, want nil: the operator's wait was charged to the turn", err)
	}
	if turn.Elapsed < 10*time.Millisecond {
		t.Fatalf("Elapsed = %s, want at least the stored 10ms: the resume reset the turn's clock", turn.Elapsed)
	}
}
