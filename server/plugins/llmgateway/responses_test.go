package llmgateway

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// responsesFixture points one workspace provider at a stand-in upstream.
func responsesFixture(t *testing.T, u *upstream) *httpFixture {
	t.Helper()
	f := newHTTPFixture(t, 1, 1, "member")
	id := createProvider(t, f, ProviderInput{Name: "stub", Backend: "openai", APIKey: "k-test",
		BaseURL: "http://placeholder.invalid"})
	withUpstream(t, f, u, id)
	return f
}

// responsesSent is the conversation the gateway built for the provider.
func responsesSent(t *testing.T, u *upstream) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal([]byte(u.last(t).body), &body); err != nil {
		t.Fatalf("the upstream body is not JSON: %v", err)
	}
	return body
}

// TestResponsesAnswersWithAnOutputText proves the plain turn works: the input
// becomes messages, and the completion becomes a message item with output_text.
func TestResponsesAnswersWithAnOutputText(t *testing.T) {
	u := &upstream{chatBody: `{"id":"chatcmpl-1","model":"stub-model",
		"choices":[{"message":{"content":"pong"},"finish_reason":"stop"}],
		"usage":{"prompt_tokens":11,"completion_tokens":2}}`}
	f := responsesFixture(t, u)

	status, body := f.do(t, http.MethodPost, "/v1/responses", `{
		"model":"stub-model",
		"instructions":"be terse",
		"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"ping"}]}]}`)
	if status != http.StatusOK {
		t.Fatalf("status = %d %v", status, body)
	}
	if body["object"] != "response" || body["status"] != "completed" {
		t.Fatalf("response = %v", body)
	}
	output, _ := body["output"].([]any)
	if len(output) != 1 {
		t.Fatalf("output = %v", body["output"])
	}
	item, _ := output[0].(map[string]any)
	if item["type"] != "message" || item["role"] != "assistant" {
		t.Fatalf("item = %v", item)
	}
	parts, _ := item["content"].([]any)
	if len(parts) != 1 {
		t.Fatalf("content = %v", item["content"])
	}
	part, _ := parts[0].(map[string]any)
	if part["type"] != "output_text" || part["text"] != "pong" {
		t.Fatalf("part = %v", part)
	}
	usage, _ := body["usage"].(map[string]any)
	if usage["input_tokens"] != float64(11) || usage["output_tokens"] != float64(2) || usage["total_tokens"] != float64(13) {
		t.Fatalf("usage = %v", usage)
	}

	// The provider reads a system message and a user message.
	sent := responsesSent(t, u)
	messages, _ := sent["messages"].([]any)
	if len(messages) != 2 {
		t.Fatalf("messages = %v", sent["messages"])
	}
	first, _ := messages[0].(map[string]any)
	if first["role"] != "system" || first["content"] != "be terse" {
		t.Fatalf("system message = %v", first)
	}
}

