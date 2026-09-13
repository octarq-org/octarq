package api

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/octarq-org/octarq/internal/models"
)

func TestAbuseMore(t *testing.T) {
	h, srv, db := newTestHandlerRaw(t)
	adminCookies := loginCookies(t, srv)

	// Create regular member
	memberUser := models.User{Email: "memberabuse@example.com"}
	db.Create(&memberUser)
	db.Create(&models.OrgMember{OrgID: 1, UserID: memberUser.ID, Role: "member"})
	memberCookies := sessionCookies(t, memberUser.ID, 1)

	// 1. notifyAbuse with OrgID == 0 -> no-op
	h.notifyAbuse(models.AbuseReport{ID: 1, OrgID: 0, Slug: "test"})

	// 2. notifyAbuse with OrgID != 0 and notification channels
	db.Create(&models.NotificationChannel{
		OrgID:   1,
		Name:    "Abuse Channel",
		Type:    "webhook",
		Config:  `{"url":"https://example.com/webhook"}`,
		Enabled: true,
	})
	h.notifyAbuse(models.AbuseReport{ID: 2, OrgID: 1, Slug: "phishing-link", Reason: "phishing", Description: "bad"})

	// 3. updateAbuseReport with invalid status
	rep := models.AbuseReport{
		OrgID:  1,
		Slug:   "bad-slug",
		Status: "open",
	}
	db.Create(&rep)

	rec := do(srv, "PUT", fmt.Sprintf("/api/abuse/%d", rep.ID), adminCookies, `{"status":"invalid-status"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("update abuse invalid status: got %d, want 400", rec.Code)
	}

	// 4. updateAbuseReport by non-admin -> 403
	rec = do(srv, "PUT", fmt.Sprintf("/api/abuse/%d", rep.ID), memberCookies, `{"status":"reviewed"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("member update abuse: got %d, want 403", rec.Code)
	}

	// 5. listAbuseReports with status filter
	rec = do(srv, "GET", "/api/abuse?status=reviewed", adminCookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list abuse filtered: got %d", rec.Code)
	}

	// 6. listAbuseReports by non-admin -> 403
	rec = do(srv, "GET", "/api/abuse", memberCookies, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("member list abuse: got %d, want 403", rec.Code)
	}

	// 7. Nil Ctx guards
	ctx := context.Background()
	if _, err := h.submitAbuse(ctx, &SubmitAbuseInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in submitAbuse")
	}
	if _, err := h.listAbuseReports(ctx, &ListAbuseReportsInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in listAbuseReports")
	}
	if _, err := h.updateAbuseReport(ctx, &UpdateAbuseReportInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in updateAbuseReport")
	}
}
