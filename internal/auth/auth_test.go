package auth

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/led/config"
	"github.com/octarq-org/led/internal/crypto"
	"github.com/octarq-org/led/internal/models"
	"gorm.io/gorm"
)

// testEnvStore backs crypto.EnableEnvelope with the test DB's settings table.
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
	m.SetSession(rec, 1, 42)

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}
	tokCookie := cookies[0]

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
	m.SetSession(rec, 1, 42)
	tokCookie := rec.Result().Cookies()[0]

	// 1. Invalid token
	reqBad := httptest.NewRequest(http.MethodGet, "/api/links", nil)
	reqBad.AddCookie(&http.Cookie{Name: cookieName, Value: "garbage_token"})
	if m.Authed(reqBad) {
		t.Fatal("expected invalid token to be rejected")
	}

	// 2. Expired session (the DB stores only the SHA-256 hash of the cookie).
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

	// 3. Clear session
	rec2 := httptest.NewRecorder()
	m.SetSession(rec2, 2, 42)
	tokCookie2 := rec2.Result().Cookies()[0]
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

func TestAuthedReadsCookie(t *testing.T) {
	db := testDB(t)
	m := testManager(t).WithDB(db)
	rec := httptest.NewRecorder()
	m.SetSession(rec, 1, 1)
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
	m.SetSession(rec, 7, 99)
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

	raw := "led_validtoken123456789012345678901234"
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
	bad.Header.Set("Authorization", "Bearer led_unknowntoken000000000000000000000")
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
		_, _ = w.Write([]byte(fmt.Sprintf("uid=%d,orgID=%d", uid, orgID)))
	}))

	// Case 1: Unauthorized (no cookie, no bearer token)
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}

	// Case 2: Authorized via Session Cookie
	recCookie := httptest.NewRecorder()
	m.SetSession(recCookie, 42, 99)
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

	// Case 3: Authorized via Bearer Token
	raw := "led_testtoken_require_middleware_9999"
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

func TestOAuthLoadProviderConcurrency(t *testing.T) {
	db := testDB(t)
	cfg := &config.Config{SecretKey: "secret"}
	cipher := crypto.New("secret")
	if err := cipher.EnableEnvelope(testEnvStore{db}); err != nil {
		t.Fatalf("EnableEnvelope: %v", err)
	}
	m := New(cfg, cipher).WithDB(db)

	encSecret, err := cipher.Encrypt([]byte("google-secret"))
	if err != nil {
		t.Fatalf("encrypt secret: %v", err)
	}

	db.Create(&models.Setting{Key: "oauth.google.client_id", Value: "google-id"})
	db.Create(&models.Setting{Key: "oauth.google.client_secret", Value: encSecret})

	handler := NewOAuthHandler(db, "http://localhost", m, cipher)

	const workers = 10
	done := make(chan bool, workers)
	for i := 0; i < workers; i++ {
		go func() {
			ok := handler.loadProvider("google")
			if !ok {
				t.Errorf("expected loadProvider to succeed")
			}
			done <- true
		}()
	}

	for i := 0; i < workers; i++ {
		<-done
	}
}

func TestOAuthHandlerUpsertUser(t *testing.T) {
	db := testDB(t)
	cfg := &config.Config{SecretKey: "secret"}
	m := New(cfg, crypto.New("secret")).WithDB(db)
	handler := NewOAuthHandler(db, "http://localhost", m, crypto.New("secret"))

	InitGothStore("secret")

	reqBegin := httptest.NewRequest(http.MethodGet, "/auth/begin/unconfigured", nil)
	reqBegin.SetPathValue("provider", "unconfigured")
	recBegin := httptest.NewRecorder()
	handler.Begin(recBegin, reqBegin)
	if recBegin.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 for unconfigured provider, got %d", recBegin.Code)
	}

	reqCallback := httptest.NewRequest(http.MethodGet, "/auth/callback/unconfigured", nil)
	reqCallback.SetPathValue("provider", "unconfigured")
	recCallback := httptest.NewRecorder()
	handler.Callback(recCallback, reqCallback)
	if recCallback.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 for unconfigured provider, got %d", recCallback.Code)
	}

	u, o, err := handler.upsertUser("alice@example.com", "http://avatar", "google")
	if err != nil {
		t.Fatalf("upsertUser failed: %v", err)
	}
	if u.Email != "alice@example.com" {
		t.Errorf("user email = %q, want alice@example.com", u.Email)
	}
	if o.Name != "alice@example.com" || o.Slug != "alice-example-com" {
		t.Errorf("org mismatch: %+v", o)
	}

	u2, o2, err := handler.upsertUser("alice@example.com", "http://avatar", "google")
	if err != nil {
		t.Fatalf("upsertUser existing failed: %v", err)
	}
	if u2.ID != u.ID || o2.ID != o.ID {
		t.Errorf("upsertUser existing did not return same user/org")
	}
}
