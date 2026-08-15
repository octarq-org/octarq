package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/octarq-org/octarq/internal/models"
	"gorm.io/gorm"
)

// TestAdminLoginDoesNotDuplicateOrg pins the bootstrap fix: org_members has a
// composite primary key and NO id column, so Order("id") on the membership
// lookup (auth.go bootstrapOrgID / bootstrapUserID) errored out and every admin
// login minted a fresh org — the session then pointed at the newest org while
// the data hung on the first. Two logins must resolve to the one preset org,
// and the session must carry it.
func TestAdminLoginDoesNotDuplicateOrg(t *testing.T) {
	h, srv, db := newTestHandlerRaw(t)

	admin := models.User{Email: "admin", PasswordHash: "", IsInstanceAdmin: true, EmailVerified: true}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("preset admin user: %v", err)
	}
	org := models.Org{Name: "admin", Slug: "preset-admin-org", InboundToken: "t"}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("preset org: %v", err)
	}
	if err := db.Create(&models.OrgMember{OrgID: org.ID, UserID: admin.ID, Role: "owner"}).Error; err != nil {
		t.Fatalf("preset membership: %v", err)
	}

	var lastRec *httptest.ResponseRecorder
	for i := 1; i <= 2; i++ {
		rec := do(srv, "POST", "/api/auth/login", nil, `{"email":"admin","password":"pw"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("admin login #%d: got %d (%s)", i, rec.Code, rec.Body.String())
		}
		lastRec = rec
	}

	var orgCount, memberCount int64
	if err := db.Model(&models.Org{}).Count(&orgCount).Error; err != nil {
		t.Fatalf("count orgs: %v", err)
	}
	if orgCount != 1 {
		t.Fatalf("orgs after two admin logins: got %d, want 1 (each login minted a new org)", orgCount)
	}
	if err := db.Model(&models.OrgMember{}).Count(&memberCount).Error; err != nil {
		t.Fatalf("count memberships: %v", err)
	}
	if memberCount != 1 {
		t.Fatalf("org_members after two admin logins: got %d, want 1", memberCount)
	}
	if got := h.bootstrapOrgID(); got != org.ID {
		t.Fatalf("bootstrapOrgID after two logins: got %d, want %d (org resolution drifted)", got, org.ID)
	}

	// The session handed out on login must carry the preset org, not a fresh one.
	me := do(srv, "GET", "/api/auth/me", lastRec.Result().Cookies(), "")
	if me.Code != http.StatusOK {
		t.Fatalf("me after second login: got %d (%s)", me.Code, me.Body.String())
	}
	var body struct {
		OrgID uint `json:"orgId"`
	}
	if err := json.Unmarshal(me.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode me body: %v", err)
	}
	if body.OrgID != org.ID {
		t.Fatalf("session org after second login: got %d, want %d", body.OrgID, org.ID)
	}
}

// TestLoginAuditCarriesActorAndOrg pins the audit attribution fix: user.login
// was written before a session existed, so the request-derived org/actor were
// always 0/0 and logins were untraceable. The audit write is async (auditAs
// spawns a goroutine), so this polls the table with a deadline instead of a
// fixed sleep.
func TestLoginAuditCarriesActorAndOrg(t *testing.T) {
	_, srv, db := newTestHandlerRaw(t)

	rec := do(srv, "POST", "/api/auth/login", nil, `{"email":"admin","password":"pw"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin login: got %d (%s)", rec.Code, rec.Body.String())
	}

	var user models.User
	if err := db.Where("email = ?", "admin").First(&user).Error; err != nil {
		t.Fatalf("load admin user: %v", err)
	}
	var member models.OrgMember
	if err := db.Where("user_id = ?", user.ID).First(&member).Error; err != nil {
		t.Fatalf("load admin membership: %v", err)
	}

	log := waitForAudit(t, db, "user.login")
	if log.OrgID != member.OrgID {
		t.Fatalf("user.login audit row: org_id=%d, want %d (the org the login resolved)", log.OrgID, member.OrgID)
	}
	if log.ActorID != user.ID {
		t.Fatalf("user.login audit row: actor_id=%d, want %d", log.ActorID, user.ID)
	}
}

// waitForAudit polls audit_logs until a row with the given action appears. The
// audit write is asynchronous, so a fixed time.Sleep would be flaky under
// -race; poll with a deadline instead.
func waitForAudit(t *testing.T, db *gorm.DB, action string) models.AuditLog {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var log models.AuditLog
		if err := db.Where("action = ?", action).First(&log).Error; err == nil {
			return log
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("audit row for action %q never appeared within deadline", action)
	return models.AuditLog{}
}
