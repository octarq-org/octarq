package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/octarq-org/octarq/server/llmprovider"
	"github.com/octarq-org/octarq/server/plugin"
)

var ErrMaxStepsExceeded = errors.New("harness: max steps exceeded")

var ErrMaxStepsExceedsProfile = errors.New("harness: max steps exceeds profile ceiling")

type ToolExecutor interface {
	Execute(ctx context.Context, orgID uint, name string, argsJSON string) (output string, err error)
}

type Completer interface {
	Complete(ctx context.Context, model string, system string, messages []Message) (string, error)
}

type CompletionResult struct {
	Text             string
	InputTokens      int
	OutputTokens     int
	ReasoningTokens  int
	CacheReadTokens  int
	CacheWriteTokens int
}

type ResultCompleter interface {
	CompleteResult(ctx context.Context, model string, system string, messages []Message) (CompletionResult, error)
}

type Runner interface {
	Run(ctx context.Context, s *Session, t *Turn) error
	Resume(ctx context.Context, s *Session, t *Turn) error
	MaxStepsFor(hasDestructive bool) int
}

type StreamRunner interface {
	Runner
	Stream(ctx context.Context, s *Session, t *Turn, h llmprovider.StreamHandler) error
}

type toolCall struct {
	Name     string          `json:"name"`
	ArgsJSON json.RawMessage `json:"args"`
}

func parseToolCalls(text string) ([]toolCall, string, []string) {
	const startMarker = "```tool_call\n"
	const endMarker = "\n```"

	idx := strings.Index(text, startMarker)
	if idx == -1 {
		return nil, text, nil
	}
	after := text[idx+len(startMarker):]
	endIdx := strings.Index(after, endMarker)
	if endIdx == -1 {
		return nil, text, nil
	}
	block := after[:endIdx]

	var calls []toolCall
	var invalid []string
	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var raw struct {
			Name string          `json:"name"`
			Args json.RawMessage `json:"args"`
		}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			invalid = append(invalid, line)
			continue
		}
		if raw.Name == "" {
			invalid = append(invalid, line)
			continue
		}
		args, ok := normalizeToolArgs(raw.Args)
		if !ok {
			invalid = append(invalid, line)
			continue
		}
		calls = append(calls, toolCall{Name: raw.Name, ArgsJSON: args})
	}

	remaining := strings.TrimSpace(text[:idx] + after[endIdx+len(endMarker):])
	return calls, remaining, invalid
}

func normalizeToolArgs(raw json.RawMessage) (json.RawMessage, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return json.RawMessage("{}"), true
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return nil, false
		}
		trimmed = bytes.TrimSpace([]byte(s))
		if len(trimmed) == 0 {
			return json.RawMessage("{}"), true
		}
	}
	if trimmed[0] != '{' {
		return nil, false
	}
	if !json.Valid(trimmed) {
		return nil, false
	}
	return json.RawMessage(trimmed), true
}

func wrapUntrustedFence(output string) string {
	return "```untrusted\n" + output + "\n```"
}

type defaultRunner struct {
	completer Completer
	executor  ToolExecutor
	guard     Guard
	tracer    Tracer
	profile   Profile
	model     string
	system    string
	maxSteps  int
	budget    Budget
}

type RunnerOption func(*defaultRunner)

func WithGuard(g Guard) RunnerOption {
	return func(r *defaultRunner) { r.guard = g }
}

func WithTracer(t Tracer) RunnerOption {
	return func(r *defaultRunner) { r.tracer = t }
}

func WithProfile(p Profile) RunnerOption {
	return func(r *defaultRunner) { r.profile = p }
}

func WithModel(m string) RunnerOption {
	return func(r *defaultRunner) { r.model = m }
}

func WithSystem(s string) RunnerOption {
	return func(r *defaultRunner) { r.system = s }
}

func WithMaxSteps(n int) RunnerOption {
	return func(r *defaultRunner) { r.maxSteps = n }
}

func StepBudget(p Profile, maxSteps int, hasDestructive bool) (int, error) {
	if !p.valid() {
		return 0, fmt.Errorf("harness: invalid profile %d", int(p))
	}
	if maxSteps < 0 {
		return 0, fmt.Errorf("harness: maxSteps must not be negative, got %d", maxSteps)
	}
	ceiling := p.MaxSteps(hasDestructive)
	if maxSteps == 0 {
		return ceiling, nil
	}
	if maxSteps > ceiling {
		return 0, fmt.Errorf("%w: profile %s allows at most %d steps, got %d", ErrMaxStepsExceedsProfile, p, ceiling, maxSteps)
	}
	return maxSteps, nil
}

