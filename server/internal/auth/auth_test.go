package auth

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/server/config"
	"github.com/octarq-org/octarq/server/internal/crypto"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

type testEnvStore struct{ db *gorm.DB }

func (s testEnvStore) Get(key string) (string, bool) {
	var row models.Setting
	if s.db.First(&row, "key = ?", key).Error != nil {
		return "", false
	}
	return row.Value, true
}

func (s testEnvStore) Set(key, val string) error {
	return s.db.Save(&models.Setting{Key: key, Value: val}).Error
}

func (s testEnvStore) Delete(key string) error {
	return s.db.Delete(&models.Setting{Key: key}).Error
}

func (s testEnvStore) HasEncryptedValues() (bool, error) {
	return models.HasEncryptedValues(s.db)
}

func testManager(t *testing.T) *Manager {
	t.Helper()
	cfg := &config.Config{AdminUser: "admin", AdminPassword: "pw", SecretKey: "secret"}
	return New(cfg, crypto.New(cfg.SecretKey))
}

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.AllModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestStatefulSessionRoundtrip(t *testing.T) {
	db := testDB(t)
	m := testManager(t).WithDB(db)

	rec := httptest.NewRecorder()
	m.SetSession(rec, httptest.NewRequest(http.MethodGet, "/", nil), 1, 42)

	cookies := rec.Result().Cookies()
	var tokCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == cookieName {
			tokCookie = c
		}
	}
	if tokCookie == nil {
		t.Fatalf("expected session cookie")
		return
	}

	req := httptest.NewRequest(http.MethodGet, "/api/links", nil)
	req.AddCookie(tokCookie)

	if !m.Authed(req) {
		t.Fatal("expected request to be authenticated")
	}

	if uid := m.UserID(req); uid != 1 {
		t.Errorf("UserID = %d, want 1", uid)
	}

	if orgID := m.OrgID(req); orgID != 42 {
		t.Errorf("OrgID = %d, want 42", orgID)
	}
}

func TestStatefulSessionExpiryAndInvalidation(t *testing.T) {
	db := testDB(t)
	m := testManager(t).WithDB(db)

	rec := httptest.NewRecorder()
	m.SetSession(rec, httptest.NewRequest(http.MethodGet, "/", nil), 1, 42)
	var tokCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == cookieName {
			tokCookie = c
		}
	}

	reqBad := httptest.NewRequest(http.MethodGet, "/api/links", nil)
	reqBad.AddCookie(&http.Cookie{Name: cookieName, Value: "garbage_token"})
	if m.Authed(reqBad) {
		t.Fatal("expected invalid token to be rejected")
	}

	var s models.Session
	if err := db.Where("token = ?", models.HashToken(tokCookie.Value)).First(&s).Error; err != nil {
		t.Fatalf("failed to find session: %v", err)
	}
	s.ExpiresAt = time.Now().Add(-time.Hour)
	if err := db.Save(&s).Error; err != nil {
		t.Fatalf("failed to save expired session: %v", err)
	}

	reqExp := httptest.NewRequest(http.MethodGet, "/api/links", nil)
	reqExp.AddCookie(tokCookie)
	if m.Authed(reqExp) {
		t.Fatal("expected expired session to be rejected")
	}

	rec2 := httptest.NewRecorder()
	m.SetSession(rec2, httptest.NewRequest(http.MethodGet, "/", nil), 2, 42)
	var tokCookie2 *http.Cookie
	for _, c := range rec2.Result().Cookies() {
		if c.Name == cookieName {
			tokCookie2 = c
		}
	}
	reqClear := httptest.NewRequest(http.MethodGet, "/api/links", nil)
	reqClear.AddCookie(tokCookie2)

	if !m.Authed(reqClear) {
		t.Fatal("session should be authed before clear")
	}

	recClear := httptest.NewRecorder()
	m.Clear(reqClear, recClear)

	if m.Authed(reqClear) {
		t.Fatal("session should be unauthed after clear")
	}
}

func TestRoleChangeInvalidatesSessions(t *testing.T) {
	db := testDB(t)
	m := testManager(t).WithDB(db)

	const (
		uid  = uint(55)
		orgA = uint(101)
		orgB = uint(102)
	)

	recA := httptest.NewRecorder()
	m.SetSession(recA, httptest.NewRequest(http.MethodGet, "/", nil), uid, orgA)
	var tokA *http.Cookie
	for _, c := range recA.Result().Cookies() {
		if c.Name == cookieName {
			tokA = c
		}
	}

	recB := httptest.NewRecorder()
	m.SetSession(recB, httptest.NewRequest(http.MethodGet, "/", nil), uid, orgB)
	var tokB *http.Cookie
	for _, c := range recB.Result().Cookies() {
		if c.Name == cookieName {
			tokB = c
		}
	}

	reqA := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	reqA.AddCookie(tokA)
	if !m.Authed(reqA) {
		t.Fatal("session A should be authed before revocation")
	}

	reqB := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	reqB.AddCookie(tokB)
	if !m.Authed(reqB) {
		t.Fatal("session B should be authed before revocation")
	}

	revoked := m.RevokeUserOrgSessions(uid, orgA)
	if revoked == 0 {
		t.Fatalf("RevokeUserOrgSessions revoked %d sessions, want >0", revoked)
	}

	if m.Authed(reqA) {
		t.Fatal("session A should be unauthed after RevokeUserOrgSessions")
	}

	if !m.Authed(reqB) {
		t.Fatal("session B should remain authed after orgA revocation")
	}

	var countA, countB int64
	db.Model(&models.Session{}).Where("user_id = ? AND org_id = ?", uid, orgA).Count(&countA)
	db.Model(&models.Session{}).Where("user_id = ? AND org_id = ?", uid, orgB).Count(&countB)
	if countA != 0 {
		t.Errorf("user_sessions count for orgA = %d, want 0", countA)
	}
	if countB == 0 {
		t.Errorf("user_sessions count for orgB = %d, want >0", countB)
	}
}

