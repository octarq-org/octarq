package harness

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/llmprovider"
)

// --- Streaming fakes and tests ---

type streamEvent struct {
	thinking string
	text     string
	tool     *llmprovider.ToolCallChunk
}

func thinkingEv(th string) streamEvent                { return streamEvent{thinking: th} }
func textEv(tx string) streamEvent                    { return streamEvent{text: tx} }
func toolEv(tc llmprovider.ToolCallChunk) streamEvent { return streamEvent{tool: &tc} }

type streamStep struct {
	events []streamEvent
	resp   llmprovider.Response
	err    error
}

type fakeStreamCompleter struct {
	steps     []streamStep
	callCount int
}

func (f *fakeStreamCompleter) Complete(_ context.Context, _ string, _ string, _ []Message) (string, error) {
	if f.callCount >= len(f.steps) {
		return "done", nil
	}
	st := f.steps[f.callCount]
	f.callCount++
	var sb strings.Builder
	for _, ev := range st.events {
		if ev.text != "" {
			sb.WriteString(ev.text)
		}
	}
	return sb.String(), st.err
}

func (f *fakeStreamCompleter) StreamComplete(_ context.Context, _ llmprovider.Request, h llmprovider.StreamHandler) (llmprovider.Response, error) {
	if f.callCount >= len(f.steps) {
		return llmprovider.Response{Text: "done"}, nil
	}
	st := f.steps[f.callCount]
	f.callCount++
	if st.err != nil {
		return llmprovider.Response{}, st.err
	}
	for _, ev := range st.events {
		if ev.thinking != "" && h != nil {
			h.OnThinking(ev.thinking)
		}
		if ev.text != "" && h != nil {
			h.OnText(ev.text)
		}
		if ev.tool != nil && h != nil {
			h.OnToolCall(*ev.tool)
		}
	}
	return st.resp, nil
}

type captureHandler struct {
	thinking []string
	texts    []string
	tools    []llmprovider.ToolCallChunk
}

func (c *captureHandler) OnThinking(delta string) {
	c.thinking = append(c.thinking, delta)
}

func (c *captureHandler) OnText(delta string) {
	c.texts = append(c.texts, delta)
}

func (c *captureHandler) OnToolCall(chunk llmprovider.ToolCallChunk) {
	c.tools = append(c.tools, chunk)
}

// Test 6: Stream with thinking and text accumulation and token counting.
func TestRunner_Stream_ThinkingThenText(t *testing.T) {
	t.Parallel()

	comp := &fakeStreamCompleter{
		steps: []streamStep{
			{
				events: []streamEvent{
					thinkingEv("let me "),
					thinkingEv("ponder"),
					textEv("answer "),
					textEv("is 42"),
				},
				resp: llmprovider.Response{
					InputTokens:     10,
					OutputTokens:    8,
					ReasoningTokens: 6,
					Text:            "answer is 42",
				},
			},
		},
	}
	exec := &fakeExecutor{outputs: map[string]string{}, errs: map[string]error{}}

	runner := NewRunner(comp, exec)
	sr, ok := runner.(StreamRunner)
	if !ok {
		t.Fatalf("expected runner to implement StreamRunner")
	}

	h := &captureHandler{}
	sess := &Session{ID: "s-stream-1", OrgID: 1, Channel: "web"}
	turn := &Turn{Input: "what is the answer?"}

	if err := sr.Stream(context.Background(), sess, turn, h); err != nil {
		t.Fatalf("Stream failed: %v", err)
	}

	if turn.Status != TurnStatusDone {
		t.Fatalf("expected TurnStatusDone, got %d", turn.Status)
	}
	if turn.InputTokens != 10 || turn.OutputTokens != 8 || turn.ReasoningTokens != 6 {
		t.Errorf("token usage mismatch: %d/%d/%d", turn.InputTokens, turn.OutputTokens, turn.ReasoningTokens)
	}
	if strings.Join(h.thinking, "") != "let me ponder" {
		t.Errorf("captured thinking = %q", strings.Join(h.thinking, ""))
	}
	if strings.Join(h.texts, "") != "answer is 42" {
		t.Errorf("captured text = %q", strings.Join(h.texts, ""))
	}

	// Verify history
	if len(sess.History) != 2 {
		t.Fatalf("expected 2 messages in history, got %d", len(sess.History))
	}
	if sess.History[0].Role != "user" || sess.History[0].Content != "what is the answer?" {
		t.Errorf("history[0] = %+v", sess.History[0])
	}
	if sess.History[1].Role != "assistant" || sess.History[1].Content != "answer is 42" {
		t.Errorf("history[1] = %+v", sess.History[1])
	}
}

