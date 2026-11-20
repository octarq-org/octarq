package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/internal/auth"
	"github.com/octarq-org/octarq/server/internal/authz"
	"github.com/octarq-org/octarq/server/internal/models"
	"gorm.io/gorm"
)

func bearer(srv http.Handler, method, path, raw string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Authorization", "Bearer "+raw)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

func seedMember(t *testing.T, db *gorm.DB, uid uint, role string) {
	t.Helper()
	var mem models.OrgMember
	if err := db.Where("org_id = ? AND user_id = ?", 1, uid).
		Assign(models.OrgMember{Role: role}).
		FirstOrCreate(&mem, models.OrgMember{OrgID: 1, UserID: uid, Role: role}).Error; err != nil {
		t.Fatalf("seed member: %v", err)
	}
}

func seedToken(t *testing.T, db *gorm.DB, raw string, uid uint, cap string) {
	t.Helper()
	if err := db.Create(&models.Token{
		OrgID:  1,
		UserID: uid,
		Name:   "scope-test-" + cap,
		Hash:   models.HashToken(raw),
		Prefix: raw[:8],
		Role:   cap,
	}).Error; err != nil {
		t.Fatalf("seed token: %v", err)
	}
}

func TestTokenWithNoHolderClearsNothing(t *testing.T) {
	srv, db := newTestHandler(t)
	const raw = "oct_ownerlesstoken0000000000000000001"
	seedToken(t, db, raw, 0, "owner")

	if rec := bearer(srv, http.MethodGet, "/api/webhooks", raw); rec.Code == http.StatusOK {
		t.Errorf("a token belonging to no user cleared an admin gate (body=%s)", rec.Body.String())
	}
}

func TestMemberTokenDeniedAdminRoute(t *testing.T) {
	srv, db := newTestHandler(t)
	const raw = "oct_memberscopedtoken00000000000000001"
	seedMember(t, db, 7, "owner")
	seedToken(t, db, raw, 7, "member")

	if rec := bearer(srv, http.MethodGet, "/api/webhooks", raw); rec.Code != http.StatusForbidden {
		t.Errorf("member-capped token on admin route: got %d, want 403 (body=%s)", rec.Code, rec.Body.String())
	}

	if rec := bearer(srv, http.MethodGet, "/api/overview", raw); rec.Code != http.StatusOK {
		t.Errorf("member-capped token on ungated route: got %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestAdminTokenAllowedAdminRoute(t *testing.T) {
	srv, db := newTestHandler(t)
	const raw = "oct_adminscopedtoken000000000000000001"
	seedMember(t, db, 8, "admin")
	seedToken(t, db, raw, 8, "admin")

	if rec := bearer(srv, http.MethodGet, "/api/webhooks", raw); rec.Code != http.StatusOK {
		t.Errorf("admin token held by an admin: got %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestTokenCannotOutrankItsHolder(t *testing.T) {
	srv, db := newTestHandler(t)
	const raw = "oct_ownercaptoken00000000000000000001"
	seedMember(t, db, 9, "member")
	seedToken(t, db, raw, 9, "owner")

	if rec := bearer(srv, http.MethodGet, "/api/webhooks", raw); rec.Code != http.StatusForbidden {
		t.Errorf("owner-capped token held by a plain member: got %d, want 403 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestRemovingTheHolderRevokesTheirTokens(t *testing.T) {
	srv, db := newTestHandler(t)
	const raw = "oct_offboardingtoken00000000000000001"
	seedMember(t, db, 11, "admin")
	seedToken(t, db, raw, 11, "admin")

	if rec := bearer(srv, http.MethodGet, "/api/webhooks", raw); rec.Code != http.StatusOK {
		t.Fatalf("precondition: token should work while its holder is a member, got %d", rec.Code)
	}

	if err := db.Where("org_id = ? AND user_id = ?", 1, 11).Delete(&models.OrgMember{}).Error; err != nil {
		t.Fatalf("remove member: %v", err)
	}

	if rec := bearer(srv, http.MethodGet, "/api/webhooks", raw); rec.Code == http.StatusOK {
		t.Error("a removed member's token still cleared an admin gate")
	}
}

func TestTokenCannotMintAboveOwnRole(t *testing.T) {
	srv, db := newTestHandler(t)
	const raw = "oct_adminmintertoken000000000000000001"
	seedMember(t, db, 12, "owner")
	seedToken(t, db, raw, 12, "admin")

	req := httptest.NewRequest(http.MethodPost, "/api/tokens", strings.NewReader(`{"name":"escalate","role":"owner"}`))
	req.Header.Set("Authorization", "Bearer "+raw)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("admin token minting an owner token: got %d, want 403 (body=%s)", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/tokens", strings.NewReader(`{"name":"ok","role":"member"}`))
	req.Header.Set("Authorization", "Bearer "+raw)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
		t.Errorf("admin token minting a member token: got %d, want 2xx (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestMintedTokenRecordsItsHolder(t *testing.T) {
	srv, db := newTestHandler(t)
	owner := sessionCookies(t, 1, 1)

	rec := do(srv, http.MethodPost, "/api/tokens", owner, `{"name":"attributed"}`)
	if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
		t.Fatalf("mint: got %d (body=%s)", rec.Code, rec.Body.String())
	}
	var tok models.Token
	if err := db.Where("name = ?", "attributed").First(&tok).Error; err != nil {
		t.Fatalf("minted token not found: %v", err)
	}
	if tok.UserID != 1 {
		t.Errorf("minted token UserID = %d, want 1 (the caller)", tok.UserID)
	}
}

func TestMintedTokenDefaultsToMember(t *testing.T) {
	srv, _ := newTestHandler(t)
	owner := sessionCookies(t, 1, 1)

	rec := do(srv, http.MethodPost, "/api/tokens", owner, `{"name":"defaulted"}`)
	if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
		t.Fatalf("mint: got %d (body=%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"role":"member"`) {
		t.Errorf("minted token did not default to member: body=%s", rec.Body.String())
	}
}

func TestMintTokenRejectsUnknownRole(t *testing.T) {
	srv, _ := newTestHandler(t)
	owner := sessionCookies(t, 1, 1)

	for _, role := range []string{"superuser", "MEMBER", "root"} {
		rec := do(srv, http.MethodPost, "/api/tokens", owner, `{"name":"bad","role":"`+role+`"}`)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("mint with role %q: got %d, want 400 (body=%s)", role, rec.Code, rec.Body.String())
		}
	}
}

func TestMCPAuthStampsTokenIdentity(t *testing.T) {
	h, _, db := newTestHandlerRaw(t)
	const raw = "oct_mcpidentitytoken00000000000000001"
	seedMember(t, db, 13, "owner")
	if err := db.Create(&models.Token{
		OrgID: 1, UserID: 13, Name: "mcp", Hash: models.HashToken(raw), Prefix: raw[:8], Role: "member",
	}).Error; err != nil {
		t.Fatalf("seed token: %v", err)
	}

	var gotID uint
	var gotRole string
	var gotHoldsAdmin bool
	probe := h.mcpAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotID = auth.TokenIDFromContext(r.Context())
		gotRole = auth.TokenRoleFromContext(r.Context())
		gotHoldsAdmin = h.callerHoldsRole(r, authz.RoleAdmin)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/mcp?token="+raw, nil)
	rec := httptest.NewRecorder()
	probe.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("mcpAuth rejected a valid token: got %d", rec.Code)
	}
	if gotID == 0 {
		t.Error("mcpAuth left no token id on the context — audit entries would name nobody")
	}
	if gotRole != "member" {
		t.Errorf("token role on context = %q, want \"member\"", gotRole)
	}
	if gotHoldsAdmin {
		t.Error("member-capped token cleared an admin gate over MCP")
	}
}

func TestOrgRoleReportsTheCappedRoleNotTheHolders(t *testing.T) {
	h, _, db := newTestHandlerRaw(t)
	const raw = "oct_orgrolecaptoken000000000000000001"
	seedMember(t, db, 31, "owner")
	seedToken(t, db, raw, 31, "member")

	req := httptest.NewRequest(http.MethodGet, "/api/anything", nil)
	req.Header.Set("Authorization", "Bearer "+raw)
	req, ok := h.auth.AuthenticateRequest(req)
	if !ok {
		t.Fatal("token did not authenticate")
	}

	if got := h.OrgRole(req); got != "member" {
		t.Errorf("OrgRole = %q for a member-capped token held by an owner, want \"member\" — every plugin gate reads this", got)
	}

	sess := httptest.NewRequest(http.MethodGet, "/api/anything", nil)
	for _, c := range sessionCookies(t, 31, 1) {
		sess.AddCookie(c)
	}
	sess, ok = h.auth.AuthenticateRequest(sess)
	if !ok {
		t.Fatal("session did not authenticate")
	}
	if got := h.OrgRole(sess); got != "owner" {
		t.Errorf("OrgRole = %q for the holder's own session, want \"owner\"", got)
	}
}

func TestMemberTokensAreTheirOwn(t *testing.T) {
	srv, db := newTestHandler(t)
	seedMember(t, db, 21, "member")
	seedMember(t, db, 22, "admin")
	member := sessionCookies(t, 21, 1)
	admin := sessionCookies(t, 22, 1)

	if rec := do(srv, http.MethodPost, "/api/tokens", member, `{"name":"my-cli"}`); rec.Code != http.StatusCreated {
		t.Fatalf("member minting their own token: got %d, want 201 (body=%s)", rec.Code, rec.Body.String())
	}
	if rec := do(srv, http.MethodPost, "/api/tokens", member, `{"name":"escalate","role":"admin"}`); rec.Code != http.StatusForbidden {
		t.Errorf("member minting an admin token: got %d, want 403", rec.Code)
	}

	var mine models.Token
	if err := db.Where("user_id = ?", 21).First(&mine).Error; err != nil {
		t.Fatalf("minted token not found: %v", err)
	}
	if mine.Role != string(authz.RoleMember) {
		t.Errorf("minted token role = %q, want member", mine.Role)
	}

	theirs := models.Token{OrgID: 1, UserID: 22, Name: "admin-cli", Hash: models.HashToken("oct_someoneelsestoken000000000000001"), Prefix: "oct_some", Role: "admin"}
	if err := db.Create(&theirs).Error; err != nil {
		t.Fatalf("seed admin token: %v", err)
	}

	rec := do(srv, http.MethodGet, "/api/tokens", member, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("member listing tokens: got %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, "my-cli") || strings.Contains(body, "admin-cli") {
		t.Errorf("member's token list leaked or lost rows: %s", body)
	}
	rec = do(srv, http.MethodGet, "/api/tokens", admin, "")
	if body := rec.Body.String(); !strings.Contains(body, "admin-cli") || strings.Contains(body, "my-cli") {
		t.Errorf("admin's token list shows someone else's personal credentials: %s", body)
	}

	mineID := testIDStr(mine.ID)
	if rec := do(srv, http.MethodPut, "/api/tokens/"+mineID, admin, `{"name":"seized"}`); rec.Code != http.StatusNotFound {
		t.Errorf("admin editing a member's token: got %d, want 404", rec.Code)
	}
	if rec := do(srv, http.MethodDelete, "/api/tokens/"+mineID, admin, ""); rec.Code != http.StatusNotFound {
		t.Errorf("admin revoking a member's token: got %d, want 404", rec.Code)
	}

	theirID := testIDStr(theirs.ID)
	if rec := do(srv, http.MethodPut, "/api/tokens/"+theirID, member, `{"name":"stolen"}`); rec.Code != http.StatusNotFound {
		t.Errorf("member editing another user's token: got %d, want 404", rec.Code)
	}
	if rec := do(srv, http.MethodDelete, "/api/tokens/"+theirID, member, ""); rec.Code != http.StatusNotFound {
		t.Errorf("member revoking another user's token: got %d, want 404", rec.Code)
	}
	var still models.Token
	if err := db.First(&still, theirs.ID).Error; err != nil {
		t.Fatalf("a member revoked someone else's token: %v", err)
	}
	if still.Name != "admin-cli" {
		t.Errorf("a member renamed someone else's token to %q", still.Name)
	}

	if rec := do(srv, http.MethodDelete, "/api/tokens/"+testIDStr(mine.ID), member, ""); rec.Code != http.StatusOK {
		t.Errorf("member revoking their own token: got %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestMintAgentToken_ValidationAndSuccess(t *testing.T) {
	srv, db := newTestHandler(t)
	owner := sessionCookies(t, 1, 1)

	recEmpty := do(srv, http.MethodPost, "/api/tokens", owner, `{"name":"agent-dead","is_agent":true,"allowed_tools":[]}`)
	if recEmpty.Code != http.StatusBadRequest {
		t.Errorf("mint agent token with empty tools: got %d, want 400 (body=%s)", recEmpty.Code, recEmpty.Body.String())
	}

	recInvalid := do(srv, http.MethodPost, "/api/tokens", owner, `{"name":"agent-bad","is_agent":true,"allowed_tools":["invalid tool with spaces!"]}`)
	if recInvalid.Code != http.StatusBadRequest {
		t.Errorf("mint agent token with invalid tool name: got %d, want 400 (body=%s)", recInvalid.Code, recInvalid.Body.String())
	}

	recOK := do(srv, http.MethodPost, "/api/tokens", owner, `{"name":"agent-ok","is_agent":true,"allowed_tools":["octarq_network__list_links","octarq_network__get_email_summary"]}`)
	if recOK.Code != http.StatusOK && recOK.Code != http.StatusCreated {
		t.Fatalf("mint valid agent token: got %d (body=%s)", recOK.Code, recOK.Body.String())
	}

	var tok models.Token
	if err := db.Where("name = ?", "agent-ok").First(&tok).Error; err != nil {
		t.Fatalf("agent token not found in database: %v", err)
	}
	if !tok.IsAgent {
		t.Error("expected tok.IsAgent to be true in database")
	}
	if !tok.AllowsTool("octarq_network__list_links") {
		t.Error("expected tok.AllowsTool(octarq_network__list_links) to be true")
	}
	if tok.AllowsTool("octarq_network__delete_link") {
		t.Error("expected tok.AllowsTool(octarq_network__delete_link) to be false")
	}

	tokID := testIDStr(tok.ID)
	recUpdate := do(srv, http.MethodPut, "/api/tokens/"+tokID, owner, `{"allowed_tools":["octarq_network__list_links"]}`)
	if recUpdate.Code != http.StatusOK {
		t.Fatalf("update agent token tools: got %d (body=%s)", recUpdate.Code, recUpdate.Body.String())
	}

	if err := db.First(&tok, tok.ID).Error; err != nil {
		t.Fatalf("reload agent token: %v", err)
	}
	if !strings.Contains(tok.AllowedTools, "octarq_network__list_links") || strings.Contains(tok.AllowedTools, "octarq_network__get_email_summary") {
		t.Errorf("unexpected updated allowed tools in db: %s", tok.AllowedTools)
	}
}
