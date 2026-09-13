package api

// POST /api/auth/switch-org is the only endpoint in the codebase that both
// accepts a bearer token AND mints a session cookie. That combination was an
// escalation:
//
//   - A token's role cap only applies while the request is token-authenticated:
//     effectiveRole (helpers.go) returns the raw membership role the moment
//     TokenIDFromContext == 0. A session cookie carries no token id, so a
//     session minted by a member-capped token came back carrying the HOLDER's
//     full authority — an owner's, typically.
//   - The minted session also outlived its token: deleteToken and
//     RevokeUserOrgTokens never touch the sessions table.
//   - CSRF does not stand in the way. The guard waves through any request with
//     no cookies at all (csrf.go), which is exactly what a bearer client sends.
//
// So the refusal has to live in the handler. These tests pin it from both
// sides: a token is refused, and a cookie session — what the dashboard's own
// workspace switcher uses — still works.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/internal/models"
)

// bearerJSON fires a JSON request authenticated by a bearer token only. No
// cookies: that is both how a real API client calls, and what slips past the
// CSRF guard.
func bearerJSON(srv http.Handler, method, path, raw, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+raw)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

// sessionCookieCount reports how many Set-Cookie headers on rec carry the
// dashboard session cookie.
func sessionCookieCount(rec *httptest.ResponseRecorder) int {
	n := 0
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName && c.Value != "" {
			n++
		}
	}
	return n
}

// TestSwitchOrgRefusesBearerToken: a token-authenticated switch-org must be
// refused outright and must leave no session cookie behind. If it minted one,
// the caller would walk away holding an uncapped, unrevokable credential.
func TestSwitchOrgRefusesBearerToken(t *testing.T) {
	srv, db := newTestHandler(t)
	const uid = uint(3101)
	const raw = "oct_switchorgcappedtoken000000000001"

	// The holder is an OWNER of both workspaces — the cap, not the membership,
	// is the only thing standing between this token and full authority.
	seedMember(t, db, uid, "owner")
	if err := db.Create(&models.OrgMember{OrgID: 2, UserID: uid, Role: "owner"}).Error; err != nil {
		t.Fatalf("seed second membership: %v", err)
	}
	seedToken(t, db, raw, uid, "member")

	rec := bearerJSON(srv, "POST", "/api/auth/switch-org", raw, `{"orgId":2}`)
	if rec.Code == http.StatusOK {
		t.Fatalf("bearer switch-org succeeded: got 200 (%s), want a 4xx refusal", rec.Body.String())
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("bearer switch-org: got %d (%s), want 403", rec.Code, rec.Body.String())
	}
	if n := sessionCookieCount(rec); n != 0 {
		t.Fatalf("bearer switch-org minted %d session cookie(s) — a capped token just became an uncapped session", n)
	}

	// Nothing may reach the sessions table either: a row there outlives the
	// token that caused it, since revoking a token does not delete sessions.
	var sessions int64
	db.Model(&models.Session{}).Where("user_id = ?", uid).Count(&sessions)
	if sessions != 0 {
		t.Fatalf("bearer switch-org created %d session row(s) for the token holder", sessions)
	}
}

// TestSwitchOrgStillWorksForSessionCaller is what stops the test above from
// passing for the wrong reason. The dashboard's workspace switcher is
// cookie-authenticated and must keep working: if the refusal were written as
// "refuse everyone", this test goes red.
func TestSwitchOrgStillWorksForSessionCaller(t *testing.T) {
	srv, db := newTestHandler(t)
	const orgID = uint(1)
	uid := seedOrgMember(t, db, orgID, "switcher@example.com", "owner")
	if err := db.Create(&models.OrgMember{OrgID: 2, UserID: uid, Role: "owner"}).Error; err != nil {
		t.Fatalf("seed second membership: %v", err)
	}
	cookies := sessionCookies(t, uid, orgID)

	rec := do(srv, "POST", "/api/auth/switch-org", cookies, `{"orgId":2}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("cookie switch-org: got %d (%s), want 200", rec.Code, rec.Body.String())
	}
	if n := sessionCookieCount(rec); n == 0 {
		t.Fatal("cookie switch-org returned no re-issued session cookie")
	}
}
