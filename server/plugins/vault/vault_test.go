package vault

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/server/internal/crypto"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type dbSecretStore struct{ db *gorm.DB }

func (s dbSecretStore) Get(key string) (string, bool) {
	var row models.Setting
	if s.db.First(&row, "key = ?", key).Error != nil {
		return "", false
	}
	return row.Value, true
}

func (s dbSecretStore) Set(key, val string) error {
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value"}),
	}).Create(&models.Setting{Key: key, Value: val}).Error
}

func (s dbSecretStore) Delete(key string) error {
	return s.db.Delete(&models.Setting{Key: key}).Error
}

func (s dbSecretStore) HasEncryptedValues() (bool, error) { return false, nil }

type realCipherVault struct{ c *crypto.Cipher }

func (r realCipherVault) Encrypt(p []byte) (string, error) { return r.c.Encrypt(p) }
func (r realCipherVault) Decrypt(s string) ([]byte, error) { return r.c.Decrypt(s) }
func (r realCipherVault) EncryptWithAAD(p, aad []byte) (string, error) {
	return r.c.EncryptWithAAD(p, aad)
}
func (r realCipherVault) DecryptWithAAD(s string, aad []byte) ([]byte, error) {
	return r.c.DecryptWithAAD(s, aad)
}
func (r realCipherVault) SubjectVault(ownerID uint, subject string) (plugin.CryptoVault, error) {
	sv, err := r.c.SubjectVault(ownerID, subject)
	if err != nil {
		return nil, err
	}
	return realSubjectVault{sv: sv}, nil
}
func (r realCipherVault) ShredSubjectKey(ownerID uint, subject string) error {
	return r.c.ShredSubjectKey(ownerID, subject)
}
func (r realCipherVault) HasSubjectKey(ownerID uint, subject string) bool {
	return r.c.HasSubjectKey(ownerID, subject)
}

type realSubjectVault struct{ sv *crypto.SubjectVault }

func (r realSubjectVault) Encrypt(p []byte) (string, error) { return r.sv.Encrypt(p) }
func (r realSubjectVault) Decrypt(s string) ([]byte, error) { return r.sv.Decrypt(s) }
func (r realSubjectVault) EncryptWithAAD(p, aad []byte) (string, error) {
	return r.sv.EncryptWithAAD(p, aad)
}
func (r realSubjectVault) DecryptWithAAD(s string, aad []byte) ([]byte, error) {
	return r.sv.DecryptWithAAD(s, aad)
}
func (r realSubjectVault) SubjectVault(ownerID uint, subject string) (plugin.CryptoVault, error) {
	sv, err := r.sv.SubjectVault(ownerID, subject)
	if err != nil {
		return nil, err
	}
	return realSubjectVault{sv: sv}, nil
}
func (r realSubjectVault) ShredSubjectKey(ownerID uint, subject string) error {
	return r.sv.ShredSubjectKey(ownerID, subject)
}
func (r realSubjectVault) HasSubjectKey(ownerID uint, subject string) bool {
	return r.sv.HasSubjectKey(ownerID, subject)
}

func setupVaultTest(t *testing.T) (*Plugin, *gorm.DB, *http.ServeMux) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "vault.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.Setting{}); err != nil {
		t.Fatalf("migrate settings: %v", err)
	}
	p := New()
	if err := db.AutoMigrate(p.Models()...); err != nil {
		t.Fatalf("migrate vault: %v", err)
	}
	c := crypto.New("test-master-secret-key")
	if err := c.EnableEnvelope(dbSecretStore{db: db}); err != nil {
		t.Fatalf("enable envelope: %v", err)
	}

	mux := http.NewServeMux()
	humaAPI := humago.New(mux, huma.DefaultConfig("Vault Test", "1.0.0"))
	th := &plugin.TestHost{
		CryptoMock: realCipherVault{c: c},
		SessionMock: &plugin.TestSession{
			UserIDFn:          func(r *http.Request) uint { return 10 },
			OrgIDFn:           func(r *http.Request) uint { return 1 },
			OrgRoleFn:         func(r *http.Request) string { return "owner" },
			RequireRoleFn:     func(r *http.Request, min string) bool { return true },
			RequirePermFn:     func(r *http.Request, perm, min string) bool { return true },
			IsInstanceAdminFn: func(r *http.Request) bool { return true },
		},
	}
	p.Mount(mux, &plugin.Context{
		DB:   db,
		Huma: humaAPI,
		Host: th,
	})
	return p, db, mux
}

