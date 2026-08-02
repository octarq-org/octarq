package api

// Authorization tests for organization member management. A plain member must
// not be able to escalate their own role or evict others; only owner/admin can
// manage members, only an owner can mint/remove owners, and the last owner can
// never be removed.

import (
	"fmt"
	"net/http"
	"testing"

	"gorm.io/gorm"

	"github.com/octarq-org/octarq/internal/models"
)

// seedOrgMember inserts a user + membership row and returns the user id. The
// email is namespaced by the test name because the in-memory DB cache is shared
// across tests, so bare addresses would collide on the unique-email constraint.
func seedOrgMember(t *testing.T, db *gorm.DB, orgID uint, email, role string) uint {
	t.Helper()
	email = t.Name() + "+" + email
	u := models.User{Email: email}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("create user %s: %v", email, err)
	}
	if err := db.Create(&models.OrgMember{OrgID: orgID, UserID: u.ID, Role: role}).Error; err != nil {
		t.Fatalf("create membership %s: %v", email, err)
	}
	return u.ID
}

func TestMemberCannotManageMembers(t *testing.T) {
	srv, db := newTestHandler(t)
	const org = uint(101)
	seedOrgMember(t, db, org, "owner@x.com", "owner")
	memberUID := seedOrgMember(t, db, org, "member@x.com", "member")
	memberSession := sessionCookies(t, memberUID, org)

	// A member trying to add anyone → 403.
	rec := do(srv, "POST", "/api/org/members", memberSession, `{"email":"new@x.com","role":"member"}`)
	if rec.Code != http.StatusForbidden {
		t.Errorf("member add: got %d, want 403", rec.Code)
	}

	// The self-escalation path (add own email with role=owner) must also 403.
	rec = do(srv, "POST", "/api/org/members", memberSession, `{"email":"member@x.com","role":"owner"}`)
	if rec.Code != http.StatusForbidden {
		t.Errorf("member self-escalation: got %d, want 403", rec.Code)
	}
	// Confirm the role did not change.
	var role string
	db.Model(&models.OrgMember{}).Where("org_id = ? AND user_id = ?", org, memberUID).Pluck("role", &role)
	if role != "member" {
		t.Errorf("member role escalated to %q", role)
	}
}

