package api

import (
	"context"
	"errors"

	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/server/llmprovider"
)

const aiTimeout = 60 * time.Second

var errLLMNotConfigured = errors.New("AI is not configured: add an LLM provider in Workspace → LLM Gateway")

func (h *Handler) SetLLMResolverForOrg(f func(orgID uint) (llmprovider.Provider, error)) {
	if f == nil {
		return
	}
	h.llmMu.Lock()
	h.llmResolver = f
	h.llmMu.Unlock()
}

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