func NewRunner(c Completer, exec ToolExecutor, opts ...RunnerOption) (Runner, error) {
	r := &defaultRunner{
		completer: c,
		executor:  exec,
		guard:     NopGuard{},
		tracer:    NopTracer{},
		profile:   ProfileNormal,
	}
	for _, o := range opts {
		o(r)
	}
	if _, err := StepBudget(r.profile, r.maxSteps, false); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *defaultRunner) Run(ctx context.Context, s *Session, t *Turn) error {
	r.tracer.Event(ctx, TraceEvent{Kind: TraceKindTurnStart})
	err := r.loop(ctx, s, t)
	r.tracer.Event(ctx, TraceEvent{
		Kind:             TraceKindTurnEnd,
		Status:           t.Status.String(),
		InputTokens:      t.InputTokens,
		OutputTokens:     t.OutputTokens,
		ReasoningTokens:  t.ReasoningTokens,
		CacheReadTokens:  t.CacheReadTokens,
		CacheWriteTokens: t.CacheWriteTokens,
	})
	return err
}

func (r *defaultRunner) loop(ctx context.Context, s *Session, t *Turn) error {
	t.PendingCall = nil
	t.Rounds = 0
	t.Elapsed = 0

	s.History = append(s.History, Message{Role: "user", Content: t.Input})

	return r.runRounds(ctx, s, t, time.Now())
}

func (r *defaultRunner) runRounds(ctx context.Context, s *Session, t *Turn, started time.Time) (err error) {
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

		var text string
		if rc, ok := r.completer.(ResultCompleter); ok {
			res, err := rc.CompleteResult(ctx, r.model, r.system, s.History)
			if err != nil {
				t.Status = TurnStatusFailed
				return fmt.Errorf("harness: completer error: %w", err)
			}
			text = res.Text
			t.InputTokens += res.InputTokens
			t.OutputTokens += res.OutputTokens
			t.ReasoningTokens += res.ReasoningTokens
			t.CacheReadTokens += res.CacheReadTokens
			t.CacheWriteTokens += res.CacheWriteTokens
		} else {
			var err error
			text, err = r.completer.Complete(ctx, r.model, r.system, s.History)
			if err != nil {
				t.Status = TurnStatusFailed
				return fmt.Errorf("harness: completer error: %w", err)
			}
		}

		calls, remaining, invalid := parseToolCalls(text)
		if len(calls) == 0 {
			if len(invalid) > 0 {
				reportInvalidCalls(s, text, invalid)
				continue
			}
			s.History = append(s.History, Message{Role: "assistant", Content: text})
			t.Status = TurnStatusDone
			return nil
		}

		if err := r.executeSteps(ctx, s, t, step, calls, remaining, invalid, nil); err != nil {
			if errors.Is(err, ErrApprovalRequired) {
				t.Status = TurnStatusAwaitingApproval
				return err
			}
			t.Status = TurnStatusFailed
			return err
		}
	}
}

func (r *defaultRunner) MaxStepsFor(hasDestructive bool) int {
	ceiling := r.profile.MaxSteps(hasDestructive)
	if r.maxSteps > 0 && r.maxSteps < ceiling {
		return r.maxSteps
	}
	return ceiling
}

func invalidToolCallNotice(invalid []string) string {
	var b strings.Builder
	b.WriteString("Your tool call was not executed because it could not be parsed.")
	b.WriteString(" Emit one fenced block per turn, each line a JSON object with \"name\" and")
	b.WriteString(" \"args\" (an object), for example:")
	b.WriteString("\n```tool_call\n{\"name\": \"list_domains\", \"args\": {}}\n```")
	b.WriteString("\nThe rejected line(s):")
	for _, line := range invalid {
		b.WriteString("\n- ")
		b.WriteString(line)
	}
	return b.String()
}

func reportInvalidCalls(s *Session, text string, invalid []string) {
	s.History = append(s.History, Message{Role: "assistant", Content: text})
	s.History = append(s.History, Message{
		Role:    "tool",
		Content: wrapUntrustedFence(invalidToolCallNotice(invalid)),
	})
}

func skippedCallsNotice(skipped []toolCall) string {
	var b strings.Builder
	b.WriteString("These tool calls did not run: the turn stopped for operator approval.")
	for _, tc := range skipped {
		b.WriteString("\n- ")
		b.WriteString(tc.Name)
	}
	return b.String()
}

