package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/server/llmprovider"
	"github.com/octarq-org/octarq/server/plugin"
)

// Single-step, user-triggered AI assists for the open-source build: suggest a
// short-link slug, summarize one email on demand. The backend comes from the
// LLM gateway plugin, which installs its resolver during Mount
// (ctx.SetLLMResolverForOrg) — no configured provider, no AI, and the endpoints
// say so instead of erroring. Unattended AI automation (per-email pipelines,
// briefings) lives in downstream plugins, not here.

const aiTimeout = 60 * time.Second

// errLLMNotConfigured is what the handler returns when no resolver is
// installed. The gateway installs one at Mount; a build without the gateway
// therefore has no AI, which is the documented fail-closed state.
var errLLMNotConfigured = errors.New("AI is not configured: add an LLM provider in Workspace → LLM Gateway")

// SetLLMResolverForOrg swaps the resolver behind the AI assists using an org-aware
// resolver. It backs plugin.Context.SetLLMResolverForOrg; registration happens during
// plugin Mount (startup) while reads happen per request, hence the lock.
func (h *Handler) SetLLMResolverForOrg(f func(orgID uint) (llmprovider.Provider, error)) {
	if f == nil {
		return
	}
	h.llmMu.Lock()
	h.llmResolver = f
	h.llmMu.Unlock()
}

// LLMProviderForOrg returns the provider the AI assists resolve for orgID.
// It is the read side of SetLLMResolverForOrg and backs
// plugin.Context.LLMProviderForOrg, so a plugin reads the same backend the
// assists use instead of the OCTARQ_LLM_* environment. It returns
// errLLMNotConfigured when no resolver is installed or the resolver returns no
// usable provider.
func (h *Handler) LLMProviderForOrg(orgID uint) (llmprovider.Provider, error) {
	return h.llmFor(orgID)
}

func (h *Handler) llmFor(orgID uint) (llmprovider.Provider, error) {
	h.llmMu.RLock()
	f := h.llmResolver
	h.llmMu.RUnlock()
	if f == nil {
		return nil, errLLMNotConfigured
	}
	p, err := f(orgID)
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

type AIStatusOutputBody struct {
	Configured bool   `json:"configured"`
	Provider   string `json:"provider"`
}

type AIStatusOutput struct {
	Body AIStatusOutputBody
}

func (h *Handler) aiStatus(ctx context.Context, input *AIStatusInput) (*AIStatusOutput, error) {
	var orgID uint
	if input != nil && input.Ctx != nil {
		if r, _ := humago.Unwrap(input.Ctx); r != nil {
			orgID = h.orgID(r)
		}
	}
	p, err := h.llmFor(orgID)
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

type AISuggestSlugInputBody struct {
	Target string `json:"target"`
	Title  string `json:"title,omitempty"`
}

type AISuggestSlugInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body AISuggestSlugInputBody
}

func (i *AISuggestSlugInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type AISuggestSlugOutputBody struct {
	Slugs []string `json:"slugs"`
}

type AISuggestSlugOutput struct {
	Body AISuggestSlugOutputBody
}

func (h *Handler) aiSuggestSlug(ctx context.Context, input *AISuggestSlugInput) (*AISuggestSlugOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}
	target := strings.TrimSpace(input.Body.Target)
	if target == "" {
		return nil, huma.Error400BadRequest("Target URL is required; provide a valid destination URL")
	}
	orgID, err := h.requireOrg(r)
	if err != nil {
		return nil, err
	}
	p, err := h.llmFor(orgID)
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
		return nil, huma.NewError(http.StatusBadGateway, "Could not complete AI request: "+err.Error()+"; retry or check AI configuration")
	}
	slugs := parseSlugList(resp.Text)
	if len(slugs) == 0 {
		return nil, huma.NewError(http.StatusBadGateway, "No usable slugs generated; retry with a different title or target")
	}
	out := &AISuggestSlugOutput{}
	out.Body.Slugs = slugs
	return out, nil
}

var slugRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// parseSlugList extracts valid slugs from a model reply: strict JSON first,
// then a defensive line/token sweep (models occasionally add prose or fences).
func parseSlugList(text string) []string {
	text = strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(text), "```json"), "```")
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

type AISummarizeEmailInput struct {
	Ctx huma.Context `hidden:"true"`
	ID  uint         `path:"id"`
}

func (i *AISummarizeEmailInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type AISummarizeEmailOutputBody struct {
	Summary string `json:"summary"`
}

type AISummarizeEmailOutput struct {
	Body AISummarizeEmailOutputBody
}

func (h *Handler) aiSummarizeEmail(ctx context.Context, input *AISummarizeEmailInput) (*AISummarizeEmailOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}
	orgID, err := h.requireOrg(r)
	if err != nil {
		return nil, err
	}
	var fromAddr, subject, body string
	var found bool
	// Absent getter (mail not in this build) leaves found=false → 404;
	// see plugin.EmailGetter.
	if fn, ok := plugin.LookupServiceAs[plugin.EmailGetter](h.LookupService, plugin.ServiceMailEmailGet); ok {
		fromAddr, subject, body, found = fn(orgID, input.ID)
	}
	if !found {
		return nil, huma.Error404NotFound("Email not found; verify the identifier and try again")
	}
	p, err := h.llmFor(orgID)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	// debt: naive 8000 runes, ceiling 8k → upgrade when model context >32k
	const maxBody = 8000
	runes := []rune(body)
	if len(runes) > maxBody {
		body = string(runes[:maxBody])
	}
	content := "From: " + fromAddr + "\nSubject: " + subject + "\n\n" + body

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
		return nil, huma.NewError(http.StatusBadGateway, "Could not complete AI request: "+err.Error()+"; retry or check AI configuration")
	}
	out := &AISummarizeEmailOutput{}
	out.Body.Summary = strings.TrimSpace(resp.Text)
	return out, nil
}
