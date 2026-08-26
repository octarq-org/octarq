package harness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/octarq-org/octarq/llmprovider"
	"github.com/octarq-org/octarq/plugin"
)

// ErrMaxStepsExceeded is returned when the agent loop reaches the step ceiling.
var ErrMaxStepsExceeded = errors.New("harness: max steps exceeded")

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

// Runner is the agent loop orchestrator.
//
// P3 前 API 易变.
type Runner interface {
	Run(ctx context.Context, s *Session, t *Turn) error
}

// toolCall is an internal representation of a tool invocation parsed from
// LLM output.
type toolCall struct {
	Name     string `json:"name"`
	ArgsJSON string `json:"args"`
}

// parseToolCalls extracts tool-call intents from LLM output.
// Convention: the model emits a JSON block fenced with ```tool_call ... ```
// containing {"name":"...","args":"..."} objects separated by newlines.
func parseToolCalls(text string) ([]toolCall, string) {
	const startMarker = "```tool_call\n"
	const endMarker = "\n```"

	idx := strings.Index(text, startMarker)
	if idx == -1 {
		return nil, text
	}
	after := text[idx+len(startMarker):]
	endIdx := strings.Index(after, endMarker)
	if endIdx == -1 {
		return nil, text
	}
	block := after[:endIdx]

	var calls []toolCall
	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var tc toolCall
		if err := json.Unmarshal([]byte(line), &tc); err != nil {
			continue
		}
		if tc.Name != "" {
			calls = append(calls, tc)
		}
	}

	// remaining is the text outside the tool_call block
	remaining := strings.TrimSpace(text[:idx] + after[endIdx+len(endMarker):])
	return calls, remaining
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

// NewRunner constructs a Runner with the given dependencies and options.
func NewRunner(c Completer, exec ToolExecutor, opts ...RunnerOption) Runner {
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
	return r
}

// Run executes the agent loop: call LLM → parse tool calls → guard check →
// execute tool → fence output → append history → repeat until no tool calls
// or step limit reached.
func (r *defaultRunner) Run(ctx context.Context, s *Session, t *Turn) error {
	maxSteps := r.profile.MaxSteps(false) // TODO: pass hasDestructive from caller
	t.Status = TurnStatusRunning

	// Seed history with the user's input.
	s.History = append(s.History, Message{Role: "user", Content: t.Input})

	for step := 0; ; step++ {
		if step >= maxSteps {
			t.Status = TurnStatusFailed
			return ErrMaxStepsExceeded
		}

		// Call LLM.
		text, err := r.completer.Complete(ctx, r.model, r.system, s.History)
		if err != nil {
			t.Status = TurnStatusFailed
			return fmt.Errorf("harness: completer error: %w", err)
		}

		// Parse tool calls from LLM output.
		calls, remaining := parseToolCalls(text)
		if len(calls) == 0 {
			// No tool calls — the turn is done.
			s.History = append(s.History, Message{Role: "assistant", Content: text})
			t.Status = TurnStatusDone
			return nil
		}

		// Execute each tool call.
		for _, tc := range calls {
			stepID := fmt.Sprintf("%s-step-%d-%s", s.ID, step, tc.Name)
			r.tracer.StepStart(ctx, stepID)

			st := Step{
				ToolName: tc.Name,
				ArgsJSON: tc.ArgsJSON,
			}

			// Guard check.
			if guardErr := r.guard.Allow(ctx, s.OrgID, tc.Name); guardErr != nil {
				st.Err = guardErr
				st.OutputFenced = wrapUntrustedFence(guardErr.Error())
				t.Steps = append(t.Steps, st)
				r.tracer.StepEnd(ctx, stepID, guardErr)
				// Append the guard rejection into history so the LLM knows.
				s.History = append(s.History, Message{
					Role:    "assistant",
					Content: remaining,
				})
				s.History = append(s.History, Message{
					Role:    "tool",
					Content: st.OutputFenced,
				})
				continue
			}

			// Execute tool.
			start := time.Now()
			output, execErr := r.executor.Execute(ctx, s.OrgID, tc.Name, tc.ArgsJSON)
			st.DurationMS = time.Since(start).Milliseconds()

			if execErr != nil {
				// Preserve AgentError.AgentGuidance through the Step.
				var ae *plugin.AgentError
				if errors.As(execErr, &ae) {
					st.Err = ae // keep the full AgentError
				} else {
					st.Err = execErr
				}
				// Use the error message as fenced output so the LLM sees it.
				st.OutputFenced = wrapUntrustedFence(execErr.Error())
			} else {
				st.OutputFenced = wrapUntrustedFence(output)
			}

			t.Steps = append(t.Steps, st)
			r.tracer.StepEnd(ctx, stepID, st.Err)

			// Append the assistant's tool-call intent and tool output into
			// history so the LLM can reason about results.
			s.History = append(s.History, Message{
				Role:    "assistant",
				Content: remaining,
			})
			s.History = append(s.History, Message{
				Role:    "tool",
				Content: st.OutputFenced,
			})
		}
	}
}

// CompleterAdapter adapts llmprovider.Provider to the narrow Completer
// interface. It does NOT modify the llmprovider package.
type CompleterAdapter struct {
	Provider llmprovider.Provider
}

// Complete calls the real llmprovider.Provider.Complete with a properly
// constructed Request.
func (a *CompleterAdapter) Complete(ctx context.Context, model string, system string, messages []Message) (string, error) {
	msgs := make([]llmprovider.Message, len(messages))
	for i, m := range messages {
		msgs[i] = llmprovider.Message{Role: m.Role, Content: m.Content}
	}
	resp, err := a.Provider.Complete(ctx, llmprovider.Request{
		Model:    model,
		System:   system,
		Messages: msgs,
	})
	if err != nil {
		return "", err
	}
	return resp.Text, nil
}
