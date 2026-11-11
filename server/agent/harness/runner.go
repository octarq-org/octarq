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

// ErrMaxStepsExceeded is returned when the agent loop reaches the step ceiling.
var ErrMaxStepsExceeded = errors.New("harness: max steps exceeded")

// ErrMaxStepsExceedsProfile is returned when a caller asks for more steps than
// the selected profile allows. The request is refused, not lowered.
var ErrMaxStepsExceedsProfile = errors.New("harness: max steps exceeds profile ceiling")

// ToolExecutor abstracts tool dispatch, decoupled from the registry so wiring
// can happen later.
//
// P3 前 API 易变.
type ToolExecutor interface {
	Execute(ctx context.Context, orgID uint, name string, argsJSON string) (output string, err error)
}

// Completer is the narrow LLM interface the runner depends on. It adapts to
// the real llmprovider.Provider.Complete(ctx, Request) (Response, error)
// signature via CompleterAdapter.
//
// P3 前 API 易变.
type Completer interface {
	Complete(ctx context.Context, model string, system string, messages []Message) (string, error)
}

// CompletionResult carries the LLM text response along with token accounting
// returned by the underlying provider.
type CompletionResult struct {
	Text            string
	InputTokens     int
	OutputTokens    int
	ReasoningTokens int
}

// ResultCompleter is an optional extension of Completer that propagates token
// usage from the provider response into the active Turn.
type ResultCompleter interface {
	CompleteResult(ctx context.Context, model string, system string, messages []Message) (CompletionResult, error)
}

// Runner is the agent loop orchestrator.
//
// P3 前 API 易变.
type Runner interface {
	Run(ctx context.Context, s *Session, t *Turn) error
	// Resume runs the tool call an operator approved and continues the turn.
	//
	// The approved call is taken from t.PendingCall, so the call that runs is
	// byte for byte the call the operator saw — not whatever the model writes
	// when it is asked again. The turn's step budget continues where it
	// stopped; an approval does not buy extra rounds.
	//
	// A turn with no pending call is refused with ErrNoPendingCall rather than
	// started from the beginning.
	Resume(ctx context.Context, s *Session, t *Turn) error
	// MaxStepsFor returns the step ceiling the loop enforces for a turn. It is
	// the same value the loop compares against.
	MaxStepsFor(hasDestructive bool) int
}

// StreamRunner is an optional interface implemented by runners that support
// streaming LLM outputs to a StreamHandler.
//
// P3 前 API 易变.
type StreamRunner interface {
	Runner
	Stream(ctx context.Context, s *Session, t *Turn, h llmprovider.StreamHandler) error
}

// toolCall is an internal representation of a tool invocation parsed from
// LLM output. ArgsJSON is the raw JSON object the model supplied.
type toolCall struct {
	Name     string          `json:"name"`
	ArgsJSON json.RawMessage `json:"args"`
}

// parseToolCalls extracts tool-call intents from LLM output.
//
// Convention: the model emits a JSON block fenced with ```tool_call ... ```
// containing one {"name":"...","args":{...}} object per line.
//
// A line that does not parse is returned in `invalid` instead of being
// dropped: the model is told what it got wrong and can correct itself. A
// silently dropped call would leave the model believing it had acted.
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

	// remaining is the text outside the tool_call block
	remaining := strings.TrimSpace(text[:idx] + after[endIdx+len(endMarker):])
	return calls, remaining, invalid
}

// normalizeToolArgs turns the "args" member into raw JSON object bytes.
//
// The canonical shape is an object. A JSON-encoded string holds the same
// value, so it is unwrapped rather than rejected — a model that quotes its
// arguments has still made a well-formed call. Anything else is not a call.
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

// wrapUntrustedFence wraps tool output in an untrusted fence to prevent
// prompt injection — this is the core defence line (design §3.1, §6 陷阱).
func wrapUntrustedFence(output string) string {
	return "```untrusted\n" + output + "\n```"
}

// defaultRunner is the concrete Runner implementation.
type defaultRunner struct {
	completer Completer
	executor  ToolExecutor
	guard     Guard
	tracer    Tracer
	profile   Profile
	model     string
	system    string
	// maxSteps is an explicit step budget. Zero means the profile decides. The
	// constructor refuses a negative value and a value above the profile's
	// maximum ceiling, so the loop never lowers a caller's request.
	maxSteps int
	// budget bounds what the turn may spend across rounds. The zero Budget
	// leaves it unbounded.
	budget Budget
}

// RunnerOption configures a defaultRunner.
type RunnerOption func(*defaultRunner)

// WithGuard sets the Guard. Defaults to NopGuard.
func WithGuard(g Guard) RunnerOption {
	return func(r *defaultRunner) { r.guard = g }
}

// WithTracer sets the Tracer. Defaults to NopTracer.
func WithTracer(t Tracer) RunnerOption {
	return func(r *defaultRunner) { r.tracer = t }
}

