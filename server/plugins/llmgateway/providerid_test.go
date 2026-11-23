package llmgateway

import (
	"errors"
	"testing"

	"github.com/octarq-org/octarq/server/plugin"
)

func TestProviderIDFor(t *testing.T) {
	tests := []struct {
		name      string
		provider  Provider
		wantID    string
		wantErrIs error
	}{
		{
			name:     "backend anthropic",
			provider: Provider{Backend: "anthropic"},
			wantID:   "anthropic",
		},
		{
			name:     "backend claude",
			provider: Provider{Backend: "claude"},
			wantID:   "anthropic",
		},
		{
			name:     "backend gemini",
			provider: Provider{Backend: "gemini"},
			wantID:   "google",
		},
		{
			name:     "backend openai empty baseURL",
			provider: Provider{Backend: "openai", BaseURL: ""},
			wantID:   "openai",
		},
		{
			name:     "google generativelanguage",
			provider: Provider{Backend: "openai", BaseURL: "https://generativelanguage.googleapis.com/v1beta/openai"},
			wantID:   "google",
		},
		{
			name:     "alibaba dashscope",
			provider: Provider{Backend: "openai", BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1"},
			wantID:   "alibaba",
		},
		{
			name:     "minimax chat",
			provider: Provider{Backend: "openai", BaseURL: "https://api.minimax.chat/v1"},
			wantID:   "minimax",
		},
		{
			name:     "minimaxi com",
			provider: Provider{Backend: "openai", BaseURL: "https://api.minimaxi.com/v1"},
			wantID:   "minimax",
		},
		{
			name:     "siliconflow cn",
			provider: Provider{Backend: "openai", BaseURL: "https://api.siliconflow.cn/v1"},
			wantID:   "siliconflow",
		},
		{
			name:     "siliconflow com",
			provider: Provider{Backend: "openai", BaseURL: "https://api.siliconflow.com/v1"},
			wantID:   "siliconflow",
		},
		{
			name:     "anthropic host",
			provider: Provider{Backend: "openai", BaseURL: "https://api.anthropic.com/v1"},
			wantID:   "anthropic",
		},
		{
			name:     "perplexity",
			provider: Provider{Backend: "openai", BaseURL: "https://api.perplexity.ai"},
			wantID:   "perplexity",
		},
		{
			name:     "deepinfra",
			provider: Provider{Backend: "openai", BaseURL: "https://api.deepinfra.com/v1/openai"},
			wantID:   "deepinfra",
		},
		{
			name:     "fireworks",
			provider: Provider{Backend: "openai", BaseURL: "https://api.fireworks.ai/inference/v1"},
			wantID:   "fireworks-ai",
		},
		{
			name:     "together",
			provider: Provider{Backend: "openai", BaseURL: "https://api.together.xyz/v1"},
			wantID:   "togetherai",
		},
		{
			name:     "moonshot cn",
			provider: Provider{Backend: "openai", BaseURL: "https://api.moonshot.cn/v1"},
			wantID:   "moonshotai",
		},
		{
			name:     "moonshot ai",
			provider: Provider{Backend: "openai", BaseURL: "https://api.moonshot.ai/v1"},
			wantID:   "moonshotai",
		},
		{
			name:     "zhipuai bigmodel",
			provider: Provider{Backend: "openai", BaseURL: "https://open.bigmodel.cn/api/paas/v4"},
			wantID:   "zhipuai",
		},
		{
			name:     "cerebras",
			provider: Provider{Backend: "openai", BaseURL: "https://api.cerebras.ai/v1"},
			wantID:   "cerebras",
		},
		{
			name:     "deepseek",
			provider: Provider{Backend: "openai", BaseURL: "https://api.deepseek.com/v1"},
			wantID:   "deepseek",
		},
		{
			name:     "novita",
			provider: Provider{Backend: "openai", BaseURL: "https://api.novita.ai/v3/openai"},
			wantID:   "novita-ai",
		},
		{
			name:     "mistral",
			provider: Provider{Backend: "openai", BaseURL: "https://api.mistral.ai/v1"},
			wantID:   "mistral",
		},
		{
			name:     "openrouter",
			provider: Provider{Backend: "openai", BaseURL: "https://openrouter.ai/api/v1"},
			wantID:   "openrouter",
		},
		{
			name:     "openai host",
			provider: Provider{Backend: "openai", BaseURL: "https://api.openai.com/v1"},
			wantID:   "openai",
		},
		{
			name:     "groq",
			provider: Provider{Backend: "openai", BaseURL: "https://api.groq.com/openai/v1"},
			wantID:   "groq",
		},
		{
			name:     "xai",
			provider: Provider{Backend: "openai", BaseURL: "https://api.x.ai/v1"},
			wantID:   "xai",
		},
		{
			name:     "zai",
			provider: Provider{Backend: "openai", BaseURL: "https://api.z.ai/v1"},
			wantID:   "zai",
		},
		{
			name:      "unknown host",
			provider:  Provider{Backend: "openai", BaseURL: "https://custom-gateway.local/v1"},
			wantErrIs: plugin.ErrModelUnpriced,
		},
		{
			name:      "unknown backend empty url",
			provider:  Provider{Backend: "custom", BaseURL: ""},
			wantErrIs: plugin.ErrModelUnpriced,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ProviderIDFor(tc.provider)
			if tc.wantErrIs != nil {
				if !errors.Is(err, tc.wantErrIs) {
					t.Fatalf("got err %v, want %v", err, tc.wantErrIs)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.wantID {
				t.Fatalf("got %q, want %q", got, tc.wantID)
			}
		})
	}
}

func TestExtractHostForms(t *testing.T) {
	cases := []struct{ raw, want string }{
		{"", ""},
		{"https://", ""},
		{"api.openai.com/v1", "api.openai.com"},
		{"https://API.OpenAI.com/v1", "api.openai.com"},
	}
	for _, tc := range cases {
		if got := extractHost(tc.raw); got != tc.want {
			t.Fatalf("extractHost(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}
