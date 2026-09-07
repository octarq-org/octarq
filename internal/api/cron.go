package api

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/plugin"
)

type ListCronJobsInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *ListCronJobsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ListCronJobsOutput struct {
	Body []plugin.CronJob
}

func (h *Handler) listCronJobs(ctx context.Context, input *ListCronJobsInput) (*ListCronJobsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	_, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	cronSvc := h.cronService
	if cronSvc == nil {
		if svc, found := h.LookupService(plugin.ServiceCron); found {
			if cs, ok := svc.(plugin.CronService); ok {
				cronSvc = cs
			}
		}
	}

	if cronSvc == nil {
		return &ListCronJobsOutput{Body: []plugin.CronJob{}}, nil
	}

	jobs := cronSvc.List()
	if jobs == nil {
		jobs = []plugin.CronJob{}
	}
	return &ListCronJobsOutput{Body: jobs}, nil
}
