package auth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/config"
	"github.com/octarq-org/octarq/server/internal/crypto"
	"github.com/octarq-org/octarq/server/internal/models"
)

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

	handler := NewOAuthHandler(db, m, cipher)

	const workers = 10
	done := make(chan bool, workers)
	for i := 0; i < workers; i++ {
		go func() {
			ok := handler.loadProvider("google", "google@http://localhost", "http://localhost")
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
	handler := NewOAuthHandler(db, m, crypto.New("secret"))

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

	u, o, err := handler.upsertUser("alice@example.com")
	if err != nil {
		t.Fatalf("upsertUser failed: %v", err)
	}
	if u.Email != "alice@example.com" {
		t.Errorf("user email = %q, want alice@example.com", u.Email)
	}
	if o.Name != "alice@example.com" {
		t.Errorf("org mismatch: %+v", o)
	}
	// The slug is allocated at random, never derived: it lands in public URLs
	// (the billing webhook path), so it must not spell out the founder's address.
	if o.Slug == "" || strings.Contains(o.Slug, "alice") || strings.Contains(o.Slug, "example") {
		t.Errorf("org slug %q leaks the founder's email", o.Slug)
	}

	u2, o2, err := handler.upsertUser("alice@example.com")
	if err != nil {
		t.Fatalf("upsertUser existing failed: %v", err)
	}
	if u2.ID != u.ID || o2.ID != o.ID {
		t.Errorf("upsertUser existing did not return same user/org")
	}
}

// TestOAuthUpsertRespectsRegistrationGate verifies OAuth can't provision a
// brand-new account when public registration is disabled, but still lets an
// already-provisioned user sign in.
func TestOAuthUpsertRespectsRegistrationGate(t *testing.T) {
	db := testDB(t)
	cfg := &config.Config{SecretKey: "secret"}
	m := New(cfg, crypto.New("secret")).WithDB(db)
	handler := NewOAuthHandler(db, m, crypto.New("secret"))

	// Turn registration off (invite-only instance).
	if err := db.Create(&models.Setting{Key: "allow_registration", Value: "false"}).Error; err != nil {
		t.Fatalf("seed setting: %v", err)
	}

	// Unknown email → refused with the sentinel error, no account created.
	if _, _, err := handler.upsertUser("stranger@example.com"); !errors.Is(err, ErrRegistrationDisabled) {
		t.Fatalf("expected ErrRegistrationDisabled for unknown email, got %v", err)
	}
	var count int64
	db.Model(&models.User{}).Where("email = ?", "stranger@example.com").Count(&count)
	if count != 0 {
		t.Errorf("stranger account should not have been created, found %d", count)
	}

	// A pre-existing user still resolves even while registration is off.
	existing := models.User{Email: "member@example.com"}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	org := models.Org{Name: "member@example.com", Slug: "member-example-com", InboundToken: "tok"}
	db.Create(&org)
	db.Create(&models.OrgMember{OrgID: org.ID, UserID: existing.ID, Role: "owner"})

	u, _, err := handler.upsertUser("member@example.com")
	if err != nil {
		t.Fatalf("existing user should sign in despite gate: %v", err)
	}
	if u.ID != existing.ID {
		t.Errorf("resolved wrong user: got %d want %d", u.ID, existing.ID)
	}
}
