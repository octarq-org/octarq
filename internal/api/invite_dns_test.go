package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Jungley8/led/config"
	"github.com/Jungley8/led/internal/auth"
	"github.com/Jungley8/led/internal/crypto"
	"github.com/Jungley8/led/internal/geo"
	"github.com/Jungley8/led/internal/models"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newTestHandlerWithInstance(t *testing.T) (*Handler, http.Handler, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
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
	h := New(cfg, db, cipher, authMgr, g)
	return h, h.Routes(), db
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

func TestVerifyDNS(t *testing.T) {
	h, srv, db := newTestHandlerWithInstance(t)
	const orgID = uint(1)
	adminUID := seedOrgMember(t, db, orgID, "admin@example.com", "owner")
	adminSession := sessionCookies(t, adminUID, orgID)

	dom := models.Domain{
		OrgID: orgID,
		Name:  "mytestdomain.com",
	}
	if err := db.Create(&dom).Error; err != nil {
		t.Fatalf("failed to create domain: %v", err)
	}

	// 1. Stub healthy records
	h.lookupTXT = func(name string) ([]string, error) {
		switch name {
		case "mytestdomain.com":
			return []string{"v=spf1 include:_spf.google.com ~all"}, nil
		case "_dmarc.mytestdomain.com":
			return []string{"v=DMARC1; p=reject; pct=100"}, nil
		case "default._domainkey.mytestdomain.com":
			return []string{"v=DKIM1; k=rsa; p=MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8A"}, nil
		default:
			return nil, fmt.Errorf("dns lookup failed")
		}
	}

	rec := do(srv, "GET", fmt.Sprintf("/api/domains/%d/verify-dns", dom.ID), adminSession, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("verify-dns request failed: got status %d, body %s", rec.Code, rec.Body.String())
	}

	var res struct {
		Spf struct {
			Set     bool   `json:"set"`
			Healthy bool   `json:"healthy"`
			Value   string `json:"value"`
		} `json:"spf"`
		Dmarc struct {
			Set     bool   `json:"set"`
			Healthy bool   `json:"healthy"`
			Value   string `json:"value"`
		} `json:"dmarc"`
		Dkim struct {
			Set      bool   `json:"set"`
			Healthy  bool   `json:"healthy"`
			Value    string `json:"value"`
			Selector string `json:"selector"`
		} `json:"dkim"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !res.Spf.Set || !res.Spf.Healthy || res.Spf.Value != "v=spf1 include:_spf.google.com ~all" {
		t.Errorf("SPF healthy check failed: %+v", res.Spf)
	}
	if !res.Dmarc.Set || !res.Dmarc.Healthy || res.Dmarc.Value != "v=DMARC1; p=reject; pct=100" {
		t.Errorf("DMARC healthy check failed: %+v", res.Dmarc)
	}
	if !res.Dkim.Set || !res.Dkim.Healthy || res.Dkim.Selector != "default" || !strings.Contains(res.Dkim.Value, "v=DKIM1") {
		t.Errorf("DKIM healthy check failed: %+v", res.Dkim)
	}

	// 2. Stub unhealthy records (missing/unhealthy)
	h.lookupTXT = func(name string) ([]string, error) {
		switch name {
		case "mytestdomain.com":
			// spf without v=spf1 prefix
			return []string{"spf1 ~all"}, nil
		case "_dmarc.mytestdomain.com":
			// dmarc without p=
			return []string{"v=DMARC1; pct=100"}, nil
		default:
			return nil, fmt.Errorf("dns lookup failed")
		}
	}

	recUnhealthy := do(srv, "GET", fmt.Sprintf("/api/domains/%d/verify-dns", dom.ID), adminSession, "")
	if recUnhealthy.Code != http.StatusOK {
		t.Fatalf("verify-dns unhealthy request failed: got status %d", recUnhealthy.Code)
	}

	var resUnhealthy struct {
		Spf struct {
			Set     bool `json:"set"`
			Healthy bool `json:"healthy"`
		} `json:"spf"`
		Dmarc struct {
			Set     bool `json:"set"`
			Healthy bool `json:"healthy"`
		} `json:"dmarc"`
		Dkim struct {
			Set     bool `json:"set"`
			Healthy bool `json:"healthy"`
		} `json:"dkim"`
	}

	if err := json.Unmarshal(recUnhealthy.Body.Bytes(), &resUnhealthy); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resUnhealthy.Spf.Set || resUnhealthy.Spf.Healthy {
		t.Errorf("SPF should not be set or healthy: %+v", resUnhealthy.Spf)
	}
	if !resUnhealthy.Dmarc.Set || resUnhealthy.Dmarc.Healthy {
		t.Errorf("DMARC should be set but not healthy: %+v", resUnhealthy.Dmarc)
	}
	if resUnhealthy.Dkim.Set || resUnhealthy.Dkim.Healthy {
		t.Errorf("DKIM should not be set or healthy: %+v", resUnhealthy.Dkim)
	}
}
