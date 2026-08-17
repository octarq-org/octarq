package api

// The token role cap is a ceiling for API-token requests on the permission
// seam. plugin.ResolvePerm is only allowed to refine permissions above a role
// baseline — it must never hand a capped token more than the token's own role
// permits. These tests pin that invariant at two points: the cap actually
// blocks a resolver that says yes (TestTokenCapIsACeilingOverResolver), and
// the same resolver may still widen a session's authority, because sessions
// carry no cap (TestSessionStillWidensThroughResolver). The second test is
// what stops the first from being faked by gutting the resolver entirely.

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugin"
)

// alwaysAllowResolver is the fake plugin resolver every test here shares: it
// claims to decide every permission and always allows it.
func alwaysAllowResolver(r *http.Request, permKey string) (allow, decided bool) {
	return true, true
}

// TestTokenCapIsACeilingOverResolver: a member-capped token held by a member
// must be refused an admin permission even when the resolver says yes. The cap
// is checked before the resolver is ever consulted.
func TestTokenCapIsACeilingOverResolver(t *testing.T) {
	t.Cleanup(plugin.ResetPermRegistry)
	plugin.ResetPermRegistry()
	plugin.SetPermResolver(alwaysAllowResolver)

	h, _, db := newTestHandlerRaw(t)
	seedMember(t, db, 201, "member")
	seedToken(t, db, "oct_capceilingtoken000000000000000001", 201, "member")

	req := httptest.NewRequest(http.MethodGet, "/api/webhooks", nil)
	req.Header.Set("Authorization", "Bearer oct_capceilingtoken000000000000000001")
	req, ok := h.auth.AuthenticateRequest(req)
	if !ok {
		t.Fatal("member-capped token did not authenticate")
	}

	// Without the cap pre-gate this resolver would answer (true, decided) and
	// the member-capped token would clear an admin permission it must not have.
	if h.RequirePerm(req, "dns.records.delete", "admin") {
		t.Fatal("member-capped token cleared an admin permission through an allowing resolver")
	}
}

// TestSessionStillWidensThroughResolver: the same always-allowing resolver may
// still widen a plain session caller — a member session clearing an admin
// permission is exactly the Pro custom-role behaviour the cap pre-gate must not
// break. Without this test, gutting the resolver path would make the cap test
// above pass for the wrong reason.
func TestSessionStillWidensThroughResolver(t *testing.T) {
	t.Cleanup(plugin.ResetPermRegistry)
	plugin.ResetPermRegistry()
	plugin.SetPermResolver(alwaysAllowResolver)

	h, _, db := newTestHandlerRaw(t)
	seedMember(t, db, 202, "member")

	req := httptest.NewRequest(http.MethodGet, "/api/webhooks", nil)
	for _, c := range sessionCookies(t, 202, 1) {
		req.AddCookie(c)
	}
	req, ok := h.auth.AuthenticateRequest(req)
	if !ok {
		t.Fatal("member session did not authenticate")
	}

	if !h.RequirePerm(req, "dns.records.delete", "admin") {
		t.Fatal("member session was refused an admin permission the resolver allows")
	}
}

// TestRemovingMemberRevokesTheirTokens: removing a member must delete their
// (user, org) API tokens, not just their sessions. The token row is gone and
// the raw token stops authenticating (401) immediately.
func TestRemovingMemberRevokesTheirTokens(t *testing.T) {
	srv, db := newTestHandler(t)
	const org = uint(5301)

	ownerUID := seedOrgMember(t, db, org, "owner@x.com", "owner")
	memberUID := seedOrgMember(t, db, org, "member@x.com", "member")
	ownerSession := sessionCookies(t, ownerUID, org)

	const raw = "oct_revoketoken0000000000000000000001"
	if err := db.Create(&models.Token{
		OrgID: org, UserID: memberUID, Name: "to-revoke",
		Hash: models.HashToken(raw), Prefix: raw[:8], Role: "member",
	}).Error; err != nil {
		t.Fatalf("seed token: %v", err)
	}

	// Precondition: the token is a live credential while its holder is a member.
	if rec := bearer(srv, http.MethodGet, "/api/links", raw); rec.Code != http.StatusOK {
		t.Fatalf("precondition: member token should work, got %d (body=%s)", rec.Code, rec.Body.String())
	}

	rec := do(srv, "DELETE", "/api/org/members/"+strconv.FormatUint(uint64(memberUID), 10), ownerSession, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("remove member: got %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}

	var count int64
	db.Model(&models.Token{}).Where("user_id = ? AND owner_id = ?", memberUID, org).Count(&count)
	if count != 0 {
		t.Errorf("removed member kept %d live token(s) in the org", count)
	}

	if rec := bearer(srv, http.MethodGet, "/api/links", raw); rec.Code != http.StatusUnauthorized {
		t.Errorf("removed member's token still authenticates: got %d, want 401 (body=%s)", rec.Code, rec.Body.String())
	}
}
