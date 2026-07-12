package api

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/models"
)

type ListAuditLogsInput struct {
	Ctx        huma.Context `hidden:"true"`
	Action     string       `query:"action"`
	TargetType string       `query:"targetType"`
	Limit      int          `query:"limit"`
	Offset     int          `query:"offset"`
}

func (i *ListAuditLogsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ListAuditLogsOutput struct {
	Body []models.AuditLog
}

// listAuditLogs returns audit log entries for the session org, newest first.
// Query params: action=link.create, limit=50, offset=0
func (h *Handler) listAuditLogs(ctx context.Context, input *ListAuditLogsInput) (*ListAuditLogsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	q := h.db.Where("org_id = ?", h.orgID(r)).Order("created_at DESC")
	if input.Action != "" {
		q = q.Where("action = ?", input.Action)
	}
	if input.TargetType != "" {
		q = q.Where("target_type = ?", input.TargetType)
	}
	limit := 50
	if input.Limit > 0 && input.Limit <= 500 {
		limit = input.Limit
	}
	offset := 0
	if input.Offset > 0 {
		offset = input.Offset
	}
	var logs []models.AuditLog
	q.Limit(limit).Offset(offset).Find(&logs)
	return &ListAuditLogsOutput{Body: logs}, nil
}
