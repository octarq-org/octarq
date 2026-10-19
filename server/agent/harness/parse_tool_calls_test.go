package harness

import (
	"encoding/json"
	"strings"
	"testing"
)

// The tool-call block is the model's only way to reach a tool, and the prompt
// tells it exactly one shape. These tests pin that shape down: an object for
// "args", one call per line, and a malformed line reported back instead of
// being dropped in silence.

func TestParseToolCalls_ObjectArgs(t *testing.T) {
	text := "thinking\n```tool_call\n{\"name\":\"create_link\",\"args\":{\"destination\":\"https://example.com\"}}\n```\nafter"

	calls, remaining, invalid := parseToolCalls(text)

	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d (invalid=%v)", len(calls), invalid)
	}
	if calls[0].Name != "create_link" {
		t.Errorf("name = %q, want create_link", calls[0].Name)
	}
	var args map[string]any
	if err := json.Unmarshal(calls[0].ArgsJSON, &args); err != nil {
		t.Fatalf("args are not a JSON object: %v (%s)", err, calls[0].ArgsJSON)
	}
	if args["destination"] != "https://example.com" {
		t.Errorf("args = %v, want the destination argument", args)
	}
	if !strings.Contains(remaining, "thinking") || !strings.Contains(remaining, "after") {
		t.Errorf("remaining = %q, want the text around the fence", remaining)
	}
	if strings.Contains(remaining, "tool_call") {
		t.Errorf("remaining still carries the fence: %q", remaining)
	}
}

func TestParseToolCalls_StringArgsAreNormalizedToAnObject(t *testing.T) {
	// A JSON-encoded string carries the same value as the object; both decode
	// to the same arguments.
	text := "```tool_call\n{\"name\":\"create_link\",\"args\":\"{\\\"slug\\\":\\\"promo\\\"}\"}\n```"

	calls, _, invalid := parseToolCalls(text)

	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d (invalid=%v)", len(calls), invalid)
	}
	var args map[string]any
	if err := json.Unmarshal(calls[0].ArgsJSON, &args); err != nil {
		t.Fatalf("args are not a JSON object: %v (%s)", err, calls[0].ArgsJSON)
	}
	if args["slug"] != "promo" {
		t.Errorf("args = %v, want the slug argument", args)
	}
}

func TestParseToolCalls_MissingArgsBecomeAnEmptyObject(t *testing.T) {
	text := "```tool_call\n{\"name\":\"list_domains\"}\n```"

	calls, _, _ := parseToolCalls(text)

	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if string(calls[0].ArgsJSON) != "{}" {
		t.Errorf("args = %q, want {}", calls[0].ArgsJSON)
	}
}

func TestParseToolCalls_MalformedLineIsReportedNotDropped(t *testing.T) {
	text := "```tool_call\n{\"name\":\"create_link\", oops\n```"

	calls, _, invalid := parseToolCalls(text)

	if len(calls) != 0 {
		t.Fatalf("expected no parsed call, got %d", len(calls))
	}
	if len(invalid) != 1 {
		t.Fatalf("expected 1 invalid line, got %d", len(invalid))
	}
	if invalid[0] == "" {
		t.Error("the invalid line is empty; the model would learn nothing from it")
	}
}

func TestParseToolCalls_MultipleCalls(t *testing.T) {
	text := "```tool_call\n{\"name\":\"a\",\"args\":{}}\n{\"name\":\"b\",\"args\":{\"x\":1}}\n```"

	calls, _, invalid := parseToolCalls(text)

	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d (invalid=%v)", len(calls), invalid)
	}
	if calls[0].Name != "a" || calls[1].Name != "b" {
		t.Errorf("call order wrong: %q, %q", calls[0].Name, calls[1].Name)
	}
}

func TestParseToolCalls_NoFenceLeavesTextAlone(t *testing.T) {
	text := "just an answer"

	calls, remaining, invalid := parseToolCalls(text)

	if len(calls) != 0 || len(invalid) != 0 {
		t.Fatalf("expected no calls and no invalid lines, got %d / %d", len(calls), len(invalid))
	}
	if remaining != text {
		t.Errorf("remaining = %q, want the original text", remaining)
	}
}

func TestParseToolCalls_CallWithoutNameIsInvalid(t *testing.T) {
	text := "```tool_call\n{\"args\":{}}\n```"

	calls, _, invalid := parseToolCalls(text)

	if len(calls) != 0 {
		t.Fatalf("expected no parsed call, got %d", len(calls))
	}
	if len(invalid) != 1 {
		t.Fatalf("expected 1 invalid line, got %d", len(invalid))
	}
}
