package llmprovider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tmc/langchaingo/llms"
)

// stubModel is a scripted llms.Model for the langchain adapter.
type stubModel struct {
	resp *llms.ContentResponse
	err  error

	lastMessages []llms.MessageContent
}

func (s *stubModel) GenerateContent(_ context.Context, messages []llms.MessageContent, _ ...llms.CallOption) (*llms.ContentResponse, error) {
	s.lastMessages = messages
	return s.resp, s.err
}

func (s *stubModel) Call(context.Context, string, ...llms.CallOption) (string, error) { return "", nil }

// TestLangchainCompleteBuildsMessages verifies the adapter folds the system
// prompt in first and maps user/assistant roles onto langchaingo's message
// types, then surfaces content/usage/stop fields.
func TestLangchainCompleteBuildsMessages(t *testing.T) {
	stub := &stubModel{resp: &llms.ContentResponse{
		Choices: []*llms.ContentChoice{{
			Content:        "the answer",
			StopReason:     "end_turn",
			GenerationInfo: map[string]any{"PromptTokens": int64(3), "CompletionTokens": int64(2)},
		}},
	}}
	p := newLangchain("openai", stub, "", "")

	resp, err := p.Complete(context.Background(), Request{
		System:   "be terse",
		Messages: []Message{{Role: RoleUser, Content: "hi"}, {Role: RoleAssistant, Content: "hey"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Text != "the answer" || resp.StopReason != "end_turn" {
		t.Errorf("resp = %+v", resp)
	}
	if resp.InputTokens != 3 || resp.OutputTokens != 2 {
		t.Errorf("usage = %d/%d, want 3/2", resp.InputTokens, resp.OutputTokens)
	}
	if len(stub.lastMessages) != 3 {
		t.Fatalf("messages = %d, want system+user+assistant", len(stub.lastMessages))
	}
	if stub.lastMessages[0].Role != llms.ChatMessageTypeSystem {
		t.Errorf("first message role = %q, want system", stub.lastMessages[0].Role)
	}
	if stub.lastMessages[1].Role != llms.ChatMessageTypeHuman || stub.lastMessages[2].Role != llms.ChatMessageTypeAI {
		t.Errorf("roles = %q/%q, want human/ai", stub.lastMessages[1].Role, stub.lastMessages[2].Role)
	}
}

// TestLangchainCompleteDefaultsAndErrors covers default model fallback, the
// model request override, empty-choice and transport errors.
func TestLangchainCompleteDefaultsAndErrors(t *testing.T) {
	ok := &stubModel{resp: &llms.ContentResponse{
		Choices: []*llms.ContentChoice{{Content: "c", StopReason: "stop", GenerationInfo: map[string]any{}}},
	}}
	p := newLangchain("openai", ok, "gpt-4o", "gpt-4o-mini")

	if resp, err := p.Complete(context.Background(), Request{Messages: []Message{{Content: "x"}}}); err != nil {
		t.Fatalf("Complete: %v", err)
	} else if resp.Model != "gpt-4o" {
		t.Errorf("default model = %q, want gpt-4o", resp.Model)
	}

	if resp, err := p.Complete(context.Background(), Request{Model: "my-model", Messages: []Message{{Content: "x"}}}); err != nil {
		t.Fatalf("Complete override: %v", err)
	} else if resp.Model != "my-model" {
		t.Errorf("override model = %q, want my-model", resp.Model)
	}

	empty := &stubModel{resp: &llms.ContentResponse{}}
	pe := newLangchain("openai", empty, "", "")
	if _, err := pe.Complete(context.Background(), Request{Messages: []Message{{Content: "x"}}}); err == nil {
		t.Error("empty response accepted")
	}

	boom := &stubModel{err: errors.New("upstream down")}
	pb := newLangchain("openai", boom, "", "")
	if _, err := pb.Complete(context.Background(), Request{Messages: []Message{{Content: "x"}}}); err == nil {
		t.Error("transport error swallowed")
	}

	if _, err := p.Complete(context.Background(), Request{}); err == nil {
		t.Error("empty messages accepted")
	}
}

// TestLangchainModelNames asserts the per-vendor defaults land and overrides win.
func TestLangchainModelNames(t *testing.T) {
	p := newLangchain("gemini", &stubModel{}, "cust", "cheap2")
	if p.Name() != "gemini" || p.DefaultModel() != "cust" || p.CheapModel() != "cheap2" {
		t.Errorf("custom models not applied: %+v", p)
	}
	co := newLangchain("cohere", &stubModel{}, "", "")
	if co.DefaultModel() != "command-r-plus" || co.CheapModel() != "command-r" {
		t.Errorf("cohere defaults = %q/%q", co.DefaultModel(), co.CheapModel())
	}
}

// TestTokensFromInfoTypes covers the numeric-type normalization and the
// fallback to zero when no recognized key is present.
func TestTokensFromInfoTypes(t *testing.T) {
	in, out := tokensFromInfo(map[string]any{
		"input_tokens":  float64(12),
		"output_tokens": 34,
	})
	if in != 12 || out != 34 {
		t.Errorf("tokensFromInfo = %d/%d, want 12/34", in, out)
	}
	if in, out := tokensFromInfo(map[string]any{}); in != 0 || out != 0 {
		t.Errorf("empty info = %d/%d, want 0/0", in, out)
	}
}

// TestVendorFactoriesWithOptions constructs each vendor with every option so
// the option branches in the factories are exercised (clients connect lazily).
func TestVendorFactoriesWithOptions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	client := &http.Client{Transport: http.DefaultTransport}

	cases := []struct {
		provider string
		opts     Options
	}{
		{"openai", Options{Provider: "openai", APIKey: "k", BaseURL: srv.URL, HTTPClient: client, Model: "m", CheapModel: "c"}},
		{"gemini", Options{Provider: "gemini", APIKey: "k", HTTPClient: client, Model: "gemini-m", CheapModel: "c"}},
		{"mistral", Options{Provider: "mistral", APIKey: "k", BaseURL: srv.URL, Model: "mistral-m", CheapModel: "c"}},
		{"cohere", Options{Provider: "cohere", APIKey: "k", BaseURL: srv.URL, Model: "cohere-m", CheapModel: "c"}},
		{"ollama", Options{Provider: "ollama", BaseURL: srv.URL, HTTPClient: client, Model: "llama3-70b", CheapModel: "llama3"}},
	}
	for _, tc := range cases {
		p, err := New(tc.opts)
		if err != nil {
			t.Errorf("New(%s): %v", tc.provider, err)
			continue
		}
		if p.Name() != tc.provider {
			t.Errorf("name = %q, want %q", p.Name(), tc.provider)
		}
	}
}

// TestFromEnv loads the full OCTARQ_LLM_* surface.
func TestFromEnv(t *testing.T) {
	t.Setenv("OCTARQ_LLM_PROVIDER", "openai")
	t.Setenv("OCTARQ_LLM_API_KEY", "env-key")
	t.Setenv("OCTARQ_LLM_BASE_URL", "http://env.example")
	t.Setenv("OCTARQ_LLM_MODEL", "env-model")
	t.Setenv("OCTARQ_LLM_CHEAP_MODEL", "env-cheap")
	o := OptionsFromEnv()
	if o.Provider != "openai" || o.APIKey != "env-key" || o.BaseURL != "http://env.example" ||
		o.Model != "env-model" || o.CheapModel != "env-cheap" {
		t.Errorf("OptionsFromEnv = %+v", o)
	}
	if p, err := FromEnv(); err != nil {
		t.Fatalf("FromEnv: %v", err)
	} else if p.Name() != "openai" {
		t.Errorf("FromEnv provider = %q, want openai", p.Name())
	}
}

// TestClaudeJSONNudge asserts the JSON request steers the model via the system
// prompt: a bare JSON request gets the nudge alone, and an existing system
// prompt gets it appended, never in prose or markdown fences.
func TestClaudeJSONNudge(t *testing.T) {
	var gotBodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotBodies = append(gotBodies, body)
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"id":"m","type":"message","role":"assistant","model":"c",
			"content":[{"type":"text","text":"{\"ok\":true}"}],"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer srv.Close()

	base := Options{APIKey: "k", BaseURL: srv.URL}
	req := Request{JSON: true, Messages: []Message{{Role: RoleUser, Content: "summarize"}}}

	if p, err := New(base); err != nil {
		t.Fatalf("New: %v", err)
	} else if _, err := p.Complete(context.Background(), req); err != nil {
		t.Fatalf("Complete bare JSON: %v", err)
	}
	if p, err := New(base); err != nil {
		t.Fatalf("New: %v", err)
	} else if _, err := p.Complete(context.Background(), Request{
		JSON: true, System: "be terse", Messages: []Message{{Role: RoleUser, Content: "summarize"}},
	}); err != nil {
		t.Fatalf("Complete with system: %v", err)
	}

	if len(gotBodies) != 2 {
		t.Fatalf("captured %d requests, want 2", len(gotBodies))
	}
	nudgeOnly := systemText(t, gotBodies[0]["system"])
	nudgeWithSys := systemText(t, gotBodies[1]["system"])
	if !strings.Contains(nudgeOnly, "single valid JSON value") {
		t.Errorf("bare-JSON system = %q, want the JSON nudge", nudgeOnly)
	}
	if !strings.Contains(nudgeWithSys, "be terse") || !strings.Contains(nudgeWithSys, "single valid JSON value") {
		t.Errorf("system+JSON nudge = %q, want both", nudgeWithSys)
	}
}

// systemText flattens the Messages-API system block (an array of text blocks)
// into one string for assertion.
func systemText(t *testing.T, v any) string {
	t.Helper()
	blocks, ok := v.([]any)
	if !ok {
		t.Fatalf("system payload = %#v, want a block array", v)
	}
	var sb strings.Builder
	for _, b := range blocks {
		if m, ok := b.(map[string]any); ok {
			if s, ok := m["text"].(string); ok {
				sb.WriteString(s)
				sb.WriteString("\n")
			}
		}
	}
	return strings.TrimSpace(sb.String())
}
