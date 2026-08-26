package llmprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tmc/langchaingo/llms"
)

// Ensure Claude and langchainProvider implement StreamProvider interface.
var (
	_ StreamProvider = (*Claude)(nil)
	_ StreamProvider = (*langchainProvider)(nil)
)

type captureHandler struct {
	texts    []string
	thinking []string
	tools    []ToolCallChunk
}

func (c *captureHandler) OnText(delta string) {
	c.texts = append(c.texts, delta)
}

func (c *captureHandler) OnThinking(delta string) {
	c.thinking = append(c.thinking, delta)
}

func (c *captureHandler) OnToolCall(chunk ToolCallChunk) {
	c.tools = append(c.tools, chunk)
}

func writeSSEEvent(w http.ResponseWriter, flusher http.Flusher, event, data string) {
	if event != "" {
		fmt.Fprintf(w, "event: %s\n", event)
	}
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}

func TestStreamOrderGuard(t *testing.T) {
	t.Run("ThinkingThenTextThenTool", func(t *testing.T) {
		var g streamOrderGuard
		if err := g.OnThinking("thinking step 1"); err != nil {
			t.Fatalf("unexpected error on thinking 1: %v", err)
		}
		if err := g.OnThinking("thinking step 2"); err != nil {
			t.Fatalf("unexpected error on thinking 2: %v", err)
		}
		g.OnText("text 1")
		g.OnText("text 2")
		g.OnToolCall(ToolCallChunk{Index: 0, ID: "t1", Name: "call1", ArgsJSON: "{}"})
	})

	t.Run("EmptyThinkingIgnored", func(t *testing.T) {
		var g streamOrderGuard
		g.OnText("text")
		// Empty thinking delta does not cause an error even after text.
		if err := g.OnThinking(""); err != nil {
			t.Fatalf("empty thinking delta should not error, got %v", err)
		}
	})

	t.Run("ThinkingAfterTextErrors", func(t *testing.T) {
		var g streamOrderGuard
		g.OnText("some text")
		err := g.OnThinking("late thinking")
		if err == nil {
			t.Fatal("expected ErrStreamOrder when thinking arrives after text")
			return
		}
		if !errors.Is(err, ErrStreamOrder) {
			t.Fatalf("expected errors.Is(err, ErrStreamOrder), got %v", err)
		}
	})

	t.Run("ThinkingAfterToolCallErrors", func(t *testing.T) {
		var g streamOrderGuard
		g.OnToolCall(ToolCallChunk{Index: 0, ID: "call_1", Name: "fn", ArgsJSON: "{}"})
		err := g.OnThinking("late thinking after tool")
		if err == nil {
			t.Fatal("expected ErrStreamOrder when thinking arrives after tool call")
			return
		}
		if !errors.Is(err, ErrStreamOrder) {
			t.Fatalf("expected errors.Is(err, ErrStreamOrder), got %v", err)
		}
	})
}

func TestClaudeStreamText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "flusher not supported", http.StatusInternalServerError)
			return
		}
		writeSSEEvent(w, flusher, "message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-opus-4-8","content":[],"usage":{"input_tokens":12,"output_tokens":0}}}`)
		writeSSEEvent(w, flusher, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
		writeSSEEvent(w, flusher, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello "}}`)
		writeSSEEvent(w, flusher, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"world!"}}`)
		writeSSEEvent(w, flusher, "content_block_stop", `{"type":"content_block_stop","index":0}`)
		writeSSEEvent(w, flusher, "message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":6}}`)
		writeSSEEvent(w, flusher, "message_stop", `{"type":"message_stop"}`)
	}))
	defer srv.Close()

	p, err := New(Options{APIKey: "fake-key", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	sp, ok := p.(StreamProvider)
	if !ok {
		t.Fatalf("expected Provider to implement StreamProvider")
	}

	h := &captureHandler{}
	resp, err := sp.StreamComplete(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Content: "Say hi"}},
	}, h)
	if err != nil {
		t.Fatalf("StreamComplete: %v", err)
	}

	if resp.Text != "Hello world!" {
		t.Errorf("Text = %q, want %q", resp.Text, "Hello world!")
	}
	if resp.Model != ModelClaudeOpus {
		t.Errorf("Model = %q, want %q", resp.Model, ModelClaudeOpus)
	}
	if resp.InputTokens != 12 || resp.OutputTokens != 6 {
		t.Errorf("Usage = %d/%d, want 12/6", resp.InputTokens, resp.OutputTokens)
	}
	if resp.ReasoningTokens != 0 {
		t.Errorf("ReasoningTokens = %d, want 0", resp.ReasoningTokens)
	}
	if resp.StopReason != "end_turn" {
		t.Errorf("StopReason = %q, want %q", resp.StopReason, "end_turn")
	}

	joined := strings.Join(h.texts, "")
	if joined != "Hello world!" {
		t.Errorf("Handler text joined = %q, want %q", joined, "Hello world!")
	}
	if len(h.thinking) != 0 {
		t.Errorf("Expected 0 thinking chunks, got %d", len(h.thinking))
	}
	if len(h.tools) != 0 {
		t.Errorf("Expected 0 tool chunks, got %d", len(h.tools))
	}
}

