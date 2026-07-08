// Multi-vendor backends for llmprovider, powered by the open-source langchaingo
// framework (github.com/tmc/langchaingo) — the Go analogue of LangChain.
//
// Instead of hand-writing a client per vendor, one thin adapter wraps
// langchaingo's `llms.Model` interface and we register a factory per provider:
// OpenAI (and any OpenAI-compatible endpoint via BaseURL — Groq, Together,
// DeepSeek, OpenRouter, a local server…), Google Gemini, Mistral, Cohere, and
// Ollama (local/offline). Adding another langchaingo-supported vendor is a few
// lines here, not a new file.
//
// Claude is the one exception: it stays on the official anthropic-sdk-go
// (claude.go) because langchaingo's Anthropic client always serializes
// `temperature`, which the Opus 4.7+ family rejects with a 400. Everything else
// accepts `temperature`, so the shared adapter is safe for them.
package llmprovider

import (
	"context"
	"fmt"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/cohere"
	"github.com/tmc/langchaingo/llms/googleai"
	"github.com/tmc/langchaingo/llms/mistral"
	"github.com/tmc/langchaingo/llms/ollama"
	"github.com/tmc/langchaingo/llms/openai"
)

// Default model ids per vendor. These are fallbacks only — every consumer can
// override per call (Request.Model) or per deployment (Options.Model /
// OCTARQ_LLM_MODEL / the dashboard), so a vendor renaming a model never breaks the
// build, just change the setting.
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

// langchainProvider adapts a langchaingo llms.Model to our Provider interface.
type langchainProvider struct {
	name  string
	model llms.Model
	def   string // reasoning-tier default model id
	cheap string // cheap-tier default model id
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

func (p *langchainProvider) Complete(ctx context.Context, req Request) (Response, error) {
	if len(req.Messages) == 0 {
		return Response{}, fmt.Errorf("llmprovider/%s: at least one message is required", p.name)
	}

	msgs := make([]llms.MessageContent, 0, len(req.Messages)+1)
	if req.System != "" {
		msgs = append(msgs, llms.TextParts(llms.ChatMessageTypeSystem, req.System))
	}
	for _, m := range req.Messages {
		role := llms.ChatMessageTypeHuman
		if m.Role == RoleAssistant {
			role = llms.ChatMessageTypeAI
		}
		msgs = append(msgs, llms.TextParts(role, m.Content))
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
	return Response{
		Text:         c.Content,
		Model:        model,
		InputTokens:  in,
		OutputTokens: out,
		StopReason:   c.StopReason,
	}, nil
}

// tokensFromInfo pulls token usage out of langchaingo's per-vendor
// GenerationInfo map, tolerating the different key names vendors use.
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

// --- per-vendor factories ---

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
