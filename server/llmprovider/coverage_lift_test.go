package llmprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tmc/langchaingo/llms"
)

func TestCoverageLiftClaudeBranches(t *testing.T) {
	ctx := context.Background()

	// 1. WithHTTPClient in newClaude
	customClient := &http.Client{}
	p, err := newClaude(Options{
		APIKey:     "test-key",
		HTTPClient: customClient,
	})
	if err != nil {
		t.Fatalf("newClaude with HTTPClient: %v", err)
	}
	claude := p.(*Claude)

	// 2. buildParams with Assistant message and invalid Tool Schema
	req := Request{
		Messages: []Message{
			{Role: RoleUser, Content: "user prompt"},
			{Role: RoleAssistant, Content: "assistant response"},
		},
		Tools: []ToolDef{
			{
				Name:   "bad_schema_tool",
				Schema: json.RawMessage("{bad json"),
			},
		},
	}
	_, _, err = claude.buildParams(req)
	if err == nil || !strings.Contains(err.Error(), "unmarshal tool schema") {
		t.Fatalf("expected unmarshal tool schema error, got %v", err)
	}

	// 3. Complete and StreamComplete error paths against 500 test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"type":"api_error","message":"internal failure"}}`, http.StatusInternalServerError)
	}))
	defer server.Close()

	pFail, err := newClaude(Options{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("newClaude with server.URL: %v", err)
	}
	claudeFail := pFail.(*Claude)

	validReq := Request{
		Messages: []Message{
			{Role: RoleUser, Content: "hello"},
		},
	}

	// Complete error
	_, err = claudeFail.Complete(ctx, validReq)
	if err == nil || !strings.Contains(err.Error(), "llmprovider/claude") {
		t.Errorf("expected claude Complete error, got %v", err)
	}

	// StreamComplete error
	_, err = claudeFail.StreamComplete(ctx, validReq, nil)
	if err == nil || !strings.Contains(err.Error(), "llmprovider/claude") {
		t.Errorf("expected claude StreamComplete error, got %v", err)
	}
}

func TestCoverageLiftLangchainBranches(t *testing.T) {
	ctx := context.Background()

	// 1. Complete with MaxTokens > 0, JSON mode, and empty choices
	stub := &stubModel{
		resp: &llms.ContentResponse{
			Choices: []*llms.ContentChoice{}, // empty choices to trigger error
		},
	}
	p := newLangchain("openai", stub, "gpt-4", "gpt-3.5")

	req := Request{
		MaxTokens: 100,
		JSON:      true,
		Messages:  []Message{{Role: RoleUser, Content: "generate"}},
	}
	_, err := p.Complete(ctx, req)
	if err == nil || !strings.Contains(err.Error(), "empty response") {
		t.Errorf("expected empty response error in Complete, got %v", err)
	}

	// 2. StreamComplete with MaxTokens > 0, JSON mode, and empty choices
	_, err = p.StreamComplete(ctx, req, nil)
	if err == nil || !strings.Contains(err.Error(), "empty response") {
		t.Errorf("expected empty response error in StreamComplete, got %v", err)
	}

	// 3. tokensFromInfo and reasoningTokensFromInfo type assertions
	info := map[string]any{
		"InputTokens":           int64(42),
		"OutputTokens":          float64(84.0),
		"ReasoningTokens":       int64(12),
		"reasoning_token_count": float64(24.0),
	}
	in, out := tokensFromInfo(info)
	if in != 42 || out != 84 {
		t.Errorf("tokensFromInfo: expected 42/84, got %d/%d", in, out)
	}

	rTokens := reasoningTokensFromInfo(map[string]any{"ReasoningTokens": int64(15)})
	if rTokens != 15 {
		t.Errorf("expected 15 reasoning tokens, got %d", rTokens)
	}

	rTokens2 := reasoningTokensFromInfo(map[string]any{"reasoning_tokens": float64(25.0)})
	if rTokens2 != 25 {
		t.Errorf("expected 25 reasoning tokens, got %d", rTokens2)
	}
}

func TestCoverageLiftVendorFactoryErrors(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("COHERE_API_KEY", "")

	// 1. OpenAI missing key
	_, err := New(Options{Provider: "openai", APIKey: ""})
	if err == nil || !strings.Contains(err.Error(), "llmprovider/openai") {
		t.Errorf("expected openai factory error, got %v", err)
	}

	// 2. Gemini missing key
	_, err = New(Options{Provider: "gemini", APIKey: ""})
	if err == nil || !strings.Contains(err.Error(), "llmprovider/gemini") {
		t.Errorf("expected gemini factory error, got %v", err)
	}

	// 3. Cohere missing key
	_, err = New(Options{Provider: "cohere", APIKey: ""})
	if err == nil || !strings.Contains(err.Error(), "llmprovider/cohere") {
		t.Errorf("expected cohere factory error, got %v", err)
	}
}
