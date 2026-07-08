package api

// Tenant-isolation security tests.
//
// Every "cross-org" sub-test creates a resource as org 1, then attempts to
// read / mutate / delete it as org 2 and asserts 404 (not 200/204/403).
// Returning 404 instead of 403 is intentional — it avoids leaking existence.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/led/config"
	"github.com/octarq-org/led/internal/auth"
	"github.com/octarq-org/led/internal/crypto"
	"github.com/octarq-org/led/internal/models"
	"gorm.io/gorm"
)

// sessionCookies returns the cookies that represent a session for (uid, orgID).
func sessionCookies(t *testing.T, uid, orgID uint) []*http.Cookie {
	t.Helper()
	dbName := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{})
	if err != nil {
		t.Fatalf("sessionCookies open db: %v", err)
	}
	cfg := &config.Config{SecretKey: "secret"}
	m := auth.New(cfg, crypto.New("secret")).WithDB(db)
	rec := httptest.NewRecorder()
	m.SetSession(rec, uid, orgID)
	return rec.Result().Cookies()
}

// do is a small helper that fires a request against srv with optional cookies
// and an optional JSON body, returning the response recorder.
func do(srv http.Handler, method, path string, cookies []*http.Cookie, body string) *httptest.ResponseRecorder {
	var bodyReader *strings.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	} else {
		bodyReader = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, bodyReader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

// --- unauthenticated ---

func TestUnauthenticated(t *testing.T) {
	srv, _ := newTestHandler(t)
	endpoints := []struct{ method, path string }{
		{"GET", "/api/links"},
		{"POST", "/api/links"},
		{"GET", "/api/mailboxes"},
		{"POST", "/api/mailboxes"},
		{"GET", "/api/emails"},
		{"GET", "/api/domains"},
		{"GET", "/api/tokens"},
		{"POST", "/api/tokens"},
		{"GET", "/api/overview"},
	}
	for _, e := range endpoints {
		rec := do(srv, e.method, e.path, nil, "")
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: got %d, want 401", e.method, e.path, rec.Code)
		}
	}
}

// --- link isolation ---

