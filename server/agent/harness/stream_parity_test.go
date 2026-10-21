package harness

import (
	"context"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/llmprovider"
)

// The streaming path and the non-streaming path are one loop with two
// transports. A call the model wrote wrongly must be reported on both: a
// model that believes it called a tool stops asking for the result.

// lastToolMessage returns the content of the last tool message in the
// history, or an empty string when the history holds none.
func lastToolMessage(s *Session) string {
	for i := len(s.History) - 1; i >= 0; i-- {
		if s.History[i].Role == "tool" {
			return s.History[i].Content
		}
	}
	return ""
}

func TestRunner_Stream_MalformedToolCallIsReportedNotDropped(t *testing.T) {
	t.Parallel()

	comp := &fakeStreamCompleter{
		steps: []streamStep{
			{
				events: []streamEvent{
					textEv("```tool_call\n{\"name\": \"lookup\", oops\n```"),
				},
			},
			{
				events: []streamEvent{
					textEv("I will send the call again in the right shape"),
				},
			},
		},
	}
	exec := &fakeExecutor{outputs: map[string]string{}, errs: map[string]error{}}

	runner := NewRunner(comp, exec)
	sr := runner.(StreamRunner)
	sess := &Session{ID: "s-stream-invalid", OrgID: 1, Channel: "web"}
	turn := &Turn{Input: "look up user 123"}

	if err := sr.Stream(context.Background(), sess, turn, nil); err != nil {
		t.Fatalf("Stream failed: %v", err)
	}

	if turn.Status != TurnStatusDone {
		t.Fatalf("expected TurnStatusDone, got %d", turn.Status)
	}
	if len(exec.calls) != 0 {
		t.Fatalf("a call that did not parse must not run, got %v", exec.calls)
	}
	notice := lastToolMessage(sess)
	if !strings.Contains(notice, "was not executed") {
		t.Fatalf("the model was not told that its call did not run: %q", notice)
	}
	if !strings.Contains(notice, "oops") {
		t.Fatalf("the model was not told what it sent: %q", notice)
	}
}

func TestRunner_Stream_NativeCallWithBadArgumentsIsReported(t *testing.T) {
	t.Parallel()

	comp := &fakeStreamCompleter{
		steps: []streamStep{
			{
				events: []streamEvent{
					toolEv(llmprovider.ToolCallChunk{Index: 0, ID: "call_1", Name: "lookup", ArgsJSON: `[1,2]`}),
				},
			},
			{
				events: []streamEvent{
					textEv("I will pass an object"),
				},
			},
		},
	}
	exec := &fakeExecutor{outputs: map[string]string{}, errs: map[string]error{}}

	runner := NewRunner(comp, exec)
	sr := runner.(StreamRunner)
	sess := &Session{ID: "s-stream-badargs", OrgID: 1, Channel: "web"}
	turn := &Turn{Input: "look up user 123"}

	if err := sr.Stream(context.Background(), sess, turn, nil); err != nil {
		t.Fatalf("Stream failed: %v", err)
	}

	if len(exec.calls) != 0 {
		t.Fatalf("a call with arguments that are not an object must not run, got %v", exec.calls)
	}
	notice := lastToolMessage(sess)
	if !strings.Contains(notice, "was not executed") {
		t.Fatalf("the model was not told that its call did not run: %q", notice)
	}
}

func TestRunner_Stream_MixedNativeCallsRunTheValidOneAndReportTheOther(t *testing.T) {
	t.Parallel()

	comp := &fakeStreamCompleter{
		steps: []streamStep{
			{
				events: []streamEvent{
					toolEv(llmprovider.ToolCallChunk{Index: 0, ID: "call_1", Name: "lookup", ArgsJSON: `{"id":"123"}`}),
					toolEv(llmprovider.ToolCallChunk{Index: 1, ID: "call_2", Name: "broken", ArgsJSON: `{"id":`}),
				},
			},
			{
				events: []streamEvent{
					textEv("the first call is done"),
				},
			},
		},
	}
	exec := &fakeExecutor{
		outputs: map[string]string{"lookup": `{"name":"Alice"}`},
		errs:    map[string]error{},
	}

	runner := NewRunner(comp, exec)
	sr := runner.(StreamRunner)
	sess := &Session{ID: "s-stream-mixed", OrgID: 1, Channel: "web"}
	turn := &Turn{Input: "look up user 123"}

	if err := sr.Stream(context.Background(), sess, turn, nil); err != nil {
		t.Fatalf("Stream failed: %v", err)
	}

	if len(exec.calls) != 1 || exec.calls[0] != "lookup" {
		t.Fatalf("expected only the valid call to run, got %v", exec.calls)
	}
	if len(turn.Steps) != 1 || turn.Steps[0].ToolName != "lookup" {
		t.Fatalf("expected one step for the valid call, got %+v", turn.Steps)
	}
	notice := lastToolMessage(sess)
	if !strings.Contains(notice, "was not executed") {
		t.Fatalf("the model was not told that its second call did not run: %q", notice)
	}
}

func TestRunner_Stream_NativeCallWithoutANameIsReported(t *testing.T) {
	t.Parallel()

	comp := &fakeStreamCompleter{
		steps: []streamStep{
			{
				events: []streamEvent{
					toolEv(llmprovider.ToolCallChunk{Index: 0, ID: "call_1", ArgsJSON: `{"id":"123"}`}),
				},
			},
			{
				events: []streamEvent{
					textEv("I will name the tool"),
				},
			},
		},
	}
	exec := &fakeExecutor{outputs: map[string]string{}, errs: map[string]error{}}

	runner := NewRunner(comp, exec)
	sr := runner.(StreamRunner)
	sess := &Session{ID: "s-stream-noname", OrgID: 1, Channel: "web"}
	turn := &Turn{Input: "look up user 123"}

	if err := sr.Stream(context.Background(), sess, turn, nil); err != nil {
		t.Fatalf("Stream failed: %v", err)
	}

	if len(exec.calls) != 0 {
		t.Fatalf("a call without a tool name must not run, got %v", exec.calls)
	}
	if !strings.Contains(lastToolMessage(sess), "was not executed") {
		t.Fatal("the model was not told that its call did not run")
	}
}
