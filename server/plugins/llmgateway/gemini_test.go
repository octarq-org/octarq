package llmgateway

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func geminiFixture(t *testing.T, u *upstream) *httpFixture {
	t.Helper()
	f := newHTTPFixture(t, 1, 1, "member")
	id := createProvider(t, f, ProviderInput{Name: "stub", Backend: "openai", APIKey: "k-test",
		BaseURL: "http://placeholder.invalid", Model: "stub-model"})
	withUpstream(t, f, u, id)
	return f
}

func TestGeminiModelsComesFromTheProvider(t *testing.T) {
	u := &upstream{listBody: `{"data":[{"id":"m-one"},{"id":"m-two"}]}`}
	f := geminiFixture(t, u)

	status, body := f.do(t, http.MethodGet, "/v1beta/models", "")
	if status != http.StatusOK {
		t.Fatalf("models = %d %v", status, body)
	}
	models, _ := body["models"].([]any)
	if len(models) != 2 {
		t.Fatalf("models = %v, want m-one and m-two", body["models"])
	}
	first, _ := models[0].(map[string]any)
	if first["name"] != "models/m-one" {
		t.Fatalf("name = %v, want models/m-one", first["name"])
	}
	methods, _ := first["supportedGenerationMethods"].([]any)
	if len(methods) != 3 || methods[0] != "generateContent" {
		t.Fatalf("methods = %v", first["supportedGenerationMethods"])
	}
}

func TestGeminiGeneratesContentTranslatesTheTurn(t *testing.T) {
	u := &upstream{chatBody: `{"id":"c1","model":"m","choices":[{"message":{"content":"hello"},
		"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":3}}`}
	f := geminiFixture(t, u)

	req := `{
		"systemInstruction":{"parts":[{"text":"You are terse."}]},
		"contents":[{"role":"user","parts":[{"text":"hi"}]}],
		"tools":[{"functionDeclarations":[{"name":"read","description":"Read a file",
			"parameters":{"type":"object","properties":{"path":{"type":"string"}}}}]}],
		"toolConfig":{"functionCallingConfig":{"mode":"ANY","allowedFunctionNames":["read"]}},
		"generationConfig":{"temperature":0.2,"maxOutputTokens":256,"responseMimeType":"application/json"}
	}`
	status, body := f.do(t, http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent", req)
	if status != http.StatusOK {
		t.Fatalf("generate = %d %v", status, body)
	}

	sent := sentBody(t, u)
	if sent["model"] != "stub-model" {
		t.Fatalf("upstream model = %v, want the provider's configured model", sent["model"])
	}
	if sent["max_tokens"] != float64(256) || sent["temperature"] != 0.2 {
		t.Fatalf("generation config did not reach the provider: %v", sent)
	}
	if format, _ := sent["response_format"].(map[string]any); format["type"] != "json_object" {
		t.Fatalf("response_format = %v, want json_object", sent["response_format"])
	}
	choice, _ := sent["tool_choice"].(map[string]any)
	fn, _ := choice["function"].(map[string]any)
	if fn["name"] != "read" {
		t.Fatalf("tool_choice = %v, want the one allowed function", sent["tool_choice"])
	}
	msgs, _ := sent["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("messages = %v, want system and user", msgs)
	}
	sys, _ := msgs[0].(map[string]any)
	if sys["role"] != "system" || sys["content"] != "You are terse." {
		t.Fatalf("system message = %v", sys)
	}
	tools, _ := sent["tools"].([]any)
	tool, _ := tools[0].(map[string]any)
	if tool["type"] != "function" {
		t.Fatalf("tools = %v", tools)
	}

	candidates, _ := body["candidates"].([]any)
	if len(candidates) != 1 {
		t.Fatalf("candidates = %v", body["candidates"])
	}
	candidate, _ := candidates[0].(map[string]any)
	if candidate["finishReason"] != "STOP" {
		t.Fatalf("finishReason = %v", candidate["finishReason"])
	}
	content, _ := candidate["content"].(map[string]any)
	parts, _ := content["parts"].([]any)
	part, _ := parts[0].(map[string]any)
	if part["text"] != "hello" {
		t.Fatalf("parts = %v", parts)
	}
	usage, _ := body["usageMetadata"].(map[string]any)
	if usage["promptTokenCount"] != float64(11) || usage["totalTokenCount"] != float64(14) {
		t.Fatalf("usageMetadata = %v", usage)
	}
}

func TestGeminiPairsFunctionCallsWithTheirResponses(t *testing.T) {
	u := &upstream{chatBody: `{"id":"c1","model":"m","choices":[{"message":{"content":""},
		"finish_reason":"stop"}]}`}
	f := geminiFixture(t, u)

	req := `{
		"contents":[
			{"role":"user","parts":[{"text":"read a.go"}]},
			{"role":"model","parts":[{"functionCall":{"name":"read","args":{"path":"a.go"}}}]},
			{"role":"model","parts":[{"functionCall":{"name":"read","args":{"path":"b.go"}}}]},
			{"role":"user","parts":[{"functionResponse":{"name":"read","response":{"output":"package a"}}},
				{"functionResponse":{"name":"read","response":{"output":"package b"}}}]}
		]
	}`
	if status, body := f.do(t, http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent", req); status != http.StatusOK {
		t.Fatalf("generate = %d %v", status, body)
	}

	msgs, _ := sentBody(t, u)["messages"].([]any)
	if len(msgs) != 5 {
		t.Fatalf("messages = %d %v, want user, two assistant calls, two tool answers", len(msgs), msgs)
	}
	first, _ := msgs[1].(map[string]any)
	calls, _ := first["tool_calls"].([]any)
	call, _ := calls[0].(map[string]any)
	if call["id"] != "call_0" {
		t.Fatalf("first call id = %v", call["id"])
	}
	second, _ := msgs[2].(map[string]any)
	secondCalls, _ := second["tool_calls"].([]any)
	secondCall, _ := secondCalls[0].(map[string]any)
	if secondCall["id"] != "call_1" {
		t.Fatalf("second call id = %v", secondCall["id"])
	}
	for i, want := range []struct{ id, output string }{{"call_0", "package a"}, {"call_1", "package b"}} {
		msg, _ := msgs[3+i].(map[string]any)
		if msg["role"] != "tool" || msg["tool_call_id"] != want.id {
			t.Fatalf("tool %d = %v, want id %s", i, msg, want.id)
		}
		text, _ := msg["content"].(string)
		if !strings.Contains(text, want.output) {
			t.Fatalf("tool %d content = %q, want %q", i, text, want.output)
		}
	}
}

func TestGeminiStreamsFrames(t *testing.T) {
	u := &upstream{chatBody: strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"Hel"}}]}`,
		`data: {"choices":[{"delta":{"content":"lo"}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_2","type":"function","function":{"name":"read","arguments":""}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"path\":"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"a.go\"}"}}]}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: {"choices":[],"usage":{"prompt_tokens":7,"completion_tokens":42}}`,
		`data: [DONE]`,
		``,
	}, "\n\n")}
	f := geminiFixture(t, u)

	resp, err := http.Post(f.server.URL+"/v1beta/models/gemini-2.5-pro:streamGenerateContent?alt=sse",
		"application/json", strings.NewReader(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`))
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
		`"text":"Hel"`,
		`"text":"lo"`,
		`"functionCall":{"name":"read","args":{"path":"a.go"}}`,
		`"finishReason":"STOP"`,
		`"totalTokenCount":49`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
	for _, line := range strings.Split(text, "\r\n\r\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var frame map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &frame); err != nil {
			t.Fatalf("frame is not JSON: %q (%v)", line, err)
		}
		candidates, _ := frame["candidates"].([]any)
		for _, c := range candidates {
			candidate, _ := c.(map[string]any)
			content, _ := candidate["content"].(map[string]any)
			if _, ok := content["parts"].([]any); !ok {
				t.Fatalf("a candidate has no parts list: %v", candidate)
			}
		}
	}
	if sent := sentBody(t, u); sent["stream"] != true {
		t.Fatalf("the provider was not asked to stream: %v", sent)
	}
}

