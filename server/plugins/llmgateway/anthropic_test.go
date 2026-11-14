package llmgateway

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

// messagesFixture points one workspace provider at a stand-in upstream.
func messagesFixture(t *testing.T, u *upstream) *httpFixture {
	t.Helper()
	f := newHTTPFixture(t, 1, 1, "member")
	id := createProvider(t, f, ProviderInput{Name: "stub", Backend: "openai", APIKey: "k-test",
		BaseURL: "http://placeholder.invalid"})
	withUpstream(t, f, u, id)
	return f
}

// sentBody is the OpenAI request the upstream received.
func sentBody(t *testing.T, u *upstream) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal([]byte(u.last(t).body), &body); err != nil {
		t.Fatalf("the upstream body is not JSON: %v", err)
	}
	return body
}

// TestMessagesTranslatesTheRequest proves every Anthropic construct has an
// OpenAI counterpart on the way out.
func TestMessagesTranslatesTheRequest(t *testing.T) {
	u := &upstream{chatBody: `{"id":"c1","model":"m","choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`}
	f := messagesFixture(t, u)

	req := `{
		"model":"gpt-demo","max_tokens":256,"temperature":0.2,"stop_sequences":["END"],
		"system":[{"type":"text","text":"You are terse."}],
		"tools":[{"name":"read","description":"Read a file","input_schema":{"type":"object","properties":{"path":{"type":"string"}}}}],
		"tool_choice":{"type":"tool","name":"read"},
		"messages":[
			{"role":"user","content":[{"type":"text","text":"look"},{"type":"image","source":{"type":"base64","media_type":"image/jpeg","data":"AAA"}}]},
			{"role":"assistant","content":[{"type":"text","text":"reading"},{"type":"tool_use","id":"call_1","name":"read","input":{"path":"a.go"}}]},
			{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_1","content":[{"type":"text","text":"package a"}]},{"type":"text","text":"next"}]}
		]}`
	status, body := f.do(t, http.MethodPost, "/v1/messages", req)
	if status != http.StatusOK {
		t.Fatalf("messages = %d %v", status, body)
	}

	sent := sentBody(t, u)
	if sent["model"] != "gpt-demo" || sent["max_tokens"] != float64(256) || sent["temperature"] != 0.2 {
		t.Fatalf("scalars = %v", sent)
	}
	if stop, _ := sent["stop"].([]any); len(stop) != 1 || stop[0] != "END" {
		t.Fatalf("stop = %v", sent["stop"])
	}
	msgs, _ := sent["messages"].([]any)
	if len(msgs) != 5 {
		t.Fatalf("messages = %d: %v", len(msgs), msgs)
	}
	if m0, _ := msgs[0].(map[string]any); m0["role"] != "system" || m0["content"] != "You are terse." {
		t.Fatalf("system = %v", m0)
	}
	// The image and the text of the first user turn are content parts.
	m1, _ := msgs[1].(map[string]any)
	parts, _ := m1["content"].([]any)
	if len(parts) != 2 {
		t.Fatalf("user parts = %v", parts)
	}
	if p0, _ := parts[0].(map[string]any); p0["type"] != "text" || p0["text"] != "look" {
		t.Fatalf("text part = %v", parts[0])
	}
	p1, _ := parts[1].(map[string]any)
	img, _ := p1["image_url"].(map[string]any)
	if img["url"] != "data:image/jpeg;base64,AAA" {
		t.Fatalf("image = %v", p1)
	}
	// The assistant turn keeps its text and turns tool_use into tool_calls.
	m2, _ := msgs[2].(map[string]any)
	calls, _ := m2["tool_calls"].([]any)
	if m2["role"] != "assistant" || m2["content"] != "reading" || len(calls) != 1 {
		t.Fatalf("assistant = %v", m2)
	}
	call, _ := calls[0].(map[string]any)
	fn, _ := call["function"].(map[string]any)
	if call["id"] != "call_1" || fn["name"] != "read" || fn["arguments"] != `{"path":"a.go"}` {
		t.Fatalf("tool call = %v", call)
	}
	// The tool result comes before the user text and carries the same id.
	m3, _ := msgs[3].(map[string]any)
	if m3["role"] != "tool" || m3["tool_call_id"] != "call_1" || m3["content"] != "package a" {
		t.Fatalf("tool message = %v", m3)
	}
	m4, _ := msgs[4].(map[string]any)
	lastParts, _ := m4["content"].([]any)
	last, _ := lastParts[0].(map[string]any)
	if m4["role"] != "user" || last["text"] != "next" {
		t.Fatalf("trailing text = %v", m4)
	}
	tools, _ := sent["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("tools = %v", sent["tools"])
	}
	choice, _ := sent["tool_choice"].(map[string]any)
	chosen, _ := choice["function"].(map[string]any)
	if chosen["name"] != "read" {
		t.Fatalf("tool_choice = %v", sent["tool_choice"])
	}
}

