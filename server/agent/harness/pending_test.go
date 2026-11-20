package harness

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/llmprovider"
)

type argExecutor struct {
	calls []struct{ Tool, Args string }
}

func (e *argExecutor) Execute(_ context.Context, _ uint, name, argsJSON string) (string, error) {
	e.calls = append(e.calls, struct{ Tool, Args string }{name, argsJSON})
	return "ok", nil
}

type argGuard struct {
	denySubstring string
	seen          []string
}

func (g *argGuard) Allow(_ context.Context, _ uint, tool, argsJSON string) error {
	g.seen = append(g.seen, tool+" "+argsJSON)
	if strings.Contains(argsJSON, g.denySubstring) {
		return errors.New("the call touches a path the policy forbids")
	}
	return nil
}

func TestGuardReceivesTheCallArguments(t *testing.T) {
	g := &argGuard{denySubstring: "/etc/passwd"}
	comp := &fakeCompleter{responses: []string{
		toolCallBlock("read_file", `{"path":"/etc/passwd"}`),
		"nothing left to do",
	}}
	r := newTestRunner(t, comp, &argExecutor{}, WithGuard(g))
	s, turn := newBudgetTurn()

	if err := r.Run(context.Background(), s, turn); err != nil {
		t.Fatalf("Run = %v, want nil (a refused call is reported, not fatal)", err)
	}
	if len(g.seen) != 1 {
		t.Fatalf("guard consulted %d times, want 1", len(g.seen))
	}
	if g.seen[0] != `read_file {"path":"/etc/passwd"}` {
		t.Fatalf("guard saw %q, want the tool and its arguments", g.seen[0])
	}
	if turn.Status != TurnStatusDone {
		t.Errorf("turn status = %s, want done", turn.Status)
	}
}

func TestPendingCallRecordsTheExactCall(t *testing.T) {
	exec := &argExecutor{}
	comp := &fakeCompleter{responses: []string{toolCallBlock("delete_link", `{"id":"42"}`)}}
	r := newTestRunner(t, comp, exec, WithGuard(&approvalGuard{tool: "delete_link"}))
	s, turn := newBudgetTurn()

	err := r.Run(context.Background(), s, turn)
	if !errors.Is(err, ErrApprovalRequired) {
		t.Fatalf("Run = %v, want ErrApprovalRequired", err)
	}
	if turn.Status != TurnStatusAwaitingApproval {
		t.Fatalf("turn status = %s, want awaiting_approval", turn.Status)
	}
	if len(exec.calls) != 0 {
		t.Fatalf("the blocked call ran %d time(s), want 0", len(exec.calls))
	}
	p := turn.PendingCall
	if p == nil {
		t.Fatal("no pending call recorded; an approval would have nothing to bind to")
	}
	if p.Tool != "delete_link" || p.ArgsJSON != `{"id":"42"}` {
		t.Fatalf("pending call = %+v, want the refused call", p)
	}
	if p.Hash != callHash(p.Tool, p.ArgsJSON) {
		t.Fatalf("pending hash = %q, want the hash of the recorded call", p.Hash)
	}
}

func TestPendingCallHashIgnoresFormatting(t *testing.T) {
	a := newPendingCall(toolCall{Name: "t", ArgsJSON: json.RawMessage(`{"b":1,"a":2}`)})
	b := newPendingCall(toolCall{Name: "t", ArgsJSON: json.RawMessage("{\n  \"a\": 2,\n  \"b\": 1\n}")})
	if a.ArgsJSON != b.ArgsJSON || a.Hash != b.Hash {
		t.Fatalf("the same call hashed twice: %+v vs %+v", a, b)
	}

	c := newPendingCall(toolCall{Name: "t", ArgsJSON: json.RawMessage(`{"a":3,"b":1}`)})
	if c.Hash == a.Hash {
		t.Fatal("different arguments produced the same hash")
	}

	d := newPendingCall(toolCall{Name: "ab", ArgsJSON: json.RawMessage(`{}`)})
	e := newPendingCall(toolCall{Name: "a", ArgsJSON: json.RawMessage(`{"b":{}}`)})
	if d.Hash == e.Hash {
		t.Fatal("two different calls share a hash")
	}
}

