package api

// Removing a member must end their access immediately, not when their session
// happens to expire.
//
// Membership is checked once, at login: the session row carries its org and no
// later request re-reads org_members. So deleting the membership row on its own
// left a removed member with full read/write on the workspace for the remaining
// session TTL (up to 7 days) — they lost the role-gated endpoints (callerOrgRole
// returns "" without a membership row) but kept the entire data plane, which is
// the part that matters.

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/octarq-org/octarq/config"
	"github.com/octarq-org/octarq/internal/auth"
	"github.com/octarq-org/octarq/internal/crypto"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugins/links"
	"gorm.io/gorm"
)

// testAuthManager builds an auth.Manager over the test DB, matching how
// sessionCookies mints the sessions these tests then revoke.
func testAuthManager(t *testing.T, db *gorm.DB) *auth.Manager {
	t.Helper()
	return auth.New(&config.Config{SecretKey: "secret"}, crypto.New("secret")).WithDB(db)
}

func TestRemovedMemberLosesAccessImmediately(t *testing.T) {
	srv, db := newTestHandler(t)
	const org = uint(4101)

	ownerUID := seedOrgMember(t, db, org, "owner@x.com", "owner")
	memberUID := seedOrgMember(t, db, org, "member@x.com", "member")

	ownerSession := sessionCookies(t, ownerUID, org)
	memberSession := sessionCookies(t, memberUID, org)

	if err := db.Create(&links.Link{OrgID: org, Slug: "revoke-probe", Target: "https://example.com"}).Error; err != nil {
		t.Fatalf("seed link: %v", err)
	}

	// Baseline: the member can read the workspace's links.
	if rec := do(srv, "GET", "/api/links", memberSession, ""); rec.Code != http.StatusOK {
		t.Fatalf("member baseline read: got %d, want 200", rec.Code)
	}

	rec := do(srv, "DELETE", "/api/org/members/"+strconv.FormatUint(uint64(memberUID), 10), ownerSession, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("remove member: got %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}

	// The membership row is gone...
	var count int64
	db.Model(&models.OrgMember{}).Where("org_id = ? AND user_id = ?", org, memberUID).Count(&count)
	if count != 0 {
		t.Fatalf("membership row survived removal")
	}

	// ...and so is the session that was bound to it. This is the regression:
	// before the fix the same cookie still returned 200 with the org's links.
	var sessions int64
	db.Model(&models.Session{}).Where("user_id = ? AND org_id = ?", memberUID, org).Count(&sessions)
	if sessions != 0 {
		t.Errorf("removed member kept %d live session(s)", sessions)
	}

	if rec := do(srv, "GET", "/api/links", memberSession, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("removed member read after removal: got %d, want 401", rec.Code)
	}
}

// A removal in one workspace must not sign the user out of their other
// workspaces — revocation is scoped to (user, org), not to the user.
func TestRemovalLeavesOtherOrgSessionsIntact(t *testing.T) {
	_, db := newTestHandler(t)
	const orgA, orgB = uint(4201), uint(4202)

	uid := seedOrgMember(t, db, orgA, "multi@x.com", "member")
	if err := db.Create(&models.OrgMember{OrgID: orgB, UserID: uid, Role: "member"}).Error; err != nil {
		t.Fatalf("second membership: %v", err)
	}
	// Two live sessions, one per workspace.
	sessionCookies(t, uid, orgA)
	sessionCookies(t, uid, orgB)

	mgr := testAuthManager(t, db)
	if got := mgr.RevokeUserOrgSessions(uid, orgA); got == 0 {
		t.Fatalf("revoked 0 sessions for org A, want >0")
	}

	var a, b int64
	db.Model(&models.Session{}).Where("user_id = ? AND org_id = ?", uid, orgA).Count(&a)
	db.Model(&models.Session{}).Where("user_id = ? AND org_id = ?", uid, orgB).Count(&b)
	if a != 0 {
		t.Errorf("org A sessions: got %d, want 0", a)
	}
	if b == 0 {
		t.Errorf("org B sessions were revoked too; revocation must be per-org")
	}
}

// Changing a member's role revokes their sessions exactly like removal does, so
// a cached client holding a session minted under the old (higher) role cannot
// keep acting on it after a demotion.
func TestRoleChangeRevokesSessions(t *testing.T) {
	srv, db := newTestHandler(t)
	const org = uint(4301)

	ownerUID := seedOrgMember(t, db, org, "owner@x.com", "owner")
	memberUID := seedOrgMember(t, db, org, "admin@x.com", "admin")

	ownerSession := sessionCookies(t, ownerUID, org)
	memberSession := sessionCookies(t, memberUID, org)

	if rec := do(srv, "GET", "/api/org/members", memberSession, ""); rec.Code != http.StatusOK {
		t.Fatalf("baseline member read: got %d, want 200", rec.Code)
	}

	rec := do(srv, "PATCH", "/api/org/members/"+strconv.FormatUint(uint64(memberUID), 10), ownerSession, `{"role":"member"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("demote member: got %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}

	var role string
	db.Model(&models.OrgMember{}).Where("org_id = ? AND user_id = ?", org, memberUID).Pluck("role", &role)
	if role != "member" {
		t.Fatalf("role after demotion: %q, want member", role)
	}

	var sessions int64
	db.Model(&models.Session{}).Where("user_id = ? AND org_id = ?", memberUID, org).Count(&sessions)
	if sessions != 0 {
		t.Errorf("demoted member kept %d live session(s)", sessions)
	}

	if rec := do(srv, "GET", "/api/org/members", memberSession, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("demoted member session still valid: got %d, want 401", rec.Code)
	}
}