func TestVault_VersioningRollbackAndShred(t *testing.T) {
	p, db, _ := setupVaultTest(t)
	ctx := context.Background()

	meta, err := p.PutSecret(ctx, plugin.PutSecretRequest{
		OrgID:          1,
		Scope:          plugin.VaultScopeOrg,
		CollectionSlug: "app-backend",
		Environment:    "prod",
		FolderPath:     "/db",
		ItemType:       "kv_secret",
		KeyName:        "DATABASE_URL",
		Name:           "Primary DB URL",
		Payload:        map[string]any{"value": "postgres://v1"},
		ChangeNote:     "initial",
		ActorUserID:    10,
	})
	if err != nil {
		t.Fatalf("PutSecret: %v", err)
	}
	if meta.Ref != "vault://app-backend/prod/db/DATABASE_URL" {
		t.Fatalf("unexpected ref: %s", meta.Ref)
	}

	// Update creates v2
	meta2, err := p.UpdateSecret(ctx, 1, meta.ID, nil, nil, map[string]any{"value": "postgres://v2"}, nil, "rotate", 10)
	if err != nil {
		t.Fatalf("UpdateSecret: %v", err)
	}
	if meta2.CurrentVersion != 2 {
		t.Fatalf("expected version 2, got %d", meta2.CurrentVersion)
	}

	sec, err := p.ResolveByRef(ctx, 1, "vault://app-backend/prod/db/DATABASE_URL")
	if err != nil || sec.Payload["value"] != "postgres://v2" {
		t.Fatalf("ResolveByRef v2 failed: %+v, err=%v", sec, err)
	}

	// Rollback creates v3 with v1 payload
	meta3, err := p.RollbackVersion(ctx, 1, meta.ID, 1, 10)
	if err != nil || meta3.CurrentVersion != 3 {
		t.Fatalf("RollbackVersion failed: %+v, err=%v", meta3, err)
	}
	sec3, err := p.GetSecret(ctx, 1, meta.ID)
	if err != nil || sec3.Payload["value"] != "postgres://v1" {
		t.Fatalf("expected rolled back v1 value, got %+v, err=%v", sec3, err)
	}

	// Validate passes while key exists
	if err := p.Validate(ctx, p.host); err != nil {
		t.Fatalf("Validate failed unexpectedly: %v", err)
	}

	// Cross-tenant isolation check
	if _, err := p.GetSecret(ctx, 2, meta.ID); !errors.Is(err, plugin.ErrSecretNotFound) {
		t.Fatalf("expected ErrSecretNotFound across orgs, got %v", err)
	}

	// Delete hard-deletes and shreds DEK
	if err := p.DeleteSecret(ctx, 1, meta.ID); err != nil {
		t.Fatalf("DeleteSecret: %v", err)
	}
	if p.host.Crypto().HasSubjectKey(1, itemSubject(meta.SecretID)) {
		t.Fatal("subject key still exists after DeleteSecret")
	}
	var count int64
	db.Model(&VaultItemVersion{}).Where("item_id = ?", meta.ID).Count(&count)
	if count != 0 {
		t.Fatalf("expected 0 version rows after delete, got %d", count)
	}
}