// Test 7: Stream with tool call chunks accumulation and multi-step turn.
func TestRunner_Stream_ToolCalls(t *testing.T) {
	t.Parallel()

	comp := &fakeStreamCompleter{
		steps: []streamStep{
			{
				events: []streamEvent{
					toolEv(llmprovider.ToolCallChunk{Index: 0, ID: "call_1", Name: "lookup", ArgsJSON: `{"id":`}),
					toolEv(llmprovider.ToolCallChunk{Index: 0, ID: "call_1", Name: "lookup", ArgsJSON: `"123"}`}),
				},
				resp: llmprovider.Response{
					InputTokens:  15,
					OutputTokens: 10,
					StopReason:   "tool_use",
				},
			},
			{
				events: []streamEvent{
					textEv("found user 123"),
				},
				resp: llmprovider.Response{
					InputTokens:  20,
					OutputTokens: 5,
					StopReason:   "end_turn",
				},
			},
		},
	}
	exec := &fakeExecutor{
		outputs: map[string]string{"lookup": `{"name":"Alice"}`},
		errs:    map[string]error{},
	}

	runner := NewRunner(comp, exec)
	sr := runner.(StreamRunner)
	h := &captureHandler{}
	sess := &Session{ID: "s-stream-2", OrgID: 1, Channel: "web"}
	turn := &Turn{Input: "lookup user 123"}

	if err := sr.Stream(context.Background(), sess, turn, h); err != nil {
		t.Fatalf("Stream failed: %v", err)
	}

	if turn.Status != TurnStatusDone {
		t.Fatalf("expected TurnStatusDone, got %d", turn.Status)
	}
	if len(turn.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(turn.Steps))
	}
	if turn.Steps[0].ToolName != "lookup" || turn.Steps[0].ArgsJSON != `{"id":"123"}` {
		t.Errorf("step[0] = %+v", turn.Steps[0])
	}
	if !strings.HasPrefix(turn.Steps[0].OutputFenced, "```untrusted\n") {
		t.Errorf("untrusted fence missing in step output: %q", turn.Steps[0].OutputFenced)
	}
	if turn.InputTokens != 35 || turn.OutputTokens != 15 {
		t.Errorf("accumulated tokens mismatch: %d/%d", turn.InputTokens, turn.OutputTokens)
	}
}

// Test 8: Stream order violation causes TurnStatusFailed and returns ErrStreamOrder.
func TestRunner_Stream_OrderViolation(t *testing.T) {
	t.Parallel()

	comp := &fakeStreamCompleter{
		steps: []streamStep{
			{
				events: []streamEvent{
					textEv("text first"),
					thinkingEv("out of order thinking"),
				},
			},
		},
	}
	exec := &fakeExecutor{outputs: map[string]string{}, errs: map[string]error{}}

	runner := NewRunner(comp, exec)
	sr := runner.(StreamRunner)
	sess := &Session{ID: "s-stream-3", OrgID: 1, Channel: "web"}
	turn := &Turn{Input: "trigger violation"}

	err := sr.Stream(context.Background(), sess, turn, nil)
	if err == nil {
		t.Fatal("expected error on stream order violation, got nil")
	}
	if !errors.Is(err, llmprovider.ErrStreamOrder) {
		t.Fatalf("expected errors.Is(err, ErrStreamOrder), got %v", err)
	}
	if turn.Status != TurnStatusFailed {
		t.Errorf("expected TurnStatusFailed, got %d", turn.Status)
	}
	if len(turn.Steps) != 0 {
		t.Errorf("expected 0 steps on stream error, got %d", len(turn.Steps))
	}
}

// Test 9: Stream fallback to non-streaming when Completer does not implement StreamProvider.
func TestRunner_Stream_FallbackNonStreaming(t *testing.T) {
	t.Parallel()

	comp := &fakeCompleter{
		responses: []string{"plain text answer"},
	}
	exec := &fakeExecutor{outputs: map[string]string{}, errs: map[string]error{}}

	runner := NewRunner(comp, exec)
	sr, ok := runner.(StreamRunner)
	if !ok {
		t.Fatal("runner should implement StreamRunner")
	}

	sess := &Session{ID: "s-stream-4", OrgID: 1, Channel: "test"}
	turn := &Turn{Input: "hello"}

	if err := sr.Stream(context.Background(), sess, turn, nil); err != nil {
		t.Fatalf("fallback Stream failed: %v", err)
	}
	if turn.Status != TurnStatusDone {
		t.Errorf("expected TurnStatusDone, got %d", turn.Status)
	}
	if len(sess.History) != 2 || sess.History[1].Content != "plain text answer" {
		t.Errorf("unexpected history: %+v", sess.History)
	}
}

