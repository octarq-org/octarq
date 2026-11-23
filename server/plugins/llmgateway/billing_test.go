package llmgateway

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/plugin"
)

const (
	vkToken   = virtualKeyPrefix + "valid"
	usageJSON = `{"id":"chatcmpl-bill","model":"stub-model","choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":30,"completion_tokens":7,"prompt_tokens_details":{"cached_tokens":10}}}`
)

func activeAccount(allowed ...string) map[string]*plugin.SpendAccountAuth {
	return map[string]*plugin.SpendAccountAuth{
		vkToken: {
			OrgID:         1,
			AccountID:     "acct-1",
			Status:        "active",
			BudgetLimit:   50,
			BudgetUsed:    1,
			AllowedModels: allowed,
		},
	}
}

func newVirtualKeyFixture(t *testing.T, chatBody string, accounts map[string]*plugin.SpendAccountAuth) (*httpFixture, *upstream, *mockGatewaySpendLedger) {
	t.Helper()
	f := newHTTPFixture(t, 0, 0, "")
	id := createProvider(t, f, ProviderInput{
		Name: "stub", Backend: "openai", APIKey: "k-test",
		BaseURL: "http://placeholder.invalid", Model: "stub-model",
	})
	u := &upstream{chatBody: chatBody}
	withUpstream(t, f, u, id)
	ledger := &mockGatewaySpendLedger{accounts: accounts}
	f.plugin.pctx.Host = &plugin.TestHost{SessionMock: &plugin.TestSession{}, SpendLedgerMock: ledger}
	return f, u, ledger
}

func vkBearer(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token}
}

func vkDo(t *testing.T, f *httpFixture, method, path string, headers map[string]string, body string) (int, string) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, f.server.URL+path, reader)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, string(raw)
}

func recordedSpends(ledger *mockGatewaySpendLedger) []plugin.TurnSpend {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	return append([]plugin.TurnSpend(nil), ledger.recorded...)
}

func authorizedCalls(ledger *mockGatewaySpendLedger) []authorizeCall {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	return append([]authorizeCall(nil), ledger.authorized...)
}

func streamWithUsage(id string) string {
	return "data: {\"id\":\"" + id + "\",\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n" +
		"data: {\"id\":\"" + id + "\",\"choices\":[],\"usage\":{\"prompt_tokens\":30,\"completion_tokens\":7,\"prompt_tokens_details\":{\"cached_tokens\":10}}}\n\n" +
		"data: [DONE]\n\n"
}

func TestVirtualKeyChatInjectsStreamUsage(t *testing.T) {
	f, u, ledger := newVirtualKeyFixture(t, streamWithUsage("chatcmpl-stream"), activeAccount("stub-model"))
	body := `{"model":"stub-model","stream":true,"messages":[{"role":"user","content":"hi"}],"stream_options":{"trace":"keep"}}`
	status, out := vkDo(t, f, http.MethodPost, AgentChatPath, vkBearer(vkToken), body)
	if status != http.StatusOK {
		t.Fatalf("chat = %d %s", status, out)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(u.last(t).body), &sent); err != nil {
		t.Fatalf("upstream body is not JSON: %v", err)
	}
	if sent["stream"] != true {
		t.Fatalf("stream = %v, want true", sent["stream"])
	}
	options, _ := sent["stream_options"].(map[string]any)
	if options["include_usage"] != true {
		t.Fatalf("stream_options = %v, want include_usage true", sent["stream_options"])
	}
	if options["trace"] != "keep" {
		t.Fatalf("stream_options = %v, want the other keys kept", sent["stream_options"])
	}
	if sent["messages"] == nil {
		t.Fatalf("messages were dropped: %v", sent)
	}
	if spends := recordedSpends(ledger); len(spends) != 1 {
		t.Fatalf("recorded = %v, want one spend", spends)
	}
}

