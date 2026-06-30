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

	"github.com/Jungley8/led/internal/models"
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
