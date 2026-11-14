package llmgateway

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

// Claude Code speaks Anthropic's Messages API, not the OpenAI one. The gateway
// therefore serves both wire formats over the same provider configuration: an
// OpenAI-shaped client posts to /v1/chat/completions, and an Anthropic-shaped
// client posts to /v1/messages. Both resolve the same workspace provider, so a
// coding agent never holds the provider key.
//
// The Messages API is a translation, not a relay: the provider only speaks
// OpenAI, so the request is rebuilt into that shape and the answer is rebuilt
// back, streaming events included.
const (
	// AgentMessagesPath is the Anthropic Messages endpoint.
	AgentMessagesPath = "/v1/messages"
	// AgentCountTokensPath answers the client's context meter.
	AgentCountTokensPath = "/v1/messages/count_tokens"
)

// --- request shapes (Anthropic in) ---

type anthRequest struct {
	Model         string          `json:"model"`
	MaxTokens     int             `json:"max_tokens"`
	System        json.RawMessage `json:"system,omitempty"`
	Messages      []anthMessage   `json:"messages"`
	Tools         []anthTool      `json:"tools,omitempty"`
	ToolChoice    *anthToolChoice `json:"tool_choice,omitempty"`
	Temperature   *float64        `json:"temperature,omitempty"`
	TopP          *float64        `json:"top_p,omitempty"`
	StopSequences []string        `json:"stop_sequences,omitempty"`
	Stream        bool            `json:"stream,omitempty"`
}

type anthMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// anthBlock is one content block. A message's content is a string or a list of
// these; every field is optional because the type decides which are read.
type anthBlock struct {
	Type string `json:"type"`

	Text string `json:"text,omitempty"`

	Source *anthImageSource `json:"source,omitempty"`

	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

// MarshalJSON writes only the fields the block type defines. encoding/json's
// omitempty would drop an empty text, and a text block without a `text` key is
// not a text block.
func (b anthBlock) MarshalJSON() ([]byte, error) {
	m := map[string]any{"type": b.Type}
	switch b.Type {
	case "text":
		m["text"] = b.Text
	case "tool_use":
		m["id"] = b.ID
		m["name"] = b.Name
		if len(b.Input) == 0 {
			m["input"] = json.RawMessage("{}")
		} else {
			m["input"] = b.Input
		}
	default:
		type plain anthBlock
		return json.Marshal(plain(b))
	}
	return json.Marshal(m)
}

type anthImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data,omitempty"`
	URL       string `json:"url,omitempty"`
}

type anthTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type anthToolChoice struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
}

// parseBlocks reads a content field that is either a string or a block list.
func parseBlocks(raw json.RawMessage) ([]anthBlock, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}
	if strings.HasPrefix(trimmed, `"`) {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return nil, err
		}
		return []anthBlock{{Type: "text", Text: text}}, nil
	}
	var blocks []anthBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, err
	}
	return blocks, nil
}

// systemText flattens the system prompt, which is a string or a block list.
func systemText(raw json.RawMessage) (string, error) {
	blocks, err := parseBlocks(raw)
	if err != nil {
		return "", err
	}
	parts := make([]string, 0, len(blocks))
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n\n"), nil
}

// --- request shapes (OpenAI out) ---

type oaiRequest struct {
	Model         string            `json:"model"`
	Messages      []oaiMessage      `json:"messages"`
	MaxTokens     int               `json:"max_tokens,omitempty"`
	Temperature   *float64          `json:"temperature,omitempty"`
	TopP          *float64          `json:"top_p,omitempty"`
	Stop          []string          `json:"stop,omitempty"`
	Tools         []oaiTool         `json:"tools,omitempty"`
	ToolChoice    any               `json:"tool_choice,omitempty"`
	Stream        bool              `json:"stream,omitempty"`
	StreamOptions *oaiStreamOptions `json:"stream_options,omitempty"`
}

type oaiStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type oaiMessage struct {
	Role       string        `json:"role"`
	Content    any           `json:"content,omitempty"`
	ToolCalls  []oaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
}

type oaiContentPart struct {
	Type     string       `json:"type"`
	Text     string       `json:"text,omitempty"`
	ImageURL *oaiImageURL `json:"image_url,omitempty"`
}