// WithProfile sets the execution profile for step limits. Defaults to
// ProfileNormal.
func WithProfile(p Profile) RunnerOption {
	return func(r *defaultRunner) { r.profile = p }
}

// WithModel sets the LLM model identifier.
func WithModel(m string) RunnerOption {
	return func(r *defaultRunner) { r.model = m }
}

// WithSystem sets the system prompt.
func WithSystem(s string) RunnerOption {
	return func(r *defaultRunner) { r.system = s }
}

// WithMaxSteps sets the step budget for a turn. Zero means the profile
// decides. A negative value, or a value above the profile's maximum ceiling,
// makes NewRunner return ErrMaxStepsExceedsProfile: the budget is refused, not
// silently lowered.
func WithMaxSteps(n int) RunnerOption {
	return func(r *defaultRunner) { r.maxSteps = n }
}

// StepBudget resolves the step ceiling for a profile.
//
// Zero means the profile decides, so the profile's ceiling is returned. A
// positive value is returned unchanged or refused with
// ErrMaxStepsExceedsProfile when it exceeds the ceiling. The helper never
// lowers a request silently: a caller either gets the number it asked for or
// an error.
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

// NewRunner constructs a Runner with the given dependencies and options.
//
// It validates the step budget after every option is applied. A negative
// budget, or one above the selected profile's maximum ceiling, is refused with
// an error.
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

// Run executes the agent loop: call LLM → parse tool calls → guard check →
// execute tool → fence output → append history → repeat until no tool calls
// or step limit reached.
func (r *defaultRunner) Run(ctx context.Context, s *Session, t *Turn) error {
	r.tracer.Event(ctx, TraceEvent{Kind: TraceKindTurnStart})
	err := r.loop(ctx, s, t)
	r.tracer.Event(ctx, TraceEvent{
		Kind:            TraceKindTurnEnd,
		Status:          t.Status.String(),
		InputTokens:     t.InputTokens,
		OutputTokens:    t.OutputTokens,
		ReasoningTokens: t.ReasoningTokens,
	})
	return err
}

// loop is the agent loop proper. It always leaves t.Status set to the
// terminal state of the turn.
func (r *defaultRunner) loop(ctx context.Context, s *Session, t *Turn) error {
	// A new turn carries no approval from an earlier one and starts its round
	// count at zero. Resume continues both, which is why this reset lives here
	// and not in runRounds.
	t.PendingCall = nil
	t.Rounds = 0
	t.Elapsed = 0

	// Seed history with the user's input.
	s.History = append(s.History, Message{Role: "user", Content: t.Input})

	return r.runRounds(ctx, s, t, time.Now())
}

