package harness

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/plugin"
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
