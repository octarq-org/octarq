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

var ErrNoPendingCall = errors.New("harness: the turn has no pending call to resume")

func NewPendingCall(tool, argsJSON string) *PendingCall {
	args := canonicalArgs(json.RawMessage(argsJSON))
	return &PendingCall{
		Tool:     tool,
		ArgsJSON: args,
		Hash:     callHash(tool, args),
	}
}

func newPendingCall(tc toolCall) *PendingCall {
	return NewPendingCall(tc.Name, string(tc.ArgsJSON))
}

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

func callHash(tool, argsJSON string) string {
	sum := sha256.Sum256([]byte(tool + "\x00" + argsJSON))
	return hex.EncodeToString(sum[:])
}

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

type ResumeStreamRunner interface {
	StreamRunner
	ResumeStream(ctx context.Context, s *Session, t *Turn, h llmprovider.StreamHandler) error
}

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

func streamObserver(h llmprovider.StreamHandler) StepObserver {
	if o, ok := h.(StepObserver); ok {
		return o
	}
	return nil
}

func (r *defaultRunner) resumeApproved(ctx context.Context, s *Session, t *Turn, obs StepObserver) error {
	if t.PendingCall == nil {
		t.Status = TurnStatusFailed
		return ErrNoPendingCall
	}
	call := *t.PendingCall
	t.PendingCall = nil

	step := t.Rounds
	start := time.Now()
	r.executeCall(ctx, s, t, step, toolCall{
		Name:     call.Tool,
		ArgsJSON: json.RawMessage(call.ArgsJSON),
	}, obs)
	t.Rounds = step + 1
	t.Elapsed += time.Since(start)
	return nil
}

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
		var ae *plugin.AgentError
		if errors.As(execErr, &ae) {
			st.Err = ae
		} else {
			st.Err = execErr
		}
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

	s.History = append(s.History, Message{
		Role:    "tool",
		Content: st.OutputFenced,
	})
}