func TestNewPendingCallMatchesTheRecordedOne(t *testing.T) {
	comp := &fakeCompleter{responses: []string{toolCallBlock("delete_link", `{"id":"42"}`)}}
	r := newTestRunner(t, comp, &argExecutor{}, WithGuard(&approvalGuard{tool: "delete_link"}))
	s, turn := newBudgetTurn()

	if err := r.Run(context.Background(), s, turn); !errors.Is(err, ErrApprovalRequired) {
		t.Fatalf("Run = %v, want ErrApprovalRequired", err)
	}
	recorded := turn.PendingCall
	if recorded == nil {
		t.Fatal("no pending call recorded")
	}

	rebuilt := NewPendingCall(recorded.Tool, recorded.ArgsJSON)
	if *rebuilt != *recorded {
		t.Fatalf("rebuilt %+v, want the recorded %+v", *rebuilt, *recorded)
	}
	if messy := NewPendingCall(recorded.Tool, "{\n  \"id\": \"42\"\n}"); *messy != *recorded {
		t.Fatalf("re-rendered arguments changed the record: %+v", *messy)
	}
}

func TestResumeRunsTheApprovedCallNotTheNextOne(t *testing.T) {
	exec := &argExecutor{}
	comp := &fakeCompleter{responses: []string{
		toolCallBlock("delete_link", `{"id":"42"}`),
		toolCallBlock("delete_link", `{"id":"99"}`),
	}}
	r := newTestRunner(t, comp, exec, WithGuard(&approvalGuard{tool: "delete_link"}))
	s, turn := newBudgetTurn()

	if err := r.Run(context.Background(), s, turn); !errors.Is(err, ErrApprovalRequired) {
		t.Fatalf("Run = %v, want ErrApprovalRequired", err)
	}

	if err := r.Resume(context.Background(), s, turn); !errors.Is(err, ErrApprovalRequired) {
		t.Fatalf("Resume = %v, want the second call to ask for approval again", err)
	}
	if len(exec.calls) != 1 {
		t.Fatalf("executor ran %d call(s), want 1", len(exec.calls))
	}
	if exec.calls[0].Args != `{"id":"42"}` {
		t.Fatalf("executed %q, want the approved call {\"id\":\"42\"}", exec.calls[0].Args)
	}
	if turn.PendingCall == nil || turn.PendingCall.ArgsJSON != `{"id":"99"}` {
		t.Fatalf("pending call = %+v, want the second call recorded separately", turn.PendingCall)
	}
}

func TestResumeDoesNotAskTheGuardAgain(t *testing.T) {
	g := &countingApprovalGuard{tool: "delete_link"}
	exec := &argExecutor{}
	comp := &fakeCompleter{responses: []string{
		toolCallBlock("delete_link", `{"id":"42"}`),
		"nothing left to do",
	}}
	r := newTestRunner(t, comp, exec, WithGuard(g))
	s, turn := newBudgetTurn()

	if err := r.Run(context.Background(), s, turn); !errors.Is(err, ErrApprovalRequired) {
		t.Fatalf("Run = %v, want ErrApprovalRequired", err)
	}
	if g.calls != 1 {
		t.Fatalf("guard consulted %d times before the decision, want 1", g.calls)
	}

	err := r.Resume(context.Background(), s, turn)
	if err != nil {
		t.Fatalf("Resume = %v, want nil", err)
	}
	if g.calls != 1 {
		t.Fatalf("guard consulted %d times in total, want 1: the approval is the answer", g.calls)
	}
	if len(exec.calls) != 1 {
		t.Fatalf("executor ran %d call(s), want 1", len(exec.calls))
	}
	if turn.Status != TurnStatusDone {
		t.Errorf("turn status = %s, want done", turn.Status)
	}
}

