package api

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/server/internal/buildinfo"
)

type InstanceBuildInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *InstanceBuildInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type InstanceBuildOutput struct {
	Body buildinfo.Info
}

func (h *Handler) instanceBuild(ctx context.Context, input *InstanceBuildInput) (*InstanceBuildOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}
	if !h.isInstanceAdmin(r) {
		return nil, huma.Error403Forbidden("Instance administrator role required; contact an instance administrator")
	}
	return &InstanceBuildOutput{Body: buildinfo.Get()}, nil
}