// TestResponsesKeepsTheToolConversation proves the turn after a tool call keeps
// both sides: the assistant's call and the tool's answer.
func TestResponsesKeepsTheToolConversation(t *testing.T) {
	u := &upstream{chatBody: `{"id":"c2","model":"m","choices":[{"message":{"content":"",
		"tool_calls":[{"id":"call_9","type":"function","function":{"name":"read","arguments":"{\"path\":\"a.go\"}"}}]},
		"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":3,"completion_tokens":4}}`}
	f := responsesFixture(t, u)

	status, body := f.do(t, http.MethodPost, "/v1/responses", `{
		"model":"stub-model",
		"input":[
			{"type":"message","role":"user","content":[{"type":"input_text","text":"read a.go"}]},
			{"type":"function_call","call_id":"call_1","name":"read","arguments":"{\"path\":\"a.go\"}"},
			{"type":"function_call_output","call_id":"call_1","output":"package main"}
		]}`)
	if status != http.StatusOK {
		t.Fatalf("status = %d %v", status, body)
	}

	sent := responsesSent(t, u)
	messages, _ := sent["messages"].([]any)
	if len(messages) != 3 {
		t.Fatalf("messages = %v", sent["messages"])
	}
	assistant, _ := messages[1].(map[string]any)
	calls, _ := assistant["tool_calls"].([]any)
	if assistant["role"] != "assistant" || len(calls) != 1 {
		t.Fatalf("assistant message = %v", assistant)
	}
	call, _ := calls[0].(map[string]any)
	if call["id"] != "call_1" {
		t.Fatalf("tool call id = %v", call["id"])
	}
	tool, _ := messages[2].(map[string]any)
	if tool["role"] != "tool" || tool["tool_call_id"] != "call_1" || tool["content"] != "package main" {
		t.Fatalf("tool message = %v", tool)
	}

	// The answer's call becomes a function_call item the agent can run.
	output, _ := body["output"].([]any)
	if len(output) != 1 {
		t.Fatalf("output = %v", body["output"])
	}
	item, _ := output[0].(map[string]any)
	if item["type"] != "function_call" || item["call_id"] != "call_9" || item["name"] != "read" {
		t.Fatalf("function_call item = %v", item)
	}
	if item["arguments"] != `{"path":"a.go"}` {
		t.Fatalf("arguments = %v", item["arguments"])
	}
}

// TestResponsesFlattensToolLists covers the three tool shapes Codex sends: a
// function, a namespace holding functions, and a hosted tool it cannot call.
func TestResponsesFlattensToolLists(t *testing.T) {
	u := &upstream{chatBody: `{"choices":[{"message":{"content":""},"finish_reason":"stop"}]}`}
	f := responsesFixture(t, u)

	status, body := f.do(t, http.MethodPost, "/v1/responses", `{
		"model":"stub-model",
		"input":"go",
		"tools":[
			{"type":"function","name":"exec_command","parameters":{"type":"object","properties":{}}},
			{"type":"web_search"},
			{"type":"namespace","name":"multi_agent_v1","tools":[
				{"type":"function","name":"close_agent","parameters":{"type":"object","properties":{}}}
			]}
		]}`)
	if status != http.StatusOK {
		t.Fatalf("status = %d %v", status, body)
	}
	sent := responsesSent(t, u)
	tools, _ := sent["tools"].([]any)
	names := make([]string, 0, len(tools))
	for _, raw := range tools {
		tool, _ := raw.(map[string]any)
		fn, _ := tool["function"].(map[string]any)
		names = append(names, fn["name"].(string))
	}
	if len(names) != 2 || names[0] != "exec_command" || names[1] != "close_agent" {
		t.Fatalf("tool names = %v", names)
	}
}

// TestResponsesRefusesStateItCannotKeep keeps a stored-response client from
// losing the conversation instead of failing.
func TestResponsesRefusesStateItCannotKeep(t *testing.T) {
	f := responsesFixture(t, &upstream{chatBody: `{}`})
	status, body := f.do(t, http.MethodPost, "/v1/responses",
		`{"model":"m","input":"hi","previous_response_id":"resp_1"}`)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d %v", status, body)
	}
	errBody, _ := body["error"].(map[string]any)
	message, _ := errBody["message"].(string)
	if !strings.Contains(message, "previous_response_id") {
		t.Fatalf("message = %q", message)
	}
}

// TestResponsesRefusesAnUnknownItem names the item it cannot translate, rather
// than dropping part of the turn.
func TestResponsesRefusesAnUnknownItem(t *testing.T) {
	f := responsesFixture(t, &upstream{chatBody: `{}`})
	status, body := f.do(t, http.MethodPost, "/v1/responses",
		`{"model":"m","input":[{"type":"item_reference","id":"msg_1"}]}`)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d %v", status, body)
	}
	errBody, _ := body["error"].(map[string]any)
	message, _ := errBody["message"].(string)
	if !strings.Contains(message, "item_reference") {
		t.Fatalf("message = %q", message)
	}
}

