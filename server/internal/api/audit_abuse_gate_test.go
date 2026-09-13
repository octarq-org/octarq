package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/octarq-org/octarq/internal/models"
)

func TestAuditAndAbuseRoleGates(t *testing.T) {
	_, srv, db := newTestHandlerRaw(t)

	db.Create(&models.Org{ID: 1, Name: "Test Workspace", Slug: "test-workspace"})

	db.Create(&models.User{ID: 2, Email: "admin@example.com"})
	db.Create(&models.OrgMember{OrgID: 1, UserID: 2, Role: "admin"})

	db.Create(&models.User{ID: 3, Email: "member@example.com"})
	db.Create(&models.OrgMember{OrgID: 1, UserID: 3, Role: "member"})

	// Create an abuse report so PUT /api/abuse/{id} target exists
	rep := models.AbuseReport{
		OrgID:       1,
		Slug:        "test-slug",
		Reason:      "spam",
		Description: "test spam",
		ReporterIP:  "127.0.0.1",
		Status:      "open",
	}
	db.Create(&rep)

	memberCookies := sessionCookies(t, 3, 1)
	adminCookies := sessionCookies(t, 2, 1)

	doReq := func(method, path string, cookies []*http.Cookie, body string) *httptest.ResponseRecorder {
		var req *http.Request
		if body != "" {
			req = httptest.NewRequest(method, path, bytes.NewBufferString(body))
			req.Header.Set("Content-Type", "application/json")
		} else {
			req = httptest.NewRequest(method, path, nil)
		}
		for _, c := range cookies {
			req.AddCookie(c)
		}
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		return rec
	}

	// 1. Member calling GET /api/audit, GET /api/abuse, PUT /api/abuse/{id} -> ALL 403
	t.Run("Member denied", func(t *testing.T) {
		if rec := doReq("GET", "/api/audit", memberCookies, ""); rec.Code != http.StatusForbidden {
			t.Fatalf("expected GET /api/audit for member to be 403, got %d", rec.Code)
		}
		if rec := doReq("GET", "/api/abuse", memberCookies, ""); rec.Code != http.StatusForbidden {
			t.Fatalf("expected GET /api/abuse for member to be 403, got %d", rec.Code)
		}
		if rec := doReq("PUT", "/api/abuse/1", memberCookies, `{"status":"reviewed"}`); rec.Code != http.StatusForbidden {
			t.Fatalf("expected PUT /api/abuse/1 for member to be 403, got %d", rec.Code)
		}
	})

	// 2. Admin calling -> ALL PASS (200 OK)
	t.Run("Admin allowed", func(t *testing.T) {
		if rec := doReq("GET", "/api/audit", adminCookies, ""); rec.Code != http.StatusOK {
			t.Fatalf("expected GET /api/audit for admin to be 200, got %d: body=%s", rec.Code, rec.Body.String())
		}
		if rec := doReq("GET", "/api/abuse", adminCookies, ""); rec.Code != http.StatusOK {
			t.Fatalf("expected GET /api/abuse for admin to be 200, got %d: body=%s", rec.Code, rec.Body.String())
		}
		if rec := doReq("PUT", "/api/abuse/1", adminCookies, `{"status":"reviewed"}`); rec.Code != http.StatusOK {
			t.Fatalf("expected PUT /api/abuse/1 for admin to be 200, got %d: body=%s", rec.Code, rec.Body.String())
		}
	})

	// 3. Unauthenticated public POST /abuse endpoint is not gated by role
	t.Run("Public POST /abuse allowed", func(t *testing.T) {
		rec := doReq("POST", "/abuse", nil, `{"slug":"test-slug","reason":"spam","description":"hello"}`)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected POST /abuse to be 201, got %d: body=%s", rec.Code, rec.Body.String())
		}
	})
}
