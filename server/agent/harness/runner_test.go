package harness

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/plugin"
)

// --- fakes ---

// fakeCompleter returns pre-configured responses in sequence.
type fakeCompleter struct {
	responses []string
	callCount int
}

func (f *fakeCompleter) Complete(_ context.Context, _ string, _ string, _ []Message) (string, error) {
	if f.callCount >= len(f.responses) {
		return "done — no more tool calls", nil
	}
	resp := f.responses[f.callCount]
	f.callCount++
	return resp, nil
}

// fakeExecutor records calls and returns configurable outputs.
type fakeExecutor struct {
	calls   []string // tool names in order
	outputs map[string]string
	errs    map[string]error
}

func (f *fakeExecutor) Execute(_ context.Context, _ uint, name string, _ string) (string, error) {
	f.calls = append(f.calls, name)
	if e, ok := f.errs[name]; ok {
		return "", e
	}
	return f.outputs[name], nil
}

// denyGuard rejects a specific tool.
type denyGuard struct {
	denied string
}

func (g *denyGuard) Allow(_ context.Context, _ uint, tool string) error {
	if tool == g.denied {
		return fmt.Errorf("tool %q is denied by policy", tool)
	}
	return nil
}

// approvalGuard returns ErrApprovalRequired for specified tool.
type approvalGuard struct {
	tool string
}

func (g *approvalGuard) Allow(_ context.Context, _ uint, tool string) error {
	if g.tool == "" || tool == g.tool {
		return fmt.Errorf("%w: %s", ErrApprovalRequired, tool)
	}
	return nil
}

// --- helpers ---

// toolCallBlock builds a ```tool_call block for the fake completer.
func toolCallBlock(name, args string) string {
	return fmt.Sprintf("```tool_call\n{\"name\":%q,\"args\":%q}\n```", name, args)
}

// --- tests ---

// Test 1: Loop terminates at MaxSteps and returns ErrMaxStepsExceeded.
func TestRunner_MaxStepsExceeded(t *testing.T) {
	t.Parallel()

	// Build responses that always call a tool, forcing the loop to never stop
	// on its own.
	responses := make([]string, MaxStepsNormal+5)
	for i := range responses {
		responses[i] = toolCallBlock("noop", "{}")
	}

	comp := &fakeCompleter{responses: responses}
	exec := &fakeExecutor{
		outputs: map[string]string{"noop": "ok"},
		errs:    map[string]error{},
	}

	runner := NewRunner(comp, exec)
	sess := &Session{ID: "s1", OrgID: 1, Channel: "test"}
	turn := &Turn{Input: "go"}

	err := runner.Run(context.Background(), sess, turn)
	if !errors.Is(err, ErrMaxStepsExceeded) {
		t.Fatalf("expected ErrMaxStepsExceeded, got %v", err)
	}
	if turn.Status != TurnStatusFailed {
		t.Fatalf("expected TurnStatusFailed, got %d", turn.Status)
	}
	if len(turn.Steps) != MaxStepsNormal {
		t.Fatalf("expected %d steps, got %d", MaxStepsNormal, len(turn.Steps))
	}
}

// Test 2: Every tool output in history is wrapped in an untrusted fence.
func TestRunner_UntrustedFence(t *testing.T) {
	t.Parallel()

	comp := &fakeCompleter{
		responses: []string{
			toolCallBlock("lookup", "{}"),
			"final answer",
		},
	}
	exec := &fakeExecutor{
		outputs: map[string]string{"lookup": "raw data from external source"},
		errs:    map[string]error{},
	}

	runner := NewRunner(comp, exec)
	sess := &Session{ID: "s2", OrgID: 1, Channel: "test"}
	turn := &Turn{Input: "search"}

	if err := runner.Run(context.Background(), sess, turn); err != nil {
		t.Fatal(err)
	}

	// Check Step.OutputFenced
	if len(turn.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(turn.Steps))
	}
	fenced := turn.Steps[0].OutputFenced
	if !strings.HasPrefix(fenced, "```untrusted\n") || !strings.HasSuffix(fenced, "\n```") {
		t.Fatalf("step output not properly fenced: %q", fenced)
	}

	// Check that the fence appears in session history (tool message).
	found := false
	for _, m := range sess.History {
		if m.Role == "tool" && strings.Contains(m.Content, "```untrusted\n") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("untrusted fence not found in session history")
	}
}

