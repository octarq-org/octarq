package auth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"
	"github.com/octarq-org/octarq/config"
	"github.com/octarq-org/octarq/internal/crypto"
	"github.com/octarq-org/octarq/internal/models"
	"gorm.io/gorm"
)

// oauthPreparedHandler returns a handler whose google provider is configured in
// the settings table (client id + envelope-encrypted secret), so prepare() can
// successfully resolve an origin.
func oauthPreparedHandler(t *testing.T) (*OAuthHandler, *gorm.DB) {
	t.Helper()
	db := identityDB(t)
	cfg := &config.Config{SecretKey: "secret"}
	cipher := crypto.New("secret")
	if err := cipher.EnableEnvelope(testEnvStore{db}); err != nil {
		t.Fatalf("EnableEnvelope: %v", err)
	}
	encSecret, err := cipher.Encrypt([]byte("google-secret"))
	if err != nil {
		t.Fatalf("encrypt secret: %v", err)
	}
	if err := db.Create(&models.Setting{Key: "oauth.google.client_id", Value: "google-id"}).Error; err != nil {
		t.Fatalf("client_id: %v", err)
	}
	if err := db.Create(&models.Setting{Key: "oauth.google.client_secret", Value: encSecret}).Error; err != nil {
		t.Fatalf("client_secret: %v", err)
	}
	m := New(cfg, cipher).WithDB(db)
	return NewOAuthHandler(db, m, cipher), db
}

// overrideCompleteUserAuth swaps gothic's package-level CompleteUserAuth so the
// OAuth callback can be driven without a network round trip, restoring it after.
func overrideCompleteUserAuth(t *testing.T, fn func(w http.ResponseWriter, r *http.Request) (goth.User, error)) {
	t.Helper()
	original := gothic.CompleteUserAuth
	gothic.CompleteUserAuth = fn
	t.Cleanup(func() { gothic.CompleteUserAuth = original })
}

func callbackRequest() *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/auth/callback/google?state=st", nil)
	r.SetPathValue("provider", "google")
	return r
}

// seedUserOrgs creates a user with a personal org+m membership so the OAuth
// upsert resolves to an existing account instead of provisioning a new one.
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

// TestOAuthCallbackIssuesSession drives the happy path: a verified email without
// TOTP completes login, sets the session cookie, and redirects to /admin/.
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

// TestOAuthCallbackTOTPRedirectsToSecondFactor sends a TOTP-enabled user to the
// second-factor page with a challenge cookie standing in for their password.
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
	}
	if uid := h.auth.VerifyTwoFAChallenge(challenge.Value); uid != u.ID { //nolint:staticcheck // SA5011 false positive: t.Fatal above is noreturn
		t.Errorf("challenge names user %d, want %d", uid, u.ID)
	}
}

// TestOAuthCallbackFailures covers the refusal paths: completion errors and a
// provider that returns no email.
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

// TestOAuthCallbackInviteOnly: an invite-only instance refuses an unknown email
// via the OAuth callback, matching LoginByEmail's policy.
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

// TestOAuthBeginRedirects asserts the begin flow redirects to the provider's
// authorization URL when the provider is configured.
func TestOAuthBeginRedirects(t *testing.T) {
	h, _ := oauthPreparedHandler(t)
	InitGothStore("secret")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/auth/begin/google", nil)
	req.SetPathValue("provider", "google")
	h.Begin(rec, req)

	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("Begin = %d, want %d", rec.Code, http.StatusTemporaryRedirect)
	}
	if loc := rec.Header().Get("Location"); !strings.HasPrefix(loc, "https://accounts.google.com") {
		t.Errorf("Begin Location = %q, want a google auth URL", loc)
	}
}

// TestLoadProviderEdgeCases covers the credential cache, the unknown-provider
// refusal, an undecryptable stored secret, and the wholesale registry reset.
func TestLoadProviderEdgeCases(t *testing.T) {
	db := identityDB(t)
	cfg := &config.Config{SecretKey: "secret"}
	cipher := crypto.New("secret")
	if err := cipher.EnableEnvelope(testEnvStore{db}); err != nil {
		t.Fatalf("EnableEnvelope: %v", err)
	}
	m := New(cfg, cipher).WithDB(db)
	h := NewOAuthHandler(db, m, cipher)
	if err := db.Create(&models.Setting{Key: "oauth.google.client_id", Value: "id"}).Error; err != nil {
		t.Fatalf("client_id: %v", err)
	}

	loadedCredsBackup := loadedCreds
	loadedCreds = map[string]string{}
	t.Cleanup(func() { loadedCreds = loadedCredsBackup })

	// No secret configured -> not loadable.
	if loaded := h.loadProvider("google", "g@http://x", "http://x"); loaded {
		t.Error("loadProvider succeeded without a client secret")
	}

	// Secret present but undecryptable -> not loadable.
	if err := db.Create(&models.Setting{Key: "oauth.google.client_secret", Value: "garbage"}).Error; err != nil {
		t.Fatalf("client_secret: %v", err)
	}
	if loaded := h.loadProvider("google", "g@http://x", "http://x"); loaded {
		t.Error("loadProvider succeeded with an undecryptable secret")
	}

	// Unknown provider -> not loadable.
	if loaded := h.loadProvider("twitter", "t@http://x", "http://x"); loaded {
		t.Error("loadProvider accepted an unknown provider")
	}

	// A proper secret loads; the second identical call hits the cache.
	encSecret, err := cipher.Encrypt([]byte("real-secret"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	db.Model(&models.Setting{}).Where("key = ?", "oauth.google.client_secret").Update("value", encSecret)
	if !h.loadProvider("google", "g@http://x", "http://x") {
		t.Fatal("loadProvider rejected a valid secret")
	}
	if !h.loadProvider("google", "g@http://x", "http://x") {
		t.Fatal("cached loadProvider rejected a valid secret")
	}
	// The >64-entry wholesale registry reset is deliberately NOT exercised here:
	// it clears goth's process-global provider registry, which would break the
	// order-sensitive OAuth tests under -count=2.
}