func TestVirtualKeyChatLeavesNonStreamBodyUnchanged(t *testing.T) {
	f, u, _ := newVirtualKeyFixture(t, usageJSON, activeAccount("stub-model"))
	body := `{"model":"stub-model","stream":false,"messages":[{"role":"user","content":"hi"}],"response_format":{"type":"json_object"}}`
	status, out := vkDo(t, f, http.MethodPost, AgentChatPath, vkBearer(vkToken), body)
	if status != http.StatusOK {
		t.Fatalf("chat = %d %s", status, out)
	}
	if got := u.last(t).body; got != body {
		t.Fatalf("the non-stream body changed:\n sent %s\n got  %s", body, got)
	}
}

func TestVirtualKeyChatBillsCachedTokensOnce(t *testing.T) {
	f, _, ledger := newVirtualKeyFixture(t, usageJSON, activeAccount("stub-model"))
	status, out := vkDo(t, f, http.MethodPost, AgentChatPath, vkBearer(vkToken),
		`{"model":"stub-model","messages":[{"role":"user","content":"hi"}]}`)
	if status != http.StatusOK {
		t.Fatalf("chat = %d %s", status, out)
	}
	spends := recordedSpends(ledger)
	if len(spends) != 1 {
		t.Fatalf("recorded = %v, want one spend", spends)
	}
	got := spends[0]
	if got.InputTokens != 20 || got.CacheReadTokens != 10 || got.OutputTokens != 7 || got.TurnID != "chatcmpl-bill" {
		t.Fatalf("spend = %+v, want input 20, cache read 10, output 7", got)
	}
}

