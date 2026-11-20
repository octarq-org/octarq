package api

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
)

func alwaysAllowResolver(r *http.Request, permKey string) (allow, decided bool) {
	return true, true
}

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

	if h.RequirePerm(req, "dns.records.delete", "admin") {
		t.Fatal("member-capped token cleared an admin permission through an allowing resolver")
	}
}

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

	if rec := bearer(srv, http.MethodGet, "/api/overview", raw); rec.Code != http.StatusOK {
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

	if rec := bearer(srv, http.MethodGet, "/api/overview", raw); rec.Code != http.StatusUnauthorized {
		t.Errorf("removed member's token still authenticates: got %d, want 401 (body=%s)", rec.Code, rec.Body.String())
	}
}
