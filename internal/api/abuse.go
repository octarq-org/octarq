package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Jungley8/led/internal/models"
	"github.com/Jungley8/led/internal/notify"
)

var validAbuseReasons = map[string]bool{
	"spam": true, "phishing": true, "malware": true, "other": true,
}

// submitAbuse is a public (no auth) endpoint for reporting a short link.
// POST /abuse  {"slug":"abc","reason":"phishing","description":"..."}
func (h *Handler) submitAbuse(w http.ResponseWriter, r *http.Request) {
	ip := reporterIP(r)
	if !h.abuseLimiter.allow(ip) {
		writeErr(w, http.StatusTooManyRequests, "too many reports from this IP, try again later")
		return
	}
	var d struct {
		Slug        string `json:"slug"`
		Reason      string `json:"reason"`
		Description string `json:"description"`
	}
	if err := readJSON(r, &d); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	d.Slug = strings.TrimSpace(d.Slug)
	d.Reason = strings.TrimSpace(strings.ToLower(d.Reason))
	if d.Slug == "" {
		writeErr(w, http.StatusBadRequest, "slug is required")
		return
	}
	if !validAbuseReasons[d.Reason] {
		writeErr(w, http.StatusBadRequest, "reason must be one of: spam, phishing, malware, other")
		return
	}
	if len(d.Description) > 2000 {
		d.Description = d.Description[:2000]
	}

	// Resolve the slug to get the current target for context.
	var target string
	var link models.Link
	if h.db.Where("slug = ?", d.Slug).First(&link).Error == nil {
		target = link.Target
	}

	rep := models.AbuseReport{
		Slug:        d.Slug,
		Target:      target,
		Reason:      d.Reason,
		Description: d.Description,
		ReporterIP:  ip,
		Status:      "open",
	}
	if err := h.db.Create(&rep).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to save report")
		return
	}

	h.abuseLimiter.recordFailure(ip)

	// Best-effort notification to all enabled channels.
	go h.notifyAbuse(rep)

	writeJSON(w, http.StatusCreated, map[string]any{
		"ok": true,
		"id": rep.ID,
	})
}

func (h *Handler) notifyAbuse(rep models.AbuseReport) {
	msg := fmt.Sprintf(
		"🚨 Abuse report #%d\nSlug: %s\nReason: %s\nTarget: %s\nDescription: %s",
		rep.ID, rep.Slug, rep.Reason, rep.Target, rep.Description,
	)
	var channels []models.NotificationChannel
	h.db.Where("enabled = ?", true).Find(&channels)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for _, ch := range channels {
		_ = notify.Send(ctx, ch.Type, ch.Config, msg)
	}
}

// listAbuseReports returns open reports (admin-only, session required).
// GET /api/abuse?status=open
func (h *Handler) listAbuseReports(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	q := h.db.Order("created_at DESC")
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var reports []models.AbuseReport
	q.Find(&reports)
	writeJSON(w, http.StatusOK, reports)
}

// updateAbuseReport lets admins change the status of a report.
// PUT /api/abuse/{id}  {"status":"reviewed"}
func (h *Handler) updateAbuseReport(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var rep models.AbuseReport
	if h.db.First(&rep, id).Error != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	var d struct {
		Status string `json:"status"`
	}
	if err := readJSON(r, &d); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	d.Status = strings.TrimSpace(d.Status)
	if d.Status != "open" && d.Status != "reviewed" && d.Status != "dismissed" {
		writeErr(w, http.StatusBadRequest, "status must be open, reviewed, or dismissed")
		return
	}
	h.db.Model(&rep).Update("status", d.Status)
	rep.Status = d.Status
	writeJSON(w, http.StatusOK, rep)
}
