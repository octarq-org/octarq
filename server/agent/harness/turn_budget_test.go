package harness

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestResumeContinuesTheStepBudget(t *testing.T) {
	comp := &fakeCompleter{responses: []string{"nothing left to do"}}
	r := newTestRunner(t, comp, &fakeExecutor{}, WithMaxSteps(2))
	s, turn := newBudgetTurn()
	turn.Rounds = 2
	turn.PendingCall = NewPendingCall("delete_link", `{"id":"42"}`)

	err := r.Resume(context.Background(), s, turn)
	if !errors.Is(err, ErrMaxStepsExceeded) {
		t.Fatalf("Resume = %v, want ErrMaxStepsExceeded: the resumed turn started a fresh step budget", err)
	}
}

func TestResumeSpendsTheStoredWallClock(t *testing.T) {
	comp := &fakeCompleter{responses: []string{"nothing left to do"}}
	r := newTestRunner(t, comp, &fakeExecutor{}, WithBudget(Budget{MaxWallTime: time.Minute}))
	s, turn := newBudgetTurn()
	turn.Rounds = 1
	turn.Elapsed = time.Hour
	turn.PendingCall = NewPendingCall("delete_link", `{"id":"42"}`)

	err := r.Resume(context.Background(), s, turn)
	if !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("Resume = %v, want ErrBudgetExceeded: the resumed turn restarted its wall clock", err)
	}
}

func TestResumeDoesNotCountTheOperatorsWait(t *testing.T) {
	comp := &fakeCompleter{responses: []string{"nothing left to do"}}
	r := newTestRunner(t, comp, &fakeExecutor{}, WithBudget(Budget{MaxWallTime: 100 * time.Millisecond}))
	s, turn := newBudgetTurn()
	turn.Rounds = 1
	turn.Elapsed = 10 * time.Millisecond
	turn.PendingCall = NewPendingCall("delete_link", `{"id":"42"}`)

	time.Sleep(150 * time.Millisecond)

	if err := r.Resume(context.Background(), s, turn); err != nil {
		t.Fatalf("Resume = %v, want nil: the operator's wait was charged to the turn", err)
	}
	if turn.Elapsed < 10*time.Millisecond {
		t.Fatalf("Elapsed = %s, want at least the stored 10ms: the resume reset the turn's clock", turn.Elapsed)
	}
}
