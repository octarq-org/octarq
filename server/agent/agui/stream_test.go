package agui

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	agui "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
)

// recorded collects the events a stream emits.
type recorded struct {
	events []agui.Event
	types  []string
}

func (r *recorded) sink(ev agui.Event) error {
	r.events = append(r.events, ev)
	r.types = append(r.types, string(ev.Type()))
	return nil
}

func newRecorded() (*Stream, *recorded) {
	rec := &recorded{}
	return New(rec.sink, "thread-1", "run-abcdefgh"), rec
}

func jsonOf(t *testing.T, ev agui.Event) map[string]any {
	t.Helper()
	raw, err := ev.ToJSON()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

func TestStream_EmitsThinkingTextToolAndFinishInOrder(t *testing.T) {
	s, rec := newRecorded()

	s.OnThinking("thinking ")
	s.OnText("hello ")
	s.OnText("world")
	s.OnToolStart(0, "list_links", `{"limit":1}`)
	s.OnToolEnd(0, "list_links", "[]", nil)
	s.Finish("fake", "model-x", 10, 5, 2)

	want := []string{
		"THINKING_START",
		"THINKING_TEXT_MESSAGE_START",
		"THINKING_TEXT_MESSAGE_CONTENT",
		"THINKING_TEXT_MESSAGE_END",
		"THINKING_END",
		"TEXT_MESSAGE_START",
		"TEXT_MESSAGE_CONTENT",
		"TEXT_MESSAGE_CONTENT",
		"TEXT_MESSAGE_END",
		"TOOL_CALL_START",
		"TOOL_CALL_ARGS",
		"TOOL_CALL_END",
		"TOOL_CALL_RESULT",
		"RUN_FINISHED",
	}
	if strings.Join(rec.types, ",") != strings.Join(want, ",") {
		t.Fatalf("event order = %v, want %v", rec.types, want)
	}

	start := jsonOf(t, rec.events[5])
	content := jsonOf(t, rec.events[6])
	end := jsonOf(t, rec.events[8])
	if start["messageId"] != content["messageId"] || end["messageId"] != start["messageId"] {
		t.Fatalf("text events disagree on the message id: %v %v %v", start, content, end)
	}

	call := jsonOf(t, rec.events[9])
	if call["toolCallName"] != "list_links" || call["toolCallId"] == "" {
		t.Fatalf("tool call start = %v", call)
	}
	args := jsonOf(t, rec.events[10])
	if args["delta"] != `{"limit":1}` {
		t.Fatalf("tool args = %v", args)
	}
	result := jsonOf(t, rec.events[12])
	if result["content"] != "[]" || result["toolCallId"] != call["toolCallId"] {
		t.Fatalf("tool result = %v", result)
	}

	finished := jsonOf(t, rec.events[13])
	if finished["threadId"] != "thread-1" || finished["runId"] != "run-abcdefgh" {
		t.Fatalf("run finished = %v", finished)
	}
	usage, _ := finished["usage"].([]any)
	if len(usage) != 1 {
		t.Fatalf("usage = %v", finished["usage"])
	}
	entry := usage[0].(map[string]any)
	if entry["reasoningTokens"] != float64(2) || entry["totalTokens"] != float64(15) {
		t.Fatalf("usage entry = %v", entry)
	}
}

func TestStream_ClosesTextBeforeToolCalls(t *testing.T) {
	s, rec := newRecorded()
	s.OnText("about to call")
	s.OnToolStart(0, "list_links", "{}")
	s.OnToolEnd(0, "list_links", "ok", nil)

	textEnd := -1
	toolStart := -1
	for i, typ := range rec.types {
		if typ == "TEXT_MESSAGE_END" && textEnd == -1 {
			textEnd = i
		}
		if typ == "TOOL_CALL_START" && toolStart == -1 {
			toolStart = i
		}
	}
	if textEnd == -1 || toolStart == -1 || textEnd > toolStart {
		t.Fatalf("text message must close before a tool call: %v", rec.types)
	}
}

func TestStream_TruncatesHugeToolResults(t *testing.T) {
	s, rec := newRecorded()
	s.OnToolStart(0, "list_links", "{}")
	s.OnToolEnd(0, "list_links", strings.Repeat("x", maxToolResultBytes+100), nil)

	var result map[string]any
	for i, typ := range rec.types {
		if typ == "TOOL_CALL_RESULT" {
			result = jsonOf(t, rec.events[i])
		}
	}
	content, _ := result["content"].(string)
	if !strings.HasSuffix(content, "[truncated]") {
		t.Fatalf("result was not truncated: %d bytes", len(content))
	}
	if len(content) > maxToolResultBytes+len("\n[truncated]") {
		t.Fatalf("result is still too large: %d bytes", len(content))
	}
}

func TestStream_FailCarriesCodeAndStops(t *testing.T) {
	s, rec := newRecorded()
	s.Fail("workforce_spend_not_recorded", "spend was not recorded")

	if len(rec.types) != 1 || rec.types[0] != "RUN_ERROR" {
		t.Fatalf("events = %v, want one RUN_ERROR", rec.types)
	}
	ev := jsonOf(t, rec.events[0])
	if ev["code"] != "workforce_spend_not_recorded" || ev["runId"] != "run-abcdefgh" {
		t.Fatalf("run error = %v", ev)
	}
}

func TestStream_SinkFailureStopsTheRun(t *testing.T) {
	calls := 0
	s := New(func(agui.Event) error {
		calls++
		return errors.New("client gone")
	}, "t", "r")

	s.OnText("one")
	s.OnText("two")
	s.Finish("fake", "m", 1, 1, 0)

	if s.Err() == nil {
		t.Fatal("sink failure was swallowed")
	}
	if calls != 1 {
		t.Fatalf("kept writing after the sink failed: %d calls", calls)
	}
}

func TestStream_SayEmitsOneCompleteMessage(t *testing.T) {
	s, rec := newRecorded()
	s.Say("a full answer")

	if !s.WroteText() {
		t.Fatal("Say did not mark the run as having text")
	}
	want := []string{"TEXT_MESSAGE_START", "TEXT_MESSAGE_CONTENT", "TEXT_MESSAGE_END"}
	if strings.Join(rec.types, ",") != strings.Join(want, ",") {
		t.Fatalf("events = %v, want %v", rec.types, want)
	}
	if rec.events[1].(*agui.TextMessageContentEvent).Delta != "a full answer" {
		t.Fatalf("delta = %q", rec.events[1].(*agui.TextMessageContentEvent).Delta)
	}
}