// runRounds is the round loop. It assumes the history is seeded and runs until
// the turn ends, so a resumed turn can re-enter it with the history the
// approved call belongs to.
func (r *defaultRunner) runRounds(ctx context.Context, s *Session, t *Turn, started time.Time) (err error) {
	maxSteps := r.MaxStepsFor(t.HasDestructive)
	t.Status = TurnStatusRunning

	// The wall clock this call spends is the turn's, however the call ends. A
	// host that stores a turn reads it from Elapsed, so a turn that came back
	// in another process does not restart its budget.
	defer func() { t.Elapsed += time.Since(started) }()

	for step := t.Rounds; ; step++ {
		t.Rounds = step
		if step >= maxSteps {
			t.Status = TurnStatusFailed
			return ErrMaxStepsExceeded
		}

		// Budget is checked before the round, not during it. See Budget.
		if err := r.checkBudget(t, started); err != nil {
			t.Status = TurnStatusFailed
			return err
		}

		// Call LLM.
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
		} else {
			var err error
			text, err = r.completer.Complete(ctx, r.model, r.system, s.History)
			if err != nil {
				t.Status = TurnStatusFailed
				return fmt.Errorf("harness: completer error: %w", err)
			}
		}

		// Parse tool calls from LLM output.
		calls, remaining, invalid := parseToolCalls(text)
		if len(calls) == 0 {
			if len(invalid) > 0 {
				// The model tried to call a tool and got the format wrong.
				// Tell it so instead of ending the turn on a call it
				// believes it made.
				reportInvalidCalls(s, text, invalid)
				continue
			}
			// No tool calls — the turn is done.
			s.History = append(s.History, Message{Role: "assistant", Content: text})
			t.Status = TurnStatusDone
			return nil
		}

		// Execute each tool call.
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

// MaxStepsFor returns the step ceiling the loop enforces for a turn.
//
// An explicit budget below the profile's ceiling wins. Otherwise the profile's
// ceiling applies. The constructor already refused an explicit budget above
// the profile's maximum ceiling; the only remaining reduction is the
// ProfileReactorReadOnly destructive lock, which is a profile-owned safety
// invariant and cannot be raised.
func (r *defaultRunner) MaxStepsFor(hasDestructive bool) int {
	ceiling := r.profile.MaxSteps(hasDestructive)
	if r.maxSteps > 0 && r.maxSteps < ceiling {
		return r.maxSteps
	}
	return ceiling
}

// invalidToolCallNotice explains to the model why its call did not run.
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

// reportInvalidCalls hands the calls the model could not express back to it.
// A model that believes it called a tool stops asking for the result. The
// turn therefore continues with the rejected lines, and the model can correct
// itself. The same rule holds for the streaming path: a call that did not
// parse is reported there too.
func reportInvalidCalls(s *Session, text string, invalid []string) {
	s.History = append(s.History, Message{Role: "assistant", Content: text})
	s.History = append(s.History, Message{
		Role:    "tool",
		Content: wrapUntrustedFence(invalidToolCallNotice(invalid)),
	})
}

// skippedCallsNotice names the calls of a batch that did not run.
//
// The turn stops at the first call that needs operator approval, so every call
// after it is dropped. A model that is not told repeats those calls: it
// believes their results are still coming.
func skippedCallsNotice(skipped []toolCall) string {
	var b strings.Builder
	b.WriteString("These tool calls did not run: the turn stopped for operator approval.")
	for _, tc := range skipped {
		b.WriteString("\n- ")
		b.WriteString(tc.Name)
	}
	return b.String()
}

// StepObserver watches the tool calls the loop actually runs.
//
// A stream handler implements it to project each guarded call onto the wire.
// The runner reports a call only after the guard accepts it and before the
// tool executes, so a client never sees a call that cannot happen.
type StepObserver interface {
	OnToolStart(step int, tool, argsJSON string)
	OnToolEnd(step int, tool, output string, err error)
}

// executeSteps executes parsed tool calls with guard checks and tracing,
// appending results to history.
//
// A call the model wrote wrongly is reported after the calls that ran, so the
// model learns about both in one turn.
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

		// Guard check.
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
				// Record the exact call the operator is being asked about.
				// Without it the approval would apply to whatever the model
				// writes the next time it is asked, which is a different call.
				t.PendingCall = newPendingCall(tc)
				// The turn stops here, so the calls after this one never run.
				// A model that does not hear about them believes it made calls
				// whose results are still coming, and repeats them.
				if skipped := calls[i+1:]; len(skipped) > 0 {
					s.History = append(s.History, Message{
						Role:    "tool",
						Content: wrapUntrustedFence(skippedCallsNotice(skipped)),
					})
				}
				return guardErr
			}
			// Append the guard rejection into history so the LLM knows.
			s.History = append(s.History, Message{
				Role:    "tool",
				Content: st.OutputFenced,
			})
			continue
		}

		// Execute tool.
		r.executeCall(ctx, s, t, step, tc, obs)
	}

	// Tell the model about the calls that did not run. A rejected call is
	// never dropped: the model must not believe it acted.
	if len(invalid) > 0 {
		s.History = append(s.History, Message{
			Role:    "tool",
			Content: wrapUntrustedFence(invalidToolCallNotice(invalid)),
		})
	}
	return nil
}

// traceStatusForError classifies a tool outcome: a tool that ran and failed
// is an error; a tool that was never allowed to run is reported by the guard
// branch instead.
func traceStatusForError(err error) string {
	if err == nil {
		return TraceStatusOK
	}
	return TraceStatusError
}

// traceErrorCode extracts the stable business error code of a failed step.
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

// toProviderMessages converts harness Messages to llmprovider Messages.
func toProviderMessages(messages []Message) []llmprovider.Message {
	msgs := make([]llmprovider.Message, len(messages))
	for i, m := range messages {
		msgs[i] = llmprovider.Message{Role: m.Role, Content: m.Content}
	}
	return msgs
}

// CompleterAdapter adapts llmprovider.Provider to the narrow Completer
// interface. It does NOT modify the llmprovider package.
type CompleterAdapter struct {
	Provider   llmprovider.Provider
	LastResult CompletionResult
}

// CompleteResult calls the real llmprovider.Provider.Complete and propagates
// InputTokens, OutputTokens, and ReasoningTokens into CompletionResult.
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
		Text:            resp.Text,
		InputTokens:     resp.InputTokens,
		OutputTokens:    resp.OutputTokens,
		ReasoningTokens: resp.ReasoningTokens,
	}
	a.LastResult = res
	return res, nil
}

// Complete calls CompleteResult and returns the response text.
func (a *CompleterAdapter) Complete(ctx context.Context, model string, system string, messages []Message) (string, error) {
	res, err := a.CompleteResult(ctx, model, system, messages)
	if err != nil {
		return "", err
	}
	return res.Text, nil
}

// StreamComplete implements llmprovider.StreamProvider by delegating to the underlying
// Provider if it supports streaming.
func (a *CompleterAdapter) StreamComplete(ctx context.Context, req llmprovider.Request, h llmprovider.StreamHandler) (llmprovider.Response, error) {
	if sp, ok := a.Provider.(llmprovider.StreamProvider); ok {
		return sp.StreamComplete(ctx, req, h)
	}
	return llmprovider.Response{}, fmt.Errorf("llmprovider: underlying provider %T does not support streaming", a.Provider)
}