type oaiImageURL struct {
	URL string `json:"url"`
}

type oaiTool struct {
	Type     string      `json:"type"`
	Function oaiFunction `json:"function"`
}

type oaiFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type oaiToolCall struct {
	Index    int             `json:"index,omitempty"`
	ID       string          `json:"id,omitempty"`
	Type     string          `json:"type,omitempty"`
	Function oaiToolFunction `json:"function"`
}

type oaiToolFunction struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// toOpenAI rebuilds an Anthropic request as the OpenAI one the provider reads.
//
// Every Anthropic construct here has an OpenAI counterpart and none of them is
// guessed: text and images become content parts, tool_use becomes tool_calls,
// tool_result becomes a tool message, and the system prompt becomes a system
// message. A block type with no counterpart is refused by name, so a client
// never silently loses part of its context.
func (p *Plugin) toOpenAI(in *anthRequest) (*oaiRequest, error) {
	out := &oaiRequest{
		Model:       in.Model,
		MaxTokens:   in.MaxTokens,
		Temperature: in.Temperature,
		TopP:        in.TopP,
		Stop:        in.StopSequences,
	}
	system, err := systemText(in.System)
	if err != nil {
		return nil, fmt.Errorf("%w: the system prompt cannot be read", ErrInvalidProvider)
	}
	if system != "" {
		out.Messages = append(out.Messages, oaiMessage{Role: "system", Content: system})
	}
	for i, m := range in.Messages {
		blocks, err := parseBlocks(m.Content)
		if err != nil {
			return nil, fmt.Errorf("%w: message %d cannot be read", ErrInvalidProvider, i)
		}
		switch m.Role {
		case "user":
			msgs, err := userMessages(blocks)
			if err != nil {
				return nil, fmt.Errorf("%w: message %d: %v", ErrInvalidProvider, i, err)
			}
			out.Messages = append(out.Messages, msgs...)
		case "assistant":
			msg, err := assistantMessage(blocks)
			if err != nil {
				return nil, fmt.Errorf("%w: message %d: %v", ErrInvalidProvider, i, err)
			}
			out.Messages = append(out.Messages, msg)
		case "system":
			// A coding agent can put a system reminder mid-conversation. OpenAI
			// reads a system message wherever it appears, so its position is
			// kept rather than merged into the prompt at the start.
			text, err := systemText(m.Content)
			if err != nil {
				return nil, fmt.Errorf("%w: message %d cannot be read", ErrInvalidProvider, i)
			}
			out.Messages = append(out.Messages, oaiMessage{Role: "system", Content: text})
		default:
			return nil, fmt.Errorf("%w: message %d has role %q", ErrInvalidProvider, i, m.Role)
		}
	}
	for _, t := range in.Tools {
		if t.Name == "" {
			return nil, fmt.Errorf("%w: a tool has no name", ErrInvalidProvider)
		}
		params := t.InputSchema
		if len(params) == 0 {
			params = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		out.Tools = append(out.Tools, oaiTool{
			Type:     "function",
			Function: oaiFunction{Name: t.Name, Description: t.Description, Parameters: params},
		})
	}
	if len(out.Tools) > 0 {
		out.ToolChoice = toolChoice(in.ToolChoice)
	}
	return out, nil
}

// userMessages turns one Anthropic user turn into the OpenAI messages it
// implies. A tool_result is a separate OpenAI message, and OpenAI requires it
// before the next user message, so the results are emitted first.
func userMessages(blocks []anthBlock) ([]oaiMessage, error) {
	var tool []oaiMessage
	var parts []oaiContentPart
	for _, b := range blocks {
		switch b.Type {
		case "text":
			parts = append(parts, oaiContentPart{Type: "text", Text: b.Text})
		case "image":
			url, err := imageURL(b.Source)
			if err != nil {
				return nil, err
			}
			parts = append(parts, oaiContentPart{Type: "image_url", ImageURL: &oaiImageURL{URL: url}})
		case "tool_result":
			text, err := toolResultText(b.Content)
			if err != nil {
				return nil, err
			}
			tool = append(tool, oaiMessage{Role: "tool", ToolCallID: b.ToolUseID, Content: text})
		default:
			return nil, fmt.Errorf("block type %q has no OpenAI counterpart", b.Type)
		}
	}
	out := append([]oaiMessage{}, tool...)
	if len(parts) > 0 {
		out = append(out, oaiMessage{Role: "user", Content: parts})
	}
	if len(out) == 0 {
		out = append(out, oaiMessage{Role: "user", Content: ""})
	}
	return out, nil
}

// assistantMessage keeps the text and the tool calls of one assistant turn.
func assistantMessage(blocks []anthBlock) (oaiMessage, error) {
	msg := oaiMessage{Role: "assistant"}
	var text strings.Builder
	for _, b := range blocks {
		switch b.Type {
		case "text":
			text.WriteString(b.Text)
		case "tool_use":
			args := strings.TrimSpace(string(b.Input))
			if args == "" {
				args = "{}"
			}
			msg.ToolCalls = append(msg.ToolCalls, oaiToolCall{
				ID: b.ID, Type: "function",
				Function: oaiToolFunction{Name: b.Name, Arguments: args},
			})
		default:
			return msg, fmt.Errorf("assistant block type %q has no OpenAI counterpart", b.Type)
		}
	}
	if text.Len() > 0 || len(msg.ToolCalls) == 0 {
		msg.Content = text.String()
	}
	return msg, nil
}

// toolResultText flattens a tool result, which is a string or a block list.
func toolResultText(raw json.RawMessage) (string, error) {
	blocks, err := parseBlocks(raw)
	if err != nil {
		return "", err
	}
	parts := make([]string, 0, len(blocks))
	for _, b := range blocks {
		switch b.Type {
		case "text":
			parts = append(parts, b.Text)
		case "image":
			// A tool that answers with an image has no OpenAI tool-message
			// form; the client is told rather than shown a wrong answer.
			return "", fmt.Errorf("a tool result image has no OpenAI counterpart")
		}
	}
	return strings.Join(parts, "\n"), nil
}

// imageURL turns an Anthropic image source into the URL form OpenAI reads.
func imageURL(src *anthImageSource) (string, error) {
	if src == nil {
		return "", fmt.Errorf("an image block has no source")
	}
	if src.Type == "url" && src.URL != "" {
		return src.URL, nil
	}
	if src.Type == "base64" && src.Data != "" {
		media := src.MediaType
		if media == "" {
			media = "image/png"
		}
		return "data:" + media + ";base64," + src.Data, nil
	}
	return "", fmt.Errorf("an image source has no data")
}

// toolChoice maps the Anthropic choice onto the OpenAI one.
func toolChoice(c *anthToolChoice) any {
	if c == nil {
		return "auto"
	}
	switch c.Type {
	case "any":
		return "required"
	case "tool":
		return map[string]any{"type": "function", "function": map[string]string{"name": c.Name}}
	case "none":
		return "none"
	default:
		return "auto"
	}
}

// --- response shapes (Anthropic out) ---

type anthResponse struct {
	ID           string      `json:"id"`
	Type         string      `json:"type"`
	Role         string      `json:"role"`
	Model        string      `json:"model"`
	Content      []anthBlock `json:"content"`
	StopReason   string      `json:"stop_reason"`
	StopSequence *string     `json:"stop_sequence"`
	Usage        anthUsage   `json:"usage"`
}

type anthUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type oaiResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content   string        `json:"content"`
			ToolCalls []oaiToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

// stopReason maps the OpenAI finish reason onto the Anthropic one.
func stopReason(finish string) string {
	switch finish {
	case "length":
		return "max_tokens"
	case "tool_calls", "function_call":
		return "tool_use"
	case "stop":
		return "end_turn"
	case "":
		return "end_turn"
	default:
		return "end_turn"
	}
}

// fromOpenAI rebuilds one OpenAI answer as an Anthropic message.
func fromOpenAI(in *oaiResponse) *anthResponse {
	out := &anthResponse{
		ID:         messageID(in.ID),
		Type:       "message",
		Role:       "assistant",
		Model:      in.Model,
		Content:    []anthBlock{},
		StopReason: "end_turn",
		Usage:      anthUsage{InputTokens: in.Usage.PromptTokens, OutputTokens: in.Usage.CompletionTokens},
	}
	if len(in.Choices) == 0 {
		out.Content = append(out.Content, anthBlock{Type: "text", Text: ""})
		return out
	}
	choice := in.Choices[0]
	out.StopReason = stopReason(choice.FinishReason)
	if choice.Message.Content != "" {
		out.Content = append(out.Content, anthBlock{Type: "text", Text: choice.Message.Content})
	}
	for _, call := range choice.Message.ToolCalls {
		input := strings.TrimSpace(call.Function.Arguments)
		if input == "" {
			input = "{}"
		}
		out.Content = append(out.Content, anthBlock{
			Type: "tool_use", ID: call.ID, Name: call.Function.Name,
			Input: json.RawMessage(input),
		})
	}
	if len(out.Content) == 0 {
		out.Content = append(out.Content, anthBlock{Type: "text", Text: ""})
	}
	return out
}

// messageID gives the answer an id in the shape the client expects. The
// provider's id is kept when there is one, so a trace can follow the turn.
func messageID(providerID string) string {
	if providerID == "" {
		return fmt.Sprintf("msg_%d", time.Now().UnixNano())
	}
	return "msg_" + strings.TrimPrefix(providerID, "msg_")
}

// --- the routes ---

// handleAgentMessages serves POST /v1/messages.
func (p *Plugin) handleAgentMessages(w http.ResponseWriter, r *http.Request) {
	orgID, ok := p.agentOrg(w, r)
	if !ok {
		return
	}
	id, ok := agentProviderID(w, r)
	if !ok {
		return
	}
	ap, err := p.agentProviderFor(r.Context(), orgID, id)
	if err != nil {
		writeAnthError(w, http.StatusServiceUnavailable, "api_error", agentErrorText(err))
		return
	}
	body := http.MaxBytesReader(w, r.Body, agentMaxBody)
	defer body.Close()
	raw, err := io.ReadAll(body)
	if err != nil {
		writeAnthError(w, http.StatusBadRequest, "invalid_request_error", "The request body cannot be read.")
		return
	}
	var in anthRequest
	if err := json.Unmarshal(raw, &in); err != nil {
		writeAnthError(w, http.StatusBadRequest, "invalid_request_error", "The request body is not a Messages request.")
		return
	}
	if strings.TrimSpace(in.Model) == "" {
		writeAnthError(w, http.StatusBadRequest, "invalid_request_error", "The request names no model.")
		return
	}
	out, err := p.toOpenAI(&in)
	if err != nil {
		writeAnthError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	out.Stream = in.Stream
	if out.Stream {
		out.StreamOptions = &oaiStreamOptions{IncludeUsage: true}
	}
	if in.Stream {
		p.streamMessages(w, r, ap, out, in)
		return
	}
	p.completeMessages(w, r, ap, out)
}

// completeMessages sends the turn and answers with one Anthropic message.
func (p *Plugin) completeMessages(w http.ResponseWriter, r *http.Request, ap agentProvider, body *oaiRequest) {
	resp, err := p.postProvider(r.Context(), ap, "/chat/completions", body)
	if err != nil {
		writeAnthError(w, http.StatusBadGateway, "api_error", err.Error())
		return
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		writeAnthError(w, http.StatusBadGateway, "api_error", "The provider answer cannot be read.")
		return
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		writeAnthError(w, http.StatusBadGateway, "api_error",
			fmt.Sprintf("The provider refused the turn: %s", upstreamMessage(resp.StatusCode, raw)))
		return
	}
	var oai oaiResponse
	if err := json.Unmarshal(raw, &oai); err != nil {
		writeAnthError(w, http.StatusBadGateway, "api_error", "The provider answer is not a completion.")
		return
	}
	writeJSON(w, http.StatusOK, fromOpenAI(&oai))
}

// postProvider sends one JSON call to the provider's OpenAI surface.
func (p *Plugin) postProvider(ctx context.Context, ap agentProvider, path string, body any) (*http.Response, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ap.BaseURL+path, strings.NewReader(string(raw)))
	if err != nil {
		return nil, err
	}
	ap.sign(req)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if body, ok := body.(*oaiRequest); ok && body.Stream {
		req.Header.Set("Accept", "text/event-stream")
	}
	return http.DefaultClient.Do(req)
}

// --- streaming ---

// streamer writes the Anthropic event sequence while the provider streams.
type streamer struct {
	w         http.ResponseWriter
	flusher   http.Flusher
	model     string
	nextIndex int
	openIndex int // -1 when no block is open
	openText  bool
	tools     map[int]int // OpenAI tool index → Anthropic block index
	stop      string
	outTokens int
	wrote     bool
}

func newStreamer(w http.ResponseWriter, model string) *streamer {
	flusher, _ := w.(http.Flusher)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	return &streamer{w: w, flusher: flusher, model: model, openIndex: -1, tools: map[int]int{}}
}

// event writes one SSE frame. Anthropic clients read the event name as well as
// the payload, so both are written.
func (s *streamer) event(name string, payload any) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", name, raw)
	if s.flusher != nil {
		s.flusher.Flush()
	}
	s.wrote = true
}

