package llmgateway

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	AgentResponsesPath = "/v1/responses"
)

type respRequest struct {
	Model        string          `json:"model"`
	Input        json.RawMessage `json:"input"`
	Instructions json.RawMessage `json:"instructions,omitempty"`
	Tools        []respTool      `json:"tools,omitempty"`
	ToolChoice   json.RawMessage `json:"tool_choice,omitempty"`

	Temperature     *float64 `json:"temperature,omitempty"`
	TopP            *float64 `json:"top_p,omitempty"`
	MaxOutputTokens int      `json:"max_output_tokens,omitempty"`
	Stream          bool     `json:"stream,omitempty"`

	PreviousResponseID string `json:"previous_response_id,omitempty"`
}

type respTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name,omitempty"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	Tools       []respTool      `json:"tools,omitempty"`
}

type respItem struct {
	Type    string          `json:"type"`
	Role    string          `json:"role,omitempty"`
	Content json.RawMessage `json:"content,omitempty"`

	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`

	Output json.RawMessage `json:"output,omitempty"`
}

type respPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
	FileID   string `json:"file_id,omitempty"`
}

func (p *Plugin) responsesToOpenAI(in *respRequest) (*oaiRequest, error) {
	if in.PreviousResponseID != "" {
		return nil, fmt.Errorf("previous_response_id is not supported: the gateway stores no responses, so send the full input")
	}
	out := &oaiRequest{
		Model:       in.Model,
		Temperature: in.Temperature,
		TopP:        in.TopP,
		MaxTokens:   in.MaxOutputTokens,
	}
	instructions, err := respText(in.Instructions)
	if err != nil {
		return nil, fmt.Errorf("the instructions cannot be read: %w", err)
	}
	if instructions != "" {
		out.Messages = append(out.Messages, oaiMessage{Role: "system", Content: instructions})
	}
	items, err := parseRespInput(in.Input)
	if err != nil {
		return nil, err
	}
	for i := 0; i < len(items); i++ {
		item := items[i]
		switch item.Type {
		case "message":
			msg, err := respMessage(item)
			if err != nil {
				return nil, fmt.Errorf("input item %d: %w", i, err)
			}
			out.Messages = append(out.Messages, msg)
		case "function_call":
			msg := oaiMessage{Role: "assistant"}
			for ; i < len(items) && items[i].Type == "function_call"; i++ {
				call := items[i]
				args := strings.TrimSpace(call.Arguments)
				if args == "" {
					args = "{}"
				}
				msg.ToolCalls = append(msg.ToolCalls, oaiToolCall{
					ID: call.CallID, Type: "function",
					Function: oaiToolFunction{Name: call.Name, Arguments: args},
				})
			}
			i--
			out.Messages = append(out.Messages, msg)
		case "function_call_output":
			text, err := respText(item.Output)
			if err != nil {
				return nil, fmt.Errorf("input item %d: %w", i, err)
			}
			out.Messages = append(out.Messages, oaiMessage{
				Role: "tool", ToolCallID: item.CallID, Content: text,
			})
		case "reasoning":
		default:
			return nil, fmt.Errorf("input item type %q has no Chat Completions counterpart", item.Type)
		}
	}
	if len(out.Messages) == 0 {
		return nil, fmt.Errorf("the input names no message")
	}
	for _, t := range in.Tools {
		flattenRespTools(t, &out.Tools)
	}
	if len(out.Tools) > 0 {
		out.ToolChoice = respToolChoice(in.ToolChoice)
	}
	return out, nil
}

func flattenRespTools(t respTool, out *[]oaiTool) {
	switch t.Type {
	case "function":
		if t.Name == "" {
			return
		}
		params := t.Parameters
		if len(params) == 0 {
			params = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		*out = append(*out, oaiTool{
			Type:     "function",
			Function: oaiFunction{Name: t.Name, Description: t.Description, Parameters: params},
		})
	case "namespace":
		for _, nested := range t.Tools {
			flattenRespTools(nested, out)
		}
	}
}

func respToolChoice(raw json.RawMessage) any {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return "auto"
	}
	if strings.HasPrefix(trimmed, `"`) {
		var name string
		if err := json.Unmarshal(raw, &name); err != nil {
			return "auto"
		}
		switch name {
		case "auto", "none", "required":
			return name
		default:
			return "auto"
		}
	}
	var fn struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &fn); err != nil || fn.Type != "function" || fn.Name == "" {
		return "auto"
	}
	return map[string]any{"type": "function", "function": map[string]string{"name": fn.Name}}
}

