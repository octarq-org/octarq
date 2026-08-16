package api

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/buildinfo"
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

// instanceBuild reports which build of octarq is running.
//
// It is intentionally NOT public: version + commit fingerprint a self-hosted
// instance for CVE-version scanning (self-hosters upgrade on their own
// months-long schedule), and the repo has already made that call twice — the
// health endpoint deliberately omits the version and uptime sits behind the
// metrics token, default-off. This endpoint goes through the dashboard-session
// gate like every other /api/ route, and — like the rest of the instance
// surface — requires instance-admin: a tenant member reading the build
// fingerprint is the same CVE-aiming signal, just one login further in.
func (h *Handler) instanceBuild(ctx context.Context, input *InstanceBuildInput) (*InstanceBuildOutput, error) {
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
	return &InstanceBuildOutput{Body: buildinfo.Get()}, nil
}
