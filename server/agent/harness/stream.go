package harness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/octarq-org/octarq/server/llmprovider"
)

// extractStreamProvider attempts to retrieve an llmprovider.StreamProvider
// from the runner's completer, supporting direct implementation or
// *CompleterAdapter wrapping a StreamProvider.
func (r *defaultRunner) extractStreamProvider() (llmprovider.StreamProvider, bool) {
	if ca, ok := r.completer.(*CompleterAdapter); ok {
		if sp, ok := ca.Provider.(llmprovider.StreamProvider); ok {
			return sp, true
		}
		return nil, false
	}
	if sp, ok := r.completer.(llmprovider.StreamProvider); ok {
		return sp, true
	}
	return nil, false
}

// Stream executes the agent loop using a streaming provider if available.
// If the underlying completer does not support streaming, it falls back to Run.
func (r *defaultRunner) Stream(ctx context.Context, s *Session, t *Turn, h llmprovider.StreamHandler) error {
	sp, ok := r.extractStreamProvider()
	if !ok {
		return r.Run(ctx, s, t)
	}

	r.tracer.Event(ctx, TraceEvent{Kind: TraceKindTurnStart})
	err := r.streamLoop(ctx, s, t, sp, h)
	r.tracer.Event(ctx, TraceEvent{
		Kind:            TraceKindTurnEnd,
		Status:          t.Status.String(),
		InputTokens:     t.InputTokens,
		OutputTokens:    t.OutputTokens,
		ReasoningTokens: t.ReasoningTokens,
	})
	return err
}

// streamLoop is the streaming agent loop. It always leaves t.Status set to
// the terminal state of the turn.
func (r *defaultRunner) streamLoop(ctx context.Context, s *Session, t *Turn, sp llmprovider.StreamProvider, h llmprovider.StreamHandler) error {
	maxSteps := r.effectiveMaxSteps(t.HasDestructive)
	t.Status = TurnStatusRunning

	// Seed history with the user's input.
	s.History = append(s.History, Message{Role: "user", Content: t.Input})

	for step := 0; ; step++ {
		if step >= maxSteps {
			t.Status = TurnStatusFailed
			return ErrMaxStepsExceeded
		}

		msgs := toProviderMessages(s.History)
		req := llmprovider.Request{
			Model:    r.model,
			System:   r.system,
			Messages: msgs,
			Thinking: t.Thinking,
		}

		collector := newStreamCollector(h)
		resp, err := sp.StreamComplete(ctx, req, collector)
		if err != nil {
			t.Status = TurnStatusFailed
			return fmt.Errorf("harness: stream error: %w", err)
		}

		if collector.err != nil {
			t.Status = TurnStatusFailed
			return fmt.Errorf("harness: stream order error: %w", collector.err)
		}

		t.InputTokens += resp.InputTokens
		t.OutputTokens += resp.OutputTokens
		t.ReasoningTokens += resp.ReasoningTokens

		calls, remaining, invalid := collector.parsePlan()
		if len(calls) == 0 {
			if len(invalid) > 0 {
				// The model tried to call a tool and got the format wrong.
				// Tell it so instead of ending the turn on a call it
				// believes it made. The non-streaming path does the same.
				reportInvalidCalls(s, collector.fullText(), invalid)
				continue
			}
			// No tool calls — the turn is done.
			s.History = append(s.History, Message{Role: "assistant", Content: collector.fullText()})
			t.Status = TurnStatusDone
			return nil
		}

		// Execute each tool call.
		var obs StepObserver
		if o, ok := h.(StepObserver); ok {
			obs = o
		}
		if err := r.executeSteps(ctx, s, t, step, calls, remaining, invalid, obs); err != nil {
			if errors.Is(err, ErrApprovalRequired) {
				t.Status = TurnStatusAwaitingApproval
				return err
			}
			t.Status = TurnStatusFailed
			return err
		}
	}
}

type accumulatedToolCall struct {
	index    int
	id       string
	name     string
	argsJSON strings.Builder
}