func TestGeminiRefusesAProviderWithoutAModel(t *testing.T) {
	f := newHTTPFixture(t, 1, 1, "member")
	createProvider(t, f, ProviderInput{Name: "stub", Backend: "openai", APIKey: "k-test",
		BaseURL: "http://placeholder.invalid"})

	status, body := f.do(t, http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent",
		`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d %v", status, body)
	}
	errBody, _ := body["error"].(map[string]any)
	if errBody["status"] != "FAILED_PRECONDITION" {
		t.Fatalf("error = %v", body)
	}
}

func TestGeminiModelSelectorNamesAnotherModel(t *testing.T) {
	u := &upstream{chatBody: `{"id":"c1","model":"m","choices":[{"message":{"content":"ok"}}]}`}
	f := geminiFixture(t, u)

	status, _ := f.do(t, http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent?model=other-model",
		`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if sent := sentBody(t, u); sent["model"] != "other-model" {
		t.Fatalf("upstream model = %v, want other-model", sent["model"])
	}
}

func TestGeminiTreatsAMissingRoleAsUser(t *testing.T) {
	u := &upstream{chatBody: `{"id":"c1","model":"m","choices":[{"message":{"content":"ok"}}]}`}
	f := geminiFixture(t, u)

	status, body := f.do(t, http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent",
		`{"contents":[{"parts":[{"text":"hi"}]}]}`)
	if status != http.StatusOK {
		t.Fatalf("generate = %d %v", status, body)
	}
	msgs, _ := sentBody(t, u)["messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("messages = %v", msgs)
	}
	msg, _ := msgs[0].(map[string]any)
	if msg["role"] != "user" {
		t.Fatalf("role = %v, want user", msg["role"])
	}
}

func TestGeminiCountTokens(t *testing.T) {
	u := &upstream{}
	f := geminiFixture(t, u)

	status, body := f.do(t, http.MethodPost, "/v1beta/models/gemini-2.5-pro:countTokens",
		`{"contents":[{"role":"user","parts":[{"text":"`+strings.Repeat("word ", 40)+`"}]}]}`)
	if status != http.StatusOK {
		t.Fatalf("countTokens = %d %v", status, body)
	}
	if total, _ := body["totalTokens"].(float64); total <= 0 {
		t.Fatalf("totalTokens = %v", body["totalTokens"])
	}
	if len(u.requests) != 0 {
		t.Fatalf("countTokens called the provider %d times", len(u.requests))
	}
}

func TestGeminiRefusesAnUnknownAction(t *testing.T) {
	f := geminiFixture(t, &upstream{})

	status, body := f.do(t, http.MethodPost, "/v1beta/models/gemini-2.5-pro:embedContent", `{}`)
	if status != http.StatusNotFound {
		t.Fatalf("status = %d %v", status, body)
	}
}
