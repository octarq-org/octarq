package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	"github.com/octarq-org/octarq/server/internal/cron"
	"github.com/octarq-org/octarq/server/plugin"
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

func (h *Handler) cronServiceOrDefault() plugin.CronService {
	if h.cronService != nil {
		return h.cronService
	}
	if svc, found := h.LookupService(plugin.ServiceCron); found {
		if cs, ok := svc.(plugin.CronService); ok {
			return cs
		}
	}
	return nil
}

func (h *Handler) listCronJobs(ctx context.Context, input *ListCronJobsInput) (*ListCronJobsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	_, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}

	cronSvc := h.cronServiceOrDefault()
	if cronSvc == nil {
		return &ListCronJobsOutput{Body: []plugin.CronJob{}}, nil
	}

	jobs := cronSvc.List()
	if jobs == nil {
		jobs = []plugin.CronJob{}
	}
	return &ListCronJobsOutput{Body: jobs}, nil
}

func (h *Handler) requireInstanceAdmin(input huma.Context) (*http.Request, error) {
	r, _ := humago.Unwrap(input)
	if _, ok := h.auth.AuthenticateRequest(r); !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}
	if !h.IsInstanceAdmin(r) {
		return nil, huma.Error403Forbidden("This action needs an instance administrator.")
	}
	return r, nil
}

type RunCronJobInput struct {
	Ctx  huma.Context `hidden:"true"`
	Name string       `path:"name"`
}

func (i *RunCronJobInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type RunCronJobOutput struct {
	Body struct {
		Name    string `json:"name"`
		Started bool   `json:"started"`
	}
}

func (h *Handler) runCronJob(ctx context.Context, input *RunCronJobInput) (*RunCronJobOutput, error) {
	if _, err := h.requireInstanceAdmin(input.Ctx); err != nil {
		return nil, err
	}
	cronSvc, err := h.requireCronService()
	if err != nil {
		return nil, err
	}
	if err := cronSvc.StartJob(input.Name); err != nil {
		return nil, cronActionError(err)
	}
	out := &RunCronJobOutput{}
	out.Body.Name = input.Name
	out.Body.Started = true
	return out, nil
}

type UpdateCronJobInput struct {
	Ctx  huma.Context `hidden:"true"`
	Name string       `path:"name"`
	Body struct {
		Enabled *bool  `json:"enabled,omitempty"`
		Spec    string `json:"spec,omitempty" maxLength:"64"`
	}
}

func (i *UpdateCronJobInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type UpdateCronJobOutput struct {
	Body struct {
		Name        string `json:"name"`
		Spec        string `json:"spec"`
		DefaultSpec string `json:"defaultSpec"`
		Enabled     bool   `json:"enabled"`
	}
}

func (h *Handler) updateCronJob(ctx context.Context, input *UpdateCronJobInput) (*UpdateCronJobOutput, error) {
	if _, err := h.requireInstanceAdmin(input.Ctx); err != nil {
		return nil, err
	}
	cronSvc, err := h.requireCronService()
	if err != nil {
		return nil, err
	}
	if input.Body.Spec != "" {
		if _, parseErr := cron.ParseStandard(input.Body.Spec); parseErr != nil {
			return nil, huma.Error422UnprocessableEntity("Use a valid cron expression with minutes, hours, day, month, and weekday.")
		}
		if err := cronSvc.SetSpec(input.Name, input.Body.Spec); err != nil {
			return nil, cronActionError(err)
		}
	}
	if input.Body.Enabled != nil {
		if err := cronSvc.SetEnabled(input.Name, *input.Body.Enabled); err != nil {
			return nil, cronActionError(err)
		}
	}

	var job *plugin.CronJob
	for _, item := range cronSvc.List() {
		if item.Name == input.Name {
			found := item
			job = &found
			break
		}
	}
	if job == nil {
		return nil, huma.Error404NotFound("This scheduled task does not exist.")
	}
	out := &UpdateCronJobOutput{}
	out.Body.Name = job.Name
	out.Body.Spec = job.Spec
	out.Body.DefaultSpec = job.DefaultSpec
	out.Body.Enabled = job.Enabled
	return out, nil
}

func (h *Handler) requireCronService() (plugin.CronService, error) {
	cronSvc := h.cronServiceOrDefault()
	if cronSvc == nil {
		return nil, huma.Error503ServiceUnavailable("The scheduler is unavailable on this instance.")
	}
	return cronSvc, nil
}

func cronActionError(err error) error {
	switch {
	case errors.Is(err, plugin.ErrJobNotFound):
		return huma.Error404NotFound("This scheduled task does not exist.")
	case errors.Is(err, plugin.ErrJobBusy):
		return huma.Error409Conflict("This task is already running.")
	default:
		return huma.Error503ServiceUnavailable("The scheduler is unavailable on this instance.")
	}
}
