package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/server/internal/cache"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

func TestAuthContextAndCacheHelpers(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctx = plugin.WithOrgID(ctx, 5)
	ctx = WithUserID(ctx, 10)
	ctx = context.WithValue(ctx, tokenIDKey, uint(100))
	ctx = WithTokenRole(ctx, "admin")

	if plugin.OrgIDFromContext(ctx) != 5 {
		t.Errorf("OrgIDFromContext expected 5, got %d", plugin.OrgIDFromContext(ctx))
	}
	if TokenIDFromContext(ctx) != 100 {
		t.Errorf("TokenIDFromContext expected 100, got %d", TokenIDFromContext(ctx))
	}
	if TokenRoleFromContext(ctx) != "admin" {
		t.Errorf("TokenRoleFromContext expected admin, got %s", TokenRoleFromContext(ctx))
	}

	m := testManager(t)
	c := cache.New("")
	m = m.WithCache(c)
	if m.Cache() != c {
		t.Error("Cache() did not return expected cache instance")
	}

	if !m.IsConfigAdmin("admin") {
		t.Error("IsConfigAdmin expected true for admin")
	}
	if m.IsConfigAdmin("other_user") {
		t.Error("IsConfigAdmin expected false for other_user")
	}

	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.Session{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	m = m.WithDB(db)

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	if _, ok := m.AuthenticateRequest(req); ok {
		t.Error("expected ok=false for unauthenticated request")
	}

	rawToken := "test-session-token-12345"
	hashedToken := models.HashToken(rawToken)
	sess := models.Session{
		UserID:    1,
		OrgID:     1,
		Token:     hashedToken,
		ExpiresAt: time.Now().Add(time.Hour),
	}
	db.Create(&sess)

	reqCookie := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	reqCookie.AddCookie(&http.Cookie{Name: "octarq_session", Value: rawToken})

	reqAuth, ok := m.AuthenticateRequest(reqCookie)
	if !ok {
		t.Fatal("expected AuthenticateRequest to succeed")
	}

	if sessID := m.SessionID(reqAuth); sessID != sess.ID {
		t.Errorf("SessionID expected %d, got %d", sess.ID, sessID)
	}
	if uid := m.UserID(reqAuth); uid != 1 {
		t.Errorf("UserID expected 1, got %d", uid)
	}

	m.TouchSession(reqCookie)
	ip := reporterIP(reqCookie)
	if ip == "" {
		t.Error("reporterIP returned empty string")
	}

	if numRevoked := m.RevokeUserOrgSessions(1, 1); numRevoked != 1 {
		t.Errorf("RevokeUserOrgSessions expected 1, got %d", numRevoked)
	}
}

func TestTokenRoleIsReadableByPlugins(t *testing.T) {
	ctx := WithTokenRole(context.Background(), "admin")
	if got := plugin.RoleFromContext(ctx); got != "admin" {
		t.Fatalf("plugin.RoleFromContext = %q, want admin", got)
	}
	if got := plugin.RoleFromContext(context.Background()); got != "" {
		t.Fatalf("role on a bare context = %q, want empty", got)
	}
	if got := TokenRoleFromContext(ctx); got != "admin" {
		t.Fatalf("TokenRoleFromContext = %q, want admin", got)
	}
}
