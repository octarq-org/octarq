package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/internal/auth"
	"github.com/octarq-org/octarq/internal/authz"
	"github.com/octarq-org/octarq/internal/models"
	"gorm.io/gorm"
)

// An API bearer token acts as the person who minted it. It borrows their
// membership rather than carrying authority of its own, and may narrow itself
// below them but never above: the effective role is
// min(the holder's role in the org, the token's own).
//
// That replaces a parallel authorization path — bearer requests used to consult
// the token's role while everything else consulted the user's — with a single
// one, and it is what makes offboarding a person revoke their tokens too.
//
// GET /api/webhooks is the admin-gated route under test throughout. It used to
// be GET /api/tokens, which stopped being admin-gated when minting a personal
// token became something any member may do (listTokens now narrows a non-admin
// to their own rows instead of refusing them). The property being probed is the
// cap itself, so any admin gate serves — this one has no per-caller narrowing to
// confuse a 200 with.

func bearer(srv http.Handler, method, path, raw string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Authorization", "Bearer "+raw)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

// seedMember gives uid a membership row at the named role, which is what every
// role gate now reads for token requests too.
func seedMember(t *testing.T, db *gorm.DB, uid uint, role string) {
	t.Helper()
	var mem models.OrgMember
	if err := db.Where("org_id = ? AND user_id = ?", 1, uid).
		Assign(models.OrgMember{Role: role}).
		FirstOrCreate(&mem, models.OrgMember{OrgID: 1, UserID: uid, Role: role}).Error; err != nil {
		t.Fatalf("seed member: %v", err)
	}
}

// seedToken mints a token held by uid with its own role cap.
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

// A token that belongs to nobody has no membership to borrow, so it clears no
// gate. This is the fail-closed direction: the previous model read an empty
// role as "unrestricted", which meant an unattributable row was the most
// powerful credential in the system rather than the least.
func TestTokenWithNoHolderClearsNothing(t *testing.T) {
	srv, db := newTestHandler(t)
	const raw = "oct_ownerlesstoken0000000000000000001"
	seedToken(t, db, raw, 0, "owner")

	if rec := bearer(srv, http.MethodGet, "/api/webhooks", raw); rec.Code == http.StatusOK {
		t.Errorf("a token belonging to no user cleared an admin gate (body=%s)", rec.Body.String())
	}
}

// TestMemberTokenDeniedAdminRoute: the cap does its job even when the holder
// could have done more.
func TestMemberTokenDeniedAdminRoute(t *testing.T) {
	srv, db := newTestHandler(t)
	const raw = "oct_memberscopedtoken00000000000000001"
	seedMember(t, db, 7, "owner")
	seedToken(t, db, raw, 7, "member")

	if rec := bearer(srv, http.MethodGet, "/api/webhooks", raw); rec.Code != http.StatusForbidden {
		t.Errorf("member-capped token on admin route: got %d, want 403 (body=%s)", rec.Code, rec.Body.String())
	}

	// Positive half: the token is still a valid credential for ungated routes.
	// Without this a token that had simply stopped authenticating would pass the
	// assertion above for entirely the wrong reason.
	if rec := bearer(srv, http.MethodGet, "/api/links", raw); rec.Code != http.StatusOK {
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

// The other direction, and the reason the cap is a floor-and-ceiling rather
// than a substitute: a token stamped "owner" held by someone who is only a
// member is a member's token. Otherwise minting would be an escalation any
// member could perform on themselves.
func TestTokenCannotOutrankItsHolder(t *testing.T) {
	srv, db := newTestHandler(t)
	const raw = "oct_ownercaptoken00000000000000000001"
	seedMember(t, db, 9, "member")
	seedToken(t, db, raw, 9, "owner")

	if rec := bearer(srv, http.MethodGet, "/api/webhooks", raw); rec.Code != http.StatusForbidden {
		t.Errorf("owner-capped token held by a plain member: got %d, want 403 (body=%s)", rec.Code, rec.Body.String())
	}
}

// Offboarding. Deleting the membership is the whole revocation — the role is
// read on every request rather than frozen into the token at mint time, so
// there is no separate list of credentials to remember to clean up.
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

// TestTokenCannotMintAboveOwnRole closes the escalation loop: an admin-scoped
// token that could mint an owner-scoped one would make the cap decorative.
func TestTokenCannotMintAboveOwnRole(t *testing.T) {
	srv, db := newTestHandler(t)
	const raw = "oct_adminmintertoken000000000000000001"
	seedMember(t, db, 12, "owner") // the holder could mint owner; the token may not
	seedToken(t, db, raw, 12, "admin")

	req := httptest.NewRequest(http.MethodPost, "/api/tokens", strings.NewReader(`{"name":"escalate","role":"owner"}`))
	req.Header.Set("Authorization", "Bearer "+raw)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("admin token minting an owner token: got %d, want 403 (body=%s)", rec.Code, rec.Body.String())
	}

	// And it may still mint at or below its own level.
	req = httptest.NewRequest(http.MethodPost, "/api/tokens", strings.NewReader(`{"name":"ok","role":"member"}`))
	req.Header.Set("Authorization", "Bearer "+raw)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
		t.Errorf("admin token minting a member token: got %d, want 2xx (body=%s)", rec.Code, rec.Body.String())
	}
}

// A minted token records who it acts as. Without this the credential outlives
// any account it can be traced to, which is the state the whole change exists
// to end.
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

// TestMintedTokenDefaultsToMember pins the default. A caller who does not think
// about scope should get the narrow token, not a copy of their own account.
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

// TestMCPAuthStampsTokenIdentity covers the third bearer path. mcpAuth used to
// put only the org on the context, so anything an MCP client changed landed in
// the audit log with actor 0 AND no tokenId — attributable to nobody at all.
// It also means the cap applies over MCP like anywhere else.
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

// OrgRole is what plugins gate workspace administration on
// (plugin.Context.OrgRole). It is a separate surface from RequireRole, and it
// is the one with no compiler or test pressure behind it from this repo — the
// callers live in octarq-pro. Returning the holder's raw membership here would
// let every Pro gate ignore the token's cap while core's own gates honoured it,
// which is the kind of split that stays invisible until someone reports that a
// read-only CI token deleted something.
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

	// And a session for the same person still reports the full membership: the
	// cap belongs to the credential, not to the user.
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

// A token is a personal credential: it acts as whoever minted it and can never
// out-rank them (TestTokenCannotOutrankItsHolder). That is what makes minting
// safe to open up to any member — the old admin gate meant a member automating
// their own work had to be handed a credential that answered as somebody else,
// which is strictly worse for both audit and blast radius.
//
// The gate it replaces is ownership: a member sees, edits and revokes only the
// tokens they minted. Everything else is a 404 rather than a 403, so a probe
// cannot confirm that an id exists.
func TestMemberTokensAreTheirOwn(t *testing.T) {
	srv, db := newTestHandler(t)
	seedMember(t, db, 21, "member")
	seedMember(t, db, 22, "admin")
	member := sessionCookies(t, 21, 1)
	admin := sessionCookies(t, 22, 1)

	// Minting: allowed, and capped at the minter's own role.
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

	// Someone else's token, minted by the admin.
	theirs := models.Token{OrgID: 1, UserID: 22, Name: "admin-cli", Hash: models.HashToken("oct_someoneelsestoken000000000000001"), Prefix: "oct_some", Role: "admin"}
	if err := db.Create(&theirs).Error; err != nil {
		t.Fatalf("seed admin token: %v", err)
	}

	// Listing shows the member their own row and nothing else…
	rec := do(srv, http.MethodGet, "/api/tokens", member, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("member listing tokens: got %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, "my-cli") || strings.Contains(body, "admin-cli") {
		t.Errorf("member's token list leaked or lost rows: %s", body)
	}
	// …while an admin still sees the workspace's inventory.
	rec = do(srv, http.MethodGet, "/api/tokens", admin, "")
	if body := rec.Body.String(); !strings.Contains(body, "my-cli") || !strings.Contains(body, "admin-cli") {
		t.Errorf("admin's token list is not workspace-wide: %s", body)
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

	// Their own, on the other hand, is theirs to revoke.
	if rec := do(srv, http.MethodDelete, "/api/tokens/"+testIDStr(mine.ID), member, ""); rec.Code != http.StatusOK {
		t.Errorf("member revoking their own token: got %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
}
