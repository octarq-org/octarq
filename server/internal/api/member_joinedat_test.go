package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/octarq-org/octarq/internal/models"
)

// The members list reports when someone joined THIS workspace. It used to
// select users.created_at, so one person read as having joined every workspace
// on the day they registered — the same relative date on every list.
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

// A membership row written before org_members carried a timestamp has no join
// date to report. Omitting the field is the honest answer; serialising the zero
// time would render as year 1 or as "joined 2000 years ago".
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
