package llmprovider

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tmc/langchaingo/llms"
)

// Ensure langchainProvider implements StreamProvider interface.
var (
	_ StreamProvider = (*langchainProvider)(nil)
)

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
