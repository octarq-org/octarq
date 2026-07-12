package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/llmprovider"
)

// Single-step, user-triggered AI assists for the open-source build: suggest a
// short-link slug, summarize one email on demand. Configuration is BYO key via
// the OCTARQ_LLM_* environment (llmprovider.FromEnv) — no key, no AI, and the
// endpoints say so instead of erroring. Unattended AI automation (per-email
// pipelines, briefings) lives in the commercial plugins, not here.

const aiTimeout = 60 * time.Second

// errLLMNotConfigured is what the default (env-backed) resolver returns when
// nothing points at a usable backend. Injected resolvers return their own
// configuration hint (e.g. "select a provider in Inbox AI → Configure").
var errLLMNotConfigured = errors.New("AI is not configured: set OCTARQ_LLM_API_KEY (or ANTHROPIC_API_KEY), or point OCTARQ_LLM_PROVIDER/OCTARQ_LLM_BASE_URL at a local model")

// envLLMResolver is the core's default LLM resolver: OCTARQ_LLM_* environment,
// built at most once (env doesn't change at runtime). The Pro ai plugin
// replaces it via SetLLMResolver with its DB-backed provider so the assists
// follow the dashboard configuration.
func envLLMResolver() func() (llmprovider.Provider, error) {
	var (
		once sync.Once
		p    llmprovider.Provider
		err  error
	)
	return func() (llmprovider.Provider, error) {
		o := llmprovider.OptionsFromEnv()
		// Usable = a cloud key, a local Ollama, or a keyless OpenAI-compatible gateway.
		if o.APIKey == "" && o.Provider != "ollama" && o.BaseURL == "" {
			return nil, errLLMNotConfigured
		}
		once.Do(func() { p, err = llmprovider.FromEnv() })
		return p, err
	}
}

// SetLLMResolver swaps the resolver behind the AI assists. It backs
// plugin.Context.SetLLMResolver; registration happens during plugin Mount
// (startup) while reads happen per request, hence the lock.
func (h *Handler) SetLLMResolver(f func() (llmprovider.Provider, error)) {
	if f == nil {
		return
	}
	h.llmMu.Lock()
	h.llmResolver = f
	h.llmMu.Unlock()
}

func (h *Handler) llm() (llmprovider.Provider, error) {
	h.llmMu.RLock()
	f := h.llmResolver
	h.llmMu.RUnlock()
	p, err := f()
	if err == nil && p == nil {
		err = errLLMNotConfigured
	}
	return p, err
}

type AIStatusInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *AIStatusInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type AIStatusOutput struct {
	Body struct {
		Configured bool   `json:"configured"`
		Provider   string `json:"provider"`
	}
}

func (h *Handler) aiStatus(ctx context.Context, input *AIStatusInput) (*AIStatusOutput, error) {
	p, err := h.llm()
	out := &AIStatusOutput{}
	if err != nil {
		out.Body.Configured = false
		out.Body.Provider = ""
		return out, nil
	}
	out.Body.Configured = true
	out.Body.Provider = p.Name()
	return out, nil
}

type AISuggestSlugInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body struct {
		Target string `json:"target"`
		Title  string `json:"title,omitempty"`
	}
}

