package api

import (
	"net/http"
	"strconv"

	"github.com/octarq-org/octarq/internal/models"
)

// listAuditLogs returns audit log entries for the session org, newest first.
// Query params: action=link.create, limit=50, offset=0
func (h *Handler) listAuditLogs(w http.ResponseWriter, r *http.Request) {
	q := h.db.Where("org_id = ?", h.orgID(r)).Order("created_at DESC")
	if action := r.URL.Query().Get("action"); action != "" {
		q = q.Where("action = ?", action)
	}
	if tt := r.URL.Query().Get("targetType"); tt != "" {
		q = q.Where("target_type = ?", tt)
	}
	limit := 50
	if l, _ := strconv.Atoi(r.URL.Query().Get("limit")); l > 0 && l <= 500 {
		limit = l
	}
	offset := 0
	if o, _ := strconv.Atoi(r.URL.Query().Get("offset")); o > 0 {
		offset = o
	}
	var logs []models.AuditLog
	q.Limit(limit).Offset(offset).Find(&logs)
	writeJSON(w, http.StatusOK, logs)
}
