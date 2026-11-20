package llmgateway

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	AgentGeminiModelsPath = "/v1beta/models"
	agentGeminiPrefix     = "/v1beta/models/"
)

type gemRequest struct {
	Contents          []gemContent   `json:"contents"`
	SystemInstruction *gemContent    `json:"systemInstruction,omitempty"`
	Tools             []gemToolGroup `json:"tools,omitempty"`
	ToolConfig        *gemToolConfig `json:"toolConfig,omitempty"`
	GenerationConfig  *gemGeneration `json:"generationConfig,omitempty"`
}

type gemContent struct {
	Role  string    `json:"role,omitempty"`
	Parts []gemPart `json:"parts"`
}

type gemPart struct {
	Text             string           `json:"text,omitempty"`
	Thought          bool             `json:"thought,omitempty"`
	FunctionCall     *gemFunctionCall `json:"functionCall,omitempty"`
	FunctionResponse *gemFunctionResp `json:"functionResponse,omitempty"`
	InlineData       *gemInlineData   `json:"inlineData,omitempty"`
	FileData         *gemFileData     `json:"fileData,omitempty"`
}

type gemFunctionCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

type gemFunctionResp struct {
	Name     string          `json:"name"`
	Response json.RawMessage `json:"response,omitempty"`
}

type gemInlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type gemFileData struct {
	MimeType string `json:"mimeType,omitempty"`
	FileURI  string `json:"fileUri,omitempty"`
}

type gemToolGroup struct {
	FunctionDeclarations []gemFunctionDecl `json:"functionDeclarations,omitempty"`
}

type gemFunctionDecl struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type gemToolConfig struct {
	FunctionCallingConfig *gemCallingConfig `json:"functionCallingConfig,omitempty"`
}

type gemCallingConfig struct {
	Mode                 string   `json:"mode,omitempty"`
	AllowedFunctionNames []string `json:"allowedFunctionNames,omitempty"`
}

type gemGeneration struct {
	Temperature        *float64        `json:"temperature,omitempty"`
	TopP               *float64        `json:"topP,omitempty"`
	MaxOutputTokens    int             `json:"maxOutputTokens,omitempty"`
	StopSequences      []string        `json:"stopSequences,omitempty"`
	ResponseMimeType   string          `json:"responseMimeType,omitempty"`
	ResponseJSONSchema json.RawMessage `json:"responseJsonSchema,omitempty"`
}

func geminiToOpenAI(in *gemRequest, model string) (*oaiRequest, error) {
	out := &oaiRequest{Model: model}
	if in.SystemInstruction != nil {
		if text := gemPartsText(in.SystemInstruction.Parts); text != "" {
			out.Messages = append(out.Messages, oaiMessage{Role: "system", Content: text})
		}
	}
	pending := map[string][]string{}
	callSeq := 0
	for i, c := range in.Contents {
		switch c.Role {
		case "user", "":
			msgs, err := geminiUserMessages(c.Parts, pending)
			if err != nil {
				return nil, fmt.Errorf("%w: content %d: %v", ErrInvalidProvider, i, err)
			}
			out.Messages = append(out.Messages, msgs...)
		case "model":
			msg, minted, err := geminiModelMessage(c.Parts, callSeq, pending)
			if err != nil {
				return nil, fmt.Errorf("%w: content %d: %v", ErrInvalidProvider, i, err)
			}
			callSeq = minted
			out.Messages = append(out.Messages, msg)
		default:
			return nil, fmt.Errorf("%w: content %d has role %q", ErrInvalidProvider, i, c.Role)
		}
	}
	for _, group := range in.Tools {
		for _, decl := range group.FunctionDeclarations {
			if decl.Name == "" {
				return nil, fmt.Errorf("%w: a function declaration has no name", ErrInvalidProvider)
			}
			params := decl.Parameters
			if len(params) == 0 {
				params = json.RawMessage(`{"type":"object","properties":{}}`)
			}
			out.Tools = append(out.Tools, oaiTool{
				Type:     "function",
				Function: oaiFunction{Name: decl.Name, Description: decl.Description, Parameters: params},
			})
		}
	}
	if len(out.Tools) > 0 {
		out.ToolChoice = geminiToolChoice(in.ToolConfig)
	}
	if g := in.GenerationConfig; g != nil {
		out.MaxTokens = g.MaxOutputTokens
		out.Temperature = g.Temperature
		out.TopP = g.TopP
		out.Stop = g.StopSequences
		if format := geminiResponseFormat(g); format != nil {
			out.ResponseFormat = format
		}
	}
	return out, nil
}