func TestOrgIsolation_Links(t *testing.T) {
	srv, _ := newTestHandler(t)
	org1 := sessionCookies(t, 1, 1)
	org2 := sessionCookies(t, 2, 2)

	// Org 1 creates a link.
	rec := do(srv, http.MethodPost, "/api/links", org1,
		`{"slug":"sec-test","target":"https://example.com"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create link: got %d — %s", rec.Code, rec.Body)
	}
	var link struct {
		ID uint `json:"id"`
	}
	json.Unmarshal(rec.Body.Bytes(), &link)
	path := fmt.Sprintf("/api/links/%d", link.ID)

	// Org 2 must not read it.
	if rec := do(srv, http.MethodGet, path, org2, ""); rec.Code != http.StatusNotFound {
		t.Errorf("cross-org GET link: got %d, want 404", rec.Code)
	}
	// Org 2 must not update it.
	if rec := do(srv, http.MethodPut, path, org2, `{"target":"https://evil.com"}`); rec.Code != http.StatusNotFound {
		t.Errorf("cross-org PUT link: got %d, want 404", rec.Code)
	}
	// Org 2 must not delete it.
	if rec := do(srv, http.MethodDelete, path, org2, ""); rec.Code != http.StatusNotFound {
		t.Errorf("cross-org DELETE link: got %d, want 404", rec.Code)
	}
	// Org 1's list must include the link; org 2's list must not.
	if rec := do(srv, http.MethodGet, "/api/links", org1, ""); !strings.Contains(rec.Body.String(), "sec-test") {
		t.Error("org1 list does not include its own link")
	}
	if rec := do(srv, http.MethodGet, "/api/links", org2, ""); strings.Contains(rec.Body.String(), "sec-test") {
		t.Error("org2 list leaked org1's link")
	}
}

// --- mailbox isolation ---

func TestOrgIsolation_Mailboxes(t *testing.T) {
	srv, _ := newTestHandler(t)
	org1 := sessionCookies(t, 1, 1)
	org2 := sessionCookies(t, 2, 2)

	rec := do(srv, http.MethodPost, "/api/mailboxes", org1,
		`{"address":"sec@org1.test"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create mailbox: got %d — %s", rec.Code, rec.Body)
	}
	var mb struct {
		ID uint `json:"id"`
	}
	json.Unmarshal(rec.Body.Bytes(), &mb)
	path := fmt.Sprintf("/api/mailboxes/%d", mb.ID)

	if rec := do(srv, http.MethodPut, path, org2, `{"note":"hacked"}`); rec.Code != http.StatusNotFound {
		t.Errorf("cross-org PUT mailbox: got %d, want 404", rec.Code)
	}
	if rec := do(srv, http.MethodDelete, path, org2, ""); rec.Code != http.StatusNotFound {
		t.Errorf("cross-org DELETE mailbox: got %d, want 404", rec.Code)
	}
	if rec := do(srv, http.MethodGet, "/api/mailboxes", org2, ""); strings.Contains(rec.Body.String(), "sec@org1.test") {
		t.Error("org2 list leaked org1's mailbox")
	}
}

// --- token isolation ---

func TestOrgIsolation_Tokens(t *testing.T) {
	srv, _ := newTestHandler(t)
	org1 := sessionCookies(t, 1, 1)
	org2 := sessionCookies(t, 2, 2)

	rec := do(srv, http.MethodPost, "/api/tokens", org1, `{"name":"org1tok"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create token: got %d — %s", rec.Code, rec.Body)
	}
	var tok struct {
		ID    uint   `json:"id"`
		Token string `json:"token"`
	}
	json.Unmarshal(rec.Body.Bytes(), &tok)
	path := fmt.Sprintf("/api/tokens/%d", tok.ID)

	// Org 2 must not delete org 1's token.
	if rec := do(srv, http.MethodDelete, path, org2, ""); rec.Code != http.StatusNotFound {
		t.Errorf("cross-org DELETE token: got %d, want 404", rec.Code)
	}
	// Org 2's token list must not include org 1's token.
	if rec := do(srv, http.MethodGet, "/api/tokens", org2, ""); strings.Contains(rec.Body.String(), "org1tok") {
		t.Error("org2 token list leaked org1's token")
	}
	// Org 1 can still delete its own token.
	if rec := do(srv, http.MethodDelete, path, org1, ""); rec.Code != http.StatusOK {
		t.Errorf("org1 self-delete token: got %d, want 200", rec.Code)
	}
}

// --- bearer token org isolation ---
//
// A bearer token belongs to org 1. It must only see org 1's links, not org 2's.

func TestBearerTokenOrgIsolation(t *testing.T) {
	srv, db := newTestHandler(t)
	org1 := sessionCookies(t, 1, 1)
	org2 := sessionCookies(t, 2, 2)

	// Org 1 and Org 2 create links.
	do(srv, http.MethodPost, "/api/links", org1, `{"slug":"bearer-org1","target":"https://org1.example"}`)
	do(srv, http.MethodPost, "/api/links", org2, `{"slug":"bearer-org2","target":"https://org2.example"}`)

	// Token for Org 1
	rawTok1 := "led_bearertesttoken1111111111111111111"
	db.Create(&models.Token{
		OrgID:  1,
		Name:   "bearer-test-1",
		Hash:   models.HashToken(rawTok1),
		Prefix: rawTok1[:8],
	})

	// Token for Org 2
	rawTok2 := "led_bearertesttoken2222222222222222222"
	db.Create(&models.Token{
		OrgID:  2,
		Name:   "bearer-test-2",
		Hash:   models.HashToken(rawTok2),
		Prefix: rawTok2[:8],
	})

	// Test Token 1
	req1 := httptest.NewRequest(http.MethodGet, "/api/links", nil)
	req1.Header.Set("Authorization", "Bearer "+rawTok1)
	rec1 := httptest.NewRecorder()
	srv.ServeHTTP(rec1, req1)

	body1 := rec1.Body.String()
	if !strings.Contains(body1, "bearer-org1") {
		t.Error("bearer token 1 cannot see its own org's link")
	}
	if strings.Contains(body1, "bearer-org2") {
		t.Error("bearer token 1 leaked org2's link")
	}

	// Test Token 2
	req2 := httptest.NewRequest(http.MethodGet, "/api/links", nil)
	req2.Header.Set("Authorization", "Bearer "+rawTok2)
	rec2 := httptest.NewRecorder()
	srv.ServeHTTP(rec2, req2)

	body2 := rec2.Body.String()
	if !strings.Contains(body2, "bearer-org2") {
		t.Error("bearer token 2 cannot see its own org's link")
	}
	if strings.Contains(body2, "bearer-org1") {
		t.Error("bearer token 2 leaked org1's link")
	}
}

// --- session token cannot be replayed after tampering ---

func TestTamperedSessionIsRejected(t *testing.T) {
	srv, _ := newTestHandler(t)
	org1 := sessionCookies(t, 1, 1)

	// Confirm the unmodified cookie works.
	if rec := do(srv, http.MethodGet, "/api/links", org1, ""); rec.Code != http.StatusOK {
		t.Fatalf("valid session rejected: %d", rec.Code)
	}

	// Corrupt the cookie value.
	tampered := make([]*http.Cookie, len(org1))
	copy(tampered, org1)
	for i, c := range tampered {
		if c.Name == "led_session" {
			cp := *c
			cp.Value = cp.Value + "tampered"
			tampered[i] = &cp
		}
	}
	if rec := do(srv, http.MethodGet, "/api/links", tampered, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("tampered session cookie: got %d, want 401", rec.Code)
	}
}