// Test 3: AgentError.AgentGuidance is preserved through Step.Err.
func TestRunner_AgentErrorGuidancePreserved(t *testing.T) {
	t.Parallel()

	comp := &fakeCompleter{
		responses: []string{
			toolCallBlock("create_link", `{"url":"bad"}`),
			"I see the error, let me fix it",
		},
	}
	agentErr := plugin.NewAgentError(422, "SLUG_TAKEN", "slug already exists",
		"Try a different slug suffix", false)
	exec := &fakeExecutor{
		outputs: map[string]string{},
		errs:    map[string]error{"create_link": agentErr},
	}

	runner := NewRunner(comp, exec)
	sess := &Session{ID: "s3", OrgID: 1, Channel: "test"}
	turn := &Turn{Input: "create link"}

	if err := runner.Run(context.Background(), sess, turn); err != nil {
		t.Fatal(err)
	}

	if len(turn.Steps) < 1 {
		t.Fatal("expected at least 1 step")
	}
	stepErr := turn.Steps[0].Err
	var recovered *plugin.AgentError
	if !errors.As(stepErr, &recovered) {
		t.Fatalf("expected *plugin.AgentError in Step.Err, got %T: %v", stepErr, stepErr)
	}
	if recovered.AgentGuidance != "Try a different slug suffix" {
		t.Fatalf("AgentGuidance lost: got %q", recovered.AgentGuidance)
	}
}

// Test 4: Guard rejection prevents tool execution.
func TestRunner_GuardDeniesExecution(t *testing.T) {
	t.Parallel()

	comp := &fakeCompleter{
		responses: []string{
			toolCallBlock("dangerous_delete", "{}"),
			"ok, I won't delete then",
		},
	}
	exec := &fakeExecutor{
		outputs: map[string]string{"dangerous_delete": "deleted"},
		errs:    map[string]error{},
	}

	runner := NewRunner(comp, exec,
		WithGuard(&denyGuard{denied: "dangerous_delete"}),
	)
	sess := &Session{ID: "s4", OrgID: 1, Channel: "test"}
	turn := &Turn{Input: "delete it"}

	if err := runner.Run(context.Background(), sess, turn); err != nil {
		t.Fatal(err)
	}

	// The executor should NOT have been called for the denied tool.
	for _, name := range exec.calls {
		if name == "dangerous_delete" {
			t.Fatal("Guard denied tool was still executed")
		}
	}

	// Step.Err should contain the denial reason.
	if len(turn.Steps) < 1 {
		t.Fatal("expected at least 1 step recording the guard denial")
	}
	if turn.Steps[0].Err == nil {
		t.Fatal("expected non-nil Err on denied step")
		return
	}
	if !strings.Contains(turn.Steps[0].Err.Error(), "denied by policy") {
		t.Fatalf("unexpected denial message: %v", turn.Steps[0].Err)
	}
}

// Test 5: Profile step limits are honoured (reactor = 3 steps).
func TestRunner_ReactorProfile(t *testing.T) {
	t.Parallel()

	responses := make([]string, 20)
	for i := range responses {
		responses[i] = toolCallBlock("noop", "{}")
	}

	comp := &fakeCompleter{responses: responses}
	exec := &fakeExecutor{
		outputs: map[string]string{"noop": "ok"},
		errs:    map[string]error{},
	}

	runner := NewRunner(comp, exec, WithProfile(ProfileReactor))
	sess := &Session{ID: "s5", OrgID: 1, Channel: "reactor"}
	turn := &Turn{Input: "react"}

	err := runner.Run(context.Background(), sess, turn)
	if !errors.Is(err, ErrMaxStepsExceeded) {
		t.Fatalf("expected ErrMaxStepsExceeded, got %v", err)
	}
	if len(turn.Steps) != MaxStepsReactor {
		t.Fatalf("reactor profile: expected %d steps, got %d", MaxStepsReactor, len(turn.Steps))
	}
}

