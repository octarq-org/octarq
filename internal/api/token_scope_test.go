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

// P2-18: an API bearer token used to be waved through every role gate, so any
// token was full read/write for its org and could, among other things, mint more
// tokens. It now carries a role ceiling that every gate compares against.
//
// GET /api/tokens is the route under test throughout: it is admin-gated, and it
// is the escalation path that matters — a token that can list and mint tokens
// can promote itself past any ceiling.

func bearer(srv http.Handler, method, path, raw string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Authorization", "Bearer "+raw)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

func seedToken(t *testing.T, db *gorm.DB, raw, role string) {
	t.Helper()
	if err := db.Create(&models.Token{
		OrgID:  1,
		Name:   "scope-test-" + role,
		Hash:   models.HashToken(raw),
		Prefix: raw[:8],
		Role:   role,
	}).Error; err != nil {
		t.Fatalf("seed token: %v", err)
	}
}

// TestLegacyTokenKeepsFullAccess is the back-compat guarantee. A token minted
// before scoping existed has role "" in the DB, and narrowing it retroactively
// would break someone's CI at a moment nobody is watching. It must still pass an
// admin gate.
func TestLegacyTokenKeepsFullAccess(t *testing.T) {
	srv, db := newTestHandler(t)
	const raw = "oct_legacyunscopedtoken00000000000001"
	seedToken(t, db, raw, "") // role "" == minted before P2-18

	if rec := bearer(srv, http.MethodGet, "/api/tokens", raw); rec.Code != http.StatusOK {
		t.Errorf("legacy token on admin route: got %d, want 200 — this is the back-compat break that must not happen (body=%s)", rec.Code, rec.Body.String())
	}
}

// TestMemberTokenDeniedAdminRoute is the actual fix: before P2-18 this returned
// 200 for every token regardless of what it was for.
func TestMemberTokenDeniedAdminRoute(t *testing.T) {
	srv, db := newTestHandler(t)
	const raw = "oct_memberscopedtoken00000000000000001"
	seedToken(t, db, raw, "member")

	if rec := bearer(srv, http.MethodGet, "/api/tokens", raw); rec.Code != http.StatusForbidden {
		t.Errorf("member token on admin route: got %d, want 403 (body=%s)", rec.Code, rec.Body.String())
	}

	// Positive half: the token is still a valid credential for ungated routes.
	// Without this a token that had simply stopped authenticating would pass the
	// assertion above for entirely the wrong reason.
	if rec := bearer(srv, http.MethodGet, "/api/links", raw); rec.Code != http.StatusOK {
		t.Errorf("member token on ungated route: got %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestAdminTokenAllowedAdminRoute(t *testing.T) {
	srv, db := newTestHandler(t)
	const raw = "oct_adminscopedtoken000000000000000001"
	seedToken(t, db, raw, "admin")

	if rec := bearer(srv, http.MethodGet, "/api/tokens", raw); rec.Code != http.StatusOK {
		t.Errorf("admin token on admin route: got %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
}

// TestTokenCannotMintAboveOwnRole closes the escalation loop: an admin-scoped
// token that could mint an owner-scoped one would make the ceiling decorative.
func TestTokenCannotMintAboveOwnRole(t *testing.T) {
	srv, db := newTestHandler(t)
	const raw = "oct_adminmintertoken000000000000000001"
	seedToken(t, db, raw, "admin")

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

// TestMintedTokenDefaultsToMember pins the default. A caller who does not think
// about scope should get the narrow token, not the workspace-wide one — the
// opposite default is how "everything is unrestricted" happened in the first place.
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

	// "" is not accepted either: it is the legacy-unrestricted marker, and there
	// must be no way to mint a fresh token carrying it.
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
// It also means the role ceiling applies over MCP like anywhere else.
func TestMCPAuthStampsTokenIdentity(t *testing.T) {
	h, _, db := newTestHandlerRaw(t)
	const raw = "oct_mcpidentitytoken00000000000000001"
	if err := db.Create(&models.Token{
		OrgID: 1, Name: "mcp", Hash: models.HashToken(raw), Prefix: raw[:8], Role: "member",
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
		t.Error("member-scoped token cleared an admin gate over MCP")
	}
}
