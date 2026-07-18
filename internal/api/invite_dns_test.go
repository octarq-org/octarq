package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/config"
	"github.com/octarq-org/octarq/internal/auth"
	"github.com/octarq-org/octarq/internal/crypto"
	"github.com/octarq-org/octarq/internal/geo"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/internal/queue"
	"gorm.io/gorm"
)

func newTestHandlerWithInstance(t *testing.T) (*Handler, http.Handler, *gorm.DB) {
	t.Helper()
	dbName := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.AllModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	db.Where("1 = 1").Delete(&models.Token{})
	db.Where("1 = 1").Delete(&models.Link{})

	cfg := &config.Config{AdminUser: "admin", AdminPassword: "pw", SecretKey: "secret"}
	cipher := crypto.New(cfg.SecretKey)
	if err := cipher.EnableEnvelope(apiEnvStore{db}); err != nil {
		t.Fatalf("EnableEnvelope: %v", err)
	}
	authMgr := auth.New(cfg, cipher).WithDB(db)
	g, _ := geo.Open("")
	h := New(cfg, db, cipher, authMgr, g, queue.New(""))
	srv := h.Routes()
	mountCoreDNS(h, db, authMgr, cipher)
	mountCoreMail(h, db, authMgr, cipher)
	return h, srv, db
}

func TestInviteFlow(t *testing.T) {
	srv, db := newTestHandler(t)
	const orgID = uint(1)

	// Create an admin user first to manage members.
	adminUID := seedOrgMember(t, db, orgID, "admin@example.com", "owner")
	adminSession := sessionCookies(t, adminUID, orgID)

	// Invite a new user
	email := t.Name() + "+newbie@example.com"
	rec := do(srv, "POST", "/api/org/members", adminSession, fmt.Sprintf(`{"email":%q,"role":"member"}`, email))
	if rec.Code != http.StatusOK {
		t.Fatalf("addOrgMember failed: %s", rec.Body.String())
	}

	var res struct {
		Ok          bool   `json:"ok"`
		InviteToken string `json:"inviteToken"`
		InviteUrl   string `json:"inviteUrl"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !res.Ok {
		t.Fatal("response ok is not true")
	}
	if res.InviteToken == "" {
		t.Fatal("expected invite token, got empty")
	}
	expectedUrl := "/admin/invite/accept?token=" + res.InviteToken
	if res.InviteUrl != expectedUrl {
		t.Fatalf("expected invite url %q, got %q", expectedUrl, res.InviteUrl)
	}

	// Verify database record has token and expiry
	var user models.User
	if err := db.Where("email = ?", t.Name()+"+newbie@example.com").First(&user).Error; err != nil {
		t.Fatalf("user not found in db: %v", err)
	}
	if user.InviteToken != res.InviteToken {
		t.Fatalf("db token %q does not match response token %q", user.InviteToken, res.InviteToken)
	}
	if user.InviteExpiresAt == nil {
		t.Fatal("db invite expires at is nil")
	}
	if user.InviteExpiresAt.Before(time.Now()) {
		t.Fatal("db invite expires at is in the past")
	}

	// Try to accept invite with empty token -> should fail
	recAcceptEmpty := do(srv, "POST", "/api/auth/invite/accept", nil, `{"token":"","password":"newpassword"}`)
	if recAcceptEmpty.Code != http.StatusBadRequest {
		t.Errorf("accept empty token: got status %d, want 400", recAcceptEmpty.Code)
	}

	// Try to accept invite with bad token -> should fail
	recAcceptBad := do(srv, "POST", "/api/auth/invite/accept", nil, `{"token":"badtoken","password":"newpassword"}`)
	if recAcceptBad.Code != http.StatusBadRequest {
		t.Errorf("accept bad token: got status %d, want 400", recAcceptBad.Code)
	}

	// Try to accept invite with expired token -> should fail
	expiredTime := time.Now().Add(-1 * time.Hour)
	db.Model(&user).Updates(map[string]any{
		"invite_expires_at": &expiredTime,
	})
	recAcceptExpired := do(srv, "POST", "/api/auth/invite/accept", nil, fmt.Sprintf(`{"token":%q,"password":"newpassword"}`, res.InviteToken))
	if recAcceptExpired.Code != http.StatusBadRequest {
		t.Errorf("accept expired token: got status %d, want 400", recAcceptExpired.Code)
	}

	// Reset expiration to future
	futureTime := time.Now().Add(24 * time.Hour)
	db.Model(&user).Updates(map[string]any{
		"invite_expires_at": &futureTime,
	})

	// Accept invite successfully
	recAcceptSuccess := do(srv, "POST", "/api/auth/invite/accept", nil, fmt.Sprintf(`{"token":%q,"password":"newpassword"}`, res.InviteToken))
	if recAcceptSuccess.Code != http.StatusOK {
		t.Fatalf("accept invite success: got status %d, body %s", recAcceptSuccess.Code, recAcceptSuccess.Body.String())
	}

	// Check user record is updated: password hashed, token/expiry cleared
	var updatedUser models.User
	if err := db.First(&updatedUser, user.ID).Error; err != nil {
		t.Fatalf("failed to query updated user: %v", err)
	}
	if updatedUser.PasswordHash == "" {
		t.Fatal("expected password hash, got empty")
	}
	if updatedUser.InviteToken != "" {
		t.Fatalf("expected invite token to be cleared, got %q", updatedUser.InviteToken)
	}
	if updatedUser.InviteExpiresAt != nil {
		t.Fatal("expected invite expiration to be nil")
	}
}

// TestVerifyDNSMailHosts checks that verification probes each enabled mail host
// (including subdomains) rather than only the apex.

// TestVerifyDNSLinkHosts checks CNAME resolution for enabled short-link hosts.
