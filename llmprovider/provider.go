// Package llmprovider abstracts large-language-model backends behind one small
// interface, the same way internal/dnsprovider abstracts DNS APIs. It is a
// public, importable package (NOT internal) on purpose: both the OSS `octarq mcp`
// server and the commercial octarq-pro AI plugins build on this single seam, so it
// must live outside internal/ where external modules can import it
// (github.com/octarq-org/octarq/llmprovider).
//
// Backends are not hand-written per vendor. The broad set — OpenAI (and any
// OpenAI-compatible endpoint), Google Gemini, Mistral, Cohere, Ollama (local) —
// is provided by the open-source langchaingo framework through one adapter
// (langchain.go). Claude is the exception, on the official anthropic-sdk-go
// (claude.go), because langchaingo's Anthropic client sends a `temperature`
// parameter the Opus 4.7+ family rejects. Adding another langchaingo-supported
// vendor is a few lines, not a new file.
//
// Design goals, inherited from the dnsprovider pattern:
//
//   - One Provider interface, a registry of named factories, and a config-driven
//     New()/FromEnv() — switch vendor by name, no caller changes.
//   - A "default" model for reasoning and a "cheap" model for high-volume
//     classification/summary, so callers pick the right cost tier per call
//     (e.g. Opus for a daily briefing, Haiku for per-email tagging).
package llmprovider

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
)

// Role constants for Message.Role. Only user/assistant turns are modelled; the
// system prompt is carried separately on Request.System (matching the Anthropic
// Messages API, where system is a top-level field rather than a message).
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// Recommended Claude model IDs, surfaced as constants so callers don't hardcode
// strings. The two-tier split is the cost lever the AI roadmap calls for:
//
//   - Opus 4.8 for complex reasoning (daily briefing synthesis, root-cause).
//   - Haiku 4.5 for cheap, high-frequency work (per-email summary/classification,
//     OTP extraction).
//
// IDs are the exact, complete strings — never append a date suffix.
const (
	ModelClaudeOpus  = "claude-opus-4-8"  // complex reasoning
	ModelClaudeHaiku = "claude-haiku-4-5" // cheap classification / summary
)

// Message is one conversational turn. Content is plain text; multimodal blocks
// (images, PDFs for invoice OCR) are a later extension and deliberately omitted
// from the v1 surface.
type Message struct {
	Role    string // RoleUser | RoleAssistant
	Content string
}

// Request is a single completion request. The zero value is invalid (Messages
// must be non-empty); Model and MaxTokens fall back to provider defaults when
// left zero.
type Request struct {
	// Model overrides the provider's default model for this call. Empty means
	// "use the provider's DefaultModel()". Pass a CheapModel() value here for
	// high-volume tasks.
	Model string
	// System is the system prompt (Anthropic top-level system field). Optional.
	System string
	// Messages is the conversation, oldest first. Must be non-empty and should
	// start with a user turn.
	Messages []Message
	// MaxTokens caps the response length. 0 falls back to a provider default.
	MaxTokens int
	// JSON asks the backend to return strict JSON when it supports doing so. It
	// is a hint: backends that can't enforce it should still steer via the
	// prompt, and callers must tolerate non-JSON and parse defensively.
	JSON bool
}

// Response is a completed generation plus token accounting for cost tracking.
type Response struct {
	Text         string
	Model        string
	InputTokens  int
	OutputTokens int
	StopReason   string
}

// Provider is the LLM backend contract. Implementations must be safe for
// concurrent use — the AI plugin calls Complete from per-email goroutines and a
// daily scheduler at once.
type Provider interface {
	// Name is the backend identifier ("claude", "ollama").
	Name() string
	// DefaultModel is the model used when Request.Model is empty — the
	// reasoning-tier model.
	DefaultModel() string
	// CheapModel is the low-cost model for high-volume classification/summary.
	// Callers pass it as Request.Model when they want the cheap tier.
	CheapModel() string
	// Complete runs one request to completion (no streaming). It returns a
	// non-nil error on transport failures, auth errors, or API errors.
	Complete(ctx context.Context, req Request) (Response, error)
}

// Options configures New(). It mirrors the env surface so FromEnv() is a thin
// wrapper. Unset fields fall back to backend-specific defaults.
type Options struct {
	// Provider selects the backend by registered name. Empty defaults to "claude".
	Provider string
	// APIKey authenticates to a cloud backend (Anthropic x-api-key). Ignored by
	// local backends.
	APIKey string
	// BaseURL overrides the backend endpoint (for proxies, gateways, or a local
	// Ollama address). Empty uses the backend default.
	BaseURL string
	// Model overrides the backend's built-in default (reasoning) model.
	Model string
	// CheapModel overrides the backend's built-in cheap model.
	CheapModel string
	// HTTPClient lets callers inject a client (timeouts, instrumentation, test
	// servers). Nil uses a backend default.
	HTTPClient *http.Client
}

// Factory builds a Provider from Options.
type Factory func(Options) (Provider, error)

var registry = map[string]Factory{}

// Register makes a backend available by name. Called from each backend's init().
func Register(name string, f Factory) { registry[name] = f }

// Names returns the registered backend names (for diagnostics / `octarq doctor`).
func Names() []string {
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	return out
}

// New constructs a Provider from Options. An empty Options.Provider defaults to
// "claude".
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

// FromEnv builds a Provider from environment variables, so the OSS `octarq mcp`
// command and the octarq-pro AI plugin share one configuration path:
//
//	OCTARQ_LLM_PROVIDER     backend name (default "claude")
//	OCTARQ_LLM_API_KEY      cloud API key; falls back to ANTHROPIC_API_KEY
//	OCTARQ_LLM_BASE_URL     endpoint override (proxy / local Ollama)
//	OCTARQ_LLM_MODEL        default reasoning model override
//	OCTARQ_LLM_CHEAP_MODEL  cheap classification/summary model override
func FromEnv() (Provider, error) {
	return New(OptionsFromEnv())
}

// OptionsFromEnv reads the OCTARQ_LLM_* environment into an Options. Exposed
// separately so callers can inspect or tweak (e.g. inject an HTTPClient) before
// calling New.
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

// orDefault returns v if non-empty (after trimming), else def.
func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}