func gemPartsText(parts []gemPart) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p.Text != "" && !p.Thought {
			out = append(out, p.Text)
		}
	}
	return strings.Join(out, "\n\n")
}

func geminiUserMessages(parts []gemPart, pending map[string][]string) ([]oaiMessage, error) {
	var tool []oaiMessage
	var content []oaiContentPart
	for _, p := range parts {
		switch {
		case p.FunctionResponse != nil:
			id := takeCallID(pending, p.FunctionResponse.Name)
			tool = append(tool, oaiMessage{
				Role: "tool", ToolCallID: id, Content: functionResponseText(p.FunctionResponse.Response),
			})
		case p.InlineData != nil:
			content = append(content, oaiContentPart{
				Type:     "image_url",
				ImageURL: &oaiImageURL{URL: "data:" + p.InlineData.MimeType + ";base64," + p.InlineData.Data},
			})
		case p.FileData != nil:
			return nil, fmt.Errorf("a file part has no OpenAI counterpart; send its bytes as inlineData")
		case p.Text != "" && !p.Thought:
			content = append(content, oaiContentPart{Type: "text", Text: p.Text})
		}
	}
	out := append([]oaiMessage{}, tool...)
	if len(content) > 0 {
		out = append(out, oaiMessage{Role: "user", Content: content})
	}
	if len(out) == 0 {
		out = append(out, oaiMessage{Role: "user", Content: ""})
	}
	return out, nil
}

func geminiModelMessage(parts []gemPart, seq int, pending map[string][]string) (oaiMessage, int, error) {
	msg := oaiMessage{Role: "assistant"}
	var text strings.Builder
	for _, p := range parts {
		switch {
		case p.FunctionCall != nil:
			args := strings.TrimSpace(string(p.FunctionCall.Args))
			if args == "" || args == "null" {
				args = "{}"
			}
			id := fmt.Sprintf("call_%d", seq)
			seq++
			pending[p.FunctionCall.Name] = append(pending[p.FunctionCall.Name], id)
			msg.ToolCalls = append(msg.ToolCalls, oaiToolCall{
				ID: id, Type: "function",
				Function: oaiToolFunction{Name: p.FunctionCall.Name, Arguments: args},
			})
		case p.Text != "" && !p.Thought:
			text.WriteString(p.Text)
		}
	}
	if text.Len() > 0 || len(msg.ToolCalls) == 0 {
		msg.Content = text.String()
	}
	return msg, seq, nil
}

func takeCallID(pending map[string][]string, name string) string {
	ids := pending[name]
	if len(ids) == 0 {
		return "call_" + name
	}
	pending[name] = ids[1:]
	return ids[0]
}

func functionResponseText(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return "{}"
	}
	return trimmed
}

func geminiToolChoice(cfg *gemToolConfig) any {
	if cfg == nil || cfg.FunctionCallingConfig == nil {
		return "auto"
	}
	c := cfg.FunctionCallingConfig
	switch strings.ToUpper(strings.TrimSpace(c.Mode)) {
	case "ANY":
		if len(c.AllowedFunctionNames) == 1 {
			return map[string]any{"type": "function", "function": map[string]string{"name": c.AllowedFunctionNames[0]}}
		}
		return "required"
	case "NONE":
		return "none"
	default:
		return "auto"
	}
}

func geminiResponseFormat(g *gemGeneration) any {
	schema := strings.TrimSpace(string(g.ResponseJSONSchema))
	if schema != "" && schema != "null" {
		return map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name": "response", "schema": json.RawMessage(schema), "strict": false,
			},
		}
	}
	if strings.EqualFold(strings.TrimSpace(g.ResponseMimeType), "application/json") {
		return map[string]any{"type": "json_object"}
	}
	return nil
}