func (i *AISuggestSlugInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type AISuggestSlugOutput struct {
	Body struct {
		Slugs []string `json:"slugs"`
	}
}

func (h *Handler) aiSuggestSlug(ctx context.Context, input *AISuggestSlugInput) (*AISuggestSlugOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	target := strings.TrimSpace(input.Body.Target)
	if target == "" {
		return nil, huma.Error400BadRequest("target is required")
	}
	p, err := h.llm()
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	prompt := "Suggest short-link slugs for this destination.\nURL: " + target
	if input.Body.Title != "" {
		prompt += "\nPage title: " + input.Body.Title
	}
	ctxCtx, cancel := context.WithTimeout(r.Context(), aiTimeout)
	defer cancel()
	resp, err := p.Complete(ctxCtx, llmprovider.Request{
		Model: p.CheapModel(),
		System: "You generate URL slugs. Reply with ONLY a JSON array of 3 to 5 strings. " +
			"Each slug is lowercase, 3-30 chars, [a-z0-9-] only, memorable, and reflects the page content.",
		Messages:  []llmprovider.Message{{Role: llmprovider.RoleUser, Content: prompt}},
		MaxTokens: 200,
		JSON:      true,
	})
	if err != nil {
		return nil, huma.NewError(http.StatusBadGateway, "AI request failed: "+err.Error())
	}
	slugs := parseSlugList(resp.Text)
	if len(slugs) == 0 {
		return nil, huma.NewError(http.StatusBadGateway, "AI returned no usable slugs")
	}
	out := &AISuggestSlugOutput{}
	out.Body.Slugs = slugs
	return out, nil
}

var slugRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// parseSlugList extracts valid slugs from a model reply: strict JSON first,
// then a defensive line/token sweep (models occasionally add prose or fences).
func parseSlugList(text string) []string {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")

	var arr []string
	if start, end := strings.Index(text, "["), strings.LastIndex(text, "]"); start >= 0 && end > start {
		_ = json.Unmarshal([]byte(text[start:end+1]), &arr)
	}
	if len(arr) == 0 {
		for _, line := range strings.Split(text, "\n") {
			arr = append(arr, strings.Trim(strings.TrimSpace(line), `-*"',`))
		}
	}
	out := make([]string, 0, 5)
	for _, s := range arr {
		s = strings.ToLower(strings.TrimSpace(s))
		if slugRe.MatchString(s) && len(s) >= 3 && len(s) <= 30 {
			out = append(out, s)
		}
		if len(out) == 5 {
			break
		}
	}
	return out
}

var htmlTagRe = regexp.MustCompile(`(?s)<style.*?</style>|<script.*?</script>|<[^>]*>`)

type AISummarizeEmailInput struct {
	Ctx huma.Context `hidden:"true"`
	ID  uint         `path:"id"`
}

func (i *AISummarizeEmailInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type AISummarizeEmailOutput struct {
	Body struct {
		Summary string `json:"summary"`
	}
}

func (h *Handler) aiSummarizeEmail(ctx context.Context, input *AISummarizeEmailInput) (*AISummarizeEmailOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	if !h.emailBelongsToOrg(input.ID, h.orgID(r)) {
		return nil, huma.Error404NotFound("not found")
	}
	var e models.Email
	if h.db.First(&e, input.ID).Error != nil {
		return nil, huma.Error404NotFound("not found")
	}
	p, err := h.llm()
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	body := e.Text
	if strings.TrimSpace(body) == "" {
		body = htmlTagRe.ReplaceAllString(e.HTML, " ")
	}
	const maxBody = 8000
	if len(body) > maxBody {
		body = body[:maxBody]
	}
	content := "From: " + e.FromAddr + "\nSubject: " + e.Subject + "\n\n" + body

	ctxCtx, cancel := context.WithTimeout(r.Context(), aiTimeout)
	defer cancel()
	resp, err := p.Complete(ctxCtx, llmprovider.Request{
		Model: p.CheapModel(),
		System: "Summarize this email in 2-3 sentences, in the same language the email is written in. " +
			"Lead with what it is (bill, verification code, newsletter, personal, ...) and any action or deadline. Plain text only.",
		Messages:  []llmprovider.Message{{Role: llmprovider.RoleUser, Content: content}},
		MaxTokens: 300,
	})
	if err != nil {
		return nil, huma.NewError(http.StatusBadGateway, "AI request failed: "+err.Error())
	}
	out := &AISummarizeEmailOutput{}
	out.Body.Summary = strings.TrimSpace(resp.Text)
	return out, nil
}

