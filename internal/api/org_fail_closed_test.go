package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/octarq-org/octarq/internal/db"
	"github.com/octarq-org/octarq/internal/models"
)

// P2-9: orgID must return 0 when unauthenticated/no org in request,
// orgDB must fail closed (where 1 = 0) when org is 0,
// and requireOrg must return a 401 error.
func TestOrgIDNoDefaultAndFailClosed(t *testing.T) {
	h, srv, db := newTestHandlerRaw(t)

	// Seed a row under org 1
	db.Create(&models.Token{
		OrgID:  1,
		Name:   "Org 1 Token",
		Hash:   "dummyhash1",
		Prefix: "oct_1111",
	})

	req := httptest.NewRequest("GET", "/api/audit", nil)

	// 1. orgID(req) must be 0 when no org in context
	if got := h.orgID(req); got != 0 {
		t.Fatalf("orgID(req) = %d, want 0 (silent fallback to 1 must be removed)", got)
	}

	// 2. orgDB(req) must fail closed (return 0 rows, not org 1 rows)
	var toks []models.Token
	if err := h.orgDB(req).Find(&toks).Error; err != nil {
		t.Fatalf("orgDB query error: %v", err)
	}
	if len(toks) != 0 {
		t.Fatalf("orgDB for org 0 returned %d rows, want 0 (failed to fail-closed)", len(toks))
	}

	// 3. requireOrg(req) must return error for org 0
	orgID, err := h.requireOrg(req)
	if orgID != 0 || err == nil {
		t.Fatalf("requireOrg(req) = (%d, %v), want (0, error)", orgID, err)
	}

	// 4. Requesting a requireOrg endpoint without session returns 401
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET /api/audit without auth = %d, want 401", rec.Code)
	}
}

// P2-10: Ordinary member must not receive inboundToken in getSettings response;
// owner/admin must receive it.
func TestGetSettingsInboundTokenRoleControl(t *testing.T) {
	_, srv, db := newTestHandlerRaw(t)
	const orgID = uint(201)

	db.Create(&models.Org{ID: orgID, Slug: "test-org-201", InboundToken: "inbound-tok-201"})

	ownerUID := seedOrgMember(t, db, orgID, "owner@p210.com", "owner")
	adminUID := seedOrgMember(t, db, orgID, "admin@p210.com", "admin")
	memberUID := seedOrgMember(t, db, orgID, "member@p210.com", "member")

	ownerSess := sessionCookies(t, ownerUID, orgID)
	adminSess := sessionCookies(t, adminUID, orgID)
	memberSess := sessionCookies(t, memberUID, orgID)

	// 1. Member GET /api/settings -> inboundToken key must NOT exist in JSON
	recMember := do(srv, "GET", "/api/settings", memberSess, "")
	if recMember.Code != http.StatusOK {
		t.Fatalf("member getSettings status = %d, want 200", recMember.Code)
	}
	var memberBody map[string]any
	if err := json.Unmarshal(recMember.Body.Bytes(), &memberBody); err != nil {
		t.Fatalf("unmarshal member body: %v", err)
	}
	if _, exists := memberBody["inboundToken"]; exists {
		t.Errorf("member received inboundToken key in settings response: %v", memberBody["inboundToken"])
	}

	// 2. Owner GET /api/settings -> inboundToken key MUST exist in JSON
	recOwner := do(srv, "GET", "/api/settings", ownerSess, "")
	if recOwner.Code != http.StatusOK {
		t.Fatalf("owner getSettings status = %d, want 200", recOwner.Code)
	}
	var ownerBody map[string]any
	if err := json.Unmarshal(recOwner.Body.Bytes(), &ownerBody); err != nil {
		t.Fatalf("unmarshal owner body: %v", err)
	}
	if tok, ok := ownerBody["inboundToken"].(string); !ok || tok == "" {
		t.Errorf("owner did not receive valid inboundToken string: %v", ownerBody["inboundToken"])
	}

	// 3. Admin GET /api/settings -> inboundToken key MUST exist in JSON
	recAdmin := do(srv, "GET", "/api/settings", adminSess, "")
	if recAdmin.Code != http.StatusOK {
		t.Fatalf("admin getSettings status = %d, want 200", recAdmin.Code)
	}
	var adminBody map[string]any
	if err := json.Unmarshal(recAdmin.Body.Bytes(), &adminBody); err != nil {
		t.Fatalf("unmarshal admin body: %v", err)
	}
	if tok, ok := adminBody["inboundToken"].(string); !ok || tok == "" {
		t.Errorf("admin did not receive valid inboundToken string: %v", adminBody["inboundToken"])
	}
}

// P2-18: Bearer token authenticated requests must record tokenId in audit meta.
func TestAuditLogIncludesTokenIDForBearerToken(t *testing.T) {
	_, srv, db := newTestHandlerRaw(t)
	const orgID = uint(301)

	rawToken := "oct_test_bearer_token_12345"
	tok := models.Token{
		OrgID:  orgID,
		Name:   "Audit Test Token",
		Hash:   models.HashToken(rawToken),
		Prefix: "oct_test",
	}
	if err := db.Create(&tok).Error; err != nil {
		t.Fatalf("create token: %v", err)
	}

	// 1. Perform request using Bearer Token
	req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/tokens/%d", tok.ID), nil)
	req.Header.Set("Authorization", "Bearer "+rawToken)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("bearer token request status = %d, want 200", rec.Code)
	}

	// Allow async audit log goroutine to write
	time.Sleep(50 * time.Millisecond)

	var logRow models.AuditLog
	if err := db.Where("org_id = ? AND action = ?", orgID, "token.delete").First(&logRow).Error; err != nil {
		t.Fatalf("audit log row not found: %v", err)
	}

	if logRow.ActorID != 0 {
		t.Errorf("logRow.ActorID = %d, want 0 for bearer token", logRow.ActorID)
	}

	var meta map[string]any
	if err := json.Unmarshal([]byte(logRow.Meta), &meta); err != nil {
		t.Fatalf("unmarshal audit meta: %v", err)
	}

	tokIDVal, ok := meta["tokenId"].(float64)
	if !ok || uint(tokIDVal) != tok.ID {
		t.Errorf("audit meta tokenId = %v, want %d", meta["tokenId"], tok.ID)
	}
}

// A session predating multi-tenancy carries org_id 0. Handler.orgID used to
// substitute org 1 for it, which is exactly the fallback P2-9 removes — so
// after the removal such a session would authenticate but resolve to no
// workspace, and every tenant-scoped request under it would 401 on a screen
// that looks logged in. Migrate deletes those rows instead, sending the user
// through a normal re-login. Pin it, or an upgrade silently produces accounts
// that appear signed in and can do nothing.
func TestMigrateDropsOrglessSessions(t *testing.T) {
	_, _, gdb := newTestHandlerRaw(t)

	keep := models.Session{UserID: 1, OrgID: 1, Token: "keep", ExpiresAt: time.Now().Add(time.Hour)}
	drop := models.Session{UserID: 2, OrgID: 0, Token: "drop", ExpiresAt: time.Now().Add(time.Hour)}
	if err := gdb.Create(&keep).Error; err != nil {
		t.Fatalf("seed org-scoped session: %v", err)
	}
	if err := gdb.Create(&drop).Error; err != nil {
		t.Fatalf("seed orgless session: %v", err)
	}

	if err := db.Migrate(gdb); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	var tokens []string
	gdb.Model(&models.Session{}).Pluck("token", &tokens)
	if len(tokens) != 1 || tokens[0] != "keep" {
		t.Fatalf("Migrate should drop only the orgless session, sessions left = %v", tokens)
	}
}
