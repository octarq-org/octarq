package api

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/readiness"
	"github.com/octarq-org/octarq/origin"
)

type InstanceReadinessInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *InstanceReadinessInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type InstanceReadinessOutput struct {
	Body []readiness.Check
}

// instanceReadiness returns the instance's capability checks as structured
// data — the same judgment the startup log prints (readiness.Evaluate),
// rendered for the dashboard instead of the log stream. The API vocabulary is
// ok|degraded|blocked; the log-only dev status collapses to ok, with the dev
// caveat still spelled out in detail. Like every other /api/instance* route it
// is instance-admin only: the checks describe the whole deployment, and the
// registration check in particular reveals whether the instance can be signed
// up to — an enumeration signal a tenant member must not read.
func (h *Handler) instanceReadiness(ctx context.Context, input *InstanceReadinessInput) (*InstanceReadinessOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	if !h.isInstanceAdmin(r) {
		return nil, huma.Error403Forbidden("instance admin role required")
	}
	checks := readiness.Evaluate(h.cfg, h.mailReady(), origin.AnyRegistered(h.db) || origin.HasSharedHosts(h.db), h.requireEmailVerification())
	for i := range checks {
		if checks[i].Status == readiness.StatusDev {
			checks[i].Status = readiness.StatusOK
		}
	}
	return &InstanceReadinessOutput{Body: checks}, nil
}