// TestMessagesTranslatesTheAnswer proves the answer comes back in Anthropic
// shape, tool calls included.
func TestMessagesTranslatesTheAnswer(t *testing.T) {
	u := &upstream{chatBody: `{"id":"chatcmpl-9","model":"gpt-demo","choices":[{"message":{"content":"hello",
		"tool_calls":[{"id":"call_7","type":"function","function":{"name":"read","arguments":"{\"path\":\"a.go\"}"}}]},
		"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":11,"completion_tokens":7}}`}
	f := messagesFixture(t, u)

	status, body := f.do(t, http.MethodPost, "/v1/messages",
		`{"model":"gpt-demo","max_tokens":64,"messages":[{"role":"user","content":"hi"}]}`)
	if status != http.StatusOK {
		t.Fatalf("messages = %d %v", status, body)
	}
	if body["type"] != "message" || body["role"] != "assistant" || body["id"] != "msg_chatcmpl-9" {
		t.Fatalf("envelope = %v", body)
	}
	if body["stop_reason"] != "tool_use" {
		t.Fatalf("stop_reason = %v", body["stop_reason"])
	}
	usage, _ := body["usage"].(map[string]any)
	if usage["input_tokens"] != float64(11) || usage["output_tokens"] != float64(7) {
		t.Fatalf("usage = %v", usage)
	}
	blocks, _ := body["content"].([]any)
	if len(blocks) != 2 {
		t.Fatalf("content = %v", blocks)
	}
	text, _ := blocks[0].(map[string]any)
	if text["type"] != "text" || text["text"] != "hello" {
		t.Fatalf("text block = %v", text)
	}
	use, _ := blocks[1].(map[string]any)
	input, _ := use["input"].(map[string]any)
	if use["type"] != "tool_use" || use["id"] != "call_7" || use["name"] != "read" || input["path"] != "a.go" {
		t.Fatalf("tool block = %v", use)
	}
}

// TestMessagesRefusesABlockWithNoCounterpart keeps a client from losing part of
// its context without being told.
func TestMessagesRefusesABlockWithNoCounterpart(t *testing.T) {
	u := &upstream{chatBody: `{}`}
	f := messagesFixture(t, u)
	status, body := f.do(t, http.MethodPost, "/v1/messages",
		`{"model":"gpt-demo","max_tokens":8,"messages":[{"role":"user","content":[{"type":"thinking","thinking":"hmm"}]}]}`)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d %v", status, body)
	}
	kind, _ := body["type"].(string)
	if kind != "error" {
		t.Fatalf("body = %v", body)
	}
}

