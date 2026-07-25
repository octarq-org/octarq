package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/internal/models"
)

func TestAdminBackupEndpoint(t *testing.T) {
	srv, db := newTestHandler(t)

	// 1. Unauthenticated request -> 401 Unauthorized
	req := httptest.NewRequest("GET", "/api/admin/backup", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated /api/admin/backup: got %d, want 401", rec.Code)
	}

	// 2. Non-instance admin user -> 403 Forbidden
	do(srv, "POST", "/api/auth/register", nil, `{"email":"regular@user.com","password":"password123"}`)
	loginRec := do(srv, "POST", "/api/auth/login", nil, `{"username":"regular@user.com","password":"password123"}`)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login regular user failed: %d", loginRec.Code)
	}
	cookies := loginRec.Result().Cookies()

	req = httptest.NewRequest("GET", "/api/admin/backup", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin /api/admin/backup: got %d, want 403", rec.Code)
	}

	// 3. Mark user as IsInstanceAdmin -> 200 OK with attachment header
	if err := db.Model(&models.User{}).Where("email = ?", "regular@user.com").Update("is_instance_admin", true).Error; err != nil {
		t.Fatalf("failed to promote user to instance admin: %v", err)
	}

	req = httptest.NewRequest("GET", "/api/admin/backup", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin /api/admin/backup: got %d, want 200", rec.Code)
	}

	disposition := rec.Header().Get("Content-Disposition")
	if !strings.Contains(disposition, "attachment; filename=") {
		t.Errorf("expected attachment Content-Disposition header, got %q", disposition)
	}
	if rec.Body.Len() == 0 {
		t.Error("expected non-empty database backup stream")
	}
}