type gemResponse struct {
	Candidates    []gemCandidate `json:"candidates"`
	UsageMetadata *gemUsage      `json:"usageMetadata,omitempty"`
	ModelVersion  string         `json:"modelVersion,omitempty"`
}

type gemCandidate struct {
	Content      gemContent `json:"content"`
	FinishReason string     `json:"finishReason,omitempty"`
	Index        int        `json:"index"`
}

type gemUsage struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

func geminiFinishReason(finish string) string {
	switch finish {
	case "length":
		return "MAX_TOKENS"
	case "content_filter":
		return "SAFETY"
	default:
		return "STOP"
	}
}

func geminiFromOpenAI(in *oaiResponse, model string) (*gemResponse, error) {
	out := &gemResponse{ModelVersion: model}
	if len(in.Choices) == 0 {
		out.Candidates = []gemCandidate{{
			Content:      gemContent{Role: "model", Parts: []gemPart{{Text: ""}}},
			FinishReason: "STOP",
		}}
		return out, nil
	}
	choice := in.Choices[0]
	candidate := gemCandidate{
		Content:      gemContent{Role: "model", Parts: []gemPart{}},
		FinishReason: geminiFinishReason(choice.FinishReason),
	}
	if choice.Message.Content != "" {
		candidate.Content.Parts = append(candidate.Content.Parts, gemPart{Text: choice.Message.Content})
	}
	for _, call := range choice.Message.ToolCalls {
		args := strings.TrimSpace(call.Function.Arguments)
		if args == "" {
			args = "{}"
		}
		if !json.Valid([]byte(args)) {
			return nil, fmt.Errorf("the provider returned tool arguments that are not JSON")
		}
		candidate.Content.Parts = append(candidate.Content.Parts, gemPart{
			FunctionCall: &gemFunctionCall{Name: call.Function.Name, Args: json.RawMessage(args)},
		})
	}
	if len(candidate.Content.Parts) == 0 {
		candidate.Content.Parts = append(candidate.Content.Parts, gemPart{Text: ""})
	}
	out.Candidates = []gemCandidate{candidate}
	out.UsageMetadata = &gemUsage{
		PromptTokenCount:     in.Usage.PromptTokens,
		CandidatesTokenCount: in.Usage.CompletionTokens,
		TotalTokenCount:      in.Usage.PromptTokens + in.Usage.CompletionTokens,
	}
	return out, nil
}

func (p *Plugin) handleGeminiModels(w http.ResponseWriter, r *http.Request) {
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
		writeGeminiError(w, http.StatusServiceUnavailable, "FAILED_PRECONDITION", agentErrorText(err))
		return
	}
	models, err := p.discoveredModels(r.Context(), ap)
	if err != nil {
		writeGeminiError(w, http.StatusBadGateway, "UNAVAILABLE", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, gemModelList{Models: geminiModelEntries(models)})
}

func (p *Plugin) handleGeminiModel(w http.ResponseWriter, r *http.Request) {
	name, action := geminiPathModel(r)
	if action != "" {
		writeGeminiError(w, http.StatusNotFound, "NOT_FOUND", "The path names no model.")
		return
	}
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
		writeGeminiError(w, http.StatusServiceUnavailable, "FAILED_PRECONDITION", agentErrorText(err))
		return
	}
	models, err := p.discoveredModels(r.Context(), ap)
	if err != nil {
		writeGeminiError(w, http.StatusBadGateway, "UNAVAILABLE", err.Error())
		return
	}
	for _, entry := range geminiModelEntries(models) {
		if entry.Name == "models/"+name {
			writeJSON(w, http.StatusOK, entry)
			return
		}
	}
	writeGeminiError(w, http.StatusNotFound, "NOT_FOUND",
		fmt.Sprintf("The workspace offers no model named %q.", name))
}

