package llmgateway

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// A role the OpenAI wire format has no message for is refused, not dropped.
func TestGeminiRefusesARoleItCannotTranslate(t *testing.T) {
	u := &upstream{chatBody: `{"id":"c1","model":"m","choices":[{"message":{"content":"ok"}}]}`}
	f := geminiFixture(t, u)

	status, body := f.do(t, http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent",
		`{"contents":[{"role":"tool","parts":[{"text":"hi"}]}]}`)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d %v", status, body)
	}
	if len(u.requests) != 0 {
		t.Fatal("the provider was called for a request that cannot be translated")
	}
}

// A file part points at a URL only Google can read, so it is refused by name.
func TestGeminiRefusesAFilePart(t *testing.T) {
	f := geminiFixture(t, &upstream{})

	status, body := f.do(t, http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent",
		`{"contents":[{"role":"user","parts":[{"fileData":{"fileUri":"gs://bucket/a.pdf"}}]}]}`)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d %v", status, body)
	}
	errBody, _ := body["error"].(map[string]any)
	if !strings.Contains(errBody["message"].(string), "inlineData") {
		t.Fatalf("message = %v", errBody["message"])
	}
}

// An image travels as a data URI, and a thought stays behind: no OpenAI field
// carries reasoning.
func TestGeminiSendsImagesAndDropsThoughts(t *testing.T) {
	u := &upstream{chatBody: `{"id":"c1","model":"m","choices":[{"message":{"content":"ok"}}]}`}
	f := geminiFixture(t, u)

	status, body := f.do(t, http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent",
		`{"contents":[{"role":"user","parts":[{"text":"look"},{"thought":true,"text":"thinking"},
			{"inlineData":{"mimeType":"image/png","data":"AAAA"}}]}]}`)
	if status != http.StatusOK {
		t.Fatalf("status = %d %v", status, body)
	}
	msgs, _ := sentBody(t, u)["messages"].([]any)
	msg, _ := msgs[0].(map[string]any)
	parts, _ := msg["content"].([]any)
	if len(parts) != 2 {
		t.Fatalf("parts = %v, want text and image", parts)
	}
	image, _ := parts[1].(map[string]any)
	url, _ := image["image_url"].(map[string]any)
	if url["url"] != "data:image/png;base64,AAAA" {
		t.Fatalf("image url = %v", url["url"])
	}
	for _, p := range parts {
		part, _ := p.(map[string]any)
		if text, _ := part["text"].(string); strings.Contains(text, "thinking") {
			t.Fatal("a thought reached the provider")
		}
	}
}

