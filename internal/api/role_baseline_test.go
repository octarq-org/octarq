package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/octarq-org/octarq/internal/models"
)

func TestRoleBaselineEnforcement(t *testing.T) {
	_, srv, db := newTestHandlerRaw(t)

	db.Create(&models.Org{ID: 1, Name: "Test Workspace", Slug: "test-workspace"})

	db.Create(&models.User{ID: 1, Email: "owner@example.com"})
	db.Create(&models.OrgMember{OrgID: 1, UserID: 1, Role: "owner"})

	db.Create(&models.User{ID: 2, Email: "admin@example.com"})
	db.Create(&models.OrgMember{OrgID: 1, UserID: 2, Role: "admin"})

	db.Create(&models.User{ID: 3, Email: "member@example.com"})
	db.Create(&models.OrgMember{OrgID: 1, UserID: 3, Role: "member"})

	memberCookies := sessionCookies(t, 3, 1)
	adminCookies := sessionCookies(t, 2, 1)
	ownerCookies := sessionCookies(t, 1, 1)

	// Helper for sending request with session cookies
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

	t.Run("Member denied admin routes", func(t *testing.T) {
		// Tokens are deliberately absent from this list. They are a personal
		// credential — one that acts as its holder and can never out-rank them —
		// so a member may mint and manage their own. What replaced the role gate
		// is an ownership scope, covered by TestMemberTokensAreTheirOwn.

		if code := doReq("GET", "/api/webhooks", memberCookies, "").Code; code != 403 {
			t.Fatalf("expected GET /api/webhooks for member to be 403, got %d", code)
		}
		if code := doReq("POST", "/api/webhooks", memberCookies, `{"name":"w","url":"https://x.com"}`).Code; code != 403 {
			t.Fatalf("expected POST /api/webhooks for member to be 403, got %d", code)
		}

		if rec := doReq("POST", "/api/notification-channels", memberCookies, `{"name":"test","type":"telegram"}`); rec.Code != 403 {
			t.Fatalf("expected POST /api/notification-channels for member to be 403, got %d: body=%s", rec.Code, rec.Body.String())
		}

		if code := doReq("GET", "/api/account/export", memberCookies, "").Code; code != 403 {
			t.Fatalf("expected GET /api/account/export for member to be 403, got %d", code)
		}
		if rec := doReq("PUT", "/api/settings", memberCookies, `{"catchAll":false}`); rec.Code != 403 {
			t.Fatalf("expected PUT /api/settings for member to be 403, got %d: body=%s", rec.Code, rec.Body.String())
		}

		// Plugin routes (links / mail / dns) are gated too, but this handler mounts
		// no plugin, so asserting on them here only ever measures a 404. Their gates
		// are covered in each plugin's own package.
	})

	t.Run("Admin denied owner-only routes", func(t *testing.T) {
		if code := doReq("DELETE", "/api/account/data", adminCookies, `{"confirm":"DELETE MY DATA"}`).Code; code != 403 {
			t.Fatalf("expected DELETE /api/account/data for admin to be 403, got %d", code)
		}
		if code := doReq("POST", "/api/org/members", adminCookies, `{"email":"member@example.com","role":"owner"}`).Code; code != 403 {
			t.Fatalf("expected POST /api/org/members with role=owner for admin to be 403, got %d", code)
		}
		if code := doReq("POST", "/api/org/members", adminCookies, `{"email":"owner@example.com","role":"admin"}`).Code; code != 403 {
			t.Fatalf("expected POST /api/org/members targeting owner for admin to be 403, got %d", code)
		}
	})

	t.Run("Admin allowed admin routes", func(t *testing.T) {
		if code := doReq("GET", "/api/tokens", adminCookies, "").Code; code != 200 {
			t.Fatalf("expected GET /api/tokens for admin to be 200, got %d", code)
		}
		if code := doReq("GET", "/api/webhooks", adminCookies, "").Code; code != 200 {
			t.Fatalf("expected GET /api/webhooks for admin to be 200, got %d", code)
		}
		if code := doReq("GET", "/api/account/export", adminCookies, "").Code; code != 200 {
			t.Fatalf("expected GET /api/account/export for admin to be 200, got %d", code)
		}
	})

	t.Run("Owner allowed owner routes", func(t *testing.T) {
		if code := doReq("DELETE", "/api/account/data", ownerCookies, `{"confirm":"DELETE MY DATA"}`).Code; code != 200 {
			t.Fatalf("expected DELETE /api/account/data for owner to be 200, got %d", code)
		}
	})
}
