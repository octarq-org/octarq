// Claude (Anthropic Messages API) backend for llmprovider.
//
// This wraps the official, open-source Anthropic Go SDK
// (github.com/anthropics/anthropic-sdk-go) rather than hand-rolling HTTP. The
// SDK is the correct client for the Opus 4.7+ family: those models reject the
// `temperature`/`top_p`/`top_k` sampling parameters with a 400, and the SDK
// omits them unless you set them — so the roadmap's default reasoning model
// (claude-opus-4-8) works out of the box. We deliberately do NOT set sampling
// params or thinking: these are short classification/summary/briefing calls,
// where adaptive thinking's latency isn't wanted and sampling params would
// break Opus.
package llmprovider

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// defaultMaxTokens caps a response when the caller doesn't specify one. Kept
// modest because every consumer (summaries, tags, OTP, briefings) is short.
const defaultMaxTokens = 1024

func init() { Register("claude", newClaude) }

// Claude is the Anthropic Messages API backend.
type Claude struct {
	client anthropic.Client
	model  string // reasoning-tier default
	cheap  string // cheap classification/summary model
}

func newClaude(o Options) (Provider, error) {
	if o.APIKey == "" {
		return nil, fmt.Errorf("llmprovider/claude: API key is required (set OCTARQ_LLM_API_KEY or ANTHROPIC_API_KEY)")
	}
	opts := []option.RequestOption{option.WithAPIKey(o.APIKey)}
	if o.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(o.BaseURL))
	}
	if o.HTTPClient != nil {
		opts = append(opts, option.WithHTTPClient(o.HTTPClient))
	}
	return &Claude{
		client: anthropic.NewClient(opts...),
		model:  orDefault(o.Model, ModelClaudeOpus),
		cheap:  orDefault(o.CheapModel, ModelClaudeHaiku),
	}, nil
}

func (c *Claude) Name() string         { return "claude" }
func (c *Claude) DefaultModel() string { return c.model }
func (c *Claude) CheapModel() string   { return c.cheap }

// Complete runs one Messages API request to completion (no streaming).
func (c *Claude) Complete(ctx context.Context, req Request) (Response, error) {
	if len(req.Messages) == 0 {
		return Response{}, fmt.Errorf("llmprovider/claude: at least one message is required")
	}

	model := orDefault(req.Model, c.model)
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}

	// The Messages API has no top-level "return JSON" switch that's portable
	// across models, so a JSON request is steered via a system-prompt nudge.
	// Callers must still parse defensively.
	system := req.System
	if req.JSON {
		const jsonNudge = "Respond with a single valid JSON value and nothing else — no prose, no markdown fences."
		if strings.TrimSpace(system) == "" {
			system = jsonNudge
		} else {
			system = system + "\n\n" + jsonNudge
		}
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: int64(maxTokens),
		Messages:  make([]anthropic.MessageParam, 0, len(req.Messages)),
	}
	if strings.TrimSpace(system) != "" {
		params.System = []anthropic.TextBlockParam{{Text: system}}
	}
	for _, m := range req.Messages {
		block := anthropic.NewTextBlock(m.Content)
		if m.Role == RoleAssistant {
			params.Messages = append(params.Messages, anthropic.NewAssistantMessage(block))
		} else {
			params.Messages = append(params.Messages, anthropic.NewUserMessage(block))
		}
	}

	resp, err := c.client.Messages.New(ctx, params)
	if err != nil {
		return Response{}, fmt.Errorf("llmprovider/claude: %w", err)
	}

	var text strings.Builder
	for _, block := range resp.Content {
		if t, ok := block.AsAny().(anthropic.TextBlock); ok {
			text.WriteString(t.Text)
		}
	}

	return Response{
		Text:         text.String(),
		Model:        orDefault(string(resp.Model), model),
		InputTokens:  int(resp.Usage.InputTokens),
		OutputTokens: int(resp.Usage.OutputTokens),
		StopReason:   string(resp.StopReason),
	}, nil
}
