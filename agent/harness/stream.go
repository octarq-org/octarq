package harness

import (
	"context"
	"fmt"
	"strings"

	"github.com/octarq-org/octarq/llmprovider"
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

	maxSteps := r.profile.MaxSteps(t.HasDestructive)
	t.Status = TurnStatusRunning

	// Seed history with the user's input.
	s.History = append(s.History, Message{Role: "user", Content: t.Input})

	for step := 0; ; step++ {
		if step >= maxSteps {
			t.Status = TurnStatusFailed
			r.tracer.TurnEnd(ctx, s.ID, TurnStatusFailed)
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
			r.tracer.TurnEnd(ctx, s.ID, TurnStatusFailed)
			return fmt.Errorf("harness: stream error: %w", err)
		}

		if collector.err != nil {
			t.Status = TurnStatusFailed
			r.tracer.TurnEnd(ctx, s.ID, TurnStatusFailed)
			return fmt.Errorf("harness: stream order error: %w", collector.err)
		}

		t.InputTokens += resp.InputTokens
		t.OutputTokens += resp.OutputTokens
		t.ReasoningTokens += resp.ReasoningTokens

		calls, remaining := collector.extractToolCalls()
		if len(calls) == 0 {
			calls, remaining = parseToolCalls(collector.fullText())
		}

		if len(calls) == 0 {
			// No tool calls — the turn is done.
			s.History = append(s.History, Message{Role: "assistant", Content: collector.fullText()})
			t.Status = TurnStatusDone
			r.tracer.TurnEnd(ctx, s.ID, TurnStatusDone)
			return nil
		}

		// Execute each tool call.
		if err := r.executeSteps(ctx, s, t, step, calls, remaining); err != nil {
			t.Status = TurnStatusFailed
			r.tracer.TurnEnd(ctx, s.ID, TurnStatusFailed)
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

func (c *streamCollector) extractToolCalls() ([]toolCall, string) {
	if len(c.toolCalls) == 0 {
		return nil, c.text.String()
	}
	var calls []toolCall
	for _, idx := range c.order {
		tc := c.toolCalls[idx]
		if tc.name != "" {
			calls = append(calls, toolCall{
				Name:     tc.name,
				ArgsJSON: tc.argsJSON.String(),
			})
		}
	}
	return calls, c.text.String()
}
