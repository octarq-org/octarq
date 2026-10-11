package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/octarq-org/octarq/server/internal/models"
)

func TestListOrgsAndUserSettings(t *testing.T) {
	srv, db := newTestHandler(t)
	cookies := loginCookies(t, srv)

	// 1. List Orgs unauthenticated -> 401
	rec := do(srv, "GET", "/api/orgs", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated list orgs: got %d, want 401", rec.Code)
	}

	// 2. List Orgs authenticated -> returns user's orgs
	rec = do(srv, "GET", "/api/orgs", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("authenticated list orgs: got %d (%s)", rec.Code, rec.Body.String())
	}
	var orgs []OrgItem
	if err := json.Unmarshal(rec.Body.Bytes(), &orgs); err != nil {
		t.Fatalf("unmarshal orgs: %v", err)
	}
	if len(orgs) == 0 {
		t.Fatalf("expected at least 1 org for admin")
	}
	if orgs[0].Role != "owner" {
		t.Errorf("expected role 'owner', got %q", orgs[0].Role)
	}

	// 3. User Settings unauthenticated -> 401
	rec = do(srv, "GET", "/api/user/settings", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated get user settings: got %d, want 401", rec.Code)
	}

	// 4. User Settings initial -> empty
	rec = do(srv, "GET", "/api/user/settings", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get user settings: got %d (%s)", rec.Code, rec.Body.String())
	}
	var settings map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &settings); err != nil {
		t.Fatalf("unmarshal settings: %v", err)
	}
	if len(settings) != 0 {
		t.Errorf("expected empty settings, got %v", settings)
	}

	// 5. Update user setting without key -> 400
	rec = do(srv, "PUT", "/api/user/settings", cookies, `{"key":"","value":"true"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("update setting empty key: got %d, want 400", rec.Code)
	}

	// 6. Update user setting valid -> 200
	rec = do(srv, "PUT", "/api/user/settings", cookies, `{"key":"theme","value":"dark"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update setting: got %d (%s)", rec.Code, rec.Body.String())
	}

	// 7. Verify setting persisted
	rec = do(srv, "GET", "/api/user/settings", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get user settings after update: got %d (%s)", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &settings); err != nil {
		t.Fatalf("unmarshal settings: %v", err)
	}
	if settings["theme"] != "dark" {
		t.Errorf("setting theme: got %q, want dark", settings["theme"])
	}

	// 8. Update same key with new value
	rec = do(srv, "PUT", "/api/user/settings", cookies, `{"key":"theme","value":"light"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update setting second time: got %d", rec.Code)
	}
	rec = do(srv, "GET", "/api/user/settings", cookies, "")
	_ = json.Unmarshal(rec.Body.Bytes(), &settings)
	if settings["theme"] != "light" {
		t.Errorf("setting theme after overwrite: got %q, want light", settings["theme"])
	}
	_ = db
}

func TestOrgLifecycleAndMemberManagement(t *testing.T) {
	srv, db := newTestHandler(t)
	ownerCookies := loginCookies(t, srv)

	// 1. Create org empty name -> 400
	rec := do(srv, "POST", "/api/orgs", ownerCookies, `{"name":""}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("create org empty name: got %d, want 400", rec.Code)
	}

	// 2. Create org valid -> 201
	rec = do(srv, "POST", "/api/orgs", ownerCookies, `{"name":"Acme Corp"}`)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("create org: got %d (%s)", rec.Code, rec.Body.String())
	}
	var newOrg models.Org
	if err := json.Unmarshal(rec.Body.Bytes(), &newOrg); err != nil {
		t.Fatalf("unmarshal created org: %v", err)
	}
	if newOrg.Name != "Acme Corp" || newOrg.Slug == "" {
		t.Errorf("unexpected created org: %+v", newOrg)
	}

	// Switch to newOrg
	rec = do(srv, "POST", "/api/auth/switch-org", ownerCookies, fmt.Sprintf(`{"orgId":%d}`, newOrg.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("switch org: got %d (%s)", rec.Code, rec.Body.String())
	}
	acmeCookies := rec.Result().Cookies()

	// 3. Update Org Name
	rec = do(srv, "PUT", "/api/org", acmeCookies, `{"name":""}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("update org empty name: got %d, want 400", rec.Code)
	}

	rec = do(srv, "PUT", "/api/org", acmeCookies, `{"name":"Acme Global"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update org name: got %d (%s)", rec.Code, rec.Body.String())
	}
	var updatedOrg models.Org
	if err := json.Unmarshal(rec.Body.Bytes(), &updatedOrg); err != nil {
		t.Fatalf("unmarshal updated org: %v", err)
	}
	if updatedOrg.Name != "Acme Global" {
		t.Errorf("got name %q, want Acme Global", updatedOrg.Name)
	}

	// 4. Create secondary user and add as member
	user2 := models.User{Email: "alice@example.com"}
	user3 := models.User{Email: "bob@example.com"}
	if err := db.Create(&user2).Error; err != nil {
		t.Fatalf("create user2: %v", err)
	}
	if err := db.Create(&user3).Error; err != nil {
		t.Fatalf("create user3: %v", err)
	}

	// Empty email -> 400
	rec = do(srv, "POST", "/api/org/members", acmeCookies, `{"email":"","role":"member"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty email add member: got %d, want 400", rec.Code)
	}

	// Add brand-new user (not in users table) -> triggers invite generation & sendInviteEmail
	rec = do(srv, "POST", "/api/org/members", acmeCookies, `{"email":"charlie_new@example.com","role":"member"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("add brand new user: got %d (%s)", rec.Code, rec.Body.String())
	}

	// Add user2 as member
	rec = do(srv, "POST", "/api/org/members", acmeCookies, `{"email":"alice@example.com","role":"member"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("add user2 as member: got %d (%s)", rec.Code, rec.Body.String())
	}

	aliceCookies := sessionCookies(t, user2.ID, newOrg.ID)

	// Alice (member) trying to rename org -> 403
	rec = do(srv, "PUT", "/api/org", aliceCookies, `{"name":"Hacked Org"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("member renaming org: got %d, want 403", rec.Code)
	}

	// Alice (member) trying to manage members -> 403
	rec = do(srv, "PATCH", fmt.Sprintf("/api/org/members/%d", user2.ID), aliceCookies, `{"role":"admin"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("member updating member: got %d, want 403", rec.Code)
	}

	// Re-invite user2 as admin -> updates existing membership
	rec = do(srv, "POST", "/api/org/members", acmeCookies, `{"email":"alice@example.com","role":"admin"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("re-invite user2 as admin: got %d (%s)", rec.Code, rec.Body.String())
	}

	// Add user3 as admin
	rec = do(srv, "POST", "/api/org/members", acmeCookies, `{"email":"bob@example.com","role":"admin"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("add user3 as admin: got %d (%s)", rec.Code, rec.Body.String())
	}

	bobCookies := sessionCookies(t, user3.ID, newOrg.ID)

	// 5. Member Role Update (PATCH /api/org/members/{userId})
	// Invalid role -> 400
	rec = do(srv, "PATCH", fmt.Sprintf("/api/org/members/%d", user2.ID), acmeCookies, `{"role":"superhero"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("update member invalid role: got %d, want 400", rec.Code)
	}

	// Target not in org -> 404
	rec = do(srv, "PATCH", "/api/org/members/999999", acmeCookies, `{"role":"admin"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("update nonexistent member: got %d, want 404", rec.Code)
	}

	// Same role -> 200 no-op
	rec = do(srv, "PATCH", fmt.Sprintf("/api/org/members/%d", user2.ID), acmeCookies, `{"role":"member"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update member same role: got %d", rec.Code)
	}

	// Bob (admin) trying to grant owner role -> 403
	rec = do(srv, "PATCH", fmt.Sprintf("/api/org/members/%d", user2.ID), bobCookies, `{"role":"owner"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("admin granting owner role: got %d, want 403", rec.Code)
	}

	// Bob (admin) trying to change an owner's role -> 403
	var ownerUser models.User
	db.Where("email = ?", "admin").First(&ownerUser)
	rec = do(srv, "PATCH", fmt.Sprintf("/api/org/members/%d", ownerUser.ID), bobCookies, `{"role":"member"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("admin demoting owner: got %d, want 403", rec.Code)
	}

	// Owner trying to demote self (only owner) -> 400/Conflict
	rec = do(srv, "PATCH", fmt.Sprintf("/api/org/members/%d", ownerUser.ID), acmeCookies, `{"role":"admin"}`)
	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusConflict {
		t.Fatalf("demote only owner: got %d, want 400 or 409", rec.Code)
	}

	// Owner promotes Bob to co-owner -> 200
	rec = do(srv, "PATCH", fmt.Sprintf("/api/org/members/%d", user3.ID), acmeCookies, `{"role":"owner"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("promote bob to owner: got %d (%s)", rec.Code, rec.Body.String())
	}

	// Co-owner Bob demotes ownerUser to admin -> 200 (since Bob is now an owner)
	bobAsOwnerCookies := sessionCookies(t, user3.ID, newOrg.ID)
	rec = do(srv, "PATCH", fmt.Sprintf("/api/org/members/%d", ownerUser.ID), bobAsOwnerCookies, `{"role":"admin"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("bob demotes ownerUser: got %d (%s)", rec.Code, rec.Body.String())
	}

	// 6. Member Removal (DELETE /api/org/members/{userId})
	// Caller removing themselves -> 400
	rec = do(srv, "DELETE", fmt.Sprintf("/api/org/members/%d", user3.ID), bobAsOwnerCookies, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("removing self: got %d, want 400", rec.Code)
	}

	// Nonexistent member -> 404
	rec = do(srv, "DELETE", "/api/org/members/999999", bobAsOwnerCookies, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("removing nonexistent member: got %d, want 404", rec.Code)
	}

	// Removing user2 (Alice) -> 200
	rec = do(srv, "DELETE", fmt.Sprintf("/api/org/members/%d", user2.ID), bobAsOwnerCookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("remove alice: got %d (%s)", rec.Code, rec.Body.String())
	}

	// Verify alice is no longer a member
	var memCount int64
	db.Model(&models.OrgMember{}).Where("org_id = ? AND user_id = ?", newOrg.ID, user2.ID).Count(&memCount)
	if memCount != 0 {
		t.Errorf("alice still in org_members")
	}

	// Bob is the only owner now. Try to remove Bob (last owner) by admin ->
	// Admin user removing Bob (owner) -> 403 (only owner can remove owner)
	ownerAsAdminCookies := sessionCookies(t, ownerUser.ID, newOrg.ID)
	rec = do(srv, "DELETE", fmt.Sprintf("/api/org/members/%d", user3.ID), ownerAsAdminCookies, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("admin removing owner: got %d, want 403", rec.Code)
	}

	// Bob promotes ownerUser back to owner -> now 2 owners
	rec = do(srv, "PATCH", fmt.Sprintf("/api/org/members/%d", ownerUser.ID), bobAsOwnerCookies, `{"role":"owner"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("bob re-promotes ownerUser to owner: got %d", rec.Code)
	}

	// Owner Bob removing ownerUser when 2 owners exist -> 200 OK
	rec = do(srv, "DELETE", fmt.Sprintf("/api/org/members/%d", ownerUser.ID), bobAsOwnerCookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("bob removes co-owner ownerUser: got %d (%s)", rec.Code, rec.Body.String())
	}

	// Bob is now the sole owner. Bob trying to remove self -> 400
	rec = do(srv, "DELETE", fmt.Sprintf("/api/org/members/%d", user3.ID), bobAsOwnerCookies, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("sole owner removing self: got %d, want 400", rec.Code)
	}

	// List org members auth
	rec = do(srv, "GET", "/api/org/members", bobAsOwnerCookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list org members: got %d (%s)", rec.Code, rec.Body.String())
	}

	// Add new invited user (non-existent email)
	rec = do(srv, "POST", "/api/org/members", bobAsOwnerCookies, `{"email":"brandnew@example.com","role":"member"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("invite new user: got %d (%s)", rec.Code, rec.Body.String())
	}

	// Add member empty email -> 400
	rec = do(srv, "POST", "/api/org/members", bobAsOwnerCookies, `{"email":"","role":"member"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invite empty email: got %d, want 400", rec.Code)
	}
}
