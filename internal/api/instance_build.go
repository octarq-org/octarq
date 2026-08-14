package api

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
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
// gate like every other /api/ route.
func (h *Handler) instanceBuild(ctx context.Context, input *InstanceBuildInput) (*InstanceBuildOutput, error) {
	return &InstanceBuildOutput{Body: buildinfo.Get()}, nil
}
