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

type HealthOutputBody struct {
	Status   string `json:"status"`
	Database string `json:"database"`
	Error    string `json:"error,omitempty"`
	Time     string `json:"time"`
}

type HealthOutput struct {
	Body HealthOutputBody
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

type StatusStatusInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *StatusStatusInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type StatusSubsystemItem struct {
	Name   string `json:"name"`
	Status string `json:"status"` // ok | degraded | down | na
	Detail string `json:"detail,omitempty"`
}

type StatusStatusOutputBody struct {
	Overall    string                `json:"overall"`
	Subsystems []StatusSubsystemItem `json:"subsystems"`
	Time       string                `json:"time"`
}

type StatusStatusOutput struct {
	Body StatusStatusOutputBody
}

// subsystemStatus returns public health status of core subsystems (database, mail, queue, overall).
// It is rate-limited and returns only status enums without exposing internal details.
func (h *Handler) subsystemStatus(ctx context.Context, input *StatusStatusInput) (*StatusStatusOutput, error) {
	if input.Ctx != nil {
		r, _ := humago.Unwrap(input.Ctx)
		if r != nil && h.statusLimiter != nil {
			ip := reporterIP(r)
			if !h.statusLimiter.allow(ip) {
				return nil, huma.Error429TooManyRequests("too many status requests")
			}
			h.statusLimiter.recordFailure(ip)
		}
	}

	dbStatus := "ok"
	if h.db == nil {
		dbStatus = "down"
	} else {
		sqlDB, err := h.db.DB()
		if err != nil || sqlDB.Ping() != nil {
			dbStatus = "down"
		}
	}

	mailStatus := "na"
	if h.db != nil && h.db.Migrator().HasTable("smtp_senders") {
		var count int64
		if err := h.db.Table("smtp_senders").Count(&count).Error; err == nil && count > 0 {
			mailStatus = "ok"
		}
	}

	queueStatus := "na"
	if h.queue != nil {
		queueStatus = "ok"
	}

	overall := "ok"
	if dbStatus == "down" {
		overall = "down"
	} else if dbStatus == "degraded" || mailStatus == "degraded" || queueStatus == "degraded" || mailStatus == "down" || queueStatus == "down" {
		overall = "degraded"
	}

	out := &StatusStatusOutput{}
	out.Body.Overall = overall
	out.Body.Subsystems = []StatusSubsystemItem{
		{Name: "database", Status: dbStatus},
		{Name: "mail", Status: mailStatus},
		{Name: "queue", Status: queueStatus},
	}
	out.Body.Time = time.Now().Format(time.RFC3339)
	return out, nil
}
