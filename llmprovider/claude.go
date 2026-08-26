// Claude (Anthropic Messages API) backend for llmprovider.
//
// This wraps the official, open-source Anthropic Go SDK
// (github.com/anthropics/anthropic-sdk-go) rather than hand-rolling HTTP. The
// SDK is the correct client for the Opus 4.7+ family: those models reject the
// `temperature`/`top_p`/`top_k` sampling parameters with a 400, and the SDK
// omits them unless you set them — so the roadmap's default reasoning model
// (claude-opus-4-8) works out of the box. We deliberately do NOT set sampling
// params: adaptive thinking is controlled explicitly via Request.Thinking and
// sampling params would break Opus.
package llmprovider

import (
	"context"
	"encoding/json"
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

func (c *Claude) buildParams(req Request) (anthropic.MessageNewParams, string, error) {
	if len(req.Messages) == 0 {
		return anthropic.MessageNewParams{}, "", fmt.Errorf("llmprovider/claude: at least one message is required")
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

	var thinking anthropic.ThinkingConfigParamUnion
	if req.Thinking {
		budget := maxTokens / 2
		if budget < 1024 {
			budget = 1024
		}
		if budget >= maxTokens {
			maxTokens = budget + 1024
		}
		thinking = anthropic.ThinkingConfigParamOfEnabled(int64(budget))
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: int64(maxTokens),
		Messages:  make([]anthropic.MessageParam, 0, len(req.Messages)),
	}
	if req.Thinking {
		params.Thinking = thinking
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

	if len(req.Tools) > 0 {
		params.Tools = make([]anthropic.ToolUnionParam, 0, len(req.Tools))
		for _, t := range req.Tools {
			var schema anthropic.ToolInputSchemaParam
			if len(t.Schema) > 0 {
				if err := json.Unmarshal(t.Schema, &schema); err != nil {
					return anthropic.MessageNewParams{}, "", fmt.Errorf("llmprovider/claude: unmarshal tool schema %q: %w", t.Name, err)
				}
			}
			tool := anthropic.ToolParam{
				Name:        t.Name,
				InputSchema: schema,
			}
			if t.Description != "" {
				tool.Description = anthropic.String(t.Description)
			}
			params.Tools = append(params.Tools, anthropic.ToolUnionParam{OfTool: &tool})
		}
	}

	return params, model, nil
}

// Complete runs one Messages API request to completion (no streaming).
func (c *Claude) Complete(ctx context.Context, req Request) (Response, error) {
	params, model, err := c.buildParams(req)
	if err != nil {
		return Response{}, err
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
		Text:            text.String(),
		Model:           orDefault(string(resp.Model), model),
		InputTokens:     int(resp.Usage.InputTokens),
		OutputTokens:    int(resp.Usage.OutputTokens),
		ReasoningTokens: 0,
		StopReason:      string(resp.StopReason),
	}, nil
}

type toolBlockInfo struct {
	id   string
	name string
}

// StreamComplete runs one Messages API request in streaming mode.
func (c *Claude) StreamComplete(ctx context.Context, req Request, h StreamHandler) (Response, error) {
	params, model, err := c.buildParams(req)
	if err != nil {
		return Response{}, err
	}

	stream := c.client.Messages.NewStreaming(ctx, params)
	defer stream.Close()

	var text strings.Builder
	modelName := model
	var inputTokens, outputTokens int
	var stopReason string
	toolsByIndex := make(map[int64]toolBlockInfo)
	var guard streamOrderGuard

	for stream.Next() {
		event := stream.Current()
		switch ev := event.AsAny().(type) {
		case anthropic.MessageStartEvent:
			if ev.Message.Model != "" {
				modelName = string(ev.Message.Model)
			}
			inputTokens = int(ev.Message.Usage.InputTokens)
		case anthropic.ContentBlockStartEvent:
			if tu, ok := ev.ContentBlock.AsAny().(anthropic.ToolUseBlock); ok {
				toolsByIndex[ev.Index] = toolBlockInfo{id: tu.ID, name: tu.Name}
			}
		case anthropic.ContentBlockDeltaEvent:
			switch delta := ev.Delta.AsAny().(type) {
			case anthropic.ThinkingDelta:
				if err := guard.OnThinking(delta.Thinking); err != nil {
					return Response{}, err
				}
				if h != nil && delta.Thinking != "" {
					h.OnThinking(delta.Thinking)
				}
			case anthropic.TextDelta:
				guard.OnText(delta.Text)
				text.WriteString(delta.Text)
				if h != nil && delta.Text != "" {
					h.OnText(delta.Text)
				}
			case anthropic.InputJSONDelta:
				tool := toolsByIndex[ev.Index]
				chunk := ToolCallChunk{
					Index:    int(ev.Index),
					ID:       tool.id,
					Name:     tool.name,
					ArgsJSON: delta.PartialJSON,
				}
				guard.OnToolCall(chunk)
				if h != nil {
					h.OnToolCall(chunk)
				}
			}
		case anthropic.MessageDeltaEvent:
			outputTokens = int(ev.Usage.OutputTokens)
			if ev.Delta.StopReason != "" {
				stopReason = string(ev.Delta.StopReason)
			}
		}
	}
	if err := stream.Err(); err != nil {
		return Response{}, fmt.Errorf("llmprovider/claude: %w", err)
	}

	return Response{
		Text:            text.String(),
		Model:           orDefault(modelName, model),
		InputTokens:     inputTokens,
		OutputTokens:    outputTokens,
		ReasoningTokens: 0, // Anthropic API rolls thinking tokens into output_tokens without a separate count.
		StopReason:      stopReason,
	}, nil
}
