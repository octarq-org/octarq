package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/server/config"
	"github.com/octarq-org/octarq/server/internal/cache"
	"github.com/octarq-org/octarq/server/internal/crypto"
	"github.com/octarq-org/octarq/server/internal/models"
	"gorm.io/gorm"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "auth_char_test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite test db: %v", err)
	}
	if err := db.AutoMigrate(&models.Session{}, &models.Token{}); err != nil {
		t.Fatalf("failed to auto migrate: %v", err)
	}
	return db
}

func newTestManager(t *testing.T, db *gorm.DB) *Manager {
	t.Helper()
	cfg := &config.Config{
		AdminUser:     "admin@example.com",
		AdminPassword: "secure-admin-password-123",
		SecretKey:     "test-secret-key-that-is-at-least-32-bytes-long",
	}
	m := New(cfg, crypto.New(cfg.SecretKey)).WithDB(db).WithCache(cache.New(""))
	return m
}

func TestCharacterization_AdminVerification(t *testing.T) {
	m := newTestManager(t, nil)

	if !m.Check("admin@example.com", "secure-admin-password-123") {
		t.Fatal("expected admin check to succeed with correct credentials")
	}
	if m.Check("admin@example.com", "wrong-password") {
		t.Fatal("expected admin check to fail with wrong password")
	}
	if m.Check("wrong@example.com", "secure-admin-password-123") {
		t.Fatal("expected admin check to fail with wrong username")
	}

	if !m.CheckAdminPassword("secure-admin-password-123") {
		t.Fatal("expected CheckAdminPassword to succeed")
	}
	if m.CheckAdminPassword("wrong") {
		t.Fatal("expected CheckAdminPassword to fail for wrong password")
	}

	if !m.IsConfigAdmin("admin@example.com") {
		t.Fatal("expected IsConfigAdmin to return true for configured admin")
	}
	if m.IsConfigAdmin("user@example.com") {
		t.Fatal("expected IsConfigAdmin to return false for regular user")
	}
}

func TestCharacterization_ContextHelpers(t *testing.T) {
	ctx := context.Background()

	ctx = WithUserID(ctx, 42)
	if got := UserIDFromContext(ctx); got != 42 {
		t.Fatalf("expected user ID 42, got %d", got)
	}

	ctx = WithTokenID(ctx, 100)
	if got := TokenIDFromContext(ctx); got != 100 {
		t.Fatalf("expected token ID 100, got %d", got)
	}

	ctx = WithTokenRole(ctx, "admin")
	if got := TokenRoleFromContext(ctx); got != "admin" {
		t.Fatalf("expected token role 'admin', got %s", got)
	}
}

func TestCharacterization_SessionLifecycleAndRevocation(t *testing.T) {
	db := newTestDB(t)
	m := newTestManager(t, db)

	// 1. Establish session from request
	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/test", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	rec := httptest.NewRecorder()

	m.SetSessionFromRequest(req, rec, 10, 20)

	cookies := rec.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == cookieName {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("expected session cookie to be set")
	}

	// 2. Authenticate subsequent request
	authedReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/test", nil)
	authedReq.AddCookie(sessionCookie)

	if !m.Authed(authedReq) {
		t.Fatal("expected request to be authed")
	}
	if got := m.UserID(authedReq); got != 10 {
		t.Fatalf("expected UserID 10, got %d", got)
	}
	if got := m.OrgID(authedReq); got != 20 {
		t.Fatalf("expected OrgID 20, got %d", got)
	}

	// 3. Touch session
	m.TouchSession(authedReq)

	// 4. Revoke user org sessions
	revoked := m.RevokeUserOrgSessions(10, 20)
	if revoked != 1 {
		t.Fatalf("expected 1 session revoked, got %d", revoked)
	}
	if m.Authed(authedReq) {
		t.Fatal("expected session to be revoked and not authed")
	}
}

func TestCharacterization_TokenBearerAuth(t *testing.T) {
	db := newTestDB(t)
	m := newTestManager(t, db)

	rawToken := "oct_test_secret_token_12345"
	tok := models.Token{
		UserID: 15,
		OrgID:  25,
		Hash:   models.HashToken(rawToken),
		Role:   "viewer",
	}
	if err := db.Create(&tok).Error; err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/items", nil)
	req.Header.Set("Authorization", "Bearer "+rawToken)

	resolvedTok, ok := m.TokenByRequest(req)
	if !ok {
		t.Fatal("expected token to be resolved")
	}
	if resolvedTok.UserID != 15 || resolvedTok.OrgID != 25 {
		t.Fatalf("unexpected token resolved: %+v", resolvedTok)
	}

	if !m.APIAuthed(req) {
		t.Fatal("expected APIAuthed to return true for bearer token")
	}

	// Middleware check
	authedReq, ok := m.AuthenticateRequest(req)
	if !ok {
		t.Fatal("expected AuthenticateRequest to succeed")
	}
	if got := TokenIDFromContext(authedReq.Context()); got != tok.ID {
		t.Fatalf("expected token ID %d, got %d", tok.ID, got)
	}

	// Revoke token
	n := m.RevokeUserOrgTokens(15, 25)
	if n != 1 {
		t.Fatalf("expected 1 token revoked, got %d", n)
	}
	if _, ok := m.TokenByRequest(req); ok {
		t.Fatal("expected revoked token to fail lookup")
	}
}