func TestClaudeStreamThinkingThenText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		writeSSEEvent(w, flusher, "message_start", `{"type":"message_start","message":{"id":"msg_2","type":"message","role":"assistant","model":"claude-opus-4-8","content":[],"usage":{"input_tokens":20,"output_tokens":0}}}`)
		writeSSEEvent(w, flusher, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`)
		writeSSEEvent(w, flusher, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"Let me ponder this."}}`)
		writeSSEEvent(w, flusher, "content_block_stop", `{"type":"content_block_stop","index":0}`)
		writeSSEEvent(w, flusher, "content_block_start", `{"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`)
		writeSSEEvent(w, flusher, "content_block_delta", `{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"The answer is 42."}}`)
		writeSSEEvent(w, flusher, "content_block_stop", `{"type":"content_block_stop","index":1}`)
		writeSSEEvent(w, flusher, "message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":15}}`)
		writeSSEEvent(w, flusher, "message_stop", `{"type":"message_stop"}`)
	}))
	defer srv.Close()

	p, err := New(Options{APIKey: "fake-key", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	sp := p.(StreamProvider)

	h := &captureHandler{}
	resp, err := sp.StreamComplete(context.Background(), Request{
		Thinking: true,
		Messages: []Message{{Role: RoleUser, Content: "Ultimate question"}},
	}, h)
	if err != nil {
		t.Fatalf("StreamComplete: %v", err)
	}

	if resp.Text != "The answer is 42." {
		t.Errorf("Text = %q, want %q", resp.Text, "The answer is 42.")
	}
	if resp.ReasoningTokens != 0 {
		t.Errorf("ReasoningTokens = %d, want 0", resp.ReasoningTokens)
	}
	if len(h.thinking) != 1 || h.thinking[0] != "Let me ponder this." {
		t.Errorf("h.thinking = %v, want ['Let me ponder this.']", h.thinking)
	}
	if len(h.texts) != 1 || h.texts[0] != "The answer is 42." {
		t.Errorf("h.texts = %v, want ['The answer is 42.']", h.texts)
	}
}

func TestClaudeStreamToolCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		writeSSEEvent(w, flusher, "message_start", `{"type":"message_start","message":{"id":"msg_3","type":"message","role":"assistant","model":"claude-opus-4-8","content":[],"usage":{"input_tokens":15,"output_tokens":0}}}`)
		writeSSEEvent(w, flusher, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_999","name":"search_db","input":{}}}`)
		writeSSEEvent(w, flusher, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"query\": \""}}`)
		writeSSEEvent(w, flusher, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"golang\"}"}}`)
		writeSSEEvent(w, flusher, "content_block_stop", `{"type":"content_block_stop","index":0}`)
		writeSSEEvent(w, flusher, "message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":22}}`)
		writeSSEEvent(w, flusher, "message_stop", `{"type":"message_stop"}`)
	}))
	defer srv.Close()

	p, err := New(Options{APIKey: "fake-key", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	sp := p.(StreamProvider)

	h := &captureHandler{}
	resp, err := sp.StreamComplete(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Content: "find golang"}},
		Tools: []ToolDef{
			{
				Name:        "search_db",
				Description: "Search the database",
				Schema:      json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`),
			},
		},
	}, h)
	if err != nil {
		t.Fatalf("StreamComplete: %v", err)
	}

	if resp.StopReason != "tool_use" {
		t.Errorf("StopReason = %q, want 'tool_use'", resp.StopReason)
	}
	if len(h.tools) != 2 {
		t.Fatalf("Expected 2 tool chunks, got %d: %+v", len(h.tools), h.tools)
	}

	for i, chunk := range h.tools {
		if chunk.Index != 0 {
			t.Errorf("chunk[%d].Index = %d, want 0", i, chunk.Index)
		}
		if chunk.ID != "toolu_999" {
			t.Errorf("chunk[%d].ID = %q, want toolu_999", i, chunk.ID)
		}
		if chunk.Name != "search_db" {
			t.Errorf("chunk[%d].Name = %q, want search_db", i, chunk.Name)
		}
	}

	fullJSON := h.tools[0].ArgsJSON + h.tools[1].ArgsJSON
	var parsed map[string]string
	if err := json.Unmarshal([]byte(fullJSON), &parsed); err != nil {
		t.Fatalf("accumulated tool JSON is invalid: %v, raw: %q", err, fullJSON)
	}
	if parsed["query"] != "golang" {
		t.Errorf("parsed query = %q, want 'golang'", parsed["query"])
	}
}