type countingApprovalGuard struct {
	tool  string
	calls int
}

func (g *countingApprovalGuard) Allow(_ context.Context, _ uint, tool, _ string) error {
	g.calls++
	if tool == g.tool {
		return ErrApprovalRequired
	}
	return nil
}

func TestResumeDoesNotBuyAnotherRound(t *testing.T) {
	exec := &argExecutor{}
	comp := &fakeCompleter{responses: []string{toolCallBlock("delete_link", `{"id":"42"}`)}}
	r := newTestRunner(t, comp, exec, WithGuard(&approvalGuard{tool: "delete_link"}), WithMaxSteps(1))
	s, turn := newBudgetTurn()

	if err := r.Run(context.Background(), s, turn); !errors.Is(err, ErrApprovalRequired) {
		t.Fatalf("Run = %v, want ErrApprovalRequired", err)
	}

	err := r.Resume(context.Background(), s, turn)
	if !errors.Is(err, ErrMaxStepsExceeded) {
		t.Fatalf("Resume = %v, want ErrMaxStepsExceeded (the approval must not add a round)", err)
	}
	if len(exec.calls) != 1 {
		t.Fatalf("executor ran %d call(s), want the approved call to run before the stop", len(exec.calls))
	}
	if turn.Status != TurnStatusFailed {
		t.Errorf("turn status = %s, want failed", turn.Status)
	}
}

func TestResumeWithoutAPendingCallIsRefused(t *testing.T) {
	exec := &argExecutor{}
	r := newTestRunner(t, &fakeCompleter{}, exec)
	s, turn := newBudgetTurn()

	err := r.Resume(context.Background(), s, turn)
	if !errors.Is(err, ErrNoPendingCall) {
		t.Fatalf("Resume = %v, want ErrNoPendingCall", err)
	}
	if turn.Status != TurnStatusFailed {
		t.Errorf("turn status = %s, want failed", turn.Status)
	}
	if len(exec.calls) != 0 {
		t.Fatalf("executor ran %d call(s), want 0", len(exec.calls))
	}
}

func TestResumeStreamRunsTheApprovedCall(t *testing.T) {
	exec := &argExecutor{}
	comp := &fakeStreamCompleter{
		steps: []streamStep{
			{events: []streamEvent{toolEv(llmprovider.ToolCallChunk{
				Index: 0, ID: "c1", Name: "delete_link", ArgsJSON: `{"id":"42"}`,
			})}},
			{events: []streamEvent{textEv("done")}, resp: llmprovider.Response{Text: "done"}},
		},
	}
	r := newTestRunner(t, comp, exec, WithGuard(&approvalGuard{tool: "delete_link"}))
	sr, ok := r.(ResumeStreamRunner)
	if !ok {
		t.Fatal("runner does not implement ResumeStreamRunner")
	}
	s, turn := newBudgetTurn()

	if err := sr.Stream(context.Background(), s, turn, &captureHandler{}); !errors.Is(err, ErrApprovalRequired) {
		t.Fatalf("Stream = %v, want ErrApprovalRequired", err)
	}
	if turn.PendingCall == nil {
		t.Fatal("no pending call recorded on the streaming path")
	}

	if err := sr.ResumeStream(context.Background(), s, turn, &captureHandler{}); err != nil {
		t.Fatalf("ResumeStream = %v, want nil", err)
	}
	if len(exec.calls) != 1 || exec.calls[0].Args != `{"id":"42"}` {
		t.Fatalf("executor ran %+v, want the approved call once", exec.calls)
	}
	if turn.Status != TurnStatusDone {
		t.Errorf("turn status = %s, want done", turn.Status)
	}
	if turn.PendingCall != nil {
		t.Error("the pending call was not cleared after it ran")
	}
}