func parseRespInput(raw json.RawMessage) ([]respItem, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, fmt.Errorf("the input is empty")
	}
	if strings.HasPrefix(trimmed, `"`) {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return nil, fmt.Errorf("the input cannot be read")
		}
		content, _ := json.Marshal(text)
		return []respItem{{Type: "message", Role: "user", Content: content}}, nil
	}
	var items []respItem
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("the input is not an item list")
	}
	return items, nil
}

func respMessage(item respItem) (oaiMessage, error) {
	parts, err := respParts(item.Content)
	if err != nil {
		return oaiMessage{}, err
	}
	switch item.Role {
	case "developer", "system":
		text, err := partsText(parts)
		if err != nil {
			return oaiMessage{}, err
		}
		return oaiMessage{Role: "system", Content: text}, nil
	case "user":
		out := make([]oaiContentPart, 0, len(parts))
		for _, part := range parts {
			switch part.Type {
			case "input_text", "output_text", "text":
				out = append(out, oaiContentPart{Type: "text", Text: part.Text})
			case "input_image":
				url := part.ImageURL
				if url == "" && part.FileID != "" {
					return oaiMessage{}, fmt.Errorf("an input_image names a file_id, and the provider cannot read one")
				}
				if url == "" {
					return oaiMessage{}, fmt.Errorf("an input_image has no URL")
				}
				out = append(out, oaiContentPart{Type: "image_url", ImageURL: &oaiImageURL{URL: url}})
			default:
				return oaiMessage{}, fmt.Errorf("content part type %q has no Chat Completions counterpart", part.Type)
			}
		}
		if len(out) == 0 {
			return oaiMessage{Role: "user", Content: ""}, nil
		}
		return oaiMessage{Role: "user", Content: out}, nil
	case "assistant":
		text, err := partsText(parts)
		if err != nil {
			return oaiMessage{}, err
		}
		return oaiMessage{Role: "assistant", Content: text}, nil
	default:
		return oaiMessage{}, fmt.Errorf("message role %q has no Chat Completions counterpart", item.Role)
	}
}

func respParts(raw json.RawMessage) ([]respPart, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}
	if strings.HasPrefix(trimmed, `"`) {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return nil, fmt.Errorf("the content cannot be read")
		}
		return []respPart{{Type: "input_text", Text: text}}, nil
	}
	var parts []respPart
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, fmt.Errorf("the content is not a part list")
	}
	return parts, nil
}

func partsText(parts []respPart) (string, error) {
	texts := make([]string, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case "input_text", "output_text", "text":
			texts = append(texts, part.Text)
		default:
			return "", fmt.Errorf("content part type %q holds no text", part.Type)
		}
	}
	return strings.Join(texts, "\n"), nil
}

func respText(raw json.RawMessage) (string, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return "", nil
	}
	if strings.HasPrefix(trimmed, `"`) {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return "", fmt.Errorf("the field cannot be read")
		}
		return text, nil
	}
	if strings.HasPrefix(trimmed, "[") {
		parts, err := respParts(raw)
		if err != nil {
			return "", err
		}
		return partsText(parts)
	}
	return trimmed, nil
}

type respResponse struct {
	ID        string       `json:"id"`
	Object    string       `json:"object"`
	CreatedAt int64        `json:"created_at"`
	Status    string       `json:"status"`
	Model     string       `json:"model"`
	Output    []respOutput `json:"output"`
	Usage     respUsage    `json:"usage,omitempty"`
}

type respOutput struct {
	Type      string         `json:"type"`
	ID        string         `json:"id"`
	Status    string         `json:"status,omitempty"`
	Role      string         `json:"role,omitempty"`
	Content   *[]respOutPart `json:"content,omitempty"`
	CallID    string         `json:"call_id,omitempty"`
	Name      string         `json:"name,omitempty"`
	Arguments string         `json:"arguments,omitempty"`
}

type respOutPart struct {
	Type        string `json:"type"`
	Text        string `json:"text"`
	Annotations []any  `json:"annotations"`
}

type respUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

