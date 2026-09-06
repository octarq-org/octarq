package monitor

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/plugin"
)

// HealthInput represents the input for GET /api/monitor/health.
type HealthInput struct {
	Ctx huma.Context `hidden:"true"`
}

// Resolve binds the huma.Context into HealthInput.
func (i *HealthInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

// HealthOutput represents the response payload for GET /api/monitor/health.
type HealthOutput struct {
	Body HealthReport
}

// RegisterRoutes registers the GET /api/monitor/health operation onto huma.API.
func RegisterRoutes(api huma.API, collector *Collector) {
	huma.Register(api, huma.Operation{
		OperationID: "getMonitorHealth",
		Method:      "GET",
		Path:        "/api/monitor/health",
		Summary:     "Get aggregated health status",
		Description: "Returns aggregated health check status and diagnostic metrics across runtime, database, storage, and registered plugin health providers.",
		Tags:        []string{"Monitor", "Public"},
		Metadata:    map[string]any{"public": true},
		Errors:      []int{http.StatusServiceUnavailable},
	}, func(ctx context.Context, input *HealthInput) (*HealthOutput, error) {
		report := collector.LatestReport(ctx)
		if report.Overall == plugin.HealthError && input != nil && input.Ctx != nil {
			_, w := humago.Unwrap(input.Ctx)
			if w != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(w).Encode(report)
				return nil, nil
			}
		}
		return &HealthOutput{Body: report}, nil
	})
}

// Handler returns a standard http.Handler that executes the health monitor endpoint.
func Handler(collector *Collector) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		report := collector.LatestReport(r.Context())
		w.Header().Set("Content-Type", "application/json")
		if report.Overall == plugin.HealthError {
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			w.WriteHeader(http.StatusOK)
		}
		_ = json.NewEncoder(w).Encode(report)
	})
}

// RegisterHTTP registers the GET /api/monitor/health endpoint directly onto an http.ServeMux.
func RegisterHTTP(mux *http.ServeMux, collector *Collector) {
	mux.Handle("GET /api/monitor/health", Handler(collector))
}
