package llmprovider

import "testing"

func TestUncachedInput(t *testing.T) {
	cases := []struct {
		name   string
		input  int
		cached int
		want   int
	}{
		{"no cache", 1000, 0, 1000},
		{"cache is a subset", 1000, 400, 600},
		{"all input cached", 1000, 1000, 0},
		{"cache exceeds input", 100, 400, 0},
		{"negative cache", 100, -5, 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := uncachedInput(tc.input, tc.cached); got != tc.want {
				t.Fatalf("uncachedInput(%d, %d) = %d, want %d", tc.input, tc.cached, got, tc.want)
			}
		})
	}
}

func TestNormalizedInput(t *testing.T) {
	cases := []struct {
		name   string
		input  int
		cached int
		info   map[string]any
		want   int
	}{
		{"provider reports non-cached input", 1000, 400, map[string]any{"NonCachedInputTokens": 600}, 600},
		{"snake case non-cached input", 1000, 400, map[string]any{"non_cached_input_tokens": 600}, 600},
		{"subtracts the cached subset", 1000, 400, map[string]any{"PromptCachedTokens": 400}, 600},
		{"no cache", 1000, 0, map[string]any{"PromptTokens": 1000}, 1000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizedInput(tc.input, tc.cached, tc.info); got != tc.want {
				t.Fatalf("normalizedInput(%d, %d) = %d, want %d", tc.input, tc.cached, got, tc.want)
			}
		})
	}
}

func TestCacheTokensFromInfo(t *testing.T) {
	cases := []struct {
		name         string
		info         map[string]any
		wantRead     int
		wantCreation int
	}{
		{"nil", nil, 0, 0},
		{"flat cached tokens", map[string]any{"cached_tokens": 400}, 400, 0},
		{"anthropic read and creation", map[string]any{"cache_read_input_tokens": 800, "cache_creation_input_tokens": 120}, 800, 120},
		{"deepseek cache hit", map[string]any{"prompt_cache_hit_tokens": 256}, 256, 0},
		{"nested openai details", map[string]any{"prompt_tokens_details": map[string]any{"cached_tokens": 640}}, 640, 0},
		{"nested pascal case details", map[string]any{"PromptTokensDetails": map[string]any{"cachedTokens": 320}}, 320, 0},
		{"langchaingo openai key", map[string]any{"PromptCachedTokens": 480}, 480, 0},
		{"langchaingo gemini key", map[string]any{"CachedTokens": 256}, 256, 0},
		{"no cache fields", map[string]any{"PromptTokens": 100}, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			read, creation := cacheTokensFromInfo(tc.info)
			if read != tc.wantRead || creation != tc.wantCreation {
				t.Fatalf("cacheTokensFromInfo = %d/%d, want %d/%d", read, creation, tc.wantRead, tc.wantCreation)
			}
		})
	}
}
