package harness

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/llmprovider"
)

// argExecutor records the arguments of every call, so a test can assert on
// what was executed rather than only on which tool was asked for.
type argExecutor struct {
	calls []struct{ Tool, Args string }
}

func (e *argExecutor) Execute(_ context.Context, _ uint, name, argsJSON string) (string, error) {
	e.calls = append(e.calls, struct{ Tool, Args string }{name, argsJSON})
	return "ok", nil
}

// argGuard refuses a call by its arguments. It exists to prove the guard sees
// them: a rule about what a call would do cannot be written without them.
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
	r := NewRunner(comp, &argExecutor{}, WithGuard(g))
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

// TestPendingCallRecordsTheExactCall pins the record an operator decides on.
func TestPendingCallRecordsTheExactCall(t *testing.T) {
	exec := &argExecutor{}
	comp := &fakeCompleter{responses: []string{toolCallBlock("delete_link", `{"id":"42"}`)}}
	r := NewRunner(comp, exec, WithGuard(&approvalGuard{tool: "delete_link"}))
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

// TestPendingCallHashIgnoresFormatting pins that one call has one hash: a model
// that writes the same object with different spacing or key order is making the
// same call, and an approval must not be sidestepped by re-rendering it.
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

	// The tool name is part of the identity, and the separator stops one name's
	// text from sliding into the other's arguments.
	d := newPendingCall(toolCall{Name: "ab", ArgsJSON: json.RawMessage(`{}`)})
	e := newPendingCall(toolCall{Name: "a", ArgsJSON: json.RawMessage(`{"b":{}}`)})
	if d.Hash == e.Hash {
		t.Fatal("two different calls share a hash")
	}
}

// TestNewPendingCallMatchesTheRecordedOne pins the constructor a host uses to
// rebuild a stored approval. What it builds must be the record the runner
// wrote; otherwise the resumed call is not the one the operator saw.
func TestNewPendingCallMatchesTheRecordedOne(t *testing.T) {
	comp := &fakeCompleter{responses: []string{toolCallBlock("delete_link", `{"id":"42"}`)}}
	r := NewRunner(comp, &argExecutor{}, WithGuard(&approvalGuard{tool: "delete_link"}))
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
	// A stored approval is read back as text; the formatting of that text is not
	// part of the call.
	if messy := NewPendingCall(recorded.Tool, "{\n  \"id\": \"42\"\n}"); *messy != *recorded {
		t.Fatalf("re-rendered arguments changed the record: %+v", *messy)
	}
}

// TestResumeRunsTheApprovedCallNotTheNextOne is the defect this exists for.
// The model writes a different call when asked again; the approval belongs to
// the first one, and that is the one that runs.
func TestResumeRunsTheApprovedCallNotTheNextOne(t *testing.T) {
	exec := &argExecutor{}
	comp := &fakeCompleter{responses: []string{
		toolCallBlock("delete_link", `{"id":"42"}`),
		toolCallBlock("delete_link", `{"id":"99"}`), // what the model would say next
	}}
	r := NewRunner(comp, exec, WithGuard(&approvalGuard{tool: "delete_link"}))
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
	// The second round is a different call, so it waits for its own decision.
	if turn.PendingCall == nil || turn.PendingCall.ArgsJSON != `{"id":"99"}` {
		t.Fatalf("pending call = %+v, want the second call recorded separately", turn.PendingCall)
	}
}

// TestResumeDoesNotAskTheGuardAgain pins that a decision answers the question
// the guard asked. Asking again would only produce the same refusal.
func TestResumeDoesNotAskTheGuardAgain(t *testing.T) {
	g := &countingApprovalGuard{tool: "delete_link"}
	exec := &argExecutor{}
	comp := &fakeCompleter{responses: []string{
		toolCallBlock("delete_link", `{"id":"42"}`),
		"nothing left to do",
	}}
	r := NewRunner(comp, exec, WithGuard(g))
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

// countingApprovalGuard counts its consultations and refuses one tool.
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

// TestResumeDoesNotBuyAnotherRound pins the step budget across an approval: the
// round that produced the call is spent, so a one-round turn ends on resume.
func TestResumeDoesNotBuyAnotherRound(t *testing.T) {
	exec := &argExecutor{}
	comp := &fakeCompleter{responses: []string{toolCallBlock("delete_link", `{"id":"42"}`)}}
	r := NewRunner(comp, exec, WithGuard(&approvalGuard{tool: "delete_link"}), WithMaxSteps(1))
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
	r := NewRunner(&fakeCompleter{}, exec)
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

// TestResumeStreamRunsTheApprovedCall keeps the streaming path honest: an
// approval granted on a streamed turn runs the same call and streams on.
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
	r := NewRunner(comp, exec, WithGuard(&approvalGuard{tool: "delete_link"}))
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
