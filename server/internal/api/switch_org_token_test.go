package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/internal/models"
)

func bearerJSON(srv http.Handler, method, path, raw, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+raw)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

func sessionCookieCount(rec *httptest.ResponseRecorder) int {
	n := 0
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName && c.Value != "" {
			n++
		}
	}
	return n
}

func TestSwitchOrgRefusesBearerToken(t *testing.T) {
	srv, db := newTestHandler(t)
	const uid = uint(3101)
	const raw = "oct_switchorgcappedtoken000000000001"

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

	var sessions int64
	db.Model(&models.Session{}).Where("user_id = ?", uid).Count(&sessions)
	if sessions != 0 {
		t.Fatalf("bearer switch-org created %d session row(s) for the token holder", sessions)
	}
}

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
