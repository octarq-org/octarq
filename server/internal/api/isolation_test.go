package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/server/config"
	"github.com/octarq-org/octarq/server/internal/auth"
	"github.com/octarq-org/octarq/server/internal/crypto"
	"github.com/octarq-org/octarq/server/internal/models"
	"gorm.io/gorm"
)

func sessionCookies(t *testing.T, uid, orgID uint) []*http.Cookie {
	t.Helper()
	dbName := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{})
	if err != nil {
		t.Fatalf("sessionCookies open db: %v", err)
	}
	if uid != 0 && orgID != 0 {
		var mem models.OrgMember
		db.Where("org_id = ? AND user_id = ?", orgID, uid).
			FirstOrCreate(&mem, models.OrgMember{OrgID: orgID, UserID: uid, Role: "owner"})
	}

	cfg := &config.Config{SecretKey: "secret"}
	m := auth.New(cfg, crypto.New("secret")).WithDB(db)
	rec := httptest.NewRecorder()
	m.SetSession(rec, httptest.NewRequest(http.MethodGet, "/", nil), uid, orgID)
	return rec.Result().Cookies()
}

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

func TestUnauthenticated(t *testing.T) {
	srv, _ := newTestHandler(t)
	endpoints := []struct{ method, path string }{
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

	if rec := do(srv, http.MethodDelete, path, org2, ""); rec.Code != http.StatusNotFound {
		t.Errorf("cross-org DELETE token: got %d, want 404", rec.Code)
	}
	if rec := do(srv, http.MethodGet, "/api/tokens", org2, ""); strings.Contains(rec.Body.String(), "org1tok") {
		t.Error("org2 token list leaked org1's token")
	}
	if rec := do(srv, http.MethodDelete, path, org1, ""); rec.Code != http.StatusOK {
		t.Errorf("org1 self-delete token: got %d, want 200", rec.Code)
	}
}

func TestBearerTokenOrgIsolation(t *testing.T) {
	srv, db := newTestHandler(t)

	rawTok1 := "oct_bearertesttoken1111111111111111111"
	db.Create(&models.Token{
		OrgID:  1,
		Name:   "bearer-test-1",
		Hash:   models.HashToken(rawTok1),
		Prefix: rawTok1[:8],
	})

	rawTok2 := "oct_bearertesttoken2222222222222222222"
	db.Create(&models.Token{
		OrgID:  2,
		Name:   "bearer-test-2",
		Hash:   models.HashToken(rawTok2),
		Prefix: rawTok2[:8],
	})

	req1 := httptest.NewRequest(http.MethodGet, "/api/tokens", nil)
	req1.Header.Set("Authorization", "Bearer "+rawTok1)
	rec1 := httptest.NewRecorder()
	srv.ServeHTTP(rec1, req1)

	body1 := rec1.Body.String()
	if !strings.Contains(body1, "bearer-test-1") {
		t.Error("bearer token 1 cannot see its own org's token")
	}
	if strings.Contains(body1, "bearer-test-2") {
		t.Error("bearer token 1 leaked org2's token")
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/tokens", nil)
	req2.Header.Set("Authorization", "Bearer "+rawTok2)
	rec2 := httptest.NewRecorder()
	srv.ServeHTTP(rec2, req2)

	body2 := rec2.Body.String()
	if !strings.Contains(body2, "bearer-test-2") {
		t.Error("bearer token 2 cannot see its own org's token")
	}
	if strings.Contains(body2, "bearer-test-1") {
		t.Error("bearer token 2 leaked org1's token")
	}
}

func TestMCPTokenZeroOrgRejected(t *testing.T) {
	srv, db := newTestHandler(t)

	rawTok := "oct_zeroorgtoken000000000000000000000"
	tok := models.Token{OrgID: 1, Name: "zero-org", Hash: models.HashToken(rawTok), Prefix: rawTok[:8]}
	if err := db.Create(&tok).Error; err != nil {
		t.Fatalf("create token: %v", err)
	}
	if err := db.Model(&models.Token{}).Where("id = ?", tok.ID).Update("owner_id", 0).Error; err != nil {
		t.Fatalf("force owner_id 0: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/mcp/sse?token="+rawTok, nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("zero-org MCP token: got %d, want 401", rec.Code)
	}
}

func TestSessionRevocation(t *testing.T) {
	srv, _ := newTestHandler(t)
	org1 := sessionCookies(t, 1, 1)
	org2 := sessionCookies(t, 2, 2)

	if rec := do(srv, http.MethodGet, "/api/overview", org1, ""); rec.Code != http.StatusOK {
		t.Fatalf("valid session rejected: %d", rec.Code)
	}

	rec := do(srv, http.MethodGet, "/api/auth/sessions", org1, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list sessions: got %d — %s", rec.Code, rec.Body)
	}
	var sessions []struct {
		ID uint `json:"id"`
	}
	json.Unmarshal(rec.Body.Bytes(), &sessions)
	if len(sessions) == 0 {
		t.Fatalf("expected at least one session, got none: %s", rec.Body)
	}
	sid := sessions[0].ID
	path := fmt.Sprintf("/api/auth/sessions/%d", sid)

	if rec := do(srv, http.MethodDelete, path, org2, ""); rec.Code != http.StatusNotFound {
		t.Errorf("cross-user revoke: got %d, want 404", rec.Code)
	}
	if rec := do(srv, http.MethodGet, "/api/overview", org1, ""); rec.Code != http.StatusOK {
		t.Errorf("session wrongly killed by cross-user revoke: %d", rec.Code)
	}

	if rec := do(srv, http.MethodDelete, path, org1, ""); rec.Code != http.StatusOK {
		t.Fatalf("self revoke: got %d — %s", rec.Code, rec.Body)
	}
	if rec := do(srv, http.MethodGet, "/api/overview", org1, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("revoked session still valid: got %d, want 401", rec.Code)
	}
}

func TestTamperedSessionIsRejected(t *testing.T) {
	srv, _ := newTestHandler(t)
	org1 := sessionCookies(t, 1, 1)

	if rec := do(srv, http.MethodGet, "/api/overview", org1, ""); rec.Code != http.StatusOK {
		t.Fatalf("valid session rejected: %d", rec.Code)
	}

	tampered := make([]*http.Cookie, len(org1))
	copy(tampered, org1)
	for i, c := range tampered {
		if c.Name == "octarq_session" {
			cp := *c
			cp.Value = cp.Value + "tampered"
			tampered[i] = &cp
		}
	}
	if rec := do(srv, http.MethodGet, "/api/overview", tampered, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("tampered session cookie: got %d, want 401", rec.Code)
	}
}
