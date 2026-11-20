package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/internal/models"
)

func TestListOrgMembersReportsMembershipDateNotAccountDate(t *testing.T) {
	srv, db := newTestHandler(t)
	cookies := loginCookies(t, srv)

	var member models.OrgMember
	if err := db.Where("org_id = ?", 1).First(&member).Error; err != nil {
		t.Fatalf("seed membership: %v", err)
	}

	accountCreated := time.Now().Add(-90 * 24 * time.Hour)
	joined := time.Now().Add(-3 * 24 * time.Hour)
	if err := db.Model(&models.User{}).Where("id = ?", member.UserID).
		Update("created_at", accountCreated).Error; err != nil {
		t.Fatalf("backdate account: %v", err)
	}
	if err := db.Model(&models.OrgMember{}).
		Where("org_id = ? AND user_id = ?", member.OrgID, member.UserID).
		Update("created_at", joined).Error; err != nil {
		t.Fatalf("backdate membership: %v", err)
	}

	rec := do(srv, "GET", "/api/org/members", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list members: got %d (%s)", rec.Code, rec.Body.String())
	}
	var items []MemberItem
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("unmarshal members: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("no members returned")
	}
	got := items[0].JoinedAt
	if got == nil || got.Sub(joined) > time.Minute || got.Sub(joined) < -time.Minute {
		t.Errorf("joinedAt = %v, want the membership date %s (account was created %s)",
			got, joined.Format(time.RFC3339), accountCreated.Format(time.RFC3339))
	}
}

func TestListOrgMembersOmitsUnknownJoinDate(t *testing.T) {
	srv, db := newTestHandler(t)
	cookies := loginCookies(t, srv)

	if err := db.Model(&models.OrgMember{}).Where("org_id = ?", 1).
		Update("created_at", time.Time{}).Error; err != nil {
		t.Fatalf("clear membership timestamp: %v", err)
	}

	rec := do(srv, "GET", "/api/org/members", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list members: got %d (%s)", rec.Code, rec.Body.String())
	}
	var items []MemberItem
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("unmarshal members: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("no members returned")
	}
	if items[0].JoinedAt != nil {
		t.Errorf("joinedAt = %v, want omitted for a row with no join timestamp", items[0].JoinedAt)
	}
}