// Test 6: Approval required guard veto halts turn with TurnStatusAwaitingApproval.
func TestRunner_StopsOnApprovalRequired(t *testing.T) {
	t.Parallel()

	comp := &fakeCompleter{
		responses: []string{
			toolCallBlock("dangerous_tool", "{}"),
			toolCallBlock("another_tool", "{}"),
		},
	}
	exec := &fakeExecutor{
		outputs: map[string]string{},
		errs:    map[string]error{},
	}

	runner := NewRunner(comp, exec,
		WithGuard(&approvalGuard{tool: "dangerous_tool"}),
	)
	sess := &Session{ID: "s-hitl", OrgID: 1, Channel: "test"}
	turn := &Turn{Input: "do dangerous operation"}

	err := runner.Run(context.Background(), sess, turn)
	if !errors.Is(err, ErrApprovalRequired) {
		t.Fatalf("expected ErrApprovalRequired, got %v", err)
	}
	if turn.Status != TurnStatusAwaitingApproval {
		t.Fatalf("expected TurnStatusAwaitingApproval, got %d", turn.Status)
	}
	if len(exec.calls) != 0 {
		t.Fatalf("expected 0 executor calls, got %d", len(exec.calls))
	}
	if comp.callCount != 1 {
		t.Fatalf("expected completer to be called 1 time, got %d", comp.callCount)
	}
	if len(turn.Steps) != 1 {
		t.Fatalf("expected 1 step recorded, got %d", len(turn.Steps))
	}
	if !errors.Is(turn.Steps[0].Err, ErrApprovalRequired) {
		t.Fatalf("expected step[0].Err to be ErrApprovalRequired, got %v", turn.Steps[0].Err)
	}
}

// (Streaming tests moved to runner_stream_test.go to enforce < 460 LOC ceiling)

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

func TestRunner_CompleterError(t *testing.T) {
	t.Parallel()
	errComp := &errorCompleter{err: errors.New("provider timeout")}
	runner := NewRunner(errComp, &fakeExecutor{})
	sess := &Session{ID: "sess-err", OrgID: 1}
	turn := &Turn{Input: "hello"}

	err := runner.Run(context.Background(), sess, turn)
	if err == nil || !strings.Contains(err.Error(), "provider timeout") {
		t.Fatalf("expected provider timeout error, got %v", err)
	}
	if turn.Status != TurnStatusFailed {
		t.Errorf("turn status = %d, want %d", turn.Status, TurnStatusFailed)
	}
}

type errorCompleter struct {
	err error
}

func (e *errorCompleter) Complete(_ context.Context, _ string, _ string, _ []Message) (string, error) {
	return "", e.err
}

func TestRunner_GenericToolError(t *testing.T) {
	t.Parallel()
	comp := &fakeCompleter{
		responses: []string{
			toolCallBlock("fail_tool", "{}"),
			"handled generic error",
		},
	}
	exec := &fakeExecutor{
		outputs: map[string]string{},
		errs:    map[string]error{"fail_tool": errors.New("network disconnect")},
	}

	runner := NewRunner(comp, exec)
	sess := &Session{ID: "sess-gen-err", OrgID: 1}
	turn := &Turn{Input: "run failing tool"}

	if err := runner.Run(context.Background(), sess, turn); err != nil {
		t.Fatalf("unexpected runner error: %v", err)
	}
	if len(turn.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(turn.Steps))
	}
	if turn.Steps[0].Err == nil || turn.Steps[0].Err.Error() != "network disconnect" {
		t.Errorf("expected network disconnect error in Step.Err, got %v", turn.Steps[0].Err)
	}
}

func TestParseToolCalls_EdgeCases(t *testing.T) {
	t.Parallel()

	// Missing end marker
	calls, rem := parseToolCalls("```tool_call\n{\"name\":\"foo\"}")
	if len(calls) != 0 || !strings.Contains(rem, "```tool_call") {
		t.Errorf("missing end marker: calls=%v, rem=%q", calls, rem)
	}

	// Bad json line inside block
	calls, rem = parseToolCalls("```tool_call\ninvalid json\n{\"name\":\"valid\",\"args\":\"{}\"}\n\n```")
	if len(calls) != 1 || calls[0].Name != "valid" {
		t.Errorf("invalid json line handling: calls=%v, rem=%q", calls, rem)
	}
}