// Test 10: CompleterAdapter stream detection and delegation.
func TestCompleterAdapter_Streaming(t *testing.T) {
	t.Parallel()

	t.Run("DelegatesWhenProviderSupportsStreaming", func(t *testing.T) {
		fakeP := &fakeStreamCompleter{
			steps: []streamStep{
				{
					events: []streamEvent{
						textEv("streamed from adapter"),
					},
					resp: llmprovider.Response{Text: "streamed from adapter"},
				},
			},
		}
		adapter := &CompleterAdapter{Provider: &fakeProviderAdapter{fakeP}}
		runner := NewRunner(adapter, &fakeExecutor{})
		sr := runner.(StreamRunner)

		sess := &Session{ID: "s-stream-5", OrgID: 1, Channel: "test"}
		turn := &Turn{Input: "hi"}
		h := &captureHandler{}

		if err := sr.Stream(context.Background(), sess, turn, h); err != nil {
			t.Fatalf("Stream failed: %v", err)
		}
		if turn.Status != TurnStatusDone {
			t.Errorf("status = %d, want done", turn.Status)
		}
		if strings.Join(h.texts, "") != "streamed from adapter" {
			t.Errorf("captured = %q", strings.Join(h.texts, ""))
		}
	})

	t.Run("FallsBackWhenProviderDoesNotSupportStreaming", func(t *testing.T) {
		nonStreamP := &fakeNonStreamProvider{text: "fallback answer"}
		adapter := &CompleterAdapter{Provider: nonStreamP}
		runner := NewRunner(adapter, &fakeExecutor{})
		sr := runner.(StreamRunner)

		sess := &Session{ID: "s-stream-6", OrgID: 1, Channel: "test"}
		turn := &Turn{Input: "hi"}

		if err := sr.Stream(context.Background(), sess, turn, nil); err != nil {
			t.Fatalf("Stream failed: %v", err)
		}
		if turn.Status != TurnStatusDone {
			t.Errorf("status = %d, want done", turn.Status)
		}
	})
}

// Test 11: Stream stops on ErrApprovalRequired and marks TurnStatusAwaitingApproval.
func TestRunner_Stream_StopsOnApprovalRequired(t *testing.T) {
	t.Parallel()

	comp := &fakeStreamCompleter{
		steps: []streamStep{
			{
				events: []streamEvent{
					toolEv(llmprovider.ToolCallChunk{Index: 0, ID: "call_1", Name: "dangerous_tool", ArgsJSON: "{}"}),
				},
				resp: llmprovider.Response{
					StopReason: "tool_use",
				},
			},
			{
				events: []streamEvent{
					textEv("should not reach here"),
				},
				resp: llmprovider.Response{
					StopReason: "end_turn",
				},
			},
		},
	}
	exec := &fakeExecutor{
		outputs: map[string]string{},
		errs:    map[string]error{},
	}

	runner := NewRunner(comp, exec,
		WithGuard(&approvalGuard{tool: "dangerous_tool"}),
	)
	sr, ok := runner.(StreamRunner)
	if !ok {
		t.Fatal("runner should implement StreamRunner")
	}

	sess := &Session{ID: "s-stream-hitl", OrgID: 1, Channel: "web"}
	turn := &Turn{Input: "do dangerous stream"}

	err := sr.Stream(context.Background(), sess, turn, nil)
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
		t.Fatalf("expected stream completer to be called 1 time, got %d", comp.callCount)
	}
}

type fakeProviderAdapter struct {
	*fakeStreamCompleter
}

func (f *fakeProviderAdapter) Name() string         { return "fake" }
func (f *fakeProviderAdapter) DefaultModel() string { return "fake-model" }
func (f *fakeProviderAdapter) CheapModel() string   { return "fake-cheap" }
func (f *fakeProviderAdapter) Complete(ctx context.Context, req llmprovider.Request) (llmprovider.Response, error) {
	txt, err := f.fakeStreamCompleter.Complete(ctx, req.Model, req.System, nil)
	return llmprovider.Response{Text: txt}, err
}

type fakeNonStreamProvider struct {
	text string
}

func (f *fakeNonStreamProvider) Name() string         { return "fake-non-stream" }
func (f *fakeNonStreamProvider) DefaultModel() string { return "fake-model" }
func (f *fakeNonStreamProvider) CheapModel() string   { return "fake-cheap" }
func (f *fakeNonStreamProvider) Complete(_ context.Context, _ llmprovider.Request) (llmprovider.Response, error) {
	return llmprovider.Response{Text: f.text}, nil
}
