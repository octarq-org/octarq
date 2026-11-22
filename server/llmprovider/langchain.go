package llmprovider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/cohere"
	"github.com/tmc/langchaingo/llms/googleai"
	"github.com/tmc/langchaingo/llms/mistral"
	"github.com/tmc/langchaingo/llms/ollama"
	"github.com/tmc/langchaingo/llms/openai"
)

var vendorDefaults = map[string]struct{ reasoning, cheap string }{
	"openai":  {"gpt-4o", "gpt-4o-mini"},
	"gemini":  {"gemini-1.5-pro", "gemini-1.5-flash"},
	"mistral": {"mistral-large-latest", "mistral-small-latest"},
	"cohere":  {"command-r-plus", "command-r"},
	"ollama":  {"llama3", "llama3"},
}

func init() {
	Register("openai", makeOpenAI)
	Register("gemini", makeGemini)
	Register("mistral", makeMistral)
	Register("cohere", makeCohere)
	Register("ollama", makeOllama)
}

type langchainProvider struct {
	name  string
	model llms.Model
	def   string
	cheap string
}

func newLangchain(name string, m llms.Model, def, cheap string) *langchainProvider {
	d := vendorDefaults[name]
	return &langchainProvider{
		name:  name,
		model: m,
		def:   orDefault(def, d.reasoning),
		cheap: orDefault(cheap, d.cheap),
	}
}

func (p *langchainProvider) Name() string         { return p.name }
func (p *langchainProvider) DefaultModel() string { return p.def }
func (p *langchainProvider) CheapModel() string   { return p.cheap }

func (p *langchainProvider) buildMessages(req Request) ([]llms.MessageContent, error) {
	if len(req.Messages) == 0 {
		return nil, fmt.Errorf("llmprovider/%s: at least one message is required", p.name)
	}

	msgs := make([]llms.MessageContent, 0, len(req.Messages)+1)
	if req.System != "" {
		msgs = append(msgs, llms.TextParts(llms.ChatMessageTypeSystem, req.System))
	}
	for _, m := range req.Messages {
		switch m.Role {
		case RoleAssistant:
			var parts []llms.ContentPart
			if m.Content != "" {
				parts = append(parts, llms.TextPart(m.Content))
			}
			for _, tc := range m.ToolCalls {
				parts = append(parts, llms.ToolCall{
					ID:   tc.ID,
					Type: "function",
					FunctionCall: &llms.FunctionCall{
						Name:      tc.Name,
						Arguments: string(tc.ArgsJSON),
					},
				})
			}
			if len(parts) == 0 {
				parts = append(parts, llms.TextPart(""))
			}
			msgs = append(msgs, llms.MessageContent{
				Role:  llms.ChatMessageTypeAI,
				Parts: parts,
			})
		case RoleTool:
			var parts []llms.ContentPart
			for _, tr := range m.ToolResults {
				parts = append(parts, llms.ToolCallResponse{
					ToolCallID: tr.ToolCallID,
					Content:    tr.Content,
				})
			}
			if len(parts) == 0 && m.Content != "" {
				parts = append(parts, llms.TextPart(m.Content))
			}
			msgs = append(msgs, llms.MessageContent{
				Role:  llms.ChatMessageTypeTool,
				Parts: parts,
			})
		default:
			msgs = append(msgs, llms.TextParts(llms.ChatMessageTypeHuman, m.Content))
		}
	}
	return msgs, nil
}

func (p *langchainProvider) Complete(ctx context.Context, req Request) (Response, error) {
	msgs, err := p.buildMessages(req)
	if err != nil {
		return Response{}, err
	}

	model := orDefault(req.Model, p.def)
	callOpts := []llms.CallOption{llms.WithModel(model)}
	if req.MaxTokens > 0 {
		callOpts = append(callOpts, llms.WithMaxTokens(req.MaxTokens))
	}
	if req.JSON {
		callOpts = append(callOpts, llms.WithJSONMode())
	}

	resp, err := p.model.GenerateContent(ctx, msgs, callOpts...)
	if err != nil {
		return Response{}, fmt.Errorf("llmprovider/%s: %w", p.name, err)
	}
	if len(resp.Choices) == 0 {
		return Response{}, fmt.Errorf("llmprovider/%s: empty response", p.name)
	}
	c := resp.Choices[0]
	in, out := tokensFromInfo(c.GenerationInfo)
	reasoning := reasoningTokensFromInfo(c.GenerationInfo)
	cacheRead, cacheCreation := cacheTokensFromInfo(c.GenerationInfo)
	in = normalizedInput(in, cacheRead, c.GenerationInfo)

	var toolCalls []ToolCall
	for _, tc := range c.ToolCalls {
		if tc.FunctionCall != nil {
			toolCalls = append(toolCalls, ToolCall{
				ID:       tc.ID,
				Name:     tc.FunctionCall.Name,
				ArgsJSON: json.RawMessage(tc.FunctionCall.Arguments),
			})
		}
	}

	return Response{
		Text:                c.Content,
		Model:               model,
		ToolCalls:           toolCalls,
		InputTokens:         in,
		OutputTokens:        out,
		ReasoningTokens:     reasoning,
		CacheReadTokens:     cacheRead,
		CacheCreationTokens: cacheCreation,
		StopReason:          c.StopReason,
	}, nil
}

