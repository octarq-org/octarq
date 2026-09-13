package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/octarq-org/octarq/internal/eventbus"
)

// realtimePingInterval is the duration between keep-alive SSE ping comments.
var realtimePingInterval = 15 * time.Second

// realtimeStream handles GET /api/realtime/stream via Server-Sent Events (SSE).
// It streams real-time events published onto the event spine (eventbus/spine.go),
// strictly filtered by the caller's authenticated organization (orgID).
func (h *Handler) realtimeStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeErr(w, r, http.StatusMethodNotAllowed, "", "method not allowed")
		return
	}

	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		writeErr(w, r, http.StatusUnauthorized, "", "unauthorized")
		return
	}

	orgID, err := h.requireOrg(r)
	if err != nil {
		writeErr(w, r, http.StatusUnauthorized, "", err.Error())
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, r, http.StatusInternalServerError, "", "streaming unsupported")
		return
	}

	// Prepare SSE response headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	// Parse optional event key filters (e.g. ?events=member.invite,link.click)
	var keys []string
	if raw := strings.TrimSpace(r.URL.Query().Get("events")); raw != "" {
		for _, k := range strings.Split(raw, ",") {
			k = strings.TrimSpace(k)
			if k != "" {
				keys = append(keys, k)
			}
		}
	}

	// Subscribe to the in-process event spine
	ch, cancel := eventbus.Subscribe(eventbus.SubscribeOpts{
		Keys: keys,
	})
	defer cancel()

	// Initial handshake event confirming connection establishment
	if err := writeRealtimeSSE(w, flusher, "connected", "", map[string]any{
		"orgId": orgID,
	}); err != nil {
		return
	}

	ticker := time.NewTicker(realtimePingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if _, err := fmt.Fprintf(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case env, ok := <-ch:
			if !ok {
				return
			}
			// Tenant boundary: never leak cross-tenant events
			if env.OrgID != orgID {
				continue
			}
			if err := writeRealtimeSSE(w, flusher, env.Key, env.ID, env); err != nil {
				return
			}
		}
	}
}

// writeRealtimeSSE formats and writes a W3C-compliant SSE frame to the response.
func writeRealtimeSSE(w http.ResponseWriter, flusher http.Flusher, event, id string, data any) error {
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if id != "" {
		if _, err := fmt.Fprintf(w, "id: %s\n", id); err != nil {
			return err
		}
	}
	if event != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
			return err
		}
	}
	// Multi-line safe data emission
	lines := strings.Split(string(b), "\n")
	for _, line := range lines {
		if _, err := fmt.Fprintf(w, "data: %s\n", line); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "\n"); err != nil {
		return err
	}
	if flusher != nil {
		flusher.Flush()
	}
	return nil
}