type StepObserver interface {
	OnToolStart(step int, tool, argsJSON string)
	OnToolEnd(step int, tool, output string, err error)
}

func (r *defaultRunner) executeSteps(ctx context.Context, s *Session, t *Turn, step int, calls []toolCall, remaining string, invalid []string, obs StepObserver) error {
	if remaining != "" {
		s.History = append(s.History, Message{
			Role:    "assistant",
			Content: remaining,
		})
	}
	for i, tc := range calls {
		st := Step{
			ToolName: tc.Name,
			ArgsJSON: string(tc.ArgsJSON),
		}

		if guardErr := r.guard.Allow(ctx, s.OrgID, tc.Name, string(tc.ArgsJSON)); guardErr != nil {
			st.Err = guardErr
			st.OutputFenced = wrapUntrustedFence(guardErr.Error())
			t.Steps = append(t.Steps, st)
			guardStatus := TraceStatusDenied
			if errors.Is(guardErr, ErrApprovalRequired) {
				guardStatus = TraceStatusBlocked
			}
			r.tracer.Event(ctx, TraceEvent{
				Kind:      TraceKindStep,
				Step:      step,
				Tool:      tc.Name,
				Status:    guardStatus,
				ErrorCode: traceErrorCode(guardErr),
			})
			if obs != nil {
				obs.OnToolStart(step, tc.Name, string(tc.ArgsJSON))
				obs.OnToolEnd(step, tc.Name, "", guardErr)
			}
			if errors.Is(guardErr, ErrApprovalRequired) {
				t.PendingCall = newPendingCall(tc)
				if skipped := calls[i+1:]; len(skipped) > 0 {
					s.History = append(s.History, Message{
						Role:    "tool",
						Content: wrapUntrustedFence(skippedCallsNotice(skipped)),
					})
				}
				return guardErr
			}
			s.History = append(s.History, Message{
				Role:    "tool",
				Content: st.OutputFenced,
			})
			continue
		}

		r.executeCall(ctx, s, t, step, tc, obs)
	}

	if len(invalid) > 0 {
		s.History = append(s.History, Message{
			Role:    "tool",
			Content: wrapUntrustedFence(invalidToolCallNotice(invalid)),
		})
	}
	return nil
}

func traceStatusForError(err error) string {
	if err == nil {
		return TraceStatusOK
	}
	return TraceStatusError
}

func traceErrorCode(err error) string {
	if err == nil {
		return ""
	}
	var ae *plugin.AgentError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return ""
}

func toProviderMessages(messages []Message) []llmprovider.Message {
	msgs := make([]llmprovider.Message, len(messages))
	for i, m := range messages {
		msgs[i] = llmprovider.Message{Role: m.Role, Content: m.Content}
	}
	return msgs
}

type CompleterAdapter struct {
	Provider   llmprovider.Provider
	LastResult CompletionResult
}

func (a *CompleterAdapter) CompleteResult(ctx context.Context, model string, system string, messages []Message) (CompletionResult, error) {
	msgs := toProviderMessages(messages)
	resp, err := a.Provider.Complete(ctx, llmprovider.Request{
		Model:    model,
		System:   system,
		Messages: msgs,
	})
	if err != nil {
		return CompletionResult{}, err
	}
	res := CompletionResult{
		Text:             resp.Text,
		InputTokens:      resp.InputTokens,
		OutputTokens:     resp.OutputTokens,
		ReasoningTokens:  resp.ReasoningTokens,
		CacheReadTokens:  resp.CacheReadTokens,
		CacheWriteTokens: resp.CacheCreationTokens,
	}
	a.LastResult = res
	return res, nil
}

func (a *CompleterAdapter) Complete(ctx context.Context, model string, system string, messages []Message) (string, error) {
	res, err := a.CompleteResult(ctx, model, system, messages)
	if err != nil {
		return "", err
	}
	return res.Text, nil
}

func (a *CompleterAdapter) StreamComplete(ctx context.Context, req llmprovider.Request, h llmprovider.StreamHandler) (llmprovider.Response, error) {
	if sp, ok := a.Provider.(llmprovider.StreamProvider); ok {
		return sp.StreamComplete(ctx, req, h)
	}
	return llmprovider.Response{}, fmt.Errorf("llmprovider: underlying provider %T does not support streaming", a.Provider)
}