func (p *Plugin) handleGeminiAction(w http.ResponseWriter, r *http.Request) {
	name, action := geminiPathModel(r)
	if name == "" || action == "" {
		writeGeminiError(w, http.StatusNotFound, "NOT_FOUND", "The path names no action.")
		return
	}
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
		writeGeminiError(w, http.StatusServiceUnavailable, "FAILED_PRECONDITION", agentErrorText(err))
		return
	}
	switch action {
	case "countTokens", "generateContent", "streamGenerateContent":
	default:
		writeGeminiError(w, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("The action %q does not exist.", action))
		return
	}
	body := http.MaxBytesReader(w, r.Body, agentMaxBody)
	defer body.Close()
	raw, err := io.ReadAll(body)
	if err != nil {
		writeGeminiError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "The request body cannot be read.")
		return
	}
	var in gemRequest
	if err := json.Unmarshal(raw, &in); err != nil {
		writeGeminiError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "The request body is not a GenerateContent request.")
		return
	}
	if action == "countTokens" {
		writeJSON(w, http.StatusOK, map[string]int{"totalTokens": estimateGeminiTokens(&in)})
		return
	}
	model, ok := p.geminiModel(w, r, ap)
	if !ok {
		return
	}
	out, err := geminiToOpenAI(&in, model)
	if err != nil {
		writeGeminiError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
		return
	}
	if action == "generateContent" {
		p.completeGemini(w, r, ap, out, model, false)
		return
	}
	if r.URL.Query().Get("alt") == "sse" {
		out.Stream = true
		out.StreamOptions = &oaiStreamOptions{IncludeUsage: true}
		p.streamGemini(w, r, ap, out, model)
		return
	}
	p.completeGemini(w, r, ap, out, model, true)
}

func geminiPathModel(r *http.Request) (string, string) {
	raw := strings.TrimPrefix(r.PathValue("model"), "models/")
	if i := strings.LastIndex(raw, ":"); i >= 0 {
		return raw[:i], raw[i+1:]
	}
	return raw, ""
}

func (p *Plugin) geminiModel(w http.ResponseWriter, r *http.Request, ap agentProvider) (string, bool) {
	if want := strings.TrimSpace(r.URL.Query().Get("model")); want != "" {
		return want, true
	}
	if ap.Model != "" {
		return ap.Model, true
	}
	writeGeminiError(w, http.StatusServiceUnavailable, "FAILED_PRECONDITION",
		"This workspace has no model configured. Add one in the console.")
	return "", false
}

func (p *Plugin) completeGemini(w http.ResponseWriter, r *http.Request, ap agentProvider, body *oaiRequest, model string, asArray bool) {
	resp, err := p.postProvider(r.Context(), ap, "/chat/completions", body)
	if err != nil {
		writeGeminiError(w, http.StatusBadGateway, "UNAVAILABLE", "The provider did not answer.")
		return
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		writeGeminiError(w, http.StatusBadGateway, "UNAVAILABLE", "The provider answer cannot be read.")
		return
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		writeGeminiError(w, http.StatusBadGateway, "UNAVAILABLE",
			fmt.Sprintf("The provider refused the turn: %s", upstreamMessage(resp.StatusCode, raw)))
		return
	}
	var oai oaiResponse
	if err := json.Unmarshal(raw, &oai); err != nil {
		writeGeminiError(w, http.StatusBadGateway, "UNAVAILABLE", "The provider answer is not a completion.")
		return
	}
	answer, err := geminiFromOpenAI(&oai, model)
	if err != nil {
		writeGeminiError(w, http.StatusBadGateway, "UNAVAILABLE", err.Error())
		return
	}
	if asArray {
		writeJSON(w, http.StatusOK, []*gemResponse{answer})
		return
	}
	writeJSON(w, http.StatusOK, answer)
}