func (s *streamer) start(inputTokens int) {
	s.event("message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id": fmt.Sprintf("msg_%d", time.Now().UnixNano()), "type": "message",
			"role": "assistant", "model": s.model, "content": []any{},
			"stop_reason": nil, "stop_sequence": nil,
			"usage": map[string]any{"input_tokens": inputTokens, "output_tokens": 0},
		},
	})
}

func (s *streamer) closeOpen() {
	if s.openIndex < 0 {
		return
	}
	s.event("content_block_stop", map[string]any{"type": "content_block_stop", "index": s.openIndex})
	s.openIndex = -1
	s.openText = false
}

func (s *streamer) textDelta(text string) {
	if text == "" {
		return
	}
	if s.openIndex < 0 || !s.openText {
		s.closeOpen()
		s.openIndex = s.nextIndex
		s.openText = true
		s.nextIndex++
		s.event("content_block_start", map[string]any{
			"type": "content_block_start", "index": s.openIndex,
			"content_block": map[string]any{"type": "text", "text": ""},
		})
	}
	s.outTokens += estimateTokens(text)
	s.event("content_block_delta", map[string]any{
		"type": "content_block_delta", "index": s.openIndex,
		"delta": map[string]any{"type": "text_delta", "text": text},
	})
}

func (s *streamer) toolDelta(call oaiToolCall) {
	block, seen := s.tools[call.Index]
	if !seen {
		s.closeOpen()
		block = s.nextIndex
		s.nextIndex++
		s.tools[call.Index] = block
		s.event("content_block_start", map[string]any{
			"type": "content_block_start", "index": block,
			"content_block": map[string]any{
				"type": "tool_use", "id": call.ID, "name": call.Function.Name, "input": map[string]any{},
			},
		})
	}
	if call.Function.Arguments != "" {
		s.outTokens += estimateTokens(call.Function.Arguments)
		s.event("content_block_delta", map[string]any{
			"type": "content_block_delta", "index": block,
			"delta": map[string]any{"type": "input_json_delta", "partial_json": call.Function.Arguments},
		})
	}
}

