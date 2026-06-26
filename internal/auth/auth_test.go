package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/Jungley8/led/config"
	"github.com/Jungley8/led/internal/crypto"
	"github.com/Jungley8/led/internal/models"
	"gorm.io/gorm"
)

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

func TestSessionIssueValidateRoundtrip(t *testing.T) {
	m := testManager(t)
	tok := m.issue(models.SingleUserID, time.Hour)
	uid, ok := m.validate(tok)
	if !ok || uid != models.SingleUserID {
		t.Fatalf("validate(issue) = (%d,%v), want (%d,true)", uid, ok, models.SingleUserID)
	}
}

func TestSessionRejectsExpired(t *testing.T) {
	m := testManager(t)
	tok := m.issue(models.SingleUserID, -time.Hour) // already expired
	if _, ok := m.validate(tok); ok {
		t.Fatal("validate accepted an expired token")
	}
}

func TestSessionRejectsTamperedSignature(t *testing.T) {
	m := testManager(t)
	tok := m.issue(models.SingleUserID, time.Hour)
	if _, ok := m.validate(tok + "x"); ok {
		t.Fatal("validate accepted a tampered signature")
	}
	if _, ok := m.validate("garbage"); ok {
		t.Fatal("validate accepted a malformed token")
	}
}

func TestAuthedReadsCookie(t *testing.T) {
	m := testManager(t)
	rec := httptest.NewRecorder()
	m.SetSession(rec, models.SingleUserID)
	req := httptest.NewRequest(http.MethodGet, "/api/links", nil)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}
	if !m.Authed(req) {
		t.Fatal("Authed rejected a freshly issued session cookie")
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
