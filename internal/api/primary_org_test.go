package api

import (
	"testing"

	"github.com/octarq-org/octarq/internal/models"
)

// P3-19: system emails (password reset, address verification) are sent through
// one workspace's mail configuration, but the user who triggers them is not in a
// workspace context — they are identified only by email address. A multi-workspace
// user therefore has no "current" org to send from.
//
// primaryOrgForUser picks the lowest org id. The point is not that the lowest is
// the *right* workspace — there is no right answer available here — but that the
// choice is stable. Reset mail arriving from a different sender each attempt
// looks like a phishing attempt to the recipient and is undebuggable for the
// operator; the original code took whatever row the database returned first.
//
// Honest limit of this test: it CANNOT prove the ORDER BY is load-bearing.
// OrgMember's primary key is (org_id, user_id), so SQLite already returns the
// lowest org_id first from storage order — deleting the ORDER BY leaves this
// green. It is on Postgres, where a scan may return rows in any order, that the
// clause actually does the work, and the suite has no Postgres to run against.
//
// What the test does catch is a genuinely unstable implementation (random pick,
// map iteration, "most recent membership"), which is the regression that would
// reach a user as mail from a different workspace each time.
func TestPrimaryOrgForUserIsDeterministic(t *testing.T) {
	h, _, db := newTestHandlerRaw(t)

	uid := seedOrgMember(t, db, 5, "multi@example.com", "member")
	// Same user, additional workspaces, inserted out of order so the lowest id is
	// neither the first nor the last row written.
	for _, org := range []uint{2, 9} {
		if err := db.Create(&models.OrgMember{OrgID: org, UserID: uid, Role: "member"}).Error; err != nil {
			t.Fatalf("seed membership in org %d: %v", org, err)
		}
	}

	first := h.primaryOrgForUser(uid)
	if first != 2 {
		t.Errorf("primaryOrgForUser = %d, want 2 (the lowest workspace id)", first)
	}
	// Stability across calls is the property that matters, so assert it directly
	// rather than trusting one lucky read.
	for i := 0; i < 5; i++ {
		if got := h.primaryOrgForUser(uid); got != first {
			t.Fatalf("primaryOrgForUser returned %d on call %d after %d: the sender workspace is not stable", got, i+2, first)
		}
	}
}

// TestPrimaryOrgForUserNoMembership: a user belonging to no workspace yields 0,
// not a default tenant. The mail seam then has no org to send from, which is the
// honest outcome — silently borrowing org 1's SMTP credentials would send mail
// from an unrelated workspace's domain.
func TestPrimaryOrgForUserNoMembership(t *testing.T) {
	h, _, db := newTestHandlerRaw(t)

	u := models.User{Email: t.Name() + "@example.com"}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	if got := h.primaryOrgForUser(u.ID); got != 0 {
		t.Errorf("primaryOrgForUser for an org-less user = %d, want 0", got)
	}
}
