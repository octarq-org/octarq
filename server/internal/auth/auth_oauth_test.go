package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/config"
	"github.com/octarq-org/octarq/server/internal/crypto"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
)

type testAuthVault struct {
	secrets map[string]string
	err     error
}

func (v *testAuthVault) PutSecret(context.Context, plugin.PutSecretRequest) (*plugin.VaultItemMeta, error) {
	return nil, nil
}
func (v *testAuthVault) UpdateSecret(context.Context, uint, uint, *string, map[string]any, map[string]any, *bool, string, uint) (*plugin.VaultItemMeta, error) {
	return nil, nil
}
func (v *testAuthVault) GetMeta(context.Context, uint, uint) (*plugin.VaultItemMeta, error) {
	return nil, nil
}
func (v *testAuthVault) GetSecret(context.Context, uint, uint) (*plugin.VaultSecret, error) {
	return nil, nil
}
func (v *testAuthVault) ResolveByRef(context.Context, uint, string) (*plugin.VaultSecret, error) {
	return nil, nil
}
func (v *testAuthVault) ResolveInstanceSecret(_ context.Context, keyName string) (*plugin.VaultSecret, error) {
	if v.err != nil {
		return nil, v.err
	}
	s, ok := v.secrets[keyName]
	if !ok {
		return nil, plugin.ErrSecretNotFound
	}
	return &plugin.VaultSecret{
		Meta:    plugin.VaultItemMeta{KeyName: keyName, Scope: plugin.VaultScopeInstance},
		Payload: map[string]any{"clientSecret": s},
	}, nil
}
func (v *testAuthVault) PutInstanceSecret(context.Context, plugin.PutSecretRequest) (*plugin.VaultItemMeta, error) {
	return nil, nil
}
func (v *testAuthVault) DeleteInstanceSecret(context.Context, string) error { return nil }
func (v *testAuthVault) ListItems(context.Context, uint, plugin.VaultFilter) ([]plugin.VaultItemMeta, error) {
	return nil, nil
}
func (v *testAuthVault) DeleteSecret(context.Context, uint, uint) error { return nil }
func (v *testAuthVault) ComputeTOTP(context.Context, uint, uint) (string, int, error) {
	return "", 0, nil
}

func TestOAuthLoadProviderConcurrency(t *testing.T) {
	db := testDB(t)
	cfg := &config.Config{SecretKey: "secret"}
	cipher := crypto.New("secret")
	if err := cipher.EnableEnvelope(testEnvStore{db}); err != nil {
		t.Fatalf("EnableEnvelope: %v", err)
	}
	m := New(cfg, cipher).WithDB(db)

	db.Create(&models.Setting{Key: "oauth.google.client_id", Value: "google-id"})
	vs := &testAuthVault{secrets: map[string]string{"instance.oauth.google": "google-secret"}}

	handler := NewOAuthHandler(db, m, vs)

	const workers = 10
	done := make(chan bool, workers)
	for i := 0; i < workers; i++ {
		go func() {
			ok := handler.loadProvider(context.Background(), "google", "google@http://localhost", "http://localhost")
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
	handler := NewOAuthHandler(db, m, nil)

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

func TestOAuthUpsertRespectsRegistrationGate(t *testing.T) {
	db := testDB(t)
	cfg := &config.Config{SecretKey: "secret"}
	m := New(cfg, crypto.New("secret")).WithDB(db)
	handler := NewOAuthHandler(db, m, nil)

	if err := db.Create(&models.Setting{Key: "allow_registration", Value: "false"}).Error; err != nil {
		t.Fatalf("seed setting: %v", err)
	}

	if _, _, err := handler.upsertUser("stranger@example.com"); !errors.Is(err, ErrRegistrationDisabled) {
		t.Fatalf("expected ErrRegistrationDisabled for unknown email, got %v", err)
	}
	var count int64
	db.Model(&models.User{}).Where("email = ?", "stranger@example.com").Count(&count)
	if count != 0 {
		t.Errorf("stranger account should not have been created, found %d", count)
	}

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