func TestVirtualKeyChatStreamBillsCachedTokensOnce(t *testing.T) {
	f, _, ledger := newVirtualKeyFixture(t, streamWithUsage("chatcmpl-stream"), activeAccount("stub-model"))
	status, out := vkDo(t, f, http.MethodPost, AgentChatPath, vkBearer(vkToken),
		`{"model":"stub-model","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	if status != http.StatusOK {
		t.Fatalf("chat = %d %s", status, out)
	}
	spends := recordedSpends(ledger)
	if len(spends) != 1 {
		t.Fatalf("recorded = %v, want one spend", spends)
	}
	got := spends[0]
	if got.InputTokens != 20 || got.CacheReadTokens != 10 || got.OutputTokens != 7 || got.TurnID != "chatcmpl-stream" {
		t.Fatalf("spend = %+v, want input 20, cache read 10, output 7", got)
	}
}

func TestVirtualKeyChatAuthorizesBeforeUpstream(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
	}{
		{"unpriced", plugin.ErrSpendModelUnpriced, http.StatusBadRequest},
		{"paused", plugin.ErrSpendAccountPaused, http.StatusPaymentRequired},
		{"budget", plugin.ErrSpendBudgetExhausted, http.StatusPaymentRequired},
		{"not found", plugin.ErrSpendAccountNotFound, http.StatusNotFound},
		{"no ledger", plugin.ErrSpendUnavailable, http.StatusConflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, u, ledger := newVirtualKeyFixture(t, usageJSON, activeAccount("stub-model"))
			ledger.authorizeErr = tc.err
			status, out := vkDo(t, f, http.MethodPost, AgentChatPath, vkBearer(vkToken),
				`{"model":"stub-model","messages":[{"role":"user","content":"hi"}]}`)
			if status != tc.status {
				t.Fatalf("chat = %d %s, want %d", status, out, tc.status)
			}
			if len(u.requests) != 0 {
				t.Fatalf("the provider was called for a refused turn: %+v", u.requests)
			}
			calls := authorizedCalls(ledger)
			if len(calls) != 1 || calls[0].model != "stub-model" || calls[0].accountID != "acct-1" {
				t.Fatalf("authorized = %+v", calls)
			}
		})
	}
}

func TestVirtualKeyChatAuthorizesTheProviderDefaultModel(t *testing.T) {
	t.Run("allowed", func(t *testing.T) {
		f, _, ledger := newVirtualKeyFixture(t, usageJSON, activeAccount("stub-model"))
		status, out := vkDo(t, f, http.MethodPost, AgentChatPath, vkBearer(vkToken),
			`{"messages":[{"role":"user","content":"hi"}]}`)
		if status != http.StatusOK {
			t.Fatalf("chat = %d %s", status, out)
		}
		calls := authorizedCalls(ledger)
		if len(calls) != 1 || calls[0].model != "stub-model" {
			t.Fatalf("authorized = %+v, want the provider default", calls)
		}
	})

	t.Run("refused", func(t *testing.T) {
		f, u, ledger := newVirtualKeyFixture(t, usageJSON, activeAccount("other-model"))
		status, out := vkDo(t, f, http.MethodPost, AgentChatPath, vkBearer(vkToken),
			`{"messages":[{"role":"user","content":"hi"}]}`)
		if status != http.StatusForbidden {
			t.Fatalf("chat = %d %s, want 403", status, out)
		}
		if len(u.requests) != 0 || len(authorizedCalls(ledger)) != 0 {
			t.Fatalf("a refused model reached the provider or the ledger")
		}
	})
}

func TestVirtualKeyChatStreamBillsTheUsageAtTheEndOfALongStream(t *testing.T) {
	var body strings.Builder
	chunk := "data: {\"choices\":[{\"delta\":{\"content\":\"" + strings.Repeat("a", 1024) + "\"}}]}\n\n"
	for i := 0; i < 2100; i++ {
		body.WriteString(chunk)
	}
	body.WriteString("data: {\"id\":\"chatcmpl-tail\",\"usage\":{\"prompt_tokens\":50,\"completion_tokens\":9,\"prompt_tokens_details\":{\"cached_tokens\":10}}}\n\n")
	body.WriteString("data: [DONE]\n\n")
	if body.Len() <= 2<<20 {
		t.Fatalf("the stream is %d bytes, want more than 2 MiB", body.Len())
	}

	f, _, ledger := newVirtualKeyFixture(t, body.String(), activeAccount("stub-model"))
	status, out := vkDo(t, f, http.MethodPost, AgentChatPath, vkBearer(vkToken),
		`{"model":"stub-model","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	if status != http.StatusOK {
		t.Fatalf("chat = %d %s", status, out)
	}
	spends := recordedSpends(ledger)
	if len(spends) != 1 {
		t.Fatalf("recorded = %v, want the usage chunk at the tail", spends)
	}
	got := spends[0]
	if got.InputTokens != 40 || got.CacheReadTokens != 10 || got.OutputTokens != 9 || got.TurnID != "chatcmpl-tail" {
		t.Fatalf("spend = %+v, want the tail usage", got)
	}
}

func TestResponsesVirtualKeyBills(t *testing.T) {
	f, _, ledger := newVirtualKeyFixture(t, usageJSON, activeAccount("stub-model"))
	status, out := vkDo(t, f, http.MethodPost, AgentResponsesPath, vkBearer(vkToken),
		`{"model":"stub-model","input":"hi"}`)
	if status != http.StatusOK {
		t.Fatalf("responses = %d %s", status, out)
	}
	spends := recordedSpends(ledger)
	if len(spends) != 1 {
		t.Fatalf("recorded = %v, want one spend", spends)
	}
	got := spends[0]
	if got.InputTokens != 20 || got.CacheReadTokens != 10 || got.OutputTokens != 7 || got.Model != "stub-model" || got.TurnID != "chatcmpl-bill" {
		t.Fatalf("spend = %+v", got)
	}
	calls := authorizedCalls(ledger)
	if len(calls) != 1 || calls[0].model != "stub-model" {
		t.Fatalf("authorized = %+v", calls)
	}
}

func TestResponsesVirtualKeyStreamBills(t *testing.T) {
	f, _, ledger := newVirtualKeyFixture(t, streamWithUsage("chatcmpl-rs"), activeAccount("stub-model"))
	status, out := vkDo(t, f, http.MethodPost, AgentResponsesPath, vkBearer(vkToken),
		`{"model":"stub-model","stream":true,"input":"hi"}`)
	if status != http.StatusOK {
		t.Fatalf("responses = %d %s", status, out)
	}
	spends := recordedSpends(ledger)
	if len(spends) != 1 {
		t.Fatalf("recorded = %v, want one spend", spends)
	}
	got := spends[0]
	if got.InputTokens != 20 || got.CacheReadTokens != 10 || got.OutputTokens != 7 || got.TurnID != "chatcmpl-rs" {
		t.Fatalf("spend = %+v", got)
	}
}

func TestMessagesVirtualKeyBills(t *testing.T) {
	f, _, ledger := newVirtualKeyFixture(t, usageJSON, activeAccount("stub-model"))
	status, out := vkDo(t, f, http.MethodPost, AgentMessagesPath, vkBearer(vkToken),
		`{"model":"stub-model","max_tokens":32,"messages":[{"role":"user","content":"hi"}]}`)
	if status != http.StatusOK {
		t.Fatalf("messages = %d %s", status, out)
	}
	spends := recordedSpends(ledger)
	if len(spends) != 1 {
		t.Fatalf("recorded = %v, want one spend", spends)
	}
	got := spends[0]
	if got.InputTokens != 20 || got.CacheReadTokens != 10 || got.OutputTokens != 7 || got.TurnID != "chatcmpl-bill" {
		t.Fatalf("spend = %+v", got)
	}
	var answer map[string]any
	if err := json.Unmarshal([]byte(out), &answer); err != nil {
		t.Fatalf("answer is not JSON: %v", err)
	}
	usage, _ := answer["usage"].(map[string]any)
	if usage["input_tokens"] != float64(20) || usage["cache_read_input_tokens"] != float64(10) {
		t.Fatalf("usage = %v, want the cached split", usage)
	}
}

func TestMessagesVirtualKeyStreamBills(t *testing.T) {
	f, _, ledger := newVirtualKeyFixture(t, streamWithUsage("chatcmpl-ms"), activeAccount("stub-model"))
	status, out := vkDo(t, f, http.MethodPost, AgentMessagesPath, vkBearer(vkToken),
		`{"model":"stub-model","max_tokens":32,"stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	if status != http.StatusOK {
		t.Fatalf("messages = %d %s", status, out)
	}
	spends := recordedSpends(ledger)
	if len(spends) != 1 {
		t.Fatalf("recorded = %v, want one spend", spends)
	}
	got := spends[0]
	if got.InputTokens != 20 || got.CacheReadTokens != 10 || got.OutputTokens != 7 || got.TurnID != "chatcmpl-ms" {
		t.Fatalf("spend = %+v", got)
	}
}

func TestMessagesVirtualKeyRefusesADisallowedModel(t *testing.T) {
	f, u, _ := newVirtualKeyFixture(t, usageJSON, activeAccount("other-model"))
	status, out := vkDo(t, f, http.MethodPost, AgentMessagesPath, vkBearer(vkToken),
		`{"model":"stub-model","max_tokens":32,"messages":[{"role":"user","content":"hi"}]}`)
	if status != http.StatusForbidden {
		t.Fatalf("messages = %d %s, want 403", status, out)
	}
	if len(u.requests) != 0 {
		t.Fatalf("the provider was called for a refused model: %+v", u.requests)
	}
	var body map[string]any
	_ = json.Unmarshal([]byte(out), &body)
	if body["type"] != "error" {
		t.Fatalf("error body = %v, want the Anthropic shape", body)
	}
}

func TestVirtualKeyCountTokensIsFreeButEnforcesModels(t *testing.T) {
	f, u, ledger := newVirtualKeyFixture(t, usageJSON, activeAccount("stub-model"))
	status, out := vkDo(t, f, http.MethodPost, AgentCountTokensPath, vkBearer(vkToken),
		`{"model":"stub-model","messages":[{"role":"user","content":"hello there"}]}`)
	if status != http.StatusOK {
		t.Fatalf("count tokens = %d %s", status, out)
	}
	if len(u.requests) != 0 || len(recordedSpends(ledger)) != 0 {
		t.Fatalf("count tokens called the provider or billed a turn")
	}

	status, out = vkDo(t, f, http.MethodPost, AgentCountTokensPath, vkBearer(vkToken),
		`{"model":"other-model","messages":[{"role":"user","content":"hello there"}]}`)
	if status != http.StatusForbidden {
		t.Fatalf("disallowed count tokens = %d %s, want 403", status, out)
	}
}

func TestVirtualKeyModelListEndpointsAreFree(t *testing.T) {
	f, u, ledger := newVirtualKeyFixture(t, usageJSON, activeAccount())
	u.listBody = `{"data":[{"id":"m-one"}]}`
	for _, path := range []string{AgentModelsPath, AgentGeminiModelsPath, AgentOpencodeConfigPath} {
		status, out := vkDo(t, f, http.MethodGet, path, vkBearer(vkToken), "")
		if status != http.StatusOK {
			t.Fatalf("%s = %d %s", path, status, out)
		}
	}
	if len(recordedSpends(ledger)) != 0 {
		t.Fatalf("a model list billed a turn")
	}
}

func TestGeminiVirtualKeyBills(t *testing.T) {
	f, _, ledger := newVirtualKeyFixture(t, usageJSON, activeAccount("stub-model"))
	status, out := vkDo(t, f, http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent", vkBearer(vkToken),
		`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	if status != http.StatusOK {
		t.Fatalf("generate = %d %s", status, out)
	}
	spends := recordedSpends(ledger)
	if len(spends) != 1 {
		t.Fatalf("recorded = %v, want one spend", spends)
	}
	got := spends[0]
	if got.InputTokens != 20 || got.CacheReadTokens != 10 || got.OutputTokens != 7 || got.Model != "stub-model" || got.TurnID != "chatcmpl-bill" {
		t.Fatalf("spend = %+v", got)
	}
}

func TestGeminiVirtualKeyStreamBills(t *testing.T) {
	f, _, ledger := newVirtualKeyFixture(t, streamWithUsage("chatcmpl-gs"), activeAccount("stub-model"))
	status, out := vkDo(t, f, http.MethodPost, "/v1beta/models/gemini-2.5-pro:streamGenerateContent?alt=sse", vkBearer(vkToken),
		`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	if status != http.StatusOK {
		t.Fatalf("stream generate = %d %s", status, out)
	}
	spends := recordedSpends(ledger)
	if len(spends) != 1 {
		t.Fatalf("recorded = %v, want one spend", spends)
	}
	got := spends[0]
	if got.InputTokens != 20 || got.CacheReadTokens != 10 || got.OutputTokens != 7 || got.TurnID != "chatcmpl-gs" {
		t.Fatalf("spend = %+v", got)
	}
}

func TestGeminiVirtualKeyCountTokensIsFree(t *testing.T) {
	f, u, ledger := newVirtualKeyFixture(t, usageJSON, activeAccount("stub-model"))
	status, out := vkDo(t, f, http.MethodPost, "/v1beta/models/gemini-2.5-pro:countTokens", vkBearer(vkToken),
		`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	if status != http.StatusOK {
		t.Fatalf("count tokens = %d %s", status, out)
	}
	if len(u.requests) != 0 || len(recordedSpends(ledger)) != 0 {
		t.Fatalf("count tokens called the provider or billed a turn")
	}

	status, out = vkDo(t, f, http.MethodPost, "/v1beta/models/gemini-2.5-pro:countTokens?model=other-model", vkBearer(vkToken),
		`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	if status != http.StatusForbidden {
		t.Fatalf("disallowed count tokens = %d %s, want 403", status, out)
	}
}

func TestGeminiModelLookupEnforcesAllowedModels(t *testing.T) {
	t.Run("allowed", func(t *testing.T) {
		f, u, _ := newVirtualKeyFixture(t, usageJSON, activeAccount("m-one"))
		u.listBody = `{"data":[{"id":"m-one"}]}`
		status, out := vkDo(t, f, http.MethodGet, "/v1beta/models/m-one", vkBearer(vkToken), "")
		if status != http.StatusOK {
			t.Fatalf("lookup = %d %s", status, out)
		}
	})
	t.Run("refused", func(t *testing.T) {
		f, u, _ := newVirtualKeyFixture(t, usageJSON, activeAccount("other-model"))
		u.listBody = `{"data":[{"id":"m-one"}]}`
		status, out := vkDo(t, f, http.MethodGet, "/v1beta/models/m-one", vkBearer(vkToken), "")
		if status != http.StatusForbidden {
			t.Fatalf("lookup = %d %s, want 403", status, out)
		}
		if len(u.requests) != 0 {
			t.Fatalf("a refused model lookup reached the provider")
		}
	})
}

func TestVirtualKeyFromAnthropicAndGeminiHeaders(t *testing.T) {
	t.Run("x-api-key on messages", func(t *testing.T) {
		f, u, ledger := newVirtualKeyFixture(t, usageJSON, activeAccount("stub-model"))
		status, out := vkDo(t, f, http.MethodPost, AgentMessagesPath,
			map[string]string{"x-api-key": vkToken},
			`{"model":"stub-model","max_tokens":32,"messages":[{"role":"user","content":"hi"}]}`)
		if status != http.StatusOK {
			t.Fatalf("messages = %d %s", status, out)
		}
		if len(recordedSpends(ledger)) != 1 {
			t.Fatalf("the x-api-key turn was not billed")
		}
		got := u.last(t)
		if got.auth != "Bearer k-test" || got.apiKey != "" {
			t.Fatalf("upstream saw auth %q and x-api-key %q, want only the provider key", got.auth, got.apiKey)
		}
	})

	t.Run("x-goog-api-key on a gemini model", func(t *testing.T) {
		f, u, _ := newVirtualKeyFixture(t, usageJSON, activeAccount("m-one"))
		u.listBody = `{"data":[{"id":"m-one"}]}`
		status, out := vkDo(t, f, http.MethodGet, "/v1beta/models/m-one",
			map[string]string{"x-goog-api-key": vkToken}, "")
		if status != http.StatusOK {
			t.Fatalf("lookup = %d %s", status, out)
		}
		got := u.last(t)
		if got.auth != "Bearer k-test" || got.apiKey != "" {
			t.Fatalf("upstream saw auth %q and x-api-key %q, want only the provider key", got.auth, got.apiKey)
		}
	})

	t.Run("key query parameter", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/models?key="+neturl.QueryEscape(vkToken), nil)
		token, ok := extractVirtualKey(req)
		if !ok || token != vkToken {
			t.Fatalf("extractVirtualKey = %q %v, want the query key", token, ok)
		}
	})
}

func TestVirtualKeyHeaderSourcesFallThrough(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/models?key=plain", nil)
	req.Header.Set("x-api-key", "plain")
	req.Header.Set("x-goog-api-key", "plain")
	if _, ok := extractVirtualKey(req); ok {
		t.Fatal("a non-virtual key was accepted from a header or the query")
	}
}

func TestBillingRefusalStatuses(t *testing.T) {
	cases := []struct {
		err    error
		status int
	}{
		{plugin.ErrSpendModelUnpriced, http.StatusBadRequest},
		{plugin.ErrModelUnpriced, http.StatusBadRequest},
		{plugin.ErrPricingUnavailable, http.StatusBadRequest},
		{plugin.ErrSpendAccountPaused, http.StatusPaymentRequired},
		{plugin.ErrSpendBudgetExhausted, http.StatusPaymentRequired},
		{plugin.ErrSpendAccountNotFound, http.StatusNotFound},
		{plugin.ErrSpendUnavailable, http.StatusConflict},
		{fmt.Errorf("boom"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		if got := billingRefusalFor(tc.err); got.status != tc.status {
			t.Fatalf("billingRefusalFor(%v) = %d, want %d", tc.err, got.status, tc.status)
		}
	}
}
