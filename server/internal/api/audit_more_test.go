package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/octarq-org/octarq/internal/models"
)

func TestAuditLogsMore(t *testing.T) {
	srv, db := newTestHandler(t)
	adminCookies := loginCookies(t, srv)

	// Create member
	memberUser := models.User{Email: "memberaudit@example.com"}
	db.Create(&memberUser)
	db.Create(&models.OrgMember{OrgID: 1, UserID: memberUser.ID, Role: "member"})
	memberCookies := sessionCookies(t, memberUser.ID, 1)

	// Seed audit logs
	db.Create(&models.AuditLog{OrgID: 1, Action: "link.create", TargetType: "link", TargetID: 10})
	db.Create(&models.AuditLog{OrgID: 1, Action: "member.role", TargetType: "user", TargetID: 20})

	// 1. Unauth -> 401
	rec := do(srv, "GET", "/api/audit", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauth audit: got %d, want 401", rec.Code)
	}

	// 2. Member (non-admin) -> 403
	rec = do(srv, "GET", "/api/audit", memberCookies, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("member audit: got %d, want 403", rec.Code)
	}

	// 3. Admin with filters: action, targetType, limit, offset
	rec = do(srv, "GET", "/api/audit?action=link.create&targetType=link&limit=10&offset=0", adminCookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("admin audit filtered: got %d (%s)", rec.Code, rec.Body.String())
	}

	// 4. Nil Ctx calls
	h, _, _ := newTestHandlerRaw(t)
	ctx := context.Background()
	if _, err := h.listAuditLogs(ctx, &ListAuditLogsInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in listAuditLogs")
	}
}
