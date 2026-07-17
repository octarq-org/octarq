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

// TestVerifyDNSMailHosts checks that verification probes each enabled mail host
// (including subdomains) rather than only the apex.
func TestVerifyDNSMailHosts(t *testing.T) {
	h, srv, db := newTestHandlerWithInstance(t)
	const orgID = uint(1)
	adminUID := seedOrgMember(t, db, orgID, "admin@example.com", "owner")
	adminSession := sessionCookies(t, adminUID, orgID)

	dom := models.Domain{
		OrgID:   orgID,
		Name:    "mytestdomain.com",
		ForMail: true,
		MailHosts: models.HostList{
			{Host: "mytestdomain.com", Enabled: true},
			{Host: "mail.mytestdomain.com", Enabled: true},
			{Host: "old.mytestdomain.com", Enabled: false}, // disabled — must be skipped
		},
	}
	if err := db.Create(&dom).Error; err != nil {
		t.Fatalf("failed to create domain: %v", err)
	}

	// Apex is healthy; the subdomain has SPF only.
	h.lookupTXT = func(name string) ([]string, error) {
		switch name {
		case "mytestdomain.com":
			return []string{"v=spf1 include:_spf.google.com ~all"}, nil
		case "_dmarc.mytestdomain.com":
			return []string{"v=DMARC1; p=reject"}, nil
		case "default._domainkey.mytestdomain.com":
			return []string{"v=DKIM1; k=rsa; p=MIIBIjANBg"}, nil
		case "mail.mytestdomain.com":
			return []string{"v=spf1 -all"}, nil
		case "old.mytestdomain.com":
			t.Errorf("disabled host old.mytestdomain.com should not be probed")
			return nil, fmt.Errorf("should not happen")
		default:
			return nil, fmt.Errorf("dns lookup failed")
		}
	}

	rec := do(srv, "GET", fmt.Sprintf("/api/domains/%d/verify-dns", dom.ID), adminSession, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("verify-dns failed: status %d, body %s", rec.Code, rec.Body.String())
	}

	var res struct {
		Spf   struct{ Set, Healthy bool } `json:"spf"`
		Hosts []struct {
			Host  string                      `json:"host"`
			Spf   struct{ Set, Healthy bool } `json:"spf"`
			Dmarc struct{ Set, Healthy bool } `json:"dmarc"`
		} `json:"hosts"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(res.Hosts) != 2 {
		t.Fatalf("expected 2 enabled mail hosts, got %d: %+v", len(res.Hosts), res.Hosts)
	}
	// Top-level mirrors the apex.
	if !res.Spf.Set || !res.Spf.Healthy {
		t.Errorf("top-level SPF should mirror healthy apex: %+v", res.Spf)
	}

	byHost := map[string]struct{ SPFHealthy, DMARCSet bool }{}
	for _, hh := range res.Hosts {
		byHost[hh.Host] = struct{ SPFHealthy, DMARCSet bool }{hh.Spf.Healthy, hh.Dmarc.Set}
	}
	if apex := byHost["mytestdomain.com"]; !apex.SPFHealthy || !apex.DMARCSet {
		t.Errorf("apex host wrong: %+v", apex)
	}
	if sub := byHost["mail.mytestdomain.com"]; !sub.SPFHealthy || sub.DMARCSet {
		t.Errorf("subdomain host should have SPF but no DMARC: %+v", sub)
	}
}

// TestVerifyDNSLinkHosts checks CNAME resolution for enabled short-link hosts.
func TestVerifyDNSLinkHosts(t *testing.T) {
	h, srv, db := newTestHandlerWithInstance(t)
	const orgID = uint(1)
	adminUID := seedOrgMember(t, db, orgID, "admin@example.com", "owner")
	adminSession := sessionCookies(t, adminUID, orgID)

	dom := models.Domain{
		OrgID:   orgID,
		Name:    "mytestdomain.com",
		ForLink: true,
		LinkHosts: models.HostList{
			{Host: "go.mytestdomain.com", Enabled: true},   // CNAME into zone → healthy
			{Host: "s.mytestdomain.com", Enabled: true},    // proxied/A-only → set, unverified
			{Host: "dead.mytestdomain.com", Enabled: true}, // NXDOMAIN → not set
			{Host: "off.mytestdomain.com", Enabled: false}, // disabled → skipped
		},
	}
	if err := db.Create(&dom).Error; err != nil {
		t.Fatalf("failed to create domain: %v", err)
	}

	h.lookupTXT = func(string) ([]string, error) { return nil, fmt.Errorf("no txt") }
	h.lookupCNAME = func(name string) (string, error) {
		switch name {
		case "go.mytestdomain.com":
			return "mytestdomain.com.", nil
		case "s.mytestdomain.com":
			return "s.mytestdomain.com.", nil // flattened/A-only: resolves to self
		case "off.mytestdomain.com":
			t.Errorf("disabled link host should not be probed")
			return "", fmt.Errorf("should not happen")
		default:
			return "", fmt.Errorf("no such host")
		}
	}

	rec := do(srv, "GET", fmt.Sprintf("/api/domains/%d/verify-dns", dom.ID), adminSession, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("verify-dns failed: status %d, body %s", rec.Code, rec.Body.String())
	}

	var res struct {
		Links []struct {
			Host    string `json:"host"`
			Set     bool   `json:"set"`
			Healthy bool   `json:"healthy"`
			CNAME   string `json:"cname"`
		} `json:"links"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(res.Links) != 3 {
		t.Fatalf("expected 3 enabled link hosts, got %d: %+v", len(res.Links), res.Links)
	}
	byHost := map[string]struct {
		Set, Healthy bool
		CNAME        string
	}{}
	for _, l := range res.Links {
		byHost[l.Host] = struct {
			Set, Healthy bool
			CNAME        string
		}{l.Set, l.Healthy, l.CNAME}
	}
	if g := byHost["go.mytestdomain.com"]; !g.Set || !g.Healthy || g.CNAME != "mytestdomain.com" {
		t.Errorf("go host should be healthy CNAME into zone: %+v", g)
	}
	if s := byHost["s.mytestdomain.com"]; !s.Set || s.Healthy {
		t.Errorf("s host should resolve but be unverified: %+v", s)
	}
	if d := byHost["dead.mytestdomain.com"]; d.Set || d.Healthy {
		t.Errorf("dead host should not resolve: %+v", d)
	}
}