func TestVault_ServiceTokenAndDotEnv(t *testing.T) {
	p, _, mux := setupVaultTest(t)
	ctx := context.Background()

	_, err := p.PutSecret(ctx, plugin.PutSecretRequest{
		OrgID:          1,
		Scope:          plugin.VaultScopeOrg,
		CollectionSlug: "api",
		Environment:    "staging",
		FolderPath:     "/",
		ItemType:       "kv_secret",
		KeyName:        "STRIPE_KEY",
		Name:           "STRIPE_KEY",
		Payload:        map[string]any{"value": "sk_test_123"},
	})
	if err != nil {
		t.Fatalf("PutSecret staging: %v", err)
	}
	_, err = p.PutSecret(ctx, plugin.PutSecretRequest{
		OrgID:          1,
		Scope:          plugin.VaultScopeOrg,
		CollectionSlug: "api",
		Environment:    "prod",
		FolderPath:     "/",
		ItemType:       "kv_secret",
		KeyName:        "PROD_ONLY",
		Name:           "PROD_ONLY",
		Payload:        map[string]any{"value": "prod_secret"},
	})
	if err != nil {
		t.Fatalf("PutSecret prod: %v", err)
	}

	_, rawToken, err := p.CreateServiceToken(ctx, 1, "ci-staging", "api", "staging", "/", "read", nil, 10)
	if err != nil {
		t.Fatalf("CreateServiceToken: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/vault/raw?format=env", nil)
	req.Header.Set("Authorization", "Bearer "+rawToken)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "STRIPE_KEY=sk_test_123") || strings.Contains(body, "PROD_ONLY") {
		t.Fatalf("unexpected service token env output: %q", body)
	}
}

func TestVault_LookupCredentialAndTOTPAndInstance(t *testing.T) {
	p, _, _ := setupVaultTest(t)
	ctx := context.Background()

	_, err := p.PutSecret(ctx, plugin.PutSecretRequest{
		OrgID:        1,
		Scope:        plugin.VaultScopeOrg,
		ItemType:     "integration",
		ProviderType: "payment",
		Provider:     "stripe",
		Name:         "Main Stripe",
		IsDefault:    true,
		Payload:      map[string]any{"secretKey": "sk_live_abc", "webhookSecret": "whsec_xyz"},
	})
	if err != nil {
		t.Fatalf("PutSecret integration: %v", err)
	}

	cred, err := p.LookupCredential(ctx, 1, plugin.CredentialQuery{ProviderType: "payment", Provider: "stripe"})
	if err != nil {
		t.Fatalf("LookupCredential: %v", err)
	}
	var decoded map[string]string
	if err := json.Unmarshal(cred.ConfigJSON, &decoded); err != nil || decoded["secretKey"] != "sk_live_abc" {
		t.Fatalf("unexpected credential payload: %s, err=%v", string(cred.ConfigJSON), err)
	}

	// TOTP item
	totpMeta, err := p.PutSecret(ctx, plugin.PutSecretRequest{
		OrgID:    1,
		Scope:    plugin.VaultScopeOrg,
		ItemType: "twofa_totp",
		Name:     "Cloudflare Root",
		Payload:  map[string]any{"secret": "JBSWY3DPEHPK3PXP"},
	})
	if err != nil {
		t.Fatalf("PutSecret TOTP: %v", err)
	}
	code, rem, err := p.ComputeTOTP(ctx, 1, totpMeta.ID)
	if err != nil || len(code) != 6 || rem <= 0 {
		t.Fatalf("ComputeTOTP failed: code=%q rem=%d err=%v", code, rem, err)
	}

	// Instance secret isolation
	if _, err := p.PutInstanceSecret(ctx, plugin.PutSecretRequest{
		ItemType: "oauth_app",
		KeyName:  "instance.oauth.google",
		Name:     "Google OAuth",
		Payload:  map[string]any{"clientId": "gid", "clientSecret": "gsecret"},
	}); err != nil {
		t.Fatalf("PutInstanceSecret: %v", err)
	}
	if _, err := p.ResolveByRef(ctx, 1, "vault://system/default/instance.oauth.google"); !errors.Is(err, plugin.ErrSecretNotFound) {
		t.Fatalf("org must not resolve instance secrets, got %v", err)
	}
	instSec, err := p.ResolveInstanceSecret(ctx, "instance.oauth.google")
	if err != nil || instSec.Payload["clientSecret"] != "gsecret" {
		t.Fatalf("ResolveInstanceSecret failed: %+v, err=%v", instSec, err)
	}
}