func (p *Plugin) streamGemini(w http.ResponseWriter, r *http.Request, ap agentProvider, body *oaiRequest, model string) {
	resp, err := p.postProvider(r.Context(), ap, "/chat/completions", body)
	if err != nil {
		writeGeminiError(w, http.StatusBadGateway, "UNAVAILABLE", "The provider did not answer.")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		writeGeminiError(w, http.StatusBadGateway, "UNAVAILABLE",
			fmt.Sprintf("The provider refused the turn: %s", upstreamMessage(resp.StatusCode, raw)))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	writeFrame := func(payload any) {
		raw, err := json.Marshal(payload)
		if err != nil {
			return
		}
		fmt.Fprintf(w, "data: %s\r\n\r\n", raw)
		if flusher != nil {
			flusher.Flush()
		}
	}

	calls := map[int]*oaiToolCall{}
	var order []int
	finish := ""
	var usage *gemUsage

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
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				writeFrame(&gemResponse{
					Candidates: []gemCandidate{{
						Content:      gemContent{Role: "model", Parts: []gemPart{{Text: choice.Delta.Content}}},
						FinishReason: "",
					}},
					ModelVersion: model,
				})
			}
			for _, delta := range choice.Delta.ToolCalls {
				call := calls[delta.Index]
				if call == nil {
					call = &oaiToolCall{Index: delta.Index}
					calls[delta.Index] = call
					order = append(order, delta.Index)
				}
				if delta.ID != "" {
					call.ID = delta.ID
				}
				if delta.Function.Name != "" {
					call.Function.Name = delta.Function.Name
				}
				call.Function.Arguments += delta.Function.Arguments
			}
			if choice.FinishReason != "" {
				finish = choice.FinishReason
			}
		}
		if chunk.Usage != nil {
			usage = &gemUsage{
				PromptTokenCount:     chunk.Usage.PromptTokens,
				CandidatesTokenCount: chunk.Usage.CompletionTokens,
				TotalTokenCount:      chunk.Usage.PromptTokens + chunk.Usage.CompletionTokens,
			}
		}
	}

	parts := make([]gemPart, 0, len(order))
	for _, index := range order {
		call := calls[index]
		args := strings.TrimSpace(call.Function.Arguments)
		if args == "" {
			args = "{}"
		}
		if !json.Valid([]byte(args)) {
			writeFrame(gemErrorBody(http.StatusBadGateway, "UNAVAILABLE",
				"The provider streamed tool arguments that are not JSON."))
			return
		}
		parts = append(parts, gemPart{FunctionCall: &gemFunctionCall{Name: call.Function.Name, Args: json.RawMessage(args)}})
	}
	if len(parts) > 0 {
		writeFrame(&gemResponse{
			Candidates:   []gemCandidate{{Content: gemContent{Role: "model", Parts: parts}, FinishReason: geminiFinishReason(finish)}},
			ModelVersion: model,
		})
	}
	if usage == nil {
		usage = &gemUsage{}
	}
	writeFrame(&gemResponse{
		Candidates:    []gemCandidate{{Content: gemContent{Role: "model", Parts: []gemPart{}}, FinishReason: geminiFinishReason(finish)}},
		UsageMetadata: usage,
		ModelVersion:  model,
	})
}

type gemModelList struct {
	Models []gemModel `json:"models"`
}

type gemModel struct {
	Name                       string   `json:"name"`
	DisplayName                string   `json:"displayName"`
	Description                string   `json:"description,omitempty"`
	SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
}

func geminiModelEntries(models []modelEntry) []gemModel {
	out := make([]gemModel, 0, len(models))
	for _, m := range models {
		out = append(out, gemModel{
			Name:                       "models/" + m.ID,
			DisplayName:                m.ID,
			SupportedGenerationMethods: []string{"generateContent", "streamGenerateContent", "countTokens"},
		})
	}
	return out
}

func estimateGeminiTokens(in *gemRequest) int {
	total := 4
	if in.SystemInstruction != nil {
		total += estimateTokens(gemPartsText(in.SystemInstruction.Parts)) + 4
	}
	for _, c := range in.Contents {
		total += 4
		for _, p := range c.Parts {
			switch {
			case p.FunctionCall != nil:
				total += estimateTokens(p.FunctionCall.Name) + estimateTokens(string(p.FunctionCall.Args)) + 8
			case p.FunctionResponse != nil:
				total += estimateTokens(string(p.FunctionResponse.Response)) + 4
			case p.InlineData != nil:
				total += 1600
			default:
				total += estimateTokens(p.Text)
			}
		}
	}
	for _, group := range in.Tools {
		for _, decl := range group.FunctionDeclarations {
			total += estimateTokens(decl.Name) + estimateTokens(decl.Description) + estimateTokens(string(decl.Parameters)) + 8
		}
	}
	return total
}

func gemErrorBody(status int, reason, message string) map[string]any {
	return map[string]any{
		"error": map[string]any{
			"code": status, "message": message, "status": reason,
		},
	}
}

func writeGeminiError(w http.ResponseWriter, status int, reason, message string) {
	writeJSON(w, status, gemErrorBody(status, reason, message))
}