// TestMessagesRefusesAToolResultImage covers the one tool answer OpenAI has no
// shape for.
func TestMessagesRefusesAToolResultImage(t *testing.T) {
	u := &upstream{chatBody: `{}`}
	f := messagesFixture(t, u)
	status, _ := f.do(t, http.MethodPost, "/v1/messages", `{"model":"gpt-demo","max_tokens":8,"messages":[
		{"role":"user","content":[{"type":"tool_result","tool_use_id":"c1","content":[{"type":"image","source":{"type":"base64","data":"AAA"}}]}]}]}`)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
}

// TestMessagesRefusesANamelessModel keeps a request that names nothing from
// reaching the provider.
func TestMessagesRefusesANamelessModel(t *testing.T) {
	u := &upstream{chatBody: `{}`}
	f := messagesFixture(t, u)
	if status, _ := f.do(t, http.MethodPost, "/v1/messages", `{"max_tokens":8,"messages":[]}`); status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
}

// TestMessagesReportsAProviderRefusal keeps the upstream message legible.
func TestMessagesReportsAProviderRefusal(t *testing.T) {
	u := &upstream{chatCode: http.StatusTooManyRequests, chatBody: `{"error":"slow down"}`}
	f := messagesFixture(t, u)
	status, body := f.do(t, http.MethodPost, "/v1/messages",
		`{"model":"gpt-demo","max_tokens":8,"messages":[{"role":"user","content":"hi"}]}`)
	if status != http.StatusBadGateway {
		t.Fatalf("status = %d %v", status, body)
	}
	errBody, _ := body["error"].(map[string]any)
	msg, _ := errBody["message"].(string)
	if !strings.Contains(msg, "slow down") {
		t.Fatalf("message = %q", msg)
	}
}

// TestMessagesNeedsASession keeps an unauthenticated composition from spending
// the provider.
func TestMessagesNeedsASession(t *testing.T) {
	f := newHTTPFixture(t, 1, 1, "member")
	f.plugin.pctx = nil
	if status, _ := f.do(t, http.MethodPost, "/v1/messages", `{"model":"m","messages":[]}`); status != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", status)
	}
}