func (p *langchainProvider) StreamComplete(ctx context.Context, req Request, h StreamHandler) (Response, error) {
	if len(req.Tools) > 0 {
		return Response{}, fmt.Errorf("llmprovider/%s: tool streaming not supported by langchaingo adapter", p.name)
	}

	msgs, err := p.buildMessages(req)
	if err != nil {
		return Response{}, err
	}

	model := orDefault(req.Model, p.def)
	callOpts := []llms.CallOption{llms.WithModel(model)}
	if req.MaxTokens > 0 {
		callOpts = append(callOpts, llms.WithMaxTokens(req.MaxTokens))
	}
	if req.JSON {
		callOpts = append(callOpts, llms.WithJSONMode())
	}

	var guard streamOrderGuard
	if req.Thinking {
		callOpts = append(callOpts, llms.WithStreamingReasoningFunc(func(ctx context.Context, reasoningChunk, chunk []byte) error {
			if len(reasoningChunk) > 0 {
				rStr := string(reasoningChunk)
				if err := guard.OnThinking(rStr); err != nil {
					return err
				}
				if h != nil {
					h.OnThinking(rStr)
				}
			}
			if len(chunk) > 0 {
				cStr := string(chunk)
				guard.OnText(cStr)
				if h != nil {
					h.OnText(cStr)
				}
			}
			return nil
		}))
	} else {
		callOpts = append(callOpts, llms.WithStreamingFunc(func(ctx context.Context, chunk []byte) error {
			if len(chunk) > 0 {
				cStr := string(chunk)
				guard.OnText(cStr)
				if h != nil {
					h.OnText(cStr)
				}
			}
			return nil
		}))
	}

	resp, err := p.model.GenerateContent(ctx, msgs, callOpts...)
	if err != nil {
		return Response{}, fmt.Errorf("llmprovider/%s: %w", p.name, err)
	}
	if len(resp.Choices) == 0 {
		return Response{}, fmt.Errorf("llmprovider/%s: empty response", p.name)
	}
	c := resp.Choices[0]
	in, out := tokensFromInfo(c.GenerationInfo)
	reasoning := reasoningTokensFromInfo(c.GenerationInfo)
	cacheRead, cacheCreation := cacheTokensFromInfo(c.GenerationInfo)
	in = normalizedInput(in, cacheRead, c.GenerationInfo)
	return Response{
		Text:                c.Content,
		Model:               model,
		InputTokens:         in,
		OutputTokens:        out,
		ReasoningTokens:     reasoning,
		CacheReadTokens:     cacheRead,
		CacheCreationTokens: cacheCreation,
		StopReason:          c.StopReason,
	}, nil
}

func tokensFromInfo(info map[string]any) (in, out int) {
	pick := func(keys ...string) int {
		for _, k := range keys {
			if v, ok := info[k]; ok {
				switch n := v.(type) {
				case int:
					return n
				case int64:
					return int(n)
				case float64:
					return int(n)
				}
			}
		}
		return 0
	}
	in = pick("InputTokens", "PromptTokens", "input_tokens", "prompt_tokens")
	out = pick("OutputTokens", "CompletionTokens", "output_tokens", "completion_tokens")
	return in, out
}

func reasoningTokensFromInfo(info map[string]any) int {
	pick := func(keys ...string) int {
		for _, k := range keys {
			if v, ok := info[k]; ok {
				switch n := v.(type) {
				case int:
					return n
				case int64:
					return int(n)
				case float64:
					return int(n)
				}
			}
		}
		return 0
	}
	return pick("ReasoningTokens", "reasoning_tokens", "ReasoningTokenCount", "reasoning_token_count")
}

