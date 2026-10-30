// Package agui projects the agent harness onto the AG-UI wire protocol.
//
// The protocol is the only chat transport: the dashboard, the command palette
// and any future client read the same event stream. The package owns the
// translation and nothing else — authentication, billing and HTTP live in the
// handler that feeds it a Sink.
package agui

import (
	"fmt"
	"strings"

	agui "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/octarq-org/octarq/server/llmprovider"
)

// maxToolResultBytes caps the tool output a client receives. A tool that reads
// a large list must not push its whole page into one SSE frame.
const maxToolResultBytes = 16 << 10

// Sink receives one event at a time, in order. An error stops the projection:
// the client is gone, so the run has nobody left to answer.
type Sink func(ev agui.Event) error

// Stream turns harness callbacks into AG-UI events.
//
// It implements llmprovider.StreamHandler for the model deltas and
// harness.StepObserver for the tool calls the loop actually plans and guards.
// Tool-call chunks from the model are deliberately ignored: a call only
// becomes a protocol event after the guard accepts it.
type Stream struct {
	sink     Sink
	threadID string
	runID    string

	textID       string
	lastTextID   string
	wroteText    bool
	thinkingOpen bool
	seq          int
	lastCallID   string

	err error
}

// New builds a stream that writes events to sink.
func New(sink Sink, threadID, runID string) *Stream {
	return &Stream{sink: sink, threadID: threadID, runID: runID}
}

// Err reports the first sink failure, if any.
func (s *Stream) Err() error { return s.err }

func (s *Stream) emit(ev agui.Event) {
	if s.err != nil {
		return
	}
	if err := s.sink(ev); err != nil {
		s.err = err
	}
}

func (s *Stream) nextID(prefix string) string {
	s.seq++
	return fmt.Sprintf("%s-%s-%d", prefix, shortID(s.runID), s.seq)
}

func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

// OnThinking streams reasoning deltas as one thinking block per step.
func (s *Stream) OnThinking(delta string) {
	if delta == "" {
		return
	}
	if !s.thinkingOpen {
		s.emit(agui.NewThinkingStartEvent())
		s.emit(agui.NewThinkingTextMessageStartEvent())
		s.thinkingOpen = true
	}
	s.emit(agui.NewThinkingTextMessageContentEvent(delta))
}

// OnText streams the answer text as one message per step.
func (s *Stream) OnText(delta string) {
	if delta == "" {
		return
	}
	s.closeThinking()
	if s.textID == "" {
		s.textID = s.nextID("msg")
		s.emit(agui.NewTextMessageStartEvent(s.textID))
	}
	s.wroteText = true
	s.emit(agui.NewTextMessageContentEvent(s.textID, delta))
}

// WroteText reports whether any answer text reached the client. A provider
// without streaming support produces none, and the handler sends the whole
// reply in one message instead.
func (s *Stream) WroteText() bool { return s.wroteText }

// Say emits a complete assistant message in one piece.
func (s *Stream) Say(text string) {
	if text == "" {
		return
	}
	s.closeThinking()
	if s.textID == "" {
		s.textID = s.nextID("msg")
		s.emit(agui.NewTextMessageStartEvent(s.textID))
	}
	s.wroteText = true
	s.emit(agui.NewTextMessageContentEvent(s.textID, text))
	s.closeText()
}

// OnToolCall ignores provider chunks. The runner reports the calls that pass
// the guard through OnToolStart and OnToolEnd, which is the only place a call
// is known to be real.
func (s *Stream) OnToolCall(llmprovider.ToolCallChunk) {}

// OnToolStart announces a guarded tool call before it runs.
func (s *Stream) OnToolStart(_ int, tool, argsJSON string) {
	s.closeThinking()
	s.closeText()
	s.lastCallID = s.nextID("call")
	s.emit(agui.NewToolCallStartEvent(s.lastCallID, tool))
	if argsJSON != "" {
		s.emit(agui.NewToolCallArgsEvent(s.lastCallID, argsJSON))
	}
	s.emit(agui.NewToolCallEndEvent(s.lastCallID))
}

// OnToolEnd reports the result of the call announced by OnToolStart.
func (s *Stream) OnToolEnd(_ int, tool, output string, err error) {
	if s.lastCallID == "" {
		return
	}
	content := output
	if err != nil {
		content = err.Error()
	}
	content = truncate(content, maxToolResultBytes)
	s.emit(agui.NewToolCallResultEvent(s.resultMessageID(), s.lastCallID, content))
	s.lastCallID = ""
}

// Finish closes the open blocks and ends the run with its token usage.
func (s *Stream) Finish(provider, model string, inputTokens, outputTokens, reasoningTokens int) {
	s.closeThinking()
	s.closeText()
	in, out, reasoning := int64(inputTokens), int64(outputTokens), int64(reasoningTokens)
	total := in + out
	s.emit(agui.NewRunFinishedEventWithOptions(s.threadID, s.runID,
		agui.WithUsage([]agui.TokenUsage{{
			Provider:        provider,
			Model:           model,
			InputTokens:     &in,
			OutputTokens:    &out,
			ReasoningTokens: &reasoning,
			TotalTokens:     &total,
		}}),
	))
}

// Fail ends the run with a protocol error. code is the stable business code
// the client translates; message is the human sentence it shows when it has
// no translation.
func (s *Stream) Fail(code, message string) {
	s.closeThinking()
	s.closeText()
	opts := []agui.RunErrorOption{agui.WithRunID(s.runID)}
	if code != "" {
		opts = append(opts, agui.WithErrorCode(code))
	}
	s.emit(agui.NewRunErrorEvent(message, opts...))
}

func (s *Stream) closeThinking() {
	if !s.thinkingOpen {
		return
	}
	s.emit(agui.NewThinkingTextMessageEndEvent())
	s.emit(agui.NewThinkingEndEvent())
	s.thinkingOpen = false
}

func (s *Stream) closeText() {
	if s.textID == "" {
		return
	}
	s.emit(agui.NewTextMessageEndEvent(s.textID))
	s.lastTextID = s.textID
	s.textID = ""
}

// resultMessageID names the assistant message a tool result belongs to. A
// call made before any text belongs to the message the run will produce, so a
// stable id is allocated and reused.
func (s *Stream) resultMessageID() string {
	if s.textID != "" {
		return s.textID
	}
	if s.lastTextID != "" {
		return s.lastTextID
	}
	if s.lastTextID = s.nextID("msg"); s.lastTextID != "" {
		return s.lastTextID
	}
	return ""
}

func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	// Cut on a rune boundary so the client never receives invalid UTF-8.
	cut := strings.ToValidUTF8(s[:limit], "")
	return cut + "\n[truncated]"
}
