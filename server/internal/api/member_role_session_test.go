package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/octarq-org/octarq/internal/models"
)

// TestMemberRoleChangeInvalidatesSessions verifies that changing a member's
// role (e.g. promoting to admin or demoting back to member) immediately
// invalidates any active session cookies bound to that user and org in user_sessions,
// while preserving sessions in other workspaces. It also asserts that audit logs
// capture actor, target, oldRole, and newRole.
func TestMemberRoleChangeInvalidatesSessions(t *testing.T) {
	srv, db := newTestHandler(t)
	const orgA = uint(5101)
	const orgB = uint(5102)

	memberEmail := t.Name() + "+membera@example.com"
	ownerUID := seedOrgMember(t, db, orgA, "owner@example.com", "owner")
	memberUID := seedOrgMember(t, db, orgA, "membera@example.com", "member")
	if err := db.Create(&models.OrgMember{OrgID: orgB, UserID: memberUID, Role: "member"}).Error; err != nil {
		t.Fatalf("second membership: %v", err)
	}

	ownerSession := sessionCookies(t, ownerUID, orgA)

	// 1. Owner promotes memberA to admin in orgA
	rec := do(srv, "PATCH", "/api/org/members/"+strconv.FormatUint(uint64(memberUID), 10), ownerSession, `{"role":"admin"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("promote member to admin: got %d (%s)", rec.Code, rec.Body.String())
	}

	// 2. MemberA signs in / obtains session cookies for orgA and orgB
	memberSessionA := sessionCookies(t, memberUID, orgA)
	memberSessionB := sessionCookies(t, memberUID, orgB)

	// 3. Baseline: as admin in orgA, memberA can access settings and admin endpoints
	rec = do(srv, "GET", "/api/settings", memberSessionA, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("admin GET /api/settings: got %d, want 200", rec.Code)
	}

	rec = do(srv, "GET", "/api/settings/inbound-token", memberSessionA, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("admin GET /api/settings/inbound-token: got %d, want 200", rec.Code)
	}

	// 4. Owner demotes memberA back to member in orgA
	rec = do(srv, "PATCH", "/api/org/members/"+strconv.FormatUint(uint64(memberUID), 10), ownerSession, `{"role":"member"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("demote member: got %d (%s)", rec.Code, rec.Body.String())
	}

	// 5. MemberA's old session cookie in orgA must now be invalidated (401 Unauthorized)
	rec = do(srv, "GET", "/api/settings", memberSessionA, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("old session cookie after demotion GET /api/settings: got %d, want 401", rec.Code)
	}

	rec = do(srv, "GET", "/api/settings/inbound-token", memberSessionA, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("old session cookie after demotion GET /api/settings/inbound-token: got %d, want 401", rec.Code)
	}

	// 6. Verify real user_sessions table: 0 sessions for memberUID in orgA
	var countA int64
	db.Model(&models.Session{}).Where("user_id = ? AND org_id = ?", memberUID, orgA).Count(&countA)
	if countA != 0 {
		t.Errorf("demoted member kept %d live session(s) in orgA, want 0", countA)
	}

	// 7. Multi-tenant constraint: memberA's session in orgB must remain intact
	var countB int64
	db.Model(&models.Session{}).Where("user_id = ? AND org_id = ?", memberUID, orgB).Count(&countB)
	if countB == 0 {
		t.Errorf("memberA session in orgB was unexpectedly revoked; must be per-org")
	}

	rec = do(srv, "GET", "/api/settings", memberSessionB, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("memberA GET /api/settings in orgB: got %d, want 200", rec.Code)
	}

	// 8. Verify audit log was recorded with actor, target, oldRole, and newRole
	var foundDemotionLog bool
	for attempt := 0; attempt < 20; attempt++ {
		var auditLogs []models.AuditLog
		db.Where("org_id = ? AND target_id = ? AND (action = ? OR action = ?)", orgA, memberUID, "member.role", "member.role_changed").
			Order("id desc").
			Find(&auditLogs)
		for _, l := range auditLogs {
			var meta map[string]any
			if err := json.Unmarshal([]byte(l.Meta), &meta); err == nil {
				if (meta["from"] == "admin" || meta["oldRole"] == "admin") &&
					(meta["to"] == "member" || meta["newRole"] == "member") {
					foundDemotionLog = true
					if l.ActorID != ownerUID {
						t.Errorf("audit log ActorID = %d, want %d", l.ActorID, ownerUID)
					}
					if l.TargetID != memberUID {
						t.Errorf("audit log TargetID = %d, want %d", l.TargetID, memberUID)
					}
					if meta["actor"] != float64(ownerUID) && meta["actor"] != ownerUID {
						t.Errorf("audit meta missing actor: %+v", meta)
					}
					if meta["target"] != float64(memberUID) && meta["target"] != memberUID {
						t.Errorf("audit meta missing target: %+v", meta)
					}
					break
				}
			}
		}
		if foundDemotionLog {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !foundDemotionLog {
		t.Fatalf("expected audit log for member demotion (admin -> member), none found")
	}
	_ = memberEmail
}

// TestAddOrgMemberRoleChangeInvalidatesSessions verifies that changing a role
// via addOrgMember (re-inviting an existing member at a new role) also kicks sessions.
func TestAddOrgMemberRoleChangeInvalidatesSessions(t *testing.T) {
	srv, db := newTestHandler(t)
	const org = uint(5201)

	targetEmail := "target@example.com"
	namespacedTargetEmail := t.Name() + "+" + targetEmail

	ownerUID := seedOrgMember(t, db, org, "owner2@example.com", "owner")
	memberUID := seedOrgMember(t, db, org, targetEmail, "admin")

	ownerSession := sessionCookies(t, ownerUID, org)
	memberSession := sessionCookies(t, memberUID, org)

	// Baseline: member can access admin endpoint
	rec := do(srv, "GET", "/api/settings/inbound-token", memberSession, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("admin baseline read: got %d, want 200", rec.Code)
	}

	// Re-invite with role "member" (demote)
	rec = do(srv, "POST", "/api/org/members", ownerSession, `{"email":"`+namespacedTargetEmail+`","role":"member"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("addOrgMember demote: got %d (%s)", rec.Code, rec.Body.String())
	}

	// Session is immediately invalid
	rec = do(srv, "GET", "/api/settings/inbound-token", memberSession, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("old session after addOrgMember demotion: got %d, want 401", rec.Code)
	}

	var sessions int64
	db.Model(&models.Session{}).Where("user_id = ? AND org_id = ?", memberUID, org).Count(&sessions)
	if sessions != 0 {
		t.Errorf("demoted member kept %d session(s) in user_sessions, want 0", sessions)
	}
}
