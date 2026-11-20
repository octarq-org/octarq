package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"
	"github.com/octarq-org/octarq/server/config"
	"github.com/octarq-org/octarq/server/internal/crypto"
	"github.com/octarq-org/octarq/server/internal/models"
	"gorm.io/gorm"
)

func oauthPreparedHandler(t *testing.T) (*OAuthHandler, *gorm.DB) {
	t.Helper()
	db := identityDB(t)
	cfg := &config.Config{SecretKey: "secret"}
	cipher := crypto.New("secret")
	if err := cipher.EnableEnvelope(testEnvStore{db}); err != nil {
		t.Fatalf("EnableEnvelope: %v", err)
	}
	if err := db.Create(&models.Setting{Key: "oauth.google.client_id", Value: "google-id"}).Error; err != nil {
		t.Fatalf("client_id: %v", err)
	}
	m := New(cfg, cipher).WithDB(db)
	vs := &testAuthVault{secrets: map[string]string{"instance.oauth.google": "google-secret"}}
	return NewOAuthHandler(db, m, vs), db
}

func overrideCompleteUserAuth(t *testing.T, fn func(w http.ResponseWriter, r *http.Request) (goth.User, error)) {
	t.Helper()
	original := gothic.CompleteUserAuth
	gothic.CompleteUserAuth = fn
	t.Cleanup(func() { gothic.CompleteUserAuth = original })
}

func callbackRequest() *http.Request {
	r := httptest.NewRequest(http.MethodGet, "http://localhost/auth/callback/google?state=st", nil)
	r.SetPathValue("provider", "google")
	return r
}

func seedUserOrgs(t *testing.T, db *gorm.DB, email string) (models.User, models.Org) {
	t.Helper()
	u := models.User{Email: email, EmailVerified: true}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("user: %v", err)
	}
	slug, err := models.AllocateOrgSlug(db)
	if err != nil {
		t.Fatalf("slug: %v", err)
	}
	o := models.Org{Name: email, Slug: slug, InboundToken: "tok"}
	if err := db.Create(&o).Error; err != nil {
		t.Fatalf("org: %v", err)
	}
	if err := db.Create(&models.OrgMember{OrgID: o.ID, UserID: u.ID, Role: "owner"}).Error; err != nil {
		t.Fatalf("member: %v", err)
	}
	return u, o
}

func TestOAuthCallbackIssuesSession(t *testing.T) {
	h, db := oauthPreparedHandler(t)
	u, o := seedUserOrgs(t, db, "alice@example.com")

	var gotName string
	overrideCompleteUserAuth(t, func(w http.ResponseWriter, r *http.Request) (goth.User, error) {
		gotName = r.PathValue("provider")
		return goth.User{Email: "ALICE@Example.com"}, nil
	})

	rec := httptest.NewRecorder()
	h.Callback(rec, callbackRequest())

	if rec.Code != http.StatusFound {
		t.Fatalf("Callback = %d, want 302", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/admin/" {
		t.Errorf("Location = %q, want /admin/", loc)
	}
	if !strings.HasPrefix(gotName, "google@") {
		t.Errorf("provider name = %q, want a per-origin google name", gotName)
	}
	if cookie := sessionCookie(t, rec); cookie == "" {
		t.Error("no session cookie on successful callback")
	}
	var sessions int64
	db.Model(&models.Session{}).Where("user_id = ? AND org_id = ?", u.ID, o.ID).Count(&sessions)
	if sessions == 0 {
		t.Error("callback issued no session row")
	}
}

func TestOAuthCallbackTOTPRedirectsToSecondFactor(t *testing.T) {
	h, db := oauthPreparedHandler(t)
	u := mustUser(t, db, "totp@example.com")
	if err := db.Model(&u).Update("totp_enabled", true).Error; err != nil {
		t.Fatalf("enable totp: %v", err)
	}

	overrideCompleteUserAuth(t, func(w http.ResponseWriter, r *http.Request) (goth.User, error) {
		return goth.User{Email: "totp@example.com"}, nil
	})

	rec := httptest.NewRecorder()
	h.Callback(rec, callbackRequest())

	if rec.Code != http.StatusFound {
		t.Fatalf("Callback = %d, want 302", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/admin/?twofa=1" {
		t.Errorf("Location = %q, want /admin/?twofa=1", loc)
	}
	var challenge *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == twofaChallengeCookie {
			challenge = c
		}
	}
	if challenge == nil { //nolint:staticcheck // SA5011 false positive: t.Fatal above is noreturn
		t.Fatal("no twofa challenge cookie set")
		return
	}
	if uid := h.auth.VerifyTwoFAChallenge(challenge.Value); uid != u.ID { //nolint:staticcheck // SA5011 false positive: t.Fatal above is noreturn
		t.Errorf("challenge names user %d, want %d", uid, u.ID)
	}
}

func TestOAuthCallbackFailures(t *testing.T) {
	h, _ := oauthPreparedHandler(t)

	overrideCompleteUserAuth(t, func(w http.ResponseWriter, r *http.Request) (goth.User, error) {
		return goth.User{}, errors.New("provider refused")
	})
	rec := httptest.NewRecorder()
	h.Callback(rec, callbackRequest())
	if rec.Code != http.StatusBadRequest {
		t.Errorf("provider error = %d, want 400", rec.Code)
	}

	overrideCompleteUserAuth(t, func(w http.ResponseWriter, r *http.Request) (goth.User, error) {
		return goth.User{Email: "   "}, nil
	})
	rec = httptest.NewRecorder()
	h.Callback(rec, callbackRequest())
	if rec.Code != http.StatusBadRequest {
		t.Errorf("empty email = %d, want 400", rec.Code)
	}
}

func TestOAuthCallbackInviteOnly(t *testing.T) {
	h, db := oauthPreparedHandler(t)
	if err := db.Create(&models.Setting{Key: "allow_registration", Value: "false"}).Error; err != nil {
		t.Fatalf("setting: %v", err)
	}

	overrideCompleteUserAuth(t, func(w http.ResponseWriter, r *http.Request) (goth.User, error) {
		return goth.User{Email: "stranger@example.com"}, nil
	})
	rec := httptest.NewRecorder()
	h.Callback(rec, callbackRequest())
	if rec.Code != http.StatusForbidden {
		t.Errorf("invite-only callback = %d, want 403", rec.Code)
	}
}

func TestOAuthBeginRedirects(t *testing.T) {
	h, _ := oauthPreparedHandler(t)
	InitGothStore("secret")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/auth/begin/google", nil)
	req.SetPathValue("provider", "google")
	h.Begin(rec, req)

	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("Begin = %d, want %d", rec.Code, http.StatusTemporaryRedirect)
	}
	if loc := rec.Header().Get("Location"); !strings.HasPrefix(loc, "https://accounts.google.com") {
		t.Errorf("Begin Location = %q, want a google auth URL", loc)
	}
}

