package telemetry

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestDefaultPrometheusRegistry(t *testing.T) {
	reg := DefaultPrometheusRegistry()
	if reg == nil {
		t.Fatal("expected non-nil default prometheus registry")
	}
}

func TestRecordHTTPRequest(t *testing.T) {
	RecordHTTPRequest(http.MethodGet, "/api/links", http.StatusOK, 25*time.Millisecond)
	RecordHTTPRequest(http.MethodPost, "/api/links", http.StatusCreated, 50*time.Millisecond)
	RecordHTTPRequest(http.MethodGet, "/unknown", http.StatusNotFound, 5*time.Millisecond)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	DefaultPrometheusHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `octarq_http_requests_total{method="GET",path="/api/links",status="200"}`) {
		t.Errorf("expected GET /api/links counter in body:\n%s", body)
	}
	if !strings.Contains(body, `octarq_http_requests_total{method="POST",path="/api/links",status="201"}`) {
		t.Errorf("expected POST /api/links counter in body:\n%s", body)
	}
}

func TestMCPSessionsCounters(t *testing.T) {
	SetMCPSessions(0)
	IncMCPSessions()
	IncMCPSessions()
	DecMCPSessions()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	DefaultPrometheusHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "octarq_mcp_active_sessions 1") {
		t.Errorf("expected octarq_mcp_active_sessions 1, got:\n%s", body)
	}

	SetMCPSessions(42)
	rec2 := httptest.NewRecorder()
	DefaultPrometheusHandler().ServeHTTP(rec2, req)
	if !strings.Contains(rec2.Body.String(), "octarq_mcp_active_sessions 42") {
		t.Errorf("expected octarq_mcp_active_sessions 42, got:\n%s", rec2.Body.String())
	}
}

func TestDBOpenConnectionsMonitoring(t *testing.T) {
	SetDBOpenConnections(7)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	DefaultPrometheusHandler().ServeHTTP(rec, req)

	if !strings.Contains(rec.Body.String(), "octarq_db_open_connections 7") {
		t.Errorf("expected octarq_db_open_connections 7, got:\n%s", rec.Body.String())
	}

	// Register a real SQL DB
	gdb, err := gorm.Open(sqlite.Open(t.TempDir()+"/dbtest.db"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm open: %v", err)
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	RegisterDB(sqlDB)
	RegisterDB(nil) // nil guard test

	rec2 := httptest.NewRecorder()
	DefaultPrometheusHandler().ServeHTTP(rec2, req)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec2.Code)
	}
	if !strings.Contains(rec2.Body.String(), "octarq_db_open_connections") {
		t.Errorf("expected octarq_db_open_connections in output")
	}
}

func TestUpdateDBStatsNoTracked(t *testing.T) {
	dbMu.Lock()
	orig := trackedDBs
	trackedDBs = nil
	dbMu.Unlock()
	t.Cleanup(func() {
		dbMu.Lock()
		trackedDBs = orig
		dbMu.Unlock()
	})

	updateDBStats() // should be no-op when trackedDBs is empty
}
