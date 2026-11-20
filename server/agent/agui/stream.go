package agui

import (
	"fmt"
	"strings"

	agui "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/octarq-org/octarq/server/llmprovider"
)

const maxToolResultBytes = 16 << 10

type Sink func(ev agui.Event) error

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

func New(sink Sink, threadID, runID string) *Stream {
	return &Stream{sink: sink, threadID: threadID, runID: runID}
}

func NewStream(runID string, sink Sink) *Stream {
	return New(sink, runID, runID)
}

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

func (s *Stream) WroteText() bool { return s.wroteText }

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

func (s *Stream) OnToolCall(llmprovider.ToolCallChunk) {}

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
	cut := strings.ToValidUTF8(s[:limit], "")
	return cut + "\n[truncated]"
}
