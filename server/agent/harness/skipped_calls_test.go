package harness

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/llmprovider"
)

func callBatch(names ...string) string {
	lines := make([]string, 0, len(names))
	for _, name := range names {
		lines = append(lines, fmt.Sprintf(`{"name":%q,"args":{}}`, name))
	}
	return "```tool_call\n" + strings.Join(lines, "\n") + "\n```"
}

func skippedNote(s *Session) string {
	for _, m := range s.History {
		if m.Role == "tool" && strings.Contains(m.Content, "did not run") {
			return m.Content
		}
	}
	return ""
}

func TestApprovalRecordsTheCallsThatDidNotRun(t *testing.T) {
	exec := &fakeExecutor{outputs: map[string]string{}, errs: map[string]error{}}
	comp := &fakeCompleter{responses: []string{callBatch("list_links", "delete_link", "create_link")}}
	r := newTestRunner(t, comp, exec, WithGuard(&approvalGuard{tool: "delete_link"}))
	s, turn := newBudgetTurn()

	if err := r.Run(context.Background(), s, turn); !errors.Is(err, ErrApprovalRequired) {
		t.Fatalf("Run = %v, want ErrApprovalRequired", err)
	}

	if len(exec.calls) != 1 || exec.calls[0] != "list_links" {
		t.Fatalf("executed %v, want only list_links", exec.calls)
	}

	note := skippedNote(s)
	if note == "" {
		t.Fatal("the history does not say which calls did not run; the model will repeat them")
	}
	if !strings.Contains(note, "create_link") {
		t.Fatalf("notice = %q, want the call that was dropped", note)
	}
	for _, ran := range []string{"list_links", "delete_link"} {
		if strings.Contains(note, ran) {
			t.Fatalf("notice = %q, must not name %q", note, ran)
		}
	}
}

func TestApprovalOnTheLastCallRecordsNoGap(t *testing.T) {
	exec := &fakeExecutor{outputs: map[string]string{}, errs: map[string]error{}}
	comp := &fakeCompleter{responses: []string{callBatch("list_links", "delete_link")}}
	r := newTestRunner(t, comp, exec, WithGuard(&approvalGuard{tool: "delete_link"}))
	s, turn := newBudgetTurn()

	if err := r.Run(context.Background(), s, turn); !errors.Is(err, ErrApprovalRequired) {
		t.Fatalf("Run = %v, want ErrApprovalRequired", err)
	}
	if note := skippedNote(s); note != "" {
		t.Fatalf("notice = %q, want none: the blocked call was the last of the batch", note)
	}
}

func TestStreamApprovalRecordsTheCallsThatDidNotRun(t *testing.T) {
	comp := &fakeStreamCompleter{steps: []streamStep{{
		events: []streamEvent{
			toolEv(llmprovider.ToolCallChunk{Index: 0, ID: "c0", Name: "list_links", ArgsJSON: "{}"}),
			toolEv(llmprovider.ToolCallChunk{Index: 1, ID: "c1", Name: "delete_link", ArgsJSON: `{"id":"42"}`}),
			toolEv(llmprovider.ToolCallChunk{Index: 2, ID: "c2", Name: "create_link", ArgsJSON: `{"url":"https://example.com"}`}),
		},
		resp: llmprovider.Response{StopReason: "tool_use"},
	}}}
	exec := &fakeExecutor{outputs: map[string]string{}, errs: map[string]error{}}
	runner := newTestRunner(t, comp, exec, WithGuard(&approvalGuard{tool: "delete_link"}))
	sr, ok := runner.(StreamRunner)
	if !ok {
		t.Fatal("runner does not implement StreamRunner")
	}
	s := &Session{ID: "s", OrgID: 1}
	turn := &Turn{Input: "go"}

	if err := sr.Stream(context.Background(), s, turn, nil); !errors.Is(err, ErrApprovalRequired) {
		t.Fatalf("Stream = %v, want ErrApprovalRequired", err)
	}
	note := skippedNote(s)
	if !strings.Contains(note, "create_link") {
		t.Fatalf("notice = %q, want the call the stream dropped", note)
	}
	if strings.Contains(note, "delete_link") {
		t.Fatalf("notice = %q, must not name the pending call", note)
	}
}

func TestResumeKeepsTheGapAndTheApprovedResult(t *testing.T) {
	exec := &fakeExecutor{outputs: map[string]string{"delete_link": "deleted"}, errs: map[string]error{}}
	comp := &fakeCompleter{responses: []string{
		callBatch("delete_link", "create_link"),
		"nothing left to do",
	}}
	r := newTestRunner(t, comp, exec, WithGuard(&approvalGuard{tool: "delete_link"}))
	s, turn := newBudgetTurn()

	if err := r.Run(context.Background(), s, turn); !errors.Is(err, ErrApprovalRequired) {
		t.Fatalf("Run = %v, want ErrApprovalRequired", err)
	}
	if err := r.Resume(context.Background(), s, turn); err != nil {
		t.Fatalf("Resume = %v, want nil", err)
	}

	if !strings.Contains(skippedNote(s), "create_link") {
		t.Fatal("the resume lost the record of the calls that did not run")
	}
	found := false
	for _, m := range s.History {
		if m.Role == "tool" && strings.Contains(m.Content, "deleted") {
			found = true
		}
	}
	if !found {
		t.Fatal("the resume did not record the approved call's result")
	}
}
