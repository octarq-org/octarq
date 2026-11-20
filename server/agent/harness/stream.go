package harness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/octarq-org/octarq/server/llmprovider"
)

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

func (r *defaultRunner) streamLoop(ctx context.Context, s *Session, t *Turn, sp llmprovider.StreamProvider, h llmprovider.StreamHandler) error {
	t.PendingCall = nil
	t.Rounds = 0
	t.Elapsed = 0

	s.History = append(s.History, Message{Role: "user", Content: t.Input})

	return r.streamRounds(ctx, s, t, sp, h, time.Now())
}

func (r *defaultRunner) streamRounds(ctx context.Context, s *Session, t *Turn, sp llmprovider.StreamProvider, h llmprovider.StreamHandler, started time.Time) (err error) {
	maxSteps := r.MaxStepsFor(t.HasDestructive)
	t.Status = TurnStatusRunning

	defer func() { t.Elapsed += time.Since(started) }()

	for step := t.Rounds; ; step++ {
		t.Rounds = step
		if step >= maxSteps {
			t.Status = TurnStatusFailed
			return ErrMaxStepsExceeded
		}

		if err := r.checkBudget(t, started); err != nil {
			t.Status = TurnStatusFailed
			return err
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
				reportInvalidCalls(s, collector.fullText(), invalid)
				continue
			}
			s.History = append(s.History, Message{Role: "assistant", Content: collector.fullText()})
			t.Status = TurnStatusDone
			return nil
		}

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

func (c *streamCollector) parsePlan() ([]toolCall, string, []string) {
	calls, invalid := c.extractToolCalls()
	if len(calls) > 0 || len(invalid) > 0 {
		return calls, c.text.String(), invalid
	}
	calls, remaining, invalid := parseToolCalls(c.text.String())
	return calls, remaining, invalid
}

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

func rejectedCallLine(name, args string) string {
	trimmed := strings.TrimSpace(args)
	if trimmed == "" {
		trimmed = "{}"
	}
	return fmt.Sprintf(`{"name":%q,"args":%s}`, name, trimmed)
}