func fromOpenAIResponse(providerID, model string, in *oaiResponse) *respResponse {
	out := &respResponse{
		ID:        responseID(providerID),
		Object:    "response",
		CreatedAt: time.Now().Unix(),
		Status:    "completed",
		Model:     model,
		Output:    []respOutput{},
	}
	if in.Model != "" {
		out.Model = in.Model
	}
	if len(in.Choices) == 0 {
		out.Output = append(out.Output, respTextItem(0, ""))
	} else {
		choice := in.Choices[0]
		if choice.Message.Content != "" {
			out.Output = append(out.Output, respTextItem(0, choice.Message.Content))
		}
		for i, call := range choice.Message.ToolCalls {
			args := strings.TrimSpace(call.Function.Arguments)
			if args == "" {
				args = "{}"
			}
			callID := call.ID
			if callID == "" {
				callID = fmt.Sprintf("call_%d_%d", time.Now().UnixNano(), i)
			}
			out.Output = append(out.Output, respOutput{
				Type: "function_call", ID: callItemID(i), Status: "completed",
				CallID: callID, Name: call.Function.Name, Arguments: args,
			})
		}
		if len(out.Output) == 0 {
			out.Output = append(out.Output, respTextItem(0, ""))
		}
	}
	out.Usage = respUsage{
		InputTokens:  in.Usage.PromptTokens,
		OutputTokens: in.Usage.CompletionTokens,
		TotalTokens:  in.Usage.PromptTokens + in.Usage.CompletionTokens,
	}
	return out
}

func respTextItem(index int, text string) respOutput {
	return respOutput{
		Type: "message", ID: messageItemID(index), Status: "completed", Role: "assistant",
		Content: outputContent(respOutPart{Type: "output_text", Text: text, Annotations: []any{}}),
	}
}

func outputContent(parts ...respOutPart) *[]respOutPart {
	if parts == nil {
		parts = []respOutPart{}
	}
	return &parts
}

func responseID(providerID string) string {
	if providerID == "" {
		return fmt.Sprintf("resp_%d", time.Now().UnixNano())
	}
	return "resp_" + strings.TrimPrefix(providerID, "resp_")
}

func messageItemID(index int) string {
	return fmt.Sprintf("msg_%d_%d", time.Now().UnixNano(), index)
}

func callItemID(index int) string {
	return fmt.Sprintf("fc_%d_%d", time.Now().UnixNano(), index)
}

func (p *Plugin) handleAgentResponses(w http.ResponseWriter, r *http.Request) {
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
		writeAgentError(w, http.StatusServiceUnavailable, "invalid_request_error", agentErrorText(err))
		return
	}
	body := http.MaxBytesReader(w, r.Body, agentMaxBody)
	defer body.Close()
	raw, err := io.ReadAll(body)
	if err != nil {
		writeAgentError(w, http.StatusBadRequest, "invalid_request_error", "The request body cannot be read.")
		return
	}
	var in respRequest
	if err := json.Unmarshal(raw, &in); err != nil {
		writeAgentError(w, http.StatusBadRequest, "invalid_request_error", "The request body is not a Responses request.")
		return
	}
	if strings.TrimSpace(in.Model) == "" {
		writeAgentError(w, http.StatusBadRequest, "invalid_request_error", "The request names no model.")
		return
	}
	out, err := p.responsesToOpenAI(&in)
	if err != nil {
		writeAgentError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	out.Stream = in.Stream
	if out.Stream {
		out.StreamOptions = &oaiStreamOptions{IncludeUsage: true}
	}
	if in.Stream {
		p.streamResponses(w, r, ap, out, in.Model)
		return
	}
	p.completeResponses(w, r, ap, out, in.Model)
}

func (p *Plugin) completeResponses(w http.ResponseWriter, r *http.Request, ap agentProvider, body *oaiRequest, model string) {
	resp, err := p.postProvider(r.Context(), ap, "/chat/completions", body)
	if err != nil {
		writeAgentError(w, http.StatusBadGateway, "upstream_error", "The provider did not answer.")
		return
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		writeAgentError(w, http.StatusBadGateway, "upstream_error", "The provider answer cannot be read.")
		return
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		writeAgentError(w, http.StatusBadGateway, "upstream_error",
			fmt.Sprintf("The provider refused the turn: %s", upstreamMessage(resp.StatusCode, raw)))
		return
	}
	var oai oaiResponse
	if err := json.Unmarshal(raw, &oai); err != nil {
		writeAgentError(w, http.StatusBadGateway, "upstream_error", "The provider answer is not a completion.")
		return
	}
	writeJSON(w, http.StatusOK, fromOpenAIResponse(oai.ID, model, &oai))
}