// TestResponsesStreamsTheEventSequence walks the events Codex parses: created,
// the message item, the text deltas, and the closing completed event.
func TestResponsesStreamsTheEventSequence(t *testing.T) {
	u := &upstream{chatBody: strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"pong"}}]}`,
		`data: {"choices":[{"delta":{"content":""},"finish_reason":"stop"}]}`,
		`data: {"choices":[],"usage":{"prompt_tokens":7,"completion_tokens":1}}`,
		`data: [DONE]`,
		``,
	}, "\n\n")}
	f := responsesFixture(t, u)

	req, err := http.NewRequest(http.MethodPost, f.server.URL+"/v1/responses",
		strings.NewReader(`{"model":"stub-model","stream":true,"input":"ping"}`))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d %s", resp.StatusCode, raw)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type = %q", ct)
	}
	text := string(raw)
	for _, want := range []string{
		"event: response.created",
		"event: response.output_item.added",
		`"type":"output_text"`,
		"event: response.output_text.delta",
		`"delta":"pong"`,
		"event: response.output_text.done",
		"event: response.output_item.done",
		"event: response.completed",
		`"input_tokens":7`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
	// The closing event carries the finished items, which is where a client
	// that ignored the deltas reads the answer.
	completed := text[strings.LastIndex(text, "event: response.completed"):]
	if !strings.Contains(completed, `"text":"pong"`) || !strings.Contains(completed, `"status":"completed"`) {
		t.Fatalf("completed event = %s", completed)
	}
	sent := responsesSent(t, u)
	opts, _ := sent["stream_options"].(map[string]any)
	if sent["stream"] != true || opts["include_usage"] != true {
		t.Fatalf("stream request = %v", sent)
	}
}

// TestResponsesStreamsAToolCall proves the arguments arrive assembled, which is
// what the agent runs.
func TestResponsesStreamsAToolCall(t *testing.T) {
	u := &upstream{chatBody: strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_3","type":"function","function":{"name":"read","arguments":""}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"path\":"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"a.go\"}"}}]}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
		``,
	}, "\n\n")}
	f := responsesFixture(t, u)

	req, err := http.NewRequest(http.MethodPost, f.server.URL+"/v1/responses",
		strings.NewReader(`{"model":"stub-model","stream":true,"input":"read a.go"}`))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	text := string(raw)
	for _, want := range []string{
		`"type":"function_call"`,
		`"call_id":"call_3"`,
		"event: response.function_call_arguments.delta",
		"event: response.function_call_arguments.done",
		`"arguments":"{\"path\":\"a.go\"}"`,
		"event: response.completed",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
}

// TestResponsesKeepsCodexDeveloperMessages covers the shape Codex actually
// sends: two messages with role "developer" carrying input_text, no
// instructions field, and the user turn last.
func TestResponsesKeepsCodexDeveloperMessages(t *testing.T) {
	u := &upstream{chatBody: `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`}
	f := responsesFixture(t, u)

	status, body := f.do(t, http.MethodPost, "/v1/responses", `{
		"model":"stub-model",
		"input":[
			{"type":"message","role":"developer","content":[{"type":"input_text","text":"You are a coding agent."}]},
			{"type":"message","role":"developer","content":[{"type":"input_text","text":"Sandbox: read-only."}]},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"Run echo hi."}]}
		]}`)
	if status != http.StatusOK {
		t.Fatalf("status = %d %v", status, body)
	}
	sent := responsesSent(t, u)
	messages, _ := sent["messages"].([]any)
	if len(messages) != 3 {
		t.Fatalf("messages = %v", sent["messages"])
	}
	for i, want := range []string{"system", "system", "user"} {
		msg, _ := messages[i].(map[string]any)
		if msg["role"] != want {
			t.Fatalf("message %d role = %v, want %s", i, msg["role"], want)
		}
	}
	first, _ := messages[0].(map[string]any)
	if first["content"] != "You are a coding agent." {
		t.Fatalf("developer text = %v", first["content"])
	}
	if _, ok := sent["tool_choice"]; ok {
		t.Fatalf("tool_choice sent without tools: %v", sent["tool_choice"])
	}
}
