package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/config"
	"github.com/octarq-org/octarq/internal/auth"
	"github.com/octarq-org/octarq/internal/crypto"
	"github.com/octarq-org/octarq/internal/geo"
	"github.com/octarq-org/octarq/internal/queue"
	"gorm.io/gorm"
)

func newTestStatusHandler(t *testing.T) (*Handler, http.Handler) {
	t.Helper()
	dbName := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	cfg := &config.Config{SecretKey: "secret"}
	cipher := crypto.New(cfg.SecretKey)
	authMgr := auth.New(cfg, cipher).WithDB(db)
	g, _ := geo.Open("")
	h := New(cfg, db, cipher, authMgr, g, queue.New(""))
	srv := h.Routes()
	return h, srv
}

func TestStatusEndpoint(t *testing.T) {
	_, srv := newTestStatusHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/status: got status %d want 200", rec.Code)
	}

	var resp struct {
		Overall    string `json:"overall"`
		Subsystems []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"subsystems"`
		Time string `json:"time"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode status response: %v", err)
	}

	if resp.Overall == "" {
		t.Errorf("expected overall status to be set")
	}

	if len(resp.Subsystems) < 3 {
		t.Errorf("expected at least 3 subsystems (database, mail, queue), got %d", len(resp.Subsystems))
	}

	dbFound := false
	for _, sub := range resp.Subsystems {
		if sub.Name == "database" {
			dbFound = true
			if sub.Status != "ok" {
				t.Errorf("expected database status 'ok', got %s", sub.Status)
			}
		}
	}
	if !dbFound {
		t.Errorf("database subsystem not found in status output")
	}

	if _, err := time.Parse(time.RFC3339, resp.Time); err != nil {
		t.Errorf("invalid timestamp format: %v", err)
	}

	bodyStr := rec.Body.String()
	for _, sensitive := range []string{"password", "secret", "token", "127.0.0.1"} {
		if strings.Contains(bodyStr, sensitive) {
			t.Errorf("status endpoint leaks sensitive info: %s", sensitive)
		}
	}
}

func TestStatusEndpointRateLimiting(t *testing.T) {
	h, srv := newTestStatusHandler(t)
	// Artificially lower limit for testing
	h.statusLimiter = newRateLimiter("", "test_status", 2, time.Minute)

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i+1, rec.Code)
		}
	}

	// 3rd request should hit rate limit
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected status 429 on rate limit exceeded, got %d", rec.Code)
	}
}