type respStreamer struct {
	w         http.ResponseWriter
	flusher   http.Flusher
	response  *respResponse
	seq       int
	nextIndex int

	textOpen  bool
	textIndex int
	textID    string
	textBuf   strings.Builder

	tools map[int]*respToolStream
}

type respToolStream struct {
	index     int
	itemID    string
	callID    string
	name      string
	arguments strings.Builder
}

func newRespStreamer(w http.ResponseWriter, model string) *respStreamer {
	flusher, _ := w.(http.Flusher)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	return &respStreamer{
		w:       w,
		flusher: flusher,
		response: &respResponse{
			ID:        responseID(""),
			Object:    "response",
			CreatedAt: time.Now().Unix(),
			Status:    "in_progress",
			Model:     model,
			Output:    []respOutput{},
		},
		tools: map[int]*respToolStream{},
	}
}

func (s *respStreamer) event(name string, payload map[string]any) {
	payload["type"] = name
	payload["sequence_number"] = s.seq
	s.seq++
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", name, raw)
	if s.flusher != nil {
		s.flusher.Flush()
	}
}

func (s *respStreamer) start() {
	s.event("response.created", map[string]any{"response": s.snapshot()})
}

func (s *respStreamer) snapshot() *respResponse {
	copied := *s.response
	return &copied
}

func (s *respStreamer) textDelta(text string) {
	if text == "" {
		return
	}
	if !s.textOpen {
		s.textOpen = true
		s.textIndex = s.nextIndex
		s.nextIndex++
		s.textID = messageItemID(s.textIndex)
		s.response.Output = append(s.response.Output, respOutput{
			Type: "message", ID: s.textID, Status: "in_progress",
			Role: "assistant", Content: outputContent(),
		})
		s.event("response.output_item.added", map[string]any{
			"output_index": s.textIndex,
			"item": respOutput{
				Type: "message", ID: s.textID, Status: "in_progress",
				Role: "assistant", Content: outputContent(),
			},
		})
		s.event("response.content_part.added", map[string]any{
			"item_id": s.textID, "output_index": s.textIndex,
			"content_index": 0,
			"part":          respOutPart{Type: "output_text", Text: "", Annotations: []any{}},
		})
	}
	s.textBuf.WriteString(text)
	s.event("response.output_text.delta", map[string]any{
		"item_id": s.textID, "output_index": s.textIndex,
		"content_index": 0, "delta": text,
	})
}

func (s *respStreamer) closeText() {
	if !s.textOpen {
		return
	}
	s.textOpen = false
	text := s.textBuf.String()
	itemID := s.textID
	s.event("response.output_text.done", map[string]any{
		"item_id": itemID, "output_index": s.textIndex, "content_index": 0, "text": text,
	})
	part := respOutPart{Type: "output_text", Text: text, Annotations: []any{}}
	s.event("response.content_part.done", map[string]any{
		"item_id": itemID, "output_index": s.textIndex, "content_index": 0, "part": part,
	})
	item := respOutput{
		Type: "message", ID: itemID, Status: "completed", Role: "assistant",
		Content: outputContent(part),
	}
	s.response.Output[s.textIndex] = item
	s.event("response.output_item.done", map[string]any{
		"output_index": s.textIndex, "item": item,
	})
}

func (s *respStreamer) toolDelta(call oaiToolCall) {
	stream, open := s.tools[call.Index]
	if !open {
		s.closeText()
		stream = &respToolStream{
			index:  s.nextIndex,
			itemID: callItemID(call.Index),
			callID: call.ID,
		}
		s.nextIndex++
		s.tools[call.Index] = stream
		s.response.Output = append(s.response.Output, respOutput{
			Type: "function_call", ID: stream.itemID, Status: "in_progress",
			CallID: stream.callID, Name: call.Function.Name, Arguments: "",
		})
		s.event("response.output_item.added", map[string]any{
			"output_index": stream.index,
			"item": respOutput{
				Type: "function_call", ID: stream.itemID, Status: "in_progress",
				CallID: stream.callID, Name: call.Function.Name, Arguments: "",
			},
		})
	}
	if call.ID != "" && stream.callID == "" {
		stream.callID = call.ID
	}
	if call.Function.Name != "" && stream.name == "" {
		stream.name = call.Function.Name
	}
	if call.Function.Arguments != "" {
		stream.arguments.WriteString(call.Function.Arguments)
		s.event("response.function_call_arguments.delta", map[string]any{
			"item_id": stream.itemID, "output_index": stream.index,
			"delta": call.Function.Arguments,
		})
	}
}

