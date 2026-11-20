package api

import (
	"context"
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
	Status string `json:"status"`
	Time   string `json:"time"`
}

type HealthOutput struct {
	Body HealthOutputBody
}

func (h *Handler) health(ctx context.Context, input *HealthInput) (*HealthOutput, error) {
	out := &HealthOutput{}
	out.Body.Status = "healthy"
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
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type StatusStatusOutputBody struct {
	Overall    string                `json:"overall"`
	Subsystems []StatusSubsystemItem `json:"subsystems"`
	Time       string                `json:"time"`
	Version    string                `json:"version,omitempty"`
	Revision   string                `json:"revision,omitempty"`
}

type StatusStatusOutput struct {
	Body StatusStatusOutputBody
}

func (h *Handler) subsystemStatus(ctx context.Context, input *StatusStatusInput) (*StatusStatusOutput, error) {
	if input.Ctx != nil {
		r, _ := humago.Unwrap(input.Ctx)
		if r != nil && h.statusLimiter != nil {
			ip := reporterIP(r)
			if !h.statusLimiter.allow(ip) {
				return nil, huma.Error429TooManyRequests("Too many status requests; wait a few minutes before trying again")
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

	build := ReadBuildInfo()
	out := &StatusStatusOutput{}
	out.Body.Overall = overall
	out.Body.Version = build.Version
	out.Body.Revision = build.Revision
	out.Body.Subsystems = []StatusSubsystemItem{
		{Name: "database", Status: dbStatus},
		{Name: "mail", Status: mailStatus},
		{Name: "queue", Status: queueStatus},
	}
	out.Body.Time = time.Now().Format(time.RFC3339)
	return out, nil
}
