package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHealthEndpoint(t *testing.T) {
	srv, _ := newTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/health: got status %d want 200", rec.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["status"] != "healthy" {
		t.Errorf("expected status 'healthy', got %v", resp["status"])
	}

	if resp["database"] != "up" {
		t.Errorf("expected database 'up', got %v", resp["database"])
	}

	timeStr, ok := resp["time"].(string)
	if !ok {
		t.Fatalf("expected time to be a string, got %T", resp["time"])
	}

	if _, err := time.Parse(time.RFC3339, timeStr); err != nil {
		t.Errorf("invalid time format: %v", err)
	}
}
