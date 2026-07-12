package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
)

type HealthInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *HealthInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type HealthOutput struct {
	Body struct {
		Status   string `json:"status"`
		Database string `json:"database"`
		Error    string `json:"error,omitempty"`
		Time     string `json:"time"`
	}
}

// health verifies system dependencies (specifically database connectivity)
// and returns the status of the service.
func (h *Handler) health(ctx context.Context, input *HealthInput) (*HealthOutput, error) {
	sqlDB, err := h.db.DB()
	if err != nil {
		out := &HealthOutput{}
		out.Body.Status = "unhealthy"
		out.Body.Database = "down"
		out.Body.Error = err.Error()
		out.Body.Time = time.Now().Format(time.RFC3339)
		_, w := humago.Unwrap(input.Ctx)
		writeJSON(w, http.StatusServiceUnavailable, out.Body)
		return nil, nil
	}

	err = sqlDB.Ping()
	if err != nil {
		out := &HealthOutput{}
		out.Body.Status = "unhealthy"
		out.Body.Database = "down"
		out.Body.Error = err.Error()
		out.Body.Time = time.Now().Format(time.RFC3339)
		_, w := humago.Unwrap(input.Ctx)
		writeJSON(w, http.StatusServiceUnavailable, out.Body)
		return nil, nil
	}

	out := &HealthOutput{}
	out.Body.Status = "healthy"
	out.Body.Database = "up"
	out.Body.Time = time.Now().Format(time.RFC3339)
	return out, nil
}