// TestMessagesStreamsAnthropicEvents walks the event sequence a client parses:
// message_start, text, a tool block, then the closing events.
func TestMessagesStreamsAnthropicEvents(t *testing.T) {
	u := &upstream{chatBody: strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"Hel"}}]}`,
		`data: {"choices":[{"delta":{"content":"lo"}}]}`,
		`data: {"choices":[{"delta":{"content":""},"finish_reason":"tool_calls"}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_2","type":"function","function":{"name":"read","arguments":""}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"path\":"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"a.go\"}"}}]}}]}`,
		`data: {"choices":[],"usage":{"completion_tokens":42}}`,
		`data: [DONE]`,
		``,
	}, "\n\n")}
	f := messagesFixture(t, u)

	req, err := http.NewRequest(http.MethodPost, f.server.URL+"/v1/messages",
		strings.NewReader(`{"model":"gpt-demo","max_tokens":64,"stream":true,"messages":[{"role":"user","content":"hi"}]}`))
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
		"event: message_start",
		`"content_block":{"text":"","type":"text"}`,
		`"text":"Hel"`,
		`"type":"tool_use"`,
		`"partial_json":"{\"path\":"`,
		`"partial_json":"\"a.go\"}"`,
		"event: content_block_stop",
		`"stop_reason":"tool_use"`,
		`"output_tokens":42`,
		"event: message_done",
	} {
		if want == "event: message_done" {
			if !strings.Contains(text, "event: message_stop") {
				t.Fatalf("no message_stop in:\n%s", text)
			}
			continue
		}
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
	// The provider was asked to include usage, which is where output_tokens
	// comes from.
	sent := sentBody(t, u)
	opts, _ := sent["stream_options"].(map[string]any)
	if sent["stream"] != true || opts["include_usage"] != true {
		t.Fatalf("stream request = %v", sent)
	}
}

// TestCountTokensAnswersAnEstimate covers the context meter.
func TestCountTokensAnswersAnEstimate(t *testing.T) {
	u := &upstream{chatBody: `{}`}
	f := messagesFixture(t, u)
	status, body := f.do(t, http.MethodPost, "/v1/messages/count_tokens",
		`{"model":"gpt-demo","system":"be terse","messages":[{"role":"user","content":"hello there"}]}`)
	if status != http.StatusOK {
		t.Fatalf("status = %d %v", status, body)
	}
	n, _ := body["input_tokens"].(float64)
	if n < 5 {
		t.Fatalf("input_tokens = %v", body["input_tokens"])
	}
}

// TestMessagesErrorsAreAnthropicShaped keeps the client's error parser working.
func TestMessagesErrorsAreAnthropicShaped(t *testing.T) {
	f := messagesFixture(t, &upstream{chatBody: `{}`})
	status, body := f.do(t, http.MethodPost, "/v1/messages", `{`)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	errBody, _ := body["error"].(map[string]any)
	if body["type"] != "error" || errBody["type"] != "invalid_request_error" {
		t.Fatalf("body = %v", body)
	}
}

// TestStopReason covers the finish-reason table.
func TestStopReason(t *testing.T) {
	for in, want := range map[string]string{
		"stop": "end_turn", "length": "max_tokens", "tool_calls": "tool_use",
		"function_call": "tool_use", "": "end_turn", "content_filter": "end_turn",
	} {
		if got := stopReason(in); got != want {
			t.Fatalf("stopReason(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestParseBlocks covers the two shapes a content field takes.
func TestParseBlocks(t *testing.T) {
	blocks, err := parseBlocks(json.RawMessage(`"plain"`))
	if err != nil || len(blocks) != 1 || blocks[0].Text != "plain" {
		t.Fatalf("string content = %v %v", blocks, err)
	}
	blocks, err = parseBlocks(json.RawMessage(`[{"type":"text","text":"a"}]`))
	if err != nil || len(blocks) != 1 || blocks[0].Text != "a" {
		t.Fatalf("list content = %v %v", blocks, err)
	}
	if blocks, err := parseBlocks(nil); err != nil || blocks != nil {
		t.Fatalf("empty content = %v %v", blocks, err)
	}
	if _, err := parseBlocks(json.RawMessage(`42`)); err == nil {
		t.Fatal("a number is not content")
	}
}

// TestSystemText covers the two shapes a system prompt takes.
func TestSystemText(t *testing.T) {
	got, err := systemText(json.RawMessage(`[{"type":"text","text":"a"},{"type":"text","text":"b"}]`))
	if err != nil || got != "a\n\nb" {
		t.Fatalf("system = %q %v", got, err)
	}
	if _, err := systemText(json.RawMessage(`7`)); err == nil {
		t.Fatal("a number is not a system prompt")
	}
}

// TestToolChoice covers the choice table.
func TestToolChoice(t *testing.T) {
	if toolChoice(nil) != "auto" {
		t.Fatal("no choice should be auto")
	}
	if toolChoice(&anthToolChoice{Type: "any"}) != "required" {
		t.Fatal("any should be required")
	}
	if toolChoice(&anthToolChoice{Type: "none"}) != "none" {
		t.Fatal("none should be none")
	}
	if toolChoice(&anthToolChoice{Type: "weird"}) != "auto" {
		t.Fatal("an unknown choice should be auto")
	}
}

// TestEstimateRequestTokens grows with the payload.
func TestEstimateRequestTokens(t *testing.T) {
	small := estimateRequestTokens(&anthRequest{Messages: []anthMessage{{Role: "user", Content: json.RawMessage(`"hi"`)}}})
	big := estimateRequestTokens(&anthRequest{
		System:   json.RawMessage(`"` + strings.Repeat("x", 400) + `"`),
		Messages: []anthMessage{{Role: "user", Content: json.RawMessage(`[{"type":"image","source":{"type":"base64","data":"A"}}]`)}},
		Tools:    []anthTool{{Name: "read", Description: "Read a file", InputSchema: json.RawMessage(`{"type":"object"}`)}},
	})
	if small <= 0 || big <= small {
		t.Fatalf("small = %d, big = %d", small, big)
	}
}

// TestMessageIDKeepsTheProviderID keeps a trace followable.
func TestMessageIDKeepsTheProviderID(t *testing.T) {
	if got := messageID("chatcmpl-1"); got != "msg_chatcmpl-1" {
		t.Fatalf("id = %q", got)
	}
	if got := messageID(""); !strings.HasPrefix(got, "msg_") {
		t.Fatalf("id = %q", got)
	}
}

// TestMessagesImageSourceShapes covers the url and broken sources.
func TestMessagesImageSourceShapes(t *testing.T) {
	if got, err := imageURL(&anthImageSource{Type: "url", URL: "https://x/y.png"}); err != nil || got != "https://x/y.png" {
		t.Fatalf("url = %q %v", got, err)
	}
	if got, err := imageURL(&anthImageSource{Type: "base64", Data: "AAA"}); err != nil || got != "data:image/png;base64,AAA" {
		t.Fatalf("default media = %q %v", got, err)
	}
	if _, err := imageURL(nil); err == nil {
		t.Fatal("no source should fail")
	}
	if _, err := imageURL(&anthImageSource{Type: "base64"}); err == nil {
		t.Fatal("no data should fail")
	}
}

// TestMessagesRefusesABadTool keeps a nameless tool from reaching the provider.
func TestMessagesRefusesABadTool(t *testing.T) {
	u := &upstream{chatBody: `{}`}
	f := messagesFixture(t, u)
	status, _ := f.do(t, http.MethodPost, "/v1/messages",
		`{"model":"m","max_tokens":8,"tools":[{"input_schema":{}}],"messages":[{"role":"user","content":"hi"}]}`)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
}

// TestMessagesTruncatedAnswer covers a provider that answers with nothing
// parseable.
func TestMessagesTruncatedAnswer(t *testing.T) {
	u := &upstream{chatBody: `not json`}
	f := messagesFixture(t, u)
	status, _ := f.do(t, http.MethodPost, "/v1/messages",
		`{"model":"m","max_tokens":8,"messages":[{"role":"user","content":"hi"}]}`)
	if status != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", status)
	}
}

// TestMessagesRejectsAnUnknownRole keeps a bad turn from being guessed at.
func TestMessagesRejectsAnUnknownRole(t *testing.T) {
	u := &upstream{chatBody: `{}`}
	f := messagesFixture(t, u)
	status, _ := f.do(t, http.MethodPost, "/v1/messages",
		fmt.Sprintf(`{"model":"m","max_tokens":8,"messages":[{"role":"developer","content":"%s"}]}`, "hi"))
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
}

// TestMessagesKeepsAMidConversationSystemMessage covers the reminder a coding
// agent inserts between turns: its position is kept, not merged.
func TestMessagesKeepsAMidConversationSystemMessage(t *testing.T) {
	u := &upstream{chatBody: `{"id":"c","model":"m","choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`}
	f := messagesFixture(t, u)
	status, body := f.do(t, http.MethodPost, "/v1/messages", `{"model":"m","max_tokens":8,"messages":[
		{"role":"user","content":"hi"},
		{"role":"system","content":[{"type":"text","text":"reminder"}]},
		{"role":"user","content":"again"}]}`)
	if status != http.StatusOK {
		t.Fatalf("status = %d %v", status, body)
	}
	sent := sentBody(t, u)
	msgs, _ := sent["messages"].([]any)
	if len(msgs) != 3 {
		t.Fatalf("messages = %v", msgs)
	}
	m1, _ := msgs[1].(map[string]any)
	if m1["role"] != "system" || m1["content"] != "reminder" {
		t.Fatalf("reminder = %v", m1)
	}
}
