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
	tok := m.issue(1, 42, time.Hour)
	uid, orgID, ok := m.validate(tok)
	if !ok {
		t.Fatal("validate rejected a freshly issued token")
	}
	if uid != 1 {
		t.Errorf("uid = %d, want 1", uid)
	}
	if orgID != 42 {
		t.Errorf("orgID = %d, want 42", orgID)
	}
}

func TestSessionRejectsExpired(t *testing.T) {
	m := testManager(t)
	tok := m.issue(1, 1, -time.Hour) // already expired
	if _, _, ok := m.validate(tok); ok {
		t.Fatal("validate accepted an expired token")
	}
}

func TestSessionRejectsTamperedSignature(t *testing.T) {
	m := testManager(t)
	tok := m.issue(1, 1, time.Hour)
	if _, _, ok := m.validate(tok + "x"); ok {
		t.Fatal("validate accepted a tampered signature")
	}
	if _, _, ok := m.validate("garbage"); ok {
		t.Fatal("validate accepted a malformed token")
	}
}

// Attacker forges a token with a different orgID but keeps everything else valid.
// The HMAC must cover the full uid:orgid|exp payload, so any mutation is rejected.
func TestSessionRejectsTamperedOrgID(t *testing.T) {
	m := testManager(t)
	tok := m.issue(1, 1, time.Hour) // org 1

	// Forge: keep the real exp and sig but replace orgid with 2.
	// Format: "uid:orgid|exp|sig"
	import_parts := splitToken(tok)
	if import_parts == nil {
		t.Fatal("unexpected token format")
	}
	forged := "1:2|" + import_parts[1] + "|" + import_parts[2] // steal exp+sig from org-1 token
	if _, _, ok := m.validate(forged); ok {
		t.Fatal("validate accepted a token with tampered orgID")
	}
}

// splitToken splits "uid:orgid|exp|sig" into ["uid:orgid", "exp", "sig"].
func splitToken(tok string) []string {
	parts := make([]string, 0, 3)
	rest := tok
	for i := 0; i < 2; i++ {
		idx := len(rest) - 1
		for idx >= 0 && rest[idx] != '|' {
			idx--
		}
		if idx < 0 {
			return nil
		}
		parts = append([]string{rest[idx+1:]}, parts...)
		rest = rest[:idx]
	}
	parts = append([]string{rest}, parts...)
	return parts
}

func TestAuthedReadsCookie(t *testing.T) {
	m := testManager(t)
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
	m := testManager(t)
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
	m := testManager(t)
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