func TestAdminCannotGrantOwner(t *testing.T) {
	srv, db := newTestHandler(t)
	const org = uint(102)
	seedOrgMember(t, db, org, "owner@x.com", "owner")
	adminUID := seedOrgMember(t, db, org, "admin@x.com", "admin")
	adminSession := sessionCookies(t, adminUID, org)

	// Admin can add a regular member.
	if rec := do(srv, "POST", "/api/org/members", adminSession, `{"email":"m@x.com","role":"member"}`); rec.Code != http.StatusOK {
		t.Fatalf("admin add member: got %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	// But cannot grant the owner role.
	if rec := do(srv, "POST", "/api/org/members", adminSession, `{"email":"m2@x.com","role":"owner"}`); rec.Code != http.StatusForbidden {
		t.Errorf("admin grant owner: got %d, want 403", rec.Code)
	}
}

func TestCannotRemoveLastOwner(t *testing.T) {
	srv, db := newTestHandler(t)
	const org = uint(103)
	ownerUID := seedOrgMember(t, db, org, "owner@x.com", "owner")
	ownerSession := sessionCookies(t, ownerUID, org)

	rec := do(srv, "DELETE", fmt.Sprintf("/api/org/members/%d", ownerUID), ownerSession, "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("remove last owner: got %d, want 400", rec.Code)
	}
	var owners int64
	db.Model(&models.OrgMember{}).Where("org_id = ? AND role = ?", org, "owner").Count(&owners)
	if owners != 1 {
		t.Errorf("owner count = %d, want 1 (last owner must survive)", owners)
	}
}

func TestAdminCannotRemoveOwner(t *testing.T) {
	srv, db := newTestHandler(t)
	const org = uint(104)
	ownerUID := seedOrgMember(t, db, org, "owner@x.com", "owner")
	adminUID := seedOrgMember(t, db, org, "admin@x.com", "admin")
	adminSession := sessionCookies(t, adminUID, org)

	rec := do(srv, "DELETE", fmt.Sprintf("/api/org/members/%d", ownerUID), adminSession, "")
	if rec.Code != http.StatusForbidden {
		t.Errorf("admin remove owner: got %d, want 403", rec.Code)
	}
}

// The PATCH endpoint carries the same owner rules as POST — it exists so the
// console can say "promote this member" without re-inviting an address — so the
// rules are re-asserted against it rather than assumed to travel with them.
func TestUpdateMemberRole(t *testing.T) {
	srv, db := newTestHandler(t)
	const org = uint(140)
	ownerUID := seedOrgMember(t, db, org, "owner@x.com", "owner")
	adminUID := seedOrgMember(t, db, org, "admin@x.com", "admin")
	memberUID := seedOrgMember(t, db, org, "member@x.com", "member")
	ownerSession := sessionCookies(t, ownerUID, org)
	adminSession := sessionCookies(t, adminUID, org)
	memberSession := sessionCookies(t, memberUID, org)

	roleOf := func(uid uint) string {
		var role string
		db.Model(&models.OrgMember{}).Where("org_id = ? AND user_id = ?", org, uid).Pluck("role", &role)
		return role
	}
	path := func(uid uint) string { return fmt.Sprintf("/api/org/members/%d", uid) }

	// A member cannot re-grade anyone, themselves least of all.
	if rec := do(srv, "PATCH", path(memberUID), memberSession, `{"role":"admin"}`); rec.Code != http.StatusForbidden {
		t.Errorf("member self-promotion: got %d, want 403", rec.Code)
	}
	if got := roleOf(memberUID); got != "member" {
		t.Fatalf("member role escalated to %q", got)
	}

	// An admin may re-grade a member…
	if rec := do(srv, "PATCH", path(memberUID), adminSession, `{"role":"admin"}`); rec.Code != http.StatusOK {
		t.Errorf("admin promoting a member: got %d, want 200", rec.Code)
	}
	if got := roleOf(memberUID); got != "admin" {
		t.Errorf("role after admin promotion: %q, want admin", got)
	}
	// …but neither mint an owner…
	if rec := do(srv, "PATCH", path(memberUID), adminSession, `{"role":"owner"}`); rec.Code != http.StatusForbidden {
		t.Errorf("admin granting owner: got %d, want 403", rec.Code)
	}
	// …nor demote the one there is.
	if rec := do(srv, "PATCH", path(ownerUID), adminSession, `{"role":"member"}`); rec.Code != http.StatusForbidden {
		t.Errorf("admin demoting the owner: got %d, want 403", rec.Code)
	}
	if got := roleOf(ownerUID); got != "owner" {
		t.Fatalf("owner was demoted to %q by an admin", got)
	}

	// An owner may grant the role.
	if rec := do(srv, "PATCH", path(memberUID), ownerSession, `{"role":"owner"}`); rec.Code != http.StatusOK {
		t.Errorf("owner granting owner: got %d, want 200", rec.Code)
	}
	if got := roleOf(memberUID); got != "owner" {
		t.Errorf("role after owner grant: %q, want owner", got)
	}

	// A role outside the closed set is a 400, not a silently-applied value.
	if rec := do(srv, "PATCH", path(memberUID), ownerSession, `{"role":"superuser"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("bogus role: got %d, want 400", rec.Code)
	}
	// Someone who is not a member of this workspace is a 404, not a new row.
	if rec := do(srv, "PATCH", path(99999), ownerSession, `{"role":"admin"}`); rec.Code != http.StatusNotFound {
		t.Errorf("non-member: got %d, want 404", rec.Code)
	}
}

// Demoting the last owner strands the workspace exactly as removing them would:
// nobody is left who can grant the role back, including the person who gave it
// up. Both endpoints that can do it must refuse.
func TestLastOwnerCannotBeDemoted(t *testing.T) {
	srv, db := newTestHandler(t)
	const org = uint(141)
	ownerUID := seedOrgMember(t, db, org, "owner@x.com", "owner")
	ownerSession := sessionCookies(t, ownerUID, org)

	roleOf := func(uid uint) string {
		var role string
		db.Model(&models.OrgMember{}).Where("org_id = ? AND user_id = ?", org, uid).Pluck("role", &role)
		return role
	}

	if rec := do(srv, "PATCH", fmt.Sprintf("/api/org/members/%d", ownerUID), ownerSession, `{"role":"admin"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("PATCH demoting the last owner: got %d, want 400", rec.Code)
	}
	// The invite path re-grades an existing member too, and used to do it with
	// no last-owner guard at all.
	if rec := do(srv, "POST", "/api/org/members", ownerSession,
		fmt.Sprintf(`{"email":%q,"role":"admin"}`, t.Name()+"+owner@x.com")); rec.Code != http.StatusBadRequest {
		t.Errorf("POST demoting the last owner: got %d, want 400", rec.Code)
	}
	if got := roleOf(ownerUID); got != "owner" {
		t.Fatalf("last owner was demoted to %q — the workspace has no owner", got)
	}

	// With a second owner in place the demotion is allowed.
	otherUID := seedOrgMember(t, db, org, "owner2@x.com", "owner")
	if rec := do(srv, "PATCH", fmt.Sprintf("/api/org/members/%d", otherUID), ownerSession, `{"role":"member"}`); rec.Code != http.StatusOK {
		t.Errorf("demoting a non-last owner: got %d, want 200", rec.Code)
	}
}
