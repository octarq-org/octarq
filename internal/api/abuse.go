package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/internal/notify"
)

var validAbuseReasons = map[string]bool{
	"spam": true, "phishing": true, "malware": true, "other": true,
}

type SubmitAbuseInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body struct {
		Slug        string `json:"slug"`
		Reason      string `json:"reason"`
		Description string `json:"description"`
	}
}

func (i *SubmitAbuseInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type SubmitAbuseOutput struct {
	Body struct {
		OK bool `json:"ok"`
		ID uint `json:"id"`
	}
}

// submitAbuse is a public (no auth) endpoint for reporting a short link.
// POST /abuse  {"slug":"abc","reason":"phishing","description":"..."}
func (h *Handler) submitAbuse(ctx context.Context, input *SubmitAbuseInput) (*SubmitAbuseOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	ip := reporterIP(r)
	if !h.abuseLimiter.allow(ip) {
		return nil, huma.Error429TooManyRequests("too many reports from this IP, try again later")
	}
	slug := strings.TrimSpace(input.Body.Slug)
	reason := strings.TrimSpace(strings.ToLower(input.Body.Reason))
	if slug == "" {
		return nil, huma.Error400BadRequest("slug is required")
	}
	if !validAbuseReasons[reason] {
		return nil, huma.Error400BadRequest("reason must be one of: spam, phishing, malware, other")
	}
	description := input.Body.Description
	if len(description) > 2000 {
		description = description[:2000]
	}

	// Resolve the slug to get the current target and owning org for context.
	var target string
	var orgID uint
	var link models.Link
	if h.db.Where("slug = ?", slug).First(&link).Error == nil {
		target = link.Target
		orgID = link.OrgID
	}

	rep := models.AbuseReport{
		OrgID:       orgID,
		Slug:        slug,
		Target:      target,
		Reason:      reason,
		Description: description,
		ReporterIP:  ip,
		Status:      "open",
	}
	if err := h.db.Create(&rep).Error; err != nil {
		return nil, huma.Error500InternalServerError("failed to save report")
	}

	h.abuseLimiter.recordFailure(ip)

	// Best-effort notification to all enabled channels via task queue.
	payload, _ := json.Marshal(rep)
	_ = h.queue.Enqueue(r.Context(), "abuse.notify", payload)

	out := &SubmitAbuseOutput{}
	out.Body.OK = true
	out.Body.ID = rep.ID
	return out, nil
}

func (h *Handler) notifyAbuse(rep models.AbuseReport) {
	msg := fmt.Sprintf(
		"🚨 Abuse report #%d\nSlug: %s\nReason: %s\nTarget: %s\nDescription: %s",
		rep.ID, rep.Slug, rep.Reason, rep.Target, rep.Description,
	)
	orgID := rep.OrgID
	if orgID == 0 {
		orgID = 1 // unresolved slug falls back to the default org (owner_id default:1)
	}
	var channels []models.NotificationChannel
	h.db.Where("owner_id = ? AND enabled = ?", orgID, true).Find(&channels)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for _, ch := range channels {
		_ = notify.Send(ctx, ch.Type, ch.Config, msg)
	}
}

type ListAbuseReportsInput struct {
	Ctx    huma.Context `hidden:"true"`
	Status string       `query:"status"`
}

func (i *ListAbuseReportsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ListAbuseReportsOutput struct {
	Body []models.AbuseReport
}

// listAbuseReports returns open reports (admin-only, session required).
// GET /api/abuse?status=open
func (h *Handler) listAbuseReports(ctx context.Context, input *ListAbuseReportsInput) (*ListAbuseReportsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	q := h.orgDB(r).Order("created_at DESC")
	if input.Status != "" {
		q = q.Where("status = ?", input.Status)
	}
	var reports []models.AbuseReport
	q.Find(&reports)
	return &ListAbuseReportsOutput{Body: reports}, nil
}

type UpdateAbuseReportInput struct {
	Ctx  huma.Context `hidden:"true"`
	ID   uint         `path:"id"`
	Body struct {
		Status string `json:"status"`
	}
}

func (i *UpdateAbuseReportInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type UpdateAbuseReportOutput struct {
	Body models.AbuseReport
}

// updateAbuseReport lets admins change the status of a report.
// PUT /api/abuse/{id}  {"status":"reviewed"}
func (h *Handler) updateAbuseReport(ctx context.Context, input *UpdateAbuseReportInput) (*UpdateAbuseReportOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	var rep models.AbuseReport
	if h.orgDB(r).First(&rep, input.ID).Error != nil {
		return nil, huma.Error404NotFound("not found")
	}
	status := strings.TrimSpace(input.Body.Status)
	if status != "open" && status != "reviewed" && status != "dismissed" {
		return nil, huma.Error400BadRequest("status must be open, reviewed, or dismissed")
	}
	h.db.Model(&rep).Update("status", status)
	rep.Status = status
	return &UpdateAbuseReportOutput{Body: rep}, nil
}
