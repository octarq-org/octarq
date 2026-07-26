package api

import (
	"context"
	"log"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/models"
)

type OverviewInput struct {
	Ctx        huma.Context `hidden:"true"`
	IncludeBot bool         `query:"includeBot"`
}

func (i *OverviewInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type OverviewOutput struct {
	Body map[string]any
}

// overview returns aggregate dashboard statistics for the home page.
// Query param: includeBot=true — when present, bot clicks are counted alongside
// human clicks so the caller can compare bot vs human traffic.
func (h *Handler) overview(ctx context.Context, input *OverviewInput) (*OverviewOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	go h.auth.TouchSession(r)
	org := h.orgID(r)
	includeBot := input.IncludeBot

	count := func(model any, conds ...any) int64 {
		var n int64
		q := h.db.Model(model).Where("owner_id = ?", org)
		if len(conds) > 0 {
			q = q.Where(conds[0], conds[1:]...)
		}
		q.Count(&n)
		return n
	}

	outMap := map[string]any{
		"tokens":     count(&models.Token{}),
		"includeBot": includeBot,
	}

	// Plugins flat-merge their statistics into outMap.
	// NOTE: If two plugins define duplicate keys, the later registration will overwrite the earlier key.
	// We keep this protocol flat for compatibility, but log a warning when collisions happen.
	for _, p := range h.plugins {
		if !h.pluginActive(org, p) {
			continue
		}
		svcName := p.Name() + ".overview"
		if v, ok := h.LookupService(svcName); ok {
			if fn, ok := v.(func(orgID uint, includeBot bool) map[string]any); ok {
				for k, val := range fn(org, includeBot) {
					if _, exists := outMap[k]; exists {
						log.Printf("[overview] warning: plugin %s overwrites existing overview key %q", p.Name(), k)
					}
					outMap[k] = val
				}
			}
		}
	}

	return &OverviewOutput{Body: outMap}, nil
}
