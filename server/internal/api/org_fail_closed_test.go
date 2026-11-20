package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/internal/db"
	"github.com/octarq-org/octarq/server/internal/models"
)

func TestOrgIDNoDefaultAndFailClosed(t *testing.T) {
	h, srv, db := newTestHandlerRaw(t)

	db.Create(&models.Token{
		OrgID:  1,
		Name:   "Org 1 Token",
		Hash:   "dummyhash1",
		Prefix: "oct_1111",
	})

	req := httptest.NewRequest("GET", "/api/audit", nil)

	if got := h.orgID(req); got != 0 {
		t.Fatalf("orgID(req) = %d, want 0 (silent fallback to 1 must be removed)", got)
	}

	var toks []models.Token
	if err := h.orgDB(req).Find(&toks).Error; err != nil {
		t.Fatalf("orgDB query error: %v", err)
	}
	if len(toks) != 0 {
		t.Fatalf("orgDB for org 0 returned %d rows, want 0 (failed to fail-closed)", len(toks))
	}

	orgID, err := h.requireOrg(req)
	if orgID != 0 || err == nil {
		t.Fatalf("requireOrg(req) = (%d, %v), want (0, error)", orgID, err)
		return
	}

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET /api/audit without auth = %d, want 401", rec.Code)
	}
}

func TestGetSettingsInboundTokenRoleControl(t *testing.T) {
	_, srv, db := newTestHandlerRaw(t)
	const orgID = uint(201)
	const secret = "inbound-tok-201"

	db.Create(&models.Org{ID: orgID, Slug: "test-org-201", InboundToken: secret})

	ownerUID := seedOrgMember(t, db, orgID, "owner@p210.com", "owner")
	adminUID := seedOrgMember(t, db, orgID, "admin@p210.com", "admin")
	memberUID := seedOrgMember(t, db, orgID, "member@p210.com", "member")

	ownerSess := sessionCookies(t, ownerUID, orgID)
	adminSess := sessionCookies(t, adminUID, orgID)
	memberSess := sessionCookies(t, memberUID, orgID)

	settingsBody := func(cookies []*http.Cookie) map[string]any {
		t.Helper()
		rec := do(srv, "GET", "/api/settings", cookies, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("getSettings status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal settings body: %v", err)
		}
		return body
	}
	assertMasked := func(label string, body map[string]any) {
		t.Helper()
		if _, exists := body["inboundToken"]; exists {
			t.Errorf("%s received inboundToken key in settings response: %v", label, body["inboundToken"])
		}
		if b, err := json.Marshal(body); err == nil && strings.Contains(string(b), secret) {
			t.Errorf("%s settings dump leaked the raw inbound token: %v", label, body)
		}
	}
	assertSet := func(label string, body map[string]any) {
		t.Helper()
		if set, ok := body["inboundTokenSet"].(bool); !ok || !set {
			t.Errorf("%s did not receive inboundTokenSet=true: %v", label, body["inboundTokenSet"])
		}
	}
	tokenStatus := func(cookies []*http.Cookie) int {
		t.Helper()
		return do(srv, "GET", "/api/settings/inbound-token", cookies, "").Code
	}
	tokenBody := func(cookies []*http.Cookie) (int, string) {
		t.Helper()
		rec := do(srv, "GET", "/api/settings/inbound-token", cookies, "")
		return rec.Code, rec.Body.String()
	}

	memberBody := settingsBody(memberSess)
	assertMasked("member", memberBody)
	if _, exists := memberBody["inboundTokenSet"]; exists {
		t.Errorf("member received inboundTokenSet key in settings response: %v", memberBody["inboundTokenSet"])
	}
	if code := tokenStatus(memberSess); code != http.StatusForbidden {
		t.Errorf("member GET /api/settings/inbound-token = %d, want 403", code)
	}

	ownerBody := settingsBody(ownerSess)
	assertMasked("owner", ownerBody)
	assertSet("owner", ownerBody)
	if code, body := tokenBody(ownerSess); code != http.StatusOK || !strings.Contains(body, secret) {
		t.Errorf("owner GET /api/settings/inbound-token = %d, want 200 with token (body=%s)", code, body)
	}

	adminBody := settingsBody(adminSess)
	assertMasked("admin", adminBody)
	assertSet("admin", adminBody)
	if code, body := tokenBody(adminSess); code != http.StatusOK || !strings.Contains(body, secret) {
		t.Errorf("admin GET /api/settings/inbound-token = %d, want 200 with token (body=%s)", code, body)
	}
}

func TestAuditLogIncludesTokenIDForBearerToken(t *testing.T) {
	_, srv, db := newTestHandlerRaw(t)
	const orgID = uint(301)

	rawToken := "oct_test_bearer_token_12345"
	const holderUID = uint(301)
	if err := db.Create(&models.OrgMember{OrgID: orgID, UserID: holderUID, Role: "admin"}).Error; err != nil {
		t.Fatalf("seed member: %v", err)
	}
	tok := models.Token{
		OrgID:  orgID,
		UserID: holderUID,
		Role:   "admin",
		Name:   "Audit Test Token",
		Hash:   models.HashToken(rawToken),
		Prefix: "oct_test",
	}
	if err := db.Create(&tok).Error; err != nil {
		t.Fatalf("create token: %v", err)
	}

	req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/tokens/%d", tok.ID), nil)
	req.Header.Set("Authorization", "Bearer "+rawToken)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("bearer token request status = %d, want 200", rec.Code)
	}

	time.Sleep(50 * time.Millisecond)

	var logRow models.AuditLog
	if err := db.Where("org_id = ? AND action = ?", orgID, "token.delete").First(&logRow).Error; err != nil {
		t.Fatalf("audit log row not found: %v", err)
	}

	if logRow.ActorID != holderUID {
		t.Errorf("logRow.ActorID = %d, want %d — the token's holder", logRow.ActorID, holderUID)
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