func TestAuthedReadsCookie(t *testing.T) {
	db := testDB(t)
	m := testManager(t).WithDB(db)
	rec := httptest.NewRecorder()
	m.SetSession(rec, httptest.NewRequest(http.MethodGet, "/", nil), 1, 1)
	req := httptest.NewRequest(http.MethodGet, "/api/links", nil)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}
	if !m.Authed(req) {
		t.Fatal("Authed rejected a freshly issued session cookie")
	}
}

func TestOrgIDExtractedFromCookie(t *testing.T) {
	db := testDB(t)
	m := testManager(t).WithDB(db)
	rec := httptest.NewRecorder()
	m.SetSession(rec, httptest.NewRequest(http.MethodGet, "/", nil), 7, 99)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}
	if got := m.UserID(req); got != 7 {
		t.Errorf("UserID = %d, want 7", got)
	}
	if got := m.OrgID(req); got != 99 {
		t.Errorf("OrgID = %d, want 99", got)
	}
}

func TestOrgIDZeroWhenUnauthenticated(t *testing.T) {
	db := testDB(t)
	m := testManager(t).WithDB(db)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := m.OrgID(req); got != 0 {
		t.Errorf("OrgID on unauthed request = %d, want 0", got)
	}
	if got := m.UserID(req); got != 0 {
		t.Errorf("UserID on unauthed request = %d, want 0", got)
	}
}

func TestBearerTokenAuth(t *testing.T) {
	db := testDB(t)
	m := testManager(t).WithDB(db)

	raw := "oct_validtoken123456789012345678901234"
	tok := models.Token{Name: "ci", Hash: models.HashToken(raw), Prefix: raw[:8]}
	if err := db.Create(&tok).Error; err != nil {
		t.Fatalf("create token: %v", err)
	}

	good := httptest.NewRequest(http.MethodGet, "/api/links", nil)
	good.Header.Set("Authorization", "Bearer "+raw)
	if !m.APIAuthed(good) {
		t.Fatal("APIAuthed rejected a valid bearer token")
	}

	bad := httptest.NewRequest(http.MethodGet, "/api/links", nil)
	bad.Header.Set("Authorization", "Bearer oct_unknowntoken000000000000000000000")
	if m.APIAuthed(bad) {
		t.Fatal("APIAuthed accepted an unknown bearer token")
	}

	none := httptest.NewRequest(http.MethodGet, "/api/links", nil)
	if m.APIAuthed(none) {
		t.Fatal("APIAuthed accepted a request with no credentials")
	}
}

func TestRequireMiddleware(t *testing.T) {
	db := testDB(t)
	m := testManager(t).WithDB(db)

	handler := m.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid := m.UserID(r)
		orgID := m.OrgID(r)
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "uid=%d,orgID=%d", uid, orgID)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}

	recCookie := httptest.NewRecorder()
	m.SetSession(recCookie, httptest.NewRequest(http.MethodGet, "/", nil), 42, 99)
	reqCookie := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	for _, c := range recCookie.Result().Cookies() {
		reqCookie.AddCookie(c)
	}
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, reqCookie)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "uid=42,orgID=99" {
		t.Errorf("expected 'uid=42,orgID=99', got '%s'", rec.Body.String())
	}

	raw := "oct_testtoken_require_middleware_9999"
	tok := models.Token{
		OrgID:  88,
		Name:   "test-token",
		Hash:   models.HashToken(raw),
		Prefix: raw[:8],
	}
	if err := db.Create(&tok).Error; err != nil {
		t.Fatalf("create token: %v", err)
	}

	reqBearer := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	reqBearer.Header.Set("Authorization", "Bearer "+raw)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, reqBearer)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "uid=0,orgID=88" {
		t.Errorf("expected 'uid=0,orgID=88', got '%s'", rec.Body.String())
	}
}

func TestRequireStampsTheUserIDForPlugins(t *testing.T) {
	db := testDB(t)
	m := testManager(t).WithDB(db)

	var got uint
	handler := m.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = plugin.UserIDFromContext(r.Context())
	}))

	recCookie := httptest.NewRecorder()
	m.SetSession(recCookie, httptest.NewRequest(http.MethodGet, "/", nil), 42, 99)
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	for _, c := range recCookie.Result().Cookies() {
		req.AddCookie(c)
	}
	handler.ServeHTTP(httptest.NewRecorder(), req)
	if got != 42 {
		t.Fatalf("plugin.UserIDFromContext after a session request = %d, want 42", got)
	}

	raw := "oct_testtoken_plugin_userid_1234"
	tok := models.Token{
		UserID: 7,
		OrgID:  88,
		Name:   "plugin-userid",
		Hash:   models.HashToken(raw),
		Prefix: raw[:8],
	}
	if err := db.Create(&tok).Error; err != nil {
		t.Fatalf("create token: %v", err)
	}
	reqBearer := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	reqBearer.Header.Set("Authorization", "Bearer "+raw)
	handler.ServeHTTP(httptest.NewRecorder(), reqBearer)
	if got != 7 {
		t.Fatalf("plugin.UserIDFromContext after a token request = %d, want 7", got)
	}
}