func (s *streamer) finish(outputTokens int) {
	if outputTokens > 0 {
		s.outTokens = outputTokens
	}
	s.closeOpen()
	s.event("message_delta", map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": s.stop, "stop_sequence": nil},
		// Only output_tokens: message_delta carries no input count, and a zero
		// here would overwrite the estimate the client already read.
		"usage": map[string]any{"output_tokens": s.outTokens},
	})
	s.event("message_stop", map[string]any{"type": "message_stop"})
}

// streamMessages relays the provider's stream as Anthropic events.
func (p *Plugin) streamMessages(w http.ResponseWriter, r *http.Request, ap agentProvider, body *oaiRequest, in anthRequest) {
	resp, err := p.postProvider(r.Context(), ap, "/chat/completions", body)
	if err != nil {
		writeAnthError(w, http.StatusBadGateway, "api_error", "The provider did not answer.")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		writeAnthError(w, http.StatusBadGateway, "api_error",
			fmt.Sprintf("The provider refused the turn: %s", upstreamMessage(resp.StatusCode, raw)))
		return
	}
	st := newStreamer(w, in.Model)
	st.start(estimateRequestTokens(&in))
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64<<10), 8<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string        `json:"content"`
					ToolCalls []oaiToolCall `json:"tool_calls"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Usage *struct {
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			// A malformed frame is skipped: the stream stays open, and the
			// client still gets every event that parsed.
			continue
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				st.textDelta(choice.Delta.Content)
			}
			for _, call := range choice.Delta.ToolCalls {
				if !seenTool(st, call) && call.Function.Name == "" && call.ID == "" {
					// An arguments-only fragment for an index the provider never
					// announced: nothing to attach it to.
					continue
				}
				st.toolDelta(call)
			}
			if choice.FinishReason != "" {
				st.stop = stopReason(choice.FinishReason)
			}
		}
		if chunk.Usage != nil && chunk.Usage.CompletionTokens > 0 {
			st.outTokens = chunk.Usage.CompletionTokens
		}
	}
	if st.stop == "" {
		st.stop = "end_turn"
	}
	st.finish(st.outTokens)
}

// seenTool reports whether the streamer already opened the block for a call.
func seenTool(s *streamer, call oaiToolCall) bool {
	_, ok := s.tools[call.Index]
	return ok
}

// --- count_tokens ---

// countTokensAnswer is the client's context meter answer.
type countTokensAnswer struct {
	InputTokens int `json:"input_tokens"`
}

// handleAgentCountTokens serves POST /v1/messages/count_tokens.
//
// The number is an estimate, and the code says so: no provider on this path
// exposes a tokenizer, and the client uses the figure only to show how full the
// context is. A turn's real usage arrives with the turn.
func (p *Plugin) handleAgentCountTokens(w http.ResponseWriter, r *http.Request) {
	if _, ok := p.agentOrg(w, r); !ok {
		return
	}
	body := http.MaxBytesReader(w, r.Body, agentMaxBody)
	defer body.Close()
	raw, err := io.ReadAll(body)
	if err != nil {
		writeAnthError(w, http.StatusBadRequest, "invalid_request_error", "The request body cannot be read.")
		return
	}
	var in anthRequest
	if err := json.Unmarshal(raw, &in); err != nil {
		writeAnthError(w, http.StatusBadRequest, "invalid_request_error", "The request body is not a Messages request.")
		return
	}
	writeJSON(w, http.StatusOK, countTokensAnswer{InputTokens: estimateRequestTokens(&in)})
}

// estimateTokens approximates a token count from text length. Four characters
// per token is the usual rule for mixed prose and code; it is deliberately
// rough, and nothing bills on it.
func estimateTokens(text string) int {
	n := utf8.RuneCountInString(text)
	if n == 0 {
		return 0
	}
	return (n + 3) / 4
}

// estimateRequestTokens counts a whole request, including the tool schemas the
// provider also reads.
func estimateRequestTokens(in *anthRequest) int {
	total := 4
	if system, err := systemText(in.System); err == nil {
		total += estimateTokens(system) + 4
	}
	for _, m := range in.Messages {
		total += 4
		blocks, err := parseBlocks(m.Content)
		if err != nil {
			continue
		}
		for _, b := range blocks {
			switch b.Type {
			case "text":
				total += estimateTokens(b.Text)
			case "tool_use":
				total += estimateTokens(string(b.Input)) + estimateTokens(b.Name) + 8
			case "tool_result":
				text, _ := toolResultText(b.Content)
				total += estimateTokens(text) + 4
			case "image":
				total += 1600 // a large image is charged as a block of tokens
			}
		}
	}
	for _, t := range in.Tools {
		total += estimateTokens(t.Name) + estimateTokens(t.Description) + estimateTokens(string(t.InputSchema)) + 8
	}
	return total
}

// --- shared helpers ---

// writeJSON answers one JSON body.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// writeAnthError answers in the Anthropic error shape, which is what the client
// parses and nothing else.
func writeAnthError(w http.ResponseWriter, status int, kind, message string) {
	writeJSON(w, status, map[string]any{
		"type":  "error",
		"error": map[string]string{"type": kind, "message": message},
	})
}