// The calling mode maps onto the OpenAI choice for every mode, including the
// one that names no function.
func TestGeminiToolChoiceModes(t *testing.T) {
	cases := []struct {
		name string
		cfg  *gemToolConfig
		want any
	}{
		{"no config", nil, "auto"},
		{"empty config", &gemToolConfig{}, "auto"},
		{"auto", &gemToolConfig{FunctionCallingConfig: &gemCallingConfig{Mode: "AUTO"}}, "auto"},
		{"none", &gemToolConfig{FunctionCallingConfig: &gemCallingConfig{Mode: "NONE"}}, "none"},
		{"any", &gemToolConfig{FunctionCallingConfig: &gemCallingConfig{Mode: "ANY"}}, "required"},
		{"any with one name", &gemToolConfig{FunctionCallingConfig: &gemCallingConfig{
			Mode: "ANY", AllowedFunctionNames: []string{"read"}}},
			map[string]any{"type": "function", "function": map[string]string{"name": "read"}}},
	}
	for _, c := range cases {
		got := geminiToolChoice(c.cfg)
		if !sameJSON(t, got, c.want) {
			t.Fatalf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

// sameJSON compares two values by their JSON encoding, which is what the
// provider sees.
func sameJSON(t *testing.T, a, b any) bool {
	t.Helper()
	ea, _ := json.Marshal(a)
	eb, _ := json.Marshal(b)
	return string(ea) == string(eb)
}

// A JSON schema request keeps the schema; a bare JSON request asks for an
// object; anything else sets no format at all.
func TestGeminiResponseFormatModes(t *testing.T) {
	schema := geminiResponseFormat(&gemGeneration{
		ResponseJSONSchema: json.RawMessage(`{"type":"object"}`),
	})
	if !strings.Contains(mustJSON(t, schema), `"json_schema"`) {
		t.Fatalf("schema format = %s", mustJSON(t, schema))
	}
	object := geminiResponseFormat(&gemGeneration{ResponseMimeType: "application/json"})
	if !strings.Contains(mustJSON(t, object), `"json_object"`) {
		t.Fatalf("object format = %s", mustJSON(t, object))
	}
	if naked := geminiResponseFormat(&gemGeneration{ResponseMimeType: "text/plain"}); naked != nil {
		t.Fatalf("plain text set a format: %v", naked)
	}
	if null := geminiResponseFormat(&gemGeneration{ResponseJSONSchema: json.RawMessage(`null`)}); null != nil {
		t.Fatalf("a null schema set a format: %v", null)
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(raw)
}

// A declaration without a name cannot become an OpenAI tool.
func TestGeminiRefusesAToolWithoutAName(t *testing.T) {
	f := geminiFixture(t, &upstream{})

	status, body := f.do(t, http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent",
		`{"contents":[{"role":"user","parts":[{"text":"hi"}]}],
		  "tools":[{"functionDeclarations":[{"description":"no name"}]}]}`)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d %v", status, body)
	}
}

// A client that asks about one model gets it, and an unknown name is a 404.
func TestGeminiAnswersAModelLookup(t *testing.T) {
	u := &upstream{listBody: `{"data":[{"id":"m-one"}]}`}
	f := geminiFixture(t, u)

	status, body := f.do(t, http.MethodGet, "/v1beta/models/m-one", "")
	if status != http.StatusOK {
		t.Fatalf("lookup = %d %v", status, body)
	}
	if body["name"] != "models/m-one" {
		t.Fatalf("name = %v", body["name"])
	}
	status, body = f.do(t, http.MethodGet, "/v1beta/models/nope", "")
	if status != http.StatusNotFound {
		t.Fatalf("unknown model = %d %v", status, body)
	}
}

// The list fails closed: no provider is a 503, a provider that refuses is a 502.
func TestGeminiListFailsClosed(t *testing.T) {
	f := newHTTPFixture(t, 1, 1, "member")
	if status, body := f.do(t, http.MethodGet, "/v1beta/models", ""); status != http.StatusServiceUnavailable {
		t.Fatalf("no provider = %d %v", status, body)
	}

	u := &upstream{listBody: `{"error":"nope"}`, listCode: http.StatusInternalServerError}
	g := geminiFixture(t, u)
	if status, body := g.do(t, http.MethodGet, "/v1beta/models", ""); status != http.StatusBadGateway {
		t.Fatalf("refusing provider = %d %v", status, body)
	}
}

// A provider that refuses the turn is reported in the Gemini error shape.
func TestGeminiReportsAProviderRefusal(t *testing.T) {
	u := &upstream{chatBody: `{"error":"slow down"}`, chatCode: http.StatusTooManyRequests}
	f := geminiFixture(t, u)

	status, body := f.do(t, http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent",
		`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	if status != http.StatusBadGateway {
		t.Fatalf("status = %d %v", status, body)
	}
	errBody, _ := body["error"].(map[string]any)
	if !strings.Contains(errBody["message"].(string), "slow down") {
		t.Fatalf("message = %v", errBody["message"])
	}
}

// An answer that is not a completion is refused rather than shown as an empty
// turn.
func TestGeminiRefusesAnAnswerThatIsNotACompletion(t *testing.T) {
	u := &upstream{chatBody: `not json`}
	f := geminiFixture(t, u)

	status, body := f.do(t, http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent",
		`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	if status != http.StatusBadGateway {
		t.Fatalf("status = %d %v", status, body)
	}
}

// Tool arguments that are not JSON cannot become a functionCall.
func TestGeminiRefusesToolArgumentsThatAreNotJSON(t *testing.T) {
	u := &upstream{chatBody: `{"id":"c1","model":"m","choices":[{"message":{"content":"",
		"tool_calls":[{"id":"call_0","type":"function","function":{"name":"read","arguments":"{"}}]},
		"finish_reason":"tool_calls"}]}`}
	f := geminiFixture(t, u)

	status, body := f.do(t, http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent",
		`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	if status != http.StatusBadGateway {
		t.Fatalf("status = %d %v", status, body)
	}
}

// The same failure inside a stream closes it with an error frame the client can
// read, instead of a frame its parser rejects.
func TestGeminiStreamsTheToolArgumentFailure(t *testing.T) {
	u := &upstream{chatBody: strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_0","type":"function","function":{"name":"read","arguments":"{"}}]}}]}`,
		`data: [DONE]`,
		``,
	}, "\n\n")}
	f := geminiFixture(t, u)

	status, body := doRaw(t, f, "/v1beta/models/gemini-2.5-pro:streamGenerateContent?alt=sse")
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if !strings.Contains(body, `"status":"UNAVAILABLE"`) {
		t.Fatalf("no error frame in:\n%s", body)
	}
}

// doRaw returns the raw body, which is what a stream is.
func doRaw(t *testing.T, f *httpFixture, path string) (int, string) {
	t.Helper()
	resp, err := http.Post(f.server.URL+path, "application/json",
		strings.NewReader(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw)
}

// A refusing provider ends the stream before any frame, in the Gemini error
// shape.
func TestGeminiStreamReportsAProviderRefusal(t *testing.T) {
	u := &upstream{chatBody: `{"error":"down"}`, chatCode: http.StatusServiceUnavailable}
	f := geminiFixture(t, u)

	status, body := doRaw(t, f, "/v1beta/models/gemini-2.5-pro:streamGenerateContent?alt=sse")
	if status != http.StatusBadGateway {
		t.Fatalf("status = %d body %s", status, body)
	}
	if !strings.Contains(body, `"status":"UNAVAILABLE"`) {
		t.Fatalf("body = %s", body)
	}
}

// The non-SSE stream form answers a JSON array, as the protocol says.
func TestGeminiStreamWithoutSSEAnswersAnArray(t *testing.T) {
	u := &upstream{chatBody: `{"id":"c1","model":"m","choices":[{"message":{"content":"hi"},"finish_reason":"length"}]}`}
	f := geminiFixture(t, u)

	status, body := f.do(t, http.MethodPost, "/v1beta/models/gemini-2.5-pro:streamGenerateContent",
		`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	if status != http.StatusOK {
		t.Fatalf("status = %d %v", status, body)
	}
	if _, ok := body["candidates"]; ok {
		t.Fatalf("a single object came back, want an array: %v", body)
	}
}

// A response with no choices is still a valid turn, and a content filter is
// reported as SAFETY.
func TestGeminiMapsEmptyAndFilteredAnswers(t *testing.T) {
	u := &upstream{chatBody: `{"choices":[]}`}
	f := geminiFixture(t, u)
	status, body := f.do(t, http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent",
		`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	if status != http.StatusOK {
		t.Fatalf("status = %d %v", status, body)
	}
	candidates, _ := body["candidates"].([]any)
	if len(candidates) != 1 {
		t.Fatalf("candidates = %v", candidates)
	}

	u.chatBody = `{"choices":[{"message":{"content":""},"finish_reason":"content_filter"}]}`
	status, body = f.do(t, http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent",
		`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	if status != http.StatusOK {
		t.Fatalf("status = %d %v", status, body)
	}
	candidate, _ := body["candidates"].([]any)[0].(map[string]any)
	if candidate["finishReason"] != "SAFETY" {
		t.Fatalf("finishReason = %v, want SAFETY", candidate["finishReason"])
	}
}

// A history that answers a call it did not carry keeps a stable id, so the
// provider can pair the two.
func TestGeminiAnswersAnOrphanFunctionResponse(t *testing.T) {
	u := &upstream{chatBody: `{"id":"c1","model":"m","choices":[{"message":{"content":"ok"}}]}`}
	f := geminiFixture(t, u)

	status, body := f.do(t, http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent",
		`{"contents":[{"role":"user","parts":[{"functionResponse":{"name":"read","response":{}}}]}]}`)
	if status != http.StatusOK {
		t.Fatalf("status = %d %v", status, body)
	}
	msgs, _ := sentBody(t, u)["messages"].([]any)
	msg, _ := msgs[0].(map[string]any)
	if msg["tool_call_id"] != "call_read" || msg["content"] != "{}" {
		t.Fatalf("tool message = %v", msg)
	}
}

// The context meter counts a whole turn: system prompt, tools, calls, results
// and images.
func TestGeminiCountsAWholeTurn(t *testing.T) {
	f := geminiFixture(t, &upstream{})

	status, body := f.do(t, http.MethodPost, "/v1beta/models/gemini-2.5-pro:countTokens", `{
		"systemInstruction":{"parts":[{"text":"Be brief."}]},
		"contents":[
			{"role":"user","parts":[{"text":"read a.go"},{"inlineData":{"mimeType":"image/png","data":"AAAA"}}]},
			{"role":"model","parts":[{"functionCall":{"name":"read","args":{"path":"a.go"}}}]},
			{"role":"user","parts":[{"functionResponse":{"name":"read","response":{"output":"package a"}}}]}
		],
		"tools":[{"functionDeclarations":[{"name":"read","description":"Read a file",
			"parameters":{"type":"object"}}]}]
	}`)
	if status != http.StatusOK {
		t.Fatalf("countTokens = %d %v", status, body)
	}
	total, _ := body["totalTokens"].(float64)
	if total < 1600 {
		t.Fatalf("totalTokens = %v, want the image charged as a block", total)
	}
}

// A path segment with an action but no model cannot be served.
func TestGeminiRefusesAPathWithoutAModel(t *testing.T) {
	f := geminiFixture(t, &upstream{})

	status, body := f.do(t, http.MethodPost, "/v1beta/models/:generateContent", `{}`)
	if status != http.StatusNotFound {
		t.Fatalf("status = %d %v", status, body)
	}
}

// A request outside a workspace cannot be attributed, so the action refuses.
func TestGeminiRefusesAnUnattributedRequest(t *testing.T) {
	f := geminiFixture(t, &upstream{})
	f.plugin.pctx = nil

	if status, _ := f.do(t, http.MethodPost, "/v1beta/models/gemini-2.5-pro:countTokens", `{}`); status != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", status)
	}
}

// The provider selector is parsed strictly, on the list and on the action.
func TestGeminiRefusesAnInvalidProviderSelector(t *testing.T) {
	f := geminiFixture(t, &upstream{})

	if status, body := f.do(t, http.MethodGet, "/v1beta/models?provider=12abc", ""); status != http.StatusBadRequest {
		t.Fatalf("list = %d %v", status, body)
	}
	if status, body := f.do(t, http.MethodPost, "/v1beta/models/gemini-2.5-pro:countTokens?provider=0", `{}`); status != http.StatusBadRequest {
		t.Fatalf("action = %d %v", status, body)
	}
}

// A workspace with no provider cannot serve a Gemini turn either.
func TestGeminiActionRefusesWithoutAProvider(t *testing.T) {
	f := newHTTPFixture(t, 1, 1, "member")

	status, body := f.do(t, http.MethodPost, "/v1beta/models/gemini-2.5-pro:countTokens", `{}`)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d %v", status, body)
	}
	errBody, _ := body["error"].(map[string]any)
	if errBody["status"] != "FAILED_PRECONDITION" {
		t.Fatalf("error = %v", body)
	}
}

// A model lookup reads the provider's list, so a refusal is reported.
func TestGeminiModelLookupReportsAProviderRefusal(t *testing.T) {
	u := &upstream{listBody: `{"error":"nope"}`, listCode: http.StatusInternalServerError}
	f := geminiFixture(t, u)

	if status, body := f.do(t, http.MethodGet, "/v1beta/models/m-one", ""); status != http.StatusBadGateway {
		t.Fatalf("status = %d %v", status, body)
	}
}

// A function response that carries no object still has to say something, and
// the tool message gets an empty object rather than nothing.
func TestGeminiAnswersAFunctionResponseWithoutAnObject(t *testing.T) {
	u := &upstream{chatBody: `{"id":"c1","model":"m","choices":[{"message":{"content":"ok"}}]}`}
	f := geminiFixture(t, u)

	status, body := f.do(t, http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent",
		`{"contents":[{"role":"model","parts":[{"functionCall":{"name":"read"}}]},
			{"role":"user","parts":[{"functionResponse":{"name":"read","response":null}}]}]}`)
	if status != http.StatusOK {
		t.Fatalf("status = %d %v", status, body)
	}
	msgs, _ := sentBody(t, u)["messages"].([]any)
	msg, _ := msgs[1].(map[string]any)
	if msg["content"] != "{}" {
		t.Fatalf("tool content = %v, want an empty object", msg["content"])
	}
}

// A declaration that names no parameters still becomes a tool with an object
// schema, and a call whose arguments are literally null becomes an empty one.
func TestGeminiDefaultsAnEmptySchemaAndEmptyArguments(t *testing.T) {
	u := &upstream{chatBody: `{"id":"c1","model":"m","choices":[{"message":{"content":"ok"}}]}`}
	f := geminiFixture(t, u)

	status, body := f.do(t, http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent",
		`{"contents":[{"role":"user","parts":[{"text":"hi"}]},
			{"role":"model","parts":[{"functionCall":{"name":"ping","args":null}}]}],
		  "tools":[{"functionDeclarations":[{"name":"ping"}]}]}`)
	if status != http.StatusOK {
		t.Fatalf("status = %d %v", status, body)
	}
	sent := sentBody(t, u)
	tools, _ := sent["tools"].([]any)
	tool, _ := tools[0].(map[string]any)
	fn, _ := tool["function"].(map[string]any)
	if params, _ := fn["parameters"].(map[string]any); params["type"] != "object" {
		t.Fatalf("parameters = %v, want an object schema", fn["parameters"])
	}
	msgs, _ := sent["messages"].([]any)
	assistant, _ := msgs[1].(map[string]any)
	calls, _ := assistant["tool_calls"].([]any)
	call, _ := calls[0].(map[string]any)
	args, _ := call["function"].(map[string]any)
	if args["arguments"] != "{}" {
		t.Fatalf("arguments = %v, want {}", args["arguments"])
	}
}

// A turn made only of thoughts carries no text, so the provider still receives
// one user message rather than an empty request.
func TestGeminiSendNoTextFromAThoughtOnlyTurn(t *testing.T) {
	u := &upstream{chatBody: `{"id":"c1","model":"m","choices":[{"message":{"content":"ok"}}]}`}
	f := geminiFixture(t, u)

	status, body := f.do(t, http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent",
		`{"systemInstruction":{"parts":[{"thought":true,"text":"plan"}]},
		  "contents":[{"role":"user","parts":[{"thought":true,"text":"plan"}]}]}`)
	if status != http.StatusOK {
		t.Fatalf("status = %d %v", status, body)
	}
	msgs, _ := sentBody(t, u)["messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("messages = %v, want one user turn and no system prompt", msgs)
	}
	msg, _ := msgs[0].(map[string]any)
	if msg["role"] != "user" {
		t.Fatalf("message = %v", msg)
	}
}

// A provider that cannot be reached is reported, not retried.
func TestGeminiReportsAnUnreachableProvider(t *testing.T) {
	f := newHTTPFixture(t, 1, 1, "member")
	createProvider(t, f, ProviderInput{Name: "stub", Backend: "openai", APIKey: "k-test",
		BaseURL: "http://127.0.0.1:1", Model: "stub-model"})

	status, body := f.do(t, http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent",
		`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	if status != http.StatusBadGateway {
		t.Fatalf("status = %d %v", status, body)
	}
}

// A model path that carries an action is not a model.
func TestGeminiModelLookupRefuses(t *testing.T) {
	f := geminiFixture(t, &upstream{listBody: `{"data":[{"id":"m-one"}]}`})
	if status, body := f.do(t, http.MethodGet, "/v1beta/models/m-one:generateContent", ""); status != http.StatusNotFound {
		t.Fatalf("action in a model path = %d %v", status, body)
	}
}

// A workspace with no provider cannot answer a lookup either.
func TestGeminiModelLookupNeedsAProvider(t *testing.T) {
	f := newHTTPFixture(t, 1, 1, "member")
	if status, body := f.do(t, http.MethodGet, "/v1beta/models/m-one", ""); status != http.StatusServiceUnavailable {
		t.Fatalf("no provider = %d %v", status, body)
	}
}

// A tool call that streams with no arguments is still a call with an object.
func TestGeminiStreamsACallWithoutArguments(t *testing.T) {
	u := &upstream{chatBody: strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_0","type":"function","function":{"name":"ping"}}]}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
		``,
	}, "\n\n")}
	f := geminiFixture(t, u)

	status, body := doRaw(t, f, "/v1beta/models/gemini-2.5-pro:streamGenerateContent?alt=sse")
	if status != http.StatusOK {
		t.Fatalf("status = %d body %s", status, body)
	}
	if !strings.Contains(body, `"functionCall":{"name":"ping","args":{}}`) {
		t.Fatalf("no call frame in:\n%s", body)
	}
}

// An answer whose tool call carries no arguments becomes a call with an empty
// object, not an invalid one.
func TestGeminiAnswersACallWithoutArguments(t *testing.T) {
	u := &upstream{chatBody: `{"id":"c1","model":"m","choices":[{"message":{"content":"",
		"tool_calls":[{"id":"call_0","type":"function","function":{"name":"ping","arguments":""}}]},
		"finish_reason":"tool_calls"}]}`}
	f := geminiFixture(t, u)

	status, body := f.do(t, http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent",
		`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	if status != http.StatusOK {
		t.Fatalf("status = %d %v", status, body)
	}
	candidate, _ := body["candidates"].([]any)[0].(map[string]any)
	content, _ := candidate["content"].(map[string]any)
	parts, _ := content["parts"].([]any)
	part, _ := parts[0].(map[string]any)
	call, _ := part["functionCall"].(map[string]any)
	if len(call["args"].(map[string]any)) != 0 {
		t.Fatalf("args = %v, want {}", call["args"])
	}
}
