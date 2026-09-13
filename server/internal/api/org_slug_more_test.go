package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/octarq-org/octarq/internal/models"
)

func TestOrgSlugMore(t *testing.T) {
	srv, db := newTestHandler(t)
	ownerCookies := loginCookies(t, srv)

	// Create member
	memberUser := models.User{Email: "slugmember@example.com"}
	db.Create(&memberUser)
	db.Create(&models.OrgMember{OrgID: 1, UserID: memberUser.ID, Role: "member"})
	memberCookies := sessionCookies(t, memberUser.ID, 1)

	// 1. Get slug unauth -> 401
	rec := do(srv, "GET", "/api/org/slug", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauth get slug: got %d, want 401", rec.Code)
	}

	// 2. Get slug member (non-admin) -> 403
	rec = do(srv, "GET", "/api/org/slug", memberCookies, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("member get slug: got %d, want 403", rec.Code)
	}

	// 3. Get slug owner -> 200
	rec = do(srv, "GET", "/api/org/slug", ownerCookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("owner get slug: got %d (%s)", rec.Code, rec.Body.String())
	}

	// 4. Update slug unauth -> 401
	rec = do(srv, "PUT", "/api/org/slug", nil, `{"slug":"newslug"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauth update slug: got %d, want 401", rec.Code)
	}

	// 5. Update slug member -> 403
	rec = do(srv, "PUT", "/api/org/slug", memberCookies, `{"slug":"newslug"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("member update slug: got %d, want 403", rec.Code)
	}

	// 6. Update slug invalid regex pattern -> 400
	rec = do(srv, "PUT", "/api/org/slug", ownerCookies, `{"slug":"-invalid-"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid pattern slug: got %d, want 400", rec.Code)
	}

	// 7. Update slug same as existing -> 200 (no-op)
	var org models.Org
	db.First(&org, 1)
	rec = do(srv, "PUT", "/api/org/slug", ownerCookies, `{"slug":"`+org.Slug+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("same slug update: got %d, want 200", rec.Code)
	}

	// 8. Update slug to reserved slug -> 409
	rec = do(srv, "PUT", "/api/org/slug", ownerCookies, `{"slug":"admin"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("reserved slug update: got %d, want 409", rec.Code)
	}

	// 9. Update slug to already taken slug -> 409
	org2 := models.Org{Name: "Org 2", Slug: "org-two-taken"}
	db.Create(&org2)
	rec = do(srv, "PUT", "/api/org/slug", ownerCookies, `{"slug":"org-two-taken"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("taken slug update: got %d, want 409", rec.Code)
	}

	// 10. Update slug valid new slug -> 200
	rec = do(srv, "PUT", "/api/org/slug", ownerCookies, `{"slug":"custom-brand-slug"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("valid slug update: got %d (%s)", rec.Code, rec.Body.String())
	}

	// 11. Nil Ctx calls
	h, _, _ := newTestHandlerRaw(t)
	ctx := context.Background()
	if _, err := h.getOrgSlug(ctx, &GetOrgSlugInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in getOrgSlug")
	}
	if _, err := h.updateOrgSlug(ctx, &UpdateOrgSlugInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in updateOrgSlug")
	}
}