func TestClaudeStreamOrderViolation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		writeSSEEvent(w, flusher, "message_start", `{"type":"message_start","message":{"id":"msg_4","type":"message","role":"assistant","model":"claude-opus-4-8","content":[],"usage":{"input_tokens":10,"output_tokens":0}}}`)
		writeSSEEvent(w, flusher, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
		writeSSEEvent(w, flusher, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Premature text"}}`)
		writeSSEEvent(w, flusher, "content_block_stop", `{"type":"content_block_stop","index":0}`)
		// Out of order: thinking block arrives after text block.
		writeSSEEvent(w, flusher, "content_block_start", `{"type":"content_block_start","index":1,"content_block":{"type":"thinking","thinking":""}}`)
		writeSSEEvent(w, flusher, "content_block_delta", `{"type":"content_block_delta","index":1,"delta":{"type":"thinking_delta","thinking":"Late thinking"}}`)
		writeSSEEvent(w, flusher, "content_block_stop", `{"type":"content_block_stop","index":1}`)
		writeSSEEvent(w, flusher, "message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":10}}`)
		writeSSEEvent(w, flusher, "message_stop", `{"type":"message_stop"}`)
	}))
	defer srv.Close()

	p, err := New(Options{APIKey: "fake-key", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	sp := p.(StreamProvider)

	h := &captureHandler{}
	_, err = sp.StreamComplete(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Content: "trigger disorder"}},
	}, h)
	if err == nil {
		t.Fatal("expected error on out-of-order stream, got nil")
		return
	}
	if !errors.Is(err, ErrStreamOrder) {
		t.Fatalf("expected errors.Is(err, ErrStreamOrder), got %v", err)
	}
}

func TestClaudeToolsMappingAndThinkingBudget(t *testing.T) {
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"msg_5","type":"message","role":"assistant","model":"claude-opus-4-8",
			"content":[{"type":"text","text":"ok"}],
			"usage":{"input_tokens":10,"output_tokens":2}
		}`))
	}))
	defer srv.Close()

	p, err := New(Options{APIKey: "fake-key", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	t.Run("ToolsMapping", func(t *testing.T) {
		capturedBody = nil
		_, err := p.Complete(context.Background(), Request{
			Messages: []Message{{Role: RoleUser, Content: "test tools"}},
			Tools: []ToolDef{
				{
					Name:        "get_weather",
					Description: "Fetch weather info",
					Schema:      json.RawMessage(`{"type":"object","properties":{"location":{"type":"string"}},"required":["location"]}`),
				},
			},
		})
		if err != nil {
			t.Fatalf("Complete: %v", err)
		}

		toolsRaw, ok := capturedBody["tools"].([]any)
		if !ok || len(toolsRaw) != 1 {
			t.Fatalf("tools in body = %#v, want 1 tool", capturedBody["tools"])
		}
		toolMap, ok := toolsRaw[0].(map[string]any)
		if !ok {
			t.Fatalf("tool entry = %#v", toolsRaw[0])
		}
		if toolMap["name"] != "get_weather" {
			t.Errorf("tool name = %v, want get_weather", toolMap["name"])
		}
		if toolMap["description"] != "Fetch weather info" {
			t.Errorf("tool description = %v, want 'Fetch weather info'", toolMap["description"])
		}
		schema, ok := toolMap["input_schema"].(map[string]any)
		if !ok {
			t.Fatalf("input_schema = %#v", toolMap["input_schema"])
		}
		if schema["type"] != "object" {
			t.Errorf("schema.type = %v, want object", schema["type"])
		}
		props, ok := schema["properties"].(map[string]any)
		if !ok || props["location"] == nil {
			t.Errorf("schema properties = %#v, want location property", schema["properties"])
		}
	})

	t.Run("ThinkingBudgetLowMaxTokensRaised", func(t *testing.T) {
		capturedBody = nil
		_, err := p.Complete(context.Background(), Request{
			Thinking:  true,
			MaxTokens: 500,
			Messages:  []Message{{Role: RoleUser, Content: "think low"}},
		})
		if err != nil {
			t.Fatalf("Complete: %v", err)
		}

		// budget = max(1024, 250) = 1024; budget >= 500 -> maxTokens = 1024 + 1024 = 2048
		thinking, ok := capturedBody["thinking"].(map[string]any)
		if !ok {
			t.Fatalf("thinking in body = %#v", capturedBody["thinking"])
		}
		if thinking["type"] != "enabled" {
			t.Errorf("thinking.type = %v, want enabled", thinking["type"])
		}
		if budget, ok := thinking["budget_tokens"].(float64); !ok || int(budget) != 1024 {
			t.Errorf("thinking.budget_tokens = %v, want 1024", thinking["budget_tokens"])
		}
		if maxTok, ok := capturedBody["max_tokens"].(float64); !ok || int(maxTok) != 2048 {
			t.Errorf("max_tokens = %v, want 2048", capturedBody["max_tokens"])
		}
	})

	t.Run("ThinkingBudgetHighMaxTokensPreserved", func(t *testing.T) {
		capturedBody = nil
		_, err := p.Complete(context.Background(), Request{
			Thinking:  true,
			MaxTokens: 4000,
			Messages:  []Message{{Role: RoleUser, Content: "think high"}},
		})
		if err != nil {
			t.Fatalf("Complete: %v", err)
		}

		// budget = max(1024, 2000) = 2000; budget < 4000 -> maxTokens = 4000
		thinking, ok := capturedBody["thinking"].(map[string]any)
		if !ok {
			t.Fatalf("thinking in body = %#v", capturedBody["thinking"])
		}
		if budget, ok := thinking["budget_tokens"].(float64); !ok || int(budget) != 2000 {
			t.Errorf("thinking.budget_tokens = %v, want 2000", thinking["budget_tokens"])
		}
		if maxTok, ok := capturedBody["max_tokens"].(float64); !ok || int(maxTok) != 4000 {
			t.Errorf("max_tokens = %v, want 4000", capturedBody["max_tokens"])
		}
	})
}

// mockStreamingModel simulates streaming callback execution in langchaingo.
type mockStreamingModel struct {
	content string
	reason  string
	order   string // "normal" or "disorder"
	resp    *llms.ContentResponse
	err     error
}

func (m *mockStreamingModel) GenerateContent(ctx context.Context, messages []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, error) {
	opts := llms.CallOptions{}
	for _, opt := range options {
		opt(&opts)
	}

	if opts.StreamingReasoningFunc != nil {
		if m.order == "disorder" {
			// Send text first then reasoning to trigger order violation
			if err := opts.StreamingReasoningFunc(ctx, nil, []byte("text first")); err != nil {
				return nil, err
			}
			if err := opts.StreamingReasoningFunc(ctx, []byte("late reasoning"), nil); err != nil {
				return nil, err
			}
		} else {
			if m.reason != "" {
				if err := opts.StreamingReasoningFunc(ctx, []byte(m.reason), nil); err != nil {
					return nil, err
				}
			}
			if m.content != "" {
				if err := opts.StreamingReasoningFunc(ctx, nil, []byte(m.content)); err != nil {
					return nil, err
				}
			}
		}
	} else if opts.StreamingFunc != nil {
		if m.content != "" {
			if err := opts.StreamingFunc(ctx, []byte(m.content)); err != nil {
				return nil, err
			}
		}
	}

	return m.resp, m.err
}

func (m *mockStreamingModel) Call(context.Context, string, ...llms.CallOption) (string, error) {
	return "", nil
}

func TestLangchainStreamComplete(t *testing.T) {
	t.Run("RejectsTools", func(t *testing.T) {
		p := newLangchain("openai", &mockStreamingModel{}, "", "")
		var sp StreamProvider = p
		_, err := sp.StreamComplete(context.Background(), Request{
			Messages: []Message{{Role: RoleUser, Content: "hi"}},
			Tools:    []ToolDef{{Name: "my_tool"}},
		}, &captureHandler{})
		if err == nil {
			t.Fatal("expected error when tools passed to langchain StreamComplete")
			return
		}
		if !strings.Contains(err.Error(), "tool streaming not supported by langchaingo adapter") {
			t.Errorf("error = %q, want 'tool streaming not supported by langchaingo adapter'", err.Error())
		}
	})

	t.Run("TextStreaming", func(t *testing.T) {
		stub := &mockStreamingModel{
			content: "streamed chunk",
			resp: &llms.ContentResponse{
				Choices: []*llms.ContentChoice{{
					Content:        "streamed chunk",
					StopReason:     "stop",
					GenerationInfo: map[string]any{"PromptTokens": 5, "CompletionTokens": 3},
				}},
			},
		}
		p := newLangchain("openai", stub, "", "")
		var sp StreamProvider = p

		h := &captureHandler{}
		resp, err := sp.StreamComplete(context.Background(), Request{
			Messages: []Message{{Role: RoleUser, Content: "test"}},
		}, h)
		if err != nil {
			t.Fatalf("StreamComplete: %v", err)
		}
		if resp.Text != "streamed chunk" {
			t.Errorf("resp.Text = %q", resp.Text)
		}
		if resp.InputTokens != 5 || resp.OutputTokens != 3 {
			t.Errorf("tokens = %d/%d, want 5/3", resp.InputTokens, resp.OutputTokens)
		}
		if len(h.texts) != 1 || h.texts[0] != "streamed chunk" {
			t.Errorf("captured texts = %v", h.texts)
		}
	})

	t.Run("ThinkingAndTextStreaming", func(t *testing.T) {
		stub := &mockStreamingModel{
			reason:  "chain of thought",
			content: "final text",
			resp: &llms.ContentResponse{
				Choices: []*llms.ContentChoice{{
					Content:    "final text",
					StopReason: "stop",
					GenerationInfo: map[string]any{
						"PromptTokens":    10,
						"OutputTokens":    8,
						"ReasoningTokens": 4,
					},
				}},
			},
		}
		p := newLangchain("openai", stub, "", "")
		var sp StreamProvider = p

		h := &captureHandler{}
		resp, err := sp.StreamComplete(context.Background(), Request{
			Thinking: true,
			Messages: []Message{{Role: RoleUser, Content: "reasoning test"}},
		}, h)
		if err != nil {
			t.Fatalf("StreamComplete: %v", err)
		}
		if resp.Text != "final text" {
			t.Errorf("resp.Text = %q", resp.Text)
		}
		if resp.ReasoningTokens != 4 {
			t.Errorf("resp.ReasoningTokens = %d, want 4", resp.ReasoningTokens)
		}
		if len(h.thinking) != 1 || h.thinking[0] != "chain of thought" {
			t.Errorf("captured thinking = %v", h.thinking)
		}
		if len(h.texts) != 1 || h.texts[0] != "final text" {
			t.Errorf("captured texts = %v", h.texts)
		}
	})

	t.Run("OrderViolationErrors", func(t *testing.T) {
		stub := &mockStreamingModel{
			order: "disorder",
			resp: &llms.ContentResponse{
				Choices: []*llms.ContentChoice{{Content: "text first"}},
			},
		}
		p := newLangchain("openai", stub, "", "")
		var sp StreamProvider = p

		h := &captureHandler{}
		_, err := sp.StreamComplete(context.Background(), Request{
			Thinking: true,
			Messages: []Message{{Role: RoleUser, Content: "disorder test"}},
		}, h)
		if err == nil {
			t.Fatal("expected ErrStreamOrder when langchain streaming delivers out of order")
			return
		}
		if !errors.Is(err, ErrStreamOrder) {
			t.Fatalf("expected errors.Is(err, ErrStreamOrder), got %v", err)
		}
	})

	t.Run("EmptyMessagesError", func(t *testing.T) {
		p := newLangchain("openai", &mockStreamingModel{}, "", "")
		var sp StreamProvider = p
		if _, err := sp.StreamComplete(context.Background(), Request{}, &captureHandler{}); err == nil {
			t.Error("expected error for empty messages")
		}

		c, _ := New(Options{APIKey: "k"})
		csp, ok := c.(StreamProvider)
		if !ok {
			t.Fatal("Claude must implement StreamProvider")
		}
		if _, err := csp.StreamComplete(context.Background(), Request{}, &captureHandler{}); err == nil {
			t.Error("expected error for empty messages in Claude")
		}
	})
}
