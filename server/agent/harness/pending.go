package harness

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/octarq-org/octarq/server/llmprovider"
	"github.com/octarq-org/octarq/server/plugin"
)

// ErrNoPendingCall is returned by Resume when the turn has no approved call to
// run. Resume is not a way to start a turn; Run is.
var ErrNoPendingCall = errors.New("harness: the turn has no pending call to resume")

// newPendingCall records a call the guard refused for approval.
//
// The arguments are canonicalised first. The guard saw the call the model
// wrote, and the operator is shown these bytes; making them canonical means two
// renderings of one call produce one hash, so an approval cannot be sidestepped
// by writing the same object with different spacing or key order.
func newPendingCall(tc toolCall) *PendingCall {
	args := canonicalArgs(tc.ArgsJSON)
	return &PendingCall{
		Tool:     tc.Name,
		ArgsJSON: args,
		Hash:     callHash(tc.Name, args),
	}
}

// canonicalArgs renders argument bytes in one stable form.
//
// A document that does not parse is returned trimmed. It never reaches an
// approval: the parser already refused it, so this is a guard against a caller
// that built a toolCall by hand.
func canonicalArgs(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return "{}"
	}
	var v any
	if err := json.Unmarshal([]byte(trimmed), &v); err != nil {
		return trimmed
	}
	out, err := json.Marshal(v)
	if err != nil {
		return trimmed
	}
	return string(out)
}

// callHash identifies a tool call by name and canonical arguments. The separator
// is not a character that can appear in a tool name, so no pair of calls can
// collide by moving text across the boundary.
func callHash(tool, argsJSON string) string {
	sum := sha256.Sum256([]byte(tool + "\x00" + argsJSON))
	return hex.EncodeToString(sum[:])
}

// Resume runs the tool call an operator approved and continues the turn.
func (r *defaultRunner) Resume(ctx context.Context, s *Session, t *Turn) error {
	r.tracer.Event(ctx, TraceEvent{Kind: TraceKindTurnStart})
	err := r.resumeApproved(ctx, s, t, nil)
	if err == nil {
		err = r.runRounds(ctx, s, t, time.Now())
	}
	r.tracer.Event(ctx, TraceEvent{
		Kind:            TraceKindTurnEnd,
		Status:          t.Status.String(),
		InputTokens:     t.InputTokens,
		OutputTokens:    t.OutputTokens,
		ReasoningTokens: t.ReasoningTokens,
	})
	return err
}

// ResumeStream is Resume for a runner that streams its continuation.
//
// P3 前 API 易变.
type ResumeStreamRunner interface {
	StreamRunner
	ResumeStream(ctx context.Context, s *Session, t *Turn, h llmprovider.StreamHandler) error
}

// ResumeStream runs the approved call and streams the rest of the turn. A
// provider that cannot stream continues through Resume, exactly as Stream
// continues through Run.
func (r *defaultRunner) ResumeStream(ctx context.Context, s *Session, t *Turn, h llmprovider.StreamHandler) error {
	sp, ok := r.extractStreamProvider()
	if !ok {
		return r.Resume(ctx, s, t)
	}

	r.tracer.Event(ctx, TraceEvent{Kind: TraceKindTurnStart})
	err := r.resumeApproved(ctx, s, t, streamObserver(h))
	if err == nil {
		err = r.streamRounds(ctx, s, t, sp, h, time.Now())
	}
	r.tracer.Event(ctx, TraceEvent{
		Kind:            TraceKindTurnEnd,
		Status:          t.Status.String(),
		InputTokens:     t.InputTokens,
		OutputTokens:    t.OutputTokens,
		ReasoningTokens: t.ReasoningTokens,
	})
	return err
}

// streamObserver returns the handler's step projection when it implements one.
func streamObserver(h llmprovider.StreamHandler) StepObserver {
	if o, ok := h.(StepObserver); ok {
		return o
	}
	return nil
}

// resumeApproved executes the approved call and appends its result to the
// history, leaving the turn ready for the next round.
//
// The guard does not run again. The operator's decision is the answer to the
// question the guard asked, and asking again would only produce the same
// refusal.
func (r *defaultRunner) resumeApproved(ctx context.Context, s *Session, t *Turn, obs StepObserver) error {
	if t.PendingCall == nil {
		t.Status = TurnStatusFailed
		return ErrNoPendingCall
	}
	call := *t.PendingCall
	t.PendingCall = nil

	// The approved call belongs to the round that produced it. Counting it here
	// is what stops an approval from buying an extra round: the continuation
	// starts at the round after this one.
	step := t.rounds
	r.executeCall(ctx, s, t, step, toolCall{
		Name:     call.Tool,
		ArgsJSON: json.RawMessage(call.ArgsJSON),
	}, obs)
	t.rounds = step + 1
	return nil
}

// executeCall runs one accepted call, records the Step, traces it and appends
// its fenced output to the history.
//
// It is the second half of executeSteps, split out so Resume can run an
// approved call through the same path. A resumed call that took a different
// route would fence or trace differently from a call that was never blocked.
func (r *defaultRunner) executeCall(ctx context.Context, s *Session, t *Turn, step int, tc toolCall, obs StepObserver) {
	st := Step{
		ToolName: tc.Name,
		ArgsJSON: string(tc.ArgsJSON),
	}

	if obs != nil {
		obs.OnToolStart(step, tc.Name, string(tc.ArgsJSON))
	}
	start := time.Now()
	output, execErr := r.executor.Execute(ctx, s.OrgID, tc.Name, string(tc.ArgsJSON))
	st.DurationMS = time.Since(start).Milliseconds()
	if obs != nil {
		obs.OnToolEnd(step, tc.Name, output, execErr)
	}

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
	r.tracer.Event(ctx, TraceEvent{
		Kind:       TraceKindStep,
		Step:       step,
		Tool:       tc.Name,
		Status:     traceStatusForError(st.Err),
		ErrorCode:  traceErrorCode(st.Err),
		DurationMS: st.DurationMS,
	})

	// Append the tool output into history so the LLM can reason about results.
	s.History = append(s.History, Message{
		Role:    "tool",
		Content: st.OutputFenced,
	})
}