func (s *respStreamer) closeTools() {
	indexes := make([]int, 0, len(s.tools))
	for index := range s.tools {
		indexes = append(indexes, index)
	}
	for i := 0; i < len(indexes); i++ {
		for j := i + 1; j < len(indexes); j++ {
			if indexes[j] < indexes[i] {
				indexes[i], indexes[j] = indexes[j], indexes[i]
			}
		}
	}
	for _, index := range indexes {
		stream := s.tools[index]
		args := stream.arguments.String()
		if args == "" {
			args = "{}"
		}
		callID := stream.callID
		if callID == "" {
			callID = fmt.Sprintf("call_%d_%d", time.Now().UnixNano(), index)
		}
		s.event("response.function_call_arguments.done", map[string]any{
			"item_id": stream.itemID, "output_index": stream.index, "arguments": args,
		})
		item := respOutput{
			Type: "function_call", ID: stream.itemID, Status: "completed",
			CallID: callID, Name: stream.name, Arguments: args,
		}
		s.response.Output[stream.index] = item
		s.event("response.output_item.done", map[string]any{
			"output_index": stream.index, "item": item,
		})
	}
	s.tools = map[int]*respToolStream{}
}

func (s *respStreamer) finish(inputTokens, outputTokens int) {
	s.closeText()
	s.closeTools()
	if inputTokens > 0 {
		s.response.Usage.InputTokens = inputTokens
	}
	s.response.Usage.OutputTokens = outputTokens
	s.response.Usage.TotalTokens = s.response.Usage.InputTokens + outputTokens
	s.response.Status = "completed"
	s.event("response.completed", map[string]any{"response": s.snapshot()})
}

func (p *Plugin) streamResponses(w http.ResponseWriter, r *http.Request, ap agentProvider, body *oaiRequest, model string) {
	resp, err := p.postProvider(r.Context(), ap, "/chat/completions", body)
	if err != nil {
		writeAgentError(w, http.StatusBadGateway, "upstream_error", "The provider did not answer.")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		writeAgentError(w, http.StatusBadGateway, "upstream_error",
			fmt.Sprintf("The provider refused the turn: %s", upstreamMessage(resp.StatusCode, raw)))
		return
	}
	st := newRespStreamer(w, model)
	st.start()
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64<<10), 8<<20)
	outputTokens := 0
	inputTokens := estimateResponsesTokens(body)
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
			} `json:"choices"`
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				st.textDelta(choice.Delta.Content)
			}
			for _, call := range choice.Delta.ToolCalls {
				if call.Function.Name == "" && call.ID == "" && call.Function.Arguments == "" {
					continue
				}
				st.toolDelta(call)
			}
		}
		if chunk.Usage != nil {
			if chunk.Usage.PromptTokens > 0 {
				inputTokens = chunk.Usage.PromptTokens
			}
			if chunk.Usage.CompletionTokens > 0 {
				outputTokens = chunk.Usage.CompletionTokens
			}
		}
	}
	st.finish(inputTokens, outputTokens)
}

func estimateResponsesTokens(body *oaiRequest) int {
	total := 4
	for _, msg := range body.Messages {
		total += 4
		switch content := msg.Content.(type) {
		case string:
			total += estimateTokens(content)
		case []oaiContentPart:
			for _, part := range content {
				if part.Type == "image_url" {
					total += 1600
					continue
				}
				total += estimateTokens(part.Text)
			}
		}
		for _, call := range msg.ToolCalls {
			total += estimateTokens(call.Function.Name) + estimateTokens(call.Function.Arguments) + 8
		}
	}
	for _, tool := range body.Tools {
		total += estimateTokens(tool.Function.Name) + estimateTokens(tool.Function.Description) +
			estimateTokens(string(tool.Function.Parameters)) + 8
	}
	return total
}