type streamCollector struct {
	target    llmprovider.StreamHandler
	guard     llmprovider.StreamOrderGuard
	err       error
	text      strings.Builder
	thinking  strings.Builder
	toolCalls map[int]*accumulatedToolCall
	order     []int
}

func newStreamCollector(target llmprovider.StreamHandler) *streamCollector {
	return &streamCollector{
		target:    target,
		toolCalls: make(map[int]*accumulatedToolCall),
	}
}

func (c *streamCollector) OnThinking(delta string) {
	if c.err != nil {
		return
	}
	if err := c.guard.OnThinking(delta); err != nil {
		c.err = err
		return
	}
	c.thinking.WriteString(delta)
	if c.target != nil && delta != "" {
		c.target.OnThinking(delta)
	}
}

func (c *streamCollector) OnText(delta string) {
	if c.err != nil {
		return
	}
	c.guard.OnText(delta)
	c.text.WriteString(delta)
	if c.target != nil && delta != "" {
		c.target.OnText(delta)
	}
}

func (c *streamCollector) OnToolCall(chunk llmprovider.ToolCallChunk) {
	if c.err != nil {
		return
	}
	c.guard.OnToolCall(chunk)
	tc, exists := c.toolCalls[chunk.Index]
	if !exists {
		tc = &accumulatedToolCall{
			index: chunk.Index,
			id:    chunk.ID,
			name:  chunk.Name,
		}
		c.toolCalls[chunk.Index] = tc
		c.order = append(c.order, chunk.Index)
	}
	if chunk.ID != "" {
		tc.id = chunk.ID
	}
	if chunk.Name != "" {
		tc.name = chunk.Name
	}
	tc.argsJSON.WriteString(chunk.ArgsJSON)

	if c.target != nil {
		c.target.OnToolCall(chunk)
	}
}

func (c *streamCollector) fullText() string {
	return c.text.String()
}

// parsePlan reads the accumulated stream into the calls to run, the text to
// keep in the history, and the calls the model wrote wrongly.
//
// A provider that streams tool calls natively is the only source of calls.
// The fenced block is read only when the provider streamed no call, so a call
// is never run twice. The arguments of a native call go through the same
// normalization as a fenced call: a call whose arguments are not an object
// is a call the model wrote wrongly, not a call that runs.
func (c *streamCollector) parsePlan() ([]toolCall, string, []string) {
	calls, invalid := c.extractToolCalls()
	if len(calls) > 0 || len(invalid) > 0 {
		return calls, c.text.String(), invalid
	}
	calls, remaining, invalid := parseToolCalls(c.text.String())
	return calls, remaining, invalid
}

// extractToolCalls returns the calls the provider streamed, and the calls it
// streamed wrongly. A call without a name, or with arguments that are not a
// JSON object, is reported instead of run.
func (c *streamCollector) extractToolCalls() ([]toolCall, []string) {
	if len(c.toolCalls) == 0 {
		return nil, nil
	}
	var calls []toolCall
	var invalid []string
	for _, idx := range c.order {
		tc := c.toolCalls[idx]
		if tc.name == "" {
			invalid = append(invalid, rejectedCallLine("", tc.argsJSON.String()))
			continue
		}
		// The provider streams the arguments as a JSON object; a tool
		// without arguments arrives as an empty string.
		args, ok := normalizeToolArgs(json.RawMessage(tc.argsJSON.String()))
		if !ok {
			invalid = append(invalid, rejectedCallLine(tc.name, tc.argsJSON.String()))
			continue
		}
		calls = append(calls, toolCall{
			Name:     tc.name,
			ArgsJSON: args,
		})
	}
	return calls, invalid
}

// rejectedCallLine renders a call the model wrote wrongly, in the shape the
// notice quotes back to it.
func rejectedCallLine(name, args string) string {
	trimmed := strings.TrimSpace(args)
	if trimmed == "" {
		trimmed = "{}"
	}
	return fmt.Sprintf(`{"name":%q,"args":%s}`, name, trimmed)
}