func TestLoadProviderEdgeCases(t *testing.T) {
	db := identityDB(t)
	cfg := &config.Config{SecretKey: "secret"}
	cipher := crypto.New("secret")
	if err := cipher.EnableEnvelope(testEnvStore{db}); err != nil {
		t.Fatalf("EnableEnvelope: %v", err)
	}
	vs := &testAuthVault{secrets: map[string]string{}}
	m := New(cfg, cipher).WithDB(db)
	h := NewOAuthHandler(db, m, vs)
	if err := db.Create(&models.Setting{Key: "oauth.google.client_id", Value: "id"}).Error; err != nil {
		t.Fatalf("client_id: %v", err)
	}

	loadedCredsBackup := loadedCreds
	loadedCreds = map[string]string{}
	t.Cleanup(func() { loadedCreds = loadedCredsBackup })

	ctx := context.Background()
	if loaded := h.loadProvider(ctx, "google", "g@http://x", "http://x"); loaded {
		t.Error("loadProvider succeeded without a client secret")
	}

	vs.err = errors.New("vault unavailable")
	if loaded := h.loadProvider(ctx, "google", "g@http://x", "http://x"); loaded {
		t.Error("loadProvider succeeded on vault error")
	}
	vs.err = nil

	if loaded := h.loadProvider(ctx, "twitter", "t@http://x", "http://x"); loaded {
		t.Error("loadProvider accepted an unknown provider")
	}

	vs.secrets["instance.oauth.google"] = "real-secret"
	if !h.loadProvider(ctx, "google", "g@http://x", "http://x") {
		t.Fatal("loadProvider rejected a valid secret")
	}
	if !h.loadProvider(ctx, "google", "g@http://x", "http://x") {
		t.Fatal("cached loadProvider rejected a valid secret")
	}
}

func TestOAuthCallbackRejectsWhenSSODomainEnabled(t *testing.T) {
	h, db := oauthPreparedHandler(t)
	type SSOConfig struct {
		ID            string `gorm:"primaryKey"`
		OrgID         string
		AllowedDomain string
		Enabled       bool
	}
	if err := db.AutoMigrate(&SSOConfig{}); err != nil {
		t.Fatalf("migrate sso_configs: %v", err)
	}
	if err := db.Table("sso_configs").Create(&SSOConfig{
		ID:            "sso-1",
		OrgID:         "org-corp",
		AllowedDomain: "acme.corp",
		Enabled:       true,
	}).Error; err != nil {
		t.Fatalf("create sso config: %v", err)
	}

	seedUserOrgs(t, db, "bob@acme.corp")

	overrideCompleteUserAuth(t, func(w http.ResponseWriter, r *http.Request) (goth.User, error) {
		return goth.User{Email: "bob@Acme.corp"}, nil
	})

	rec := httptest.NewRecorder()
	h.Callback(rec, callbackRequest())

	if rec.Code != http.StatusForbidden {
		t.Fatalf("Callback = %d, want 403 Forbidden", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "managed by Enterprise SSO") {
		t.Errorf("expected rejection message, got: %s", rec.Body.String())
	}
}
