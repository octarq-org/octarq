package llmgateway

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestMessagesRefusesAnUnreadableTurn covers the shapes a client can get wrong.
func TestMessagesRefusesAnUnreadableTurn(t *testing.T) {
	cases := []struct{ name, body string }{
		{"system is not text", `{"model":"m","max_tokens":8,"system":7,"messages":[]}`},
		{"content is not text or blocks", `{"model":"m","max_tokens":8,"messages":[{"role":"user","content":42}]}`},
		{"assistant block has no counterpart", `{"model":"m","max_tokens":8,"messages":[
			{"role":"assistant","content":[{"type":"thinking","thinking":"hmm"}]}]}`},
		{"tool result content is not text", `{"model":"m","max_tokens":8,"messages":[
			{"role":"user","content":[{"type":"tool_result","tool_use_id":"c","content":5}]}]}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := messagesFixture(t, &upstream{chatBody: `{}`})
			if status, _ := f.do(t, http.MethodPost, "/v1/messages", c.body); status != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", status)
			}
		})
	}
}

// TestMessagesDefaultsWhatMayBeOmitted covers the optional fields: an empty
// user turn, a tool without a schema, and a tool call without arguments.
func TestMessagesDefaultsWhatMayBeOmitted(t *testing.T) {
	u := &upstream{chatBody: `{"id":"c","model":"m","choices":[{"message":{"content":"",
		"tool_calls":[{"id":"call_1","type":"function","function":{"name":"read"}}]},"finish_reason":"tool_calls"}]}`}
	f := messagesFixture(t, u)

	body := `{"model":"m","max_tokens":8,
		"tools":[{"name":"read"}],
		"messages":[
			{"role":"user","content":[]},
			{"role":"assistant","content":[{"type":"tool_use","id":"call_0","name":"read"}]}
		]}`
	status, out := f.do(t, http.MethodPost, "/v1/messages", body)
	if status != http.StatusOK {
		t.Fatalf("status = %d %v", status, out)
	}
	sent := sentBody(t, u)
	msgs, _ := sent["messages"].([]any)
	first, _ := msgs[0].(map[string]any)
	if first["content"] != "" {
		t.Fatalf("empty user turn = %v", first)
	}
	assistant, _ := msgs[1].(map[string]any)
	calls, _ := assistant["tool_calls"].([]any)
	call, _ := calls[0].(map[string]any)
	fn, _ := call["function"].(map[string]any)
	if fn["arguments"] != "{}" {
		t.Fatalf("arguments = %v", fn["arguments"])
	}
	tools, _ := sent["tools"].([]any)
	tool, _ := tools[0].(map[string]any)
	tf, _ := tool["function"].(map[string]any)
	if params, _ := tf["parameters"].(map[string]any); params["type"] != "object" {
		t.Fatalf("parameters = %v", tf["parameters"])
	}
	// The answer keeps the tool call even when it carries no arguments.
	blocks, _ := out["content"].([]any)
	use, _ := blocks[0].(map[string]any)
	input, _ := use["input"].(map[string]any)
	if use["type"] != "tool_use" || len(input) != 0 {
		t.Fatalf("tool block = %v", use)
	}
}

// TestMessagesAnswersAProviderWithNoChoices covers a provider that returns an
// empty answer: the client still gets a message it can render.
func TestMessagesAnswersAProviderWithNoChoices(t *testing.T) {
	f := messagesFixture(t, &upstream{chatBody: `{"id":"c","model":"m","choices":[]}`})
	status, body := f.do(t, http.MethodPost, "/v1/messages",
		`{"model":"m","max_tokens":8,"messages":[{"role":"user","content":"hi"}]}`)
	if status != http.StatusOK {
		t.Fatalf("status = %d %v", status, body)
	}
	blocks, _ := body["content"].([]any)
	if len(blocks) != 1 {
		t.Fatalf("content = %v", body["content"])
	}
	b0, _ := blocks[0].(map[string]any)
	if b0["type"] != "text" {
		t.Fatalf("empty block = %v", b0)
	}
	if text, ok := b0["text"]; !ok || text != "" {
		t.Fatalf("a text block without its text: %v", b0)
	}
}

// TestMessagesWithoutAProvider is the fail-closed state.
func TestMessagesWithoutAProvider(t *testing.T) {
	f := newHTTPFixture(t, 1, 1, "member")
	status, body := f.do(t, http.MethodPost, "/v1/messages",
		`{"model":"m","max_tokens":8,"messages":[{"role":"user","content":"hi"}]}`)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d %v", status, body)
	}
	errBody, _ := body["error"].(map[string]any)
	if msg, _ := errBody["message"].(string); !strings.Contains(msg, "no model provider") {
		t.Fatalf("message = %q", msg)
	}
}

// TestMessagesReportsAnUnbuildableCall covers a base URL no request can use.
func TestMessagesReportsAnUnbuildableCall(t *testing.T) {
	f := newHTTPFixture(t, 1, 1, "member")
	createProvider(t, f, ProviderInput{Name: "bad", Backend: "openai", APIKey: "k",
		BaseURL: "http://exa mple.invalid"})
	status, _ := f.do(t, http.MethodPost, "/v1/messages",
		`{"model":"m","max_tokens":8,"messages":[{"role":"user","content":"hi"}]}`)
	if status != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", status)
	}
}

// TestMessagesReportsAnUnreachableProvider covers a provider that is not there.
func TestMessagesReportsAnUnreachableProvider(t *testing.T) {
	f := newHTTPFixture(t, 1, 1, "member")
	createProvider(t, f, ProviderInput{Name: "gone", Backend: "openai", APIKey: "k",
		BaseURL: "http://127.0.0.1:1"})
	if status, _ := f.do(t, http.MethodPost, "/v1/messages",
		`{"model":"m","max_tokens":8,"messages":[{"role":"user","content":"hi"}]}`); status != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", status)
	}
}

// TestMessagesStreamReportsFailures covers a stream whose provider fails before
// any event is written.
func TestMessagesStreamReportsFailures(t *testing.T) {
	t.Run("unreachable", func(t *testing.T) {
		f := newHTTPFixture(t, 1, 1, "member")
		createProvider(t, f, ProviderInput{Name: "gone", Backend: "openai", APIKey: "k",
			BaseURL: "http://127.0.0.1:1"})
		status, _ := f.do(t, http.MethodPost, "/v1/messages",
			`{"model":"m","max_tokens":8,"stream":true,"messages":[{"role":"user","content":"hi"}]}`)
		if status != http.StatusBadGateway {
			t.Fatalf("status = %d, want 502", status)
		}
	})
	t.Run("refused", func(t *testing.T) {
		u := &upstream{chatCode: http.StatusPaymentRequired, chatBody: `{"error":"pay up"}`}
		f := messagesFixture(t, u)
		status, body := f.do(t, http.MethodPost, "/v1/messages",
			`{"model":"m","max_tokens":8,"stream":true,"messages":[{"role":"user","content":"hi"}]}`)
		if status != http.StatusBadGateway {
			t.Fatalf("status = %d %v", status, body)
		}
		errBody, _ := body["error"].(map[string]any)
		if msg, _ := errBody["message"].(string); !strings.Contains(msg, "pay up") {
			t.Fatalf("message = %q", msg)
		}
	})
}

// TestMessagesStreamSkipsNoise proves a malformed frame or an orphan fragment
// does not end the stream.
func TestMessagesStreamSkipsNoise(t *testing.T) {
	u := &upstream{chatBody: strings.Join([]string{
		`data: {"choices":[{"delta":{}}]}`,
		`data: not-json`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":2,"function":{"arguments":"{}"}}]}}]}`,
		`data: {"choices":[{"delta":{"content":"still here"}}]}`,
		`data: [DONE]`,
		``,
	}, "\n\n")}
	f := messagesFixture(t, u)
	req, err := http.NewRequest(http.MethodPost, f.server.URL+"/v1/messages",
		strings.NewReader(`{"model":"m","max_tokens":8,"stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	var raw strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		raw.Write(buf[:n])
		if err != nil {
			break
		}
	}
	text := raw.String()
	if !strings.Contains(text, "still here") {
		t.Fatalf("the stream ended early:\n%s", text)
	}
	// No finish_reason arrived, so the answer closes as a normal turn.
	if !strings.Contains(text, `"stop_reason":"end_turn"`) {
		t.Fatalf("no end_turn:\n%s", text)
	}
	if strings.Contains(text, `"index":2`) {
		t.Fatalf("an orphan fragment opened a block:\n%s", text)
	}
}

// TestCountTokensNeedsAWorkspaceAndANumber covers the route's guards.
func TestCountTokensNeedsAWorkspaceAndANumber(t *testing.T) {
	u := &upstream{chatBody: `{}`}
	f := messagesFixture(t, u)
	if status, _ := f.do(t, http.MethodPost, "/v1/messages/count_tokens", `{`); status != http.StatusBadRequest {
		t.Fatalf("bad body = %d, want 400", status)
	}
}

// TestEstimateRequestTokensCountsEveryBlock covers the block types a real turn
// carries.
func TestEstimateRequestTokensCountsEveryBlock(t *testing.T) {
	n := estimateRequestTokens(&anthRequest{
		Messages: []anthMessage{
			{Role: "assistant", Content: json.RawMessage(`[{"type":"tool_use","id":"c","name":"read","input":{"path":"a.go"}}]`)},
			{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"c","content":"package a"},{"type":"text","text":"next"}]`)},
		},
	})
	if n < 15 {
		t.Fatalf("tokens = %d", n)
	}
}