func cacheTokensFromInfo(info map[string]any) (read, creation int) {
	if info == nil {
		return 0, 0
	}
	pick := func(keys ...string) int {
		for _, k := range keys {
			if v, ok := info[k]; ok {
				switch n := v.(type) {
				case int:
					return n
				case int64:
					return int(n)
				case float64:
					return int(n)
				}
			}
		}
		return 0
	}
	read = pick("cached_tokens", "cache_read_input_tokens", "prompt_cache_hit_tokens",
		"PromptCachedTokens", "prompt_cached_tokens", "CachedTokens", "cachedTokens",
		"CacheReadInputTokens", "cacheReadInputTokens")
	if read == 0 {
		for _, k := range []string{"prompt_tokens_details", "PromptTokensDetails"} {
			details, ok := info[k].(map[string]any)
			if !ok {
				continue
			}
			if v, ok := pickInt(details, "cached_tokens", "cachedTokens"); ok {
				read = v
				break
			}
		}
	}
	creation = pick("cache_creation_input_tokens", "cache_creation_tokens")
	return read, creation
}

func pickInt(info map[string]any, keys ...string) (int, bool) {
	for _, k := range keys {
		switch n := info[k].(type) {
		case int:
			return n, true
		case int64:
			return int(n), true
		case float64:
			return int(n), true
		}
	}
	return 0, false
}

func uncachedInput(input, cacheRead int) int {
	if cacheRead <= 0 {
		return input
	}
	if cacheRead >= input {
		return 0
	}
	return input - cacheRead
}

func normalizedInput(input, cacheRead int, info map[string]any) int {
	if v, ok := pickInt(info, "NonCachedInputTokens", "non_cached_input_tokens"); ok {
		return v
	}
	return uncachedInput(input, cacheRead)
}

func makeOpenAI(o Options) (Provider, error) {
	opts := []openai.Option{}
	if o.APIKey != "" {
		opts = append(opts, openai.WithToken(o.APIKey))
	}
	if o.BaseURL != "" {
		opts = append(opts, openai.WithBaseURL(o.BaseURL))
	}
	if o.HTTPClient != nil {
		opts = append(opts, openai.WithHTTPClient(o.HTTPClient))
	}
	m, err := openai.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("llmprovider/openai: %w", err)
	}
	return newLangchain("openai", m, o.Model, o.CheapModel), nil
}

func makeGemini(o Options) (Provider, error) {
	opts := []googleai.Option{}
	if o.APIKey != "" {
		opts = append(opts, googleai.WithAPIKey(o.APIKey))
	}
	if o.Model != "" {
		opts = append(opts, googleai.WithDefaultModel(o.Model))
	}
	if o.HTTPClient != nil {
		opts = append(opts, googleai.WithHTTPClient(o.HTTPClient))
	}
	m, err := googleai.New(context.Background(), opts...)
	if err != nil {
		return nil, fmt.Errorf("llmprovider/gemini: %w", err)
	}
	return newLangchain("gemini", m, o.Model, o.CheapModel), nil
}

func makeMistral(o Options) (Provider, error) {
	opts := []mistral.Option{}
	if o.APIKey != "" {
		opts = append(opts, mistral.WithAPIKey(o.APIKey))
	}
	if o.BaseURL != "" {
		opts = append(opts, mistral.WithEndpoint(o.BaseURL))
	}
	if o.Model != "" {
		opts = append(opts, mistral.WithModel(o.Model))
	}
	m, err := mistral.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("llmprovider/mistral: %w", err)
	}
	return newLangchain("mistral", m, o.Model, o.CheapModel), nil
}

func makeCohere(o Options) (Provider, error) {
	opts := []cohere.Option{}
	if o.APIKey != "" {
		opts = append(opts, cohere.WithToken(o.APIKey))
	}
	if o.BaseURL != "" {
		opts = append(opts, cohere.WithBaseURL(o.BaseURL))
	}
	if o.Model != "" {
		opts = append(opts, cohere.WithModel(o.Model))
	}
	m, err := cohere.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("llmprovider/cohere: %w", err)
	}
	return newLangchain("cohere", m, o.Model, o.CheapModel), nil
}

func makeOllama(o Options) (Provider, error) {
	model := orDefault(o.Model, vendorDefaults["ollama"].reasoning)
	opts := []ollama.Option{ollama.WithModel(model)}
	if o.BaseURL != "" {
		opts = append(opts, ollama.WithServerURL(o.BaseURL))
	}
	if o.HTTPClient != nil {
		opts = append(opts, ollama.WithHTTPClient(o.HTTPClient))
	}
	m, err := ollama.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("llmprovider/ollama: %w", err)
	}
	return newLangchain("ollama", m, o.Model, o.CheapModel), nil
}
