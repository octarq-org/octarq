package api

import (
	"net/http"
	"time"
)

// health verifies system dependencies (specifically database connectivity)
// and returns the status of the service.
func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	sqlDB, err := h.db.DB()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"status":   "unhealthy",
			"database": "down",
			"error":    err.Error(),
			"time":     time.Now().Format(time.RFC3339),
		})
		return
	}

	err = sqlDB.Ping()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"status":   "unhealthy",
			"database": "down",
			"error":    err.Error(),
			"time":     time.Now().Format(time.RFC3339),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "healthy",
		"database": "up",
		"time":     time.Now().Format(time.RFC3339),
	})
}
