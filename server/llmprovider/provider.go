package llmprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

const (
	ModelClaudeOpus  = "claude-opus-4-8"
	ModelClaudeHaiku = "claude-haiku-4-5"
)

type ToolCall struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	ArgsJSON json.RawMessage `json:"argsJson"`
}

type ToolResult struct {
	ToolCallID string `json:"toolCallId"`
	Content    string `json:"content"`
	IsError    bool   `json:"isError,omitempty"`
}

type Message struct {
	Role        string       `json:"role"`
	Content     string       `json:"content,omitempty"`
	ToolCalls   []ToolCall   `json:"toolCalls,omitempty"`
	ToolResults []ToolResult `json:"toolResults,omitempty"`
}

type ToolDef struct {
	Name        string
	Description string
	Schema      json.RawMessage
}

type Request struct {
	Model     string
	System    string
	Messages  []Message
	MaxTokens int
	JSON      bool
	Thinking  bool
	Tools     []ToolDef
}

type Response struct {
	Text                string     `json:"text"`
	Model               string     `json:"model"`
	ToolCalls           []ToolCall `json:"toolCalls,omitempty"`
	InputTokens         int        `json:"inputTokens"`
	OutputTokens        int        `json:"outputTokens"`
	ReasoningTokens     int        `json:"reasoningTokens"`
	CacheReadTokens     int        `json:"cacheReadTokens,omitempty"`
	CacheCreationTokens int        `json:"cacheCreationTokens,omitempty"`
	StopReason          string     `json:"stopReason,omitempty"`
}

type Provider interface {
	Name() string
	DefaultModel() string
	CheapModel() string
	Complete(ctx context.Context, req Request) (Response, error)
}

type Options struct {
	Provider   string
	APIKey     string
	BaseURL    string
	Model      string
	CheapModel string
	HTTPClient *http.Client
}

type Factory func(Options) (Provider, error)

var registry = map[string]Factory{}

func Register(name string, f Factory) { registry[name] = f }

func Names() []string {
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	return out
}

func New(o Options) (Provider, error) {
	name := strings.TrimSpace(o.Provider)
	if name == "" {
		name = "claude"
	}
	f, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("llmprovider: unknown provider %q (have %v)", name, Names())
	}
	return f(o)
}

func FromEnv() (Provider, error) {
	return New(OptionsFromEnv())
}

func OptionsFromEnv() Options {
	apiKey := os.Getenv("OCTARQ_LLM_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	return Options{
		Provider:   os.Getenv("OCTARQ_LLM_PROVIDER"),
		APIKey:     apiKey,
		BaseURL:    os.Getenv("OCTARQ_LLM_BASE_URL"),
		Model:      os.Getenv("OCTARQ_LLM_MODEL"),
		CheapModel: os.Getenv("OCTARQ_LLM_CHEAP_MODEL"),
	}
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}
