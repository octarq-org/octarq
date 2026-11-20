package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/config"
	"github.com/octarq-org/octarq/server/internal/auth"
	"github.com/octarq-org/octarq/server/internal/crypto"
	"github.com/octarq-org/octarq/server/internal/models"
	"gorm.io/gorm"
)

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

	if rec := do(srv, "GET", "/api/org/members", memberSession, ""); rec.Code != http.StatusOK {
		t.Fatalf("member baseline read: got %d, want 200", rec.Code)
	}

	rec := do(srv, "DELETE", "/api/org/members/"+strconv.FormatUint(uint64(memberUID), 10), ownerSession, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("remove member: got %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}

	var count int64
	db.Model(&models.OrgMember{}).Where("org_id = ? AND user_id = ?", org, memberUID).Count(&count)
	if count != 0 {
		t.Fatalf("membership row survived removal")
	}

	var sessions int64
	db.Model(&models.Session{}).Where("user_id = ? AND org_id = ?", memberUID, org).Count(&sessions)
	if sessions != 0 {
		t.Errorf("removed member kept %d live session(s)", sessions)
	}

	if rec := do(srv, "GET", "/api/org/members", memberSession, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("removed member read after removal: got %d, want 401", rec.Code)
	}

	var foundRemovalLog bool
	for attempt := 0; attempt < 20; attempt++ {
		var auditLogs []models.AuditLog
		db.Where("org_id = ? AND target_id = ? AND action = ?", org, memberUID, "member.remove").
			Order("id desc").
			Find(&auditLogs)
		for _, l := range auditLogs {
			var meta map[string]any
			if err := json.Unmarshal([]byte(l.Meta), &meta); err == nil {
				if meta["oldRole"] == "member" && meta["newRole"] == "none" {
					foundRemovalLog = true
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
		if foundRemovalLog {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !foundRemovalLog {
		t.Fatalf("expected audit log for member removal with actor, target, oldRole, and newRole; none found")
	}
}

func TestRemovalLeavesOtherOrgSessionsIntact(t *testing.T) {
	_, db := newTestHandler(t)
	const orgA, orgB = uint(4201), uint(4202)

	uid := seedOrgMember(t, db, orgA, "multi@x.com", "member")
	if err := db.Create(&models.OrgMember{OrgID: orgB, UserID: uid, Role: "member"}).Error; err != nil {
		t.Fatalf("second membership: %v", err)
	}
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
