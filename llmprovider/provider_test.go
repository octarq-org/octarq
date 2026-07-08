package llmprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewUnknownProvider(t *testing.T) {
	if _, err := New(Options{Provider: "nope"}); err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestNewDefaultsToClaude(t *testing.T) {
	p, err := New(Options{APIKey: "k"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if p.Name() != "claude" {
		t.Errorf("default provider = %q, want claude", p.Name())
	}
	if p.DefaultModel() != ModelClaudeOpus {
		t.Errorf("DefaultModel = %q, want %q", p.DefaultModel(), ModelClaudeOpus)
	}
	if p.CheapModel() != ModelClaudeHaiku {
		t.Errorf("CheapModel = %q, want %q", p.CheapModel(), ModelClaudeHaiku)
	}
}

func TestClaudeRequiresAPIKey(t *testing.T) {
	if _, err := New(Options{Provider: "claude"}); err == nil {
		t.Fatal("expected error when API key missing")
	}
}

func TestClaudeModelOverrides(t *testing.T) {
	p, _ := New(Options{APIKey: "k", Model: "m1", CheapModel: "m2"})
	if p.DefaultModel() != "m1" || p.CheapModel() != "m2" {
		t.Errorf("model overrides not applied: %q / %q", p.DefaultModel(), p.CheapModel())
	}
}

// TestClaudeComplete exercises the full request/response path against a stub
// server that mimics the Anthropic Messages API, so the SDK wiring (params,
// system prompt, content/usage parsing) is verified without a live key.
func TestClaudeComplete(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "secret" {
			t.Errorf("missing/wrong x-api-key: %q", r.Header.Get("x-api-key"))
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"msg_1","type":"message","role":"assistant","model":"claude-opus-4-8",
			"stop_reason":"end_turn",
			"content":[{"type":"text","text":"hello world"}],
			"usage":{"input_tokens":11,"output_tokens":3}
		}`))
	}))
	defer srv.Close()

	p, err := New(Options{APIKey: "secret", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	resp, err := p.Complete(context.Background(), Request{
		System:   "be terse",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Text != "hello world" {
		t.Errorf("Text = %q", resp.Text)
	}
	if resp.InputTokens != 11 || resp.OutputTokens != 3 {
		t.Errorf("usage = %d/%d", resp.InputTokens, resp.OutputTokens)
	}
	// The request must NOT carry sampling params (they 400 on Opus 4.8).
	for _, banned := range []string{"temperature", "top_p", "top_k"} {
		if _, ok := gotBody[banned]; ok {
			t.Errorf("request unexpectedly carried %q (rejected by Opus 4.8)", banned)
		}
	}
	if gotBody["model"] != ModelClaudeOpus {
		t.Errorf("model in body = %v, want %q", gotBody["model"], ModelClaudeOpus)
	}
}

// TestVendorsRegistered confirms the multi-vendor backends are wired and
// construct without a live server/key (langchaingo clients connect lazily, at
// call time, not at New).
func TestVendorsRegistered(t *testing.T) {
	for _, name := range []string{"openai", "gemini", "mistral", "cohere", "ollama"} {
		p, err := New(Options{Provider: name, APIKey: "test-key"})
		if err != nil {
			t.Errorf("New(%q): %v", name, err)
			continue
		}
		if p.Name() != name {
			t.Errorf("provider name = %q, want %q", p.Name(), name)
		}
		if p.DefaultModel() == "" || p.CheapModel() == "" {
			t.Errorf("%q: missing default models", name)
		}
	}
}

func TestNamesIncludesAllVendors(t *testing.T) {
	have := map[string]bool{}
	for _, n := range Names() {
		have[n] = true
	}
	for _, want := range []string{"claude", "openai", "gemini", "mistral", "cohere", "ollama"} {
		if !have[want] {
			t.Errorf("Names() missing %q (have %v)", want, Names())
		}
	}
}

func TestOptionsFromEnv(t *testing.T) {
	t.Setenv("OCTARQ_LLM_PROVIDER", "claude")
	t.Setenv("OCTARQ_LLM_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "fallback-key")
	o := OptionsFromEnv()
	if o.Provider != "claude" {
		t.Errorf("provider = %q", o.Provider)
	}
	if o.APIKey != "fallback-key" {
		t.Errorf("APIKey should fall back to ANTHROPIC_API_KEY, got %q", o.APIKey)
	}
}
