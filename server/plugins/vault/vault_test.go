package vault

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/glebarez/sqlite"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/server/internal/crypto"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
	"golang.org/x/crypto/ssh"
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

func setupVaultTestCustom(t *testing.T, uidFn func(*http.Request) uint, auditFn func(*http.Request, string, string, uint, map[string]any)) (*Plugin, *gorm.DB, *http.ServeMux) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "vault.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.Setting{}, &models.AuditLog{}); err != nil {
		t.Fatalf("migrate settings/audit: %v", err)
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
			UserIDFn:          uidFn,
			OrgIDFn:           func(r *http.Request) uint { return 1 },
			OrgRoleFn:         func(r *http.Request) string { return "owner" },
			RequireRoleFn:     func(r *http.Request, min string) bool { return true },
			RequirePermFn:     func(r *http.Request, perm, min string) bool { return true },
			IsInstanceAdminFn: func(r *http.Request) bool { return true },
		},
	}
	p.Mount(mux, &plugin.Context{
		DB:    db,
		Huma:  humaAPI,
		Host:  th,
		Audit: auditFn,
	})
	return p, db, mux
}

func TestVault_UserScopeIsolationAndCrossUserAuth(t *testing.T) {
	var activeUID uint = 10
	p, _, mux := setupVaultTestCustom(t, func(*http.Request) uint { return activeUID }, nil)
	ctx := context.Background()

	// Missing OwnerUserID for VaultScopeUser must fail closed.
	if _, err := p.PutSecret(ctx, plugin.PutSecretRequest{
		OrgID:   1,
		Scope:   plugin.VaultScopeUser,
		KeyName: "BAD_USER_KEY",
		Name:    "Bad",
		Payload: map[string]any{"value": "nope"},
	}); err == nil {
		t.Fatal("expected error when OwnerUserID is 0 for VaultScopeUser")
	}

	// User A (10) and User B (20) store the same keyName in the same org/collection/env/folder.
	metaA, err := p.PutSecret(ctx, plugin.PutSecretRequest{
		OrgID:       1,
		Scope:       plugin.VaultScopeUser,
		OwnerUserID: 10,
		KeyName:     "PERSONAL_TOKEN",
		Name:        "User A Token",
		Payload:     map[string]any{"value": "user-a-v1"},
		ActorUserID: 10,
	})
	if err != nil {
		t.Fatalf("PutSecret user A: %v", err)
	}
	if _, err := p.UpdateSecret(ctx, 1, metaA.ID, nil, nil, map[string]any{"value": "user-a-v2"}, nil, "v2", 10); err != nil {
		t.Fatalf("UpdateSecret user A: %v", err)
	}

	metaB, err := p.PutSecret(ctx, plugin.PutSecretRequest{
		OrgID:       1,
		Scope:       plugin.VaultScopeUser,
		OwnerUserID: 20,
		KeyName:     "PERSONAL_TOKEN",
		Name:        "User B Token",
		Payload:     map[string]any{"value": "user-b-v1"},
		ActorUserID: 20,
	})
	if err != nil {
		t.Fatalf("PutSecret user B: %v", err)
	}
	if metaA.ID == metaB.ID {
		t.Fatalf("expected separate items for User A and User B, both got ID %d", metaA.ID)
	}

	secA, err := p.GetSecret(ctx, 1, metaA.ID)
	if err != nil || secA.Payload["value"] != "user-a-v2" {
		t.Fatalf("User A secret corrupted: %+v, err=%v", secA, err)
	}
	secB, err := p.GetSecret(ctx, 1, metaB.ID)
	if err != nil || secB.Payload["value"] != "user-b-v1" {
		t.Fatalf("User B secret mismatch: %+v, err=%v", secB, err)
	}

	// ResolveByRef must never resolve user-scoped items by ID.
	if _, err := p.ResolveByRef(ctx, 1, fmt.Sprintf("vault://id/%d", metaA.ID)); !errors.Is(err, plugin.ErrSecretNotFound) {
		t.Fatalf("expected ErrSecretNotFound for user-scoped vault://id ref, got %v", err)
	}

	totpA, err := p.PutSecret(ctx, plugin.PutSecretRequest{
		OrgID:       1,
		Scope:       plugin.VaultScopeUser,
		OwnerUserID: 10,
		ItemType:    "twofa_totp",
		Name:        "User A 2FA",
		Payload:     map[string]any{"secret": "JBSWY3DPEHPK3PXP"},
		ActorUserID: 10,
	})
	if err != nil {
		t.Fatalf("PutSecret user A TOTP: %v", err)
	}

	// Switch caller to User B (20) and assert 404 on rollback, default, test, and totp for User A's items.
	activeUID = 20
	checks := []struct {
		method string
		path   string
	}{
		{http.MethodPost, fmt.Sprintf("/api/vault/items/%d/rollback/1", metaA.ID)},
		{http.MethodPost, fmt.Sprintf("/api/vault/items/%d/default", metaA.ID)},
		{http.MethodPost, fmt.Sprintf("/api/vault/items/%d/test", metaA.ID)},
		{http.MethodGet, fmt.Sprintf("/api/vault/items/%d/totp", totpA.ID)},
	}
	for _, tc := range checks {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s %s as wrong user: expected 404, got %d (%s)", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
}

func TestVault_MCP_GetTOTPRejectsUserScopeAndOwnerRoleAllowsSetSecret(t *testing.T) {
	p, _, _ := setupVaultTest(t)
	ctx := context.Background()

	orgTOTP, err := p.PutSecret(ctx, plugin.PutSecretRequest{
		OrgID:    1,
		Scope:    plugin.VaultScopeOrg,
		ItemType: "twofa_totp",
		Name:     "Org 2FA",
		Payload:  map[string]any{"secret": "JBSWY3DPEHPK3PXP"},
	})
	if err != nil {
		t.Fatalf("PutSecret org TOTP: %v", err)
	}
	userTOTP, err := p.PutSecret(ctx, plugin.PutSecretRequest{
		OrgID:       1,
		Scope:       plugin.VaultScopeUser,
		OwnerUserID: 10,
		ItemType:    "twofa_totp",
		Name:        "User 2FA",
		Payload:     map[string]any{"secret": "JBSWY3DPEHPK3PXP"},
	})
	if err != nil {
		t.Fatalf("PutSecret user TOTP: %v", err)
	}

	connectWithRole := func(role string) (*mcp.ClientSession, func()) {
		srv := mcp.NewServer(&mcp.Implementation{Name: "vault-test", Version: "1"}, nil)
		p.RegisterMCP(srv)
		c := plugin.WithTokenRole(plugin.WithOrgID(ctx, 1), role)
		sT, cT := mcp.NewInMemoryTransports()
		ss, err := srv.Connect(c, sT, nil)
		if err != nil {
			t.Fatalf("srv.Connect: %v", err)
		}
		client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1"}, nil)
		cs, err := client.Connect(c, cT, nil)
		if err != nil {
			t.Fatalf("client.Connect: %v", err)
		}
		return cs, func() {
			_ = cs.Close()
			_ = ss.Close()
		}
	}

	csOwner, closeOwner := connectWithRole("owner")
	defer closeOwner()

	resOrg, err := csOwner.CallTool(ctx, &mcp.CallToolParams{
		Name:      "octarq_vault__get_totp",
		Arguments: map[string]any{"itemId": orgTOTP.ID},
	})
	if err != nil || resOrg.IsError {
		t.Fatalf("expected org TOTP to succeed via MCP, got res=%+v err=%v", resOrg, err)
	}

	resUser, err := csOwner.CallTool(ctx, &mcp.CallToolParams{
		Name:      "octarq_vault__get_totp",
		Arguments: map[string]any{"itemId": userTOTP.ID},
	})
	if err == nil && (resUser == nil || !resUser.IsError) {
		t.Fatalf("expected user-scoped TOTP to be rejected via MCP, got %+v", resUser)
	}

	resSetOwner, err := csOwner.CallTool(ctx, &mcp.CallToolParams{
		Name: "octarq_vault__set_secret",
		Arguments: map[string]any{
			"keyName": "MCP_KEY",
			"name":    "MCP Key",
			"payload": map[string]any{"value": "mcp-secret"},
		},
	})
	if err != nil || resSetOwner.IsError {
		t.Fatalf("expected owner role to allow octarq_vault__set_secret, got res=%+v err=%v", resSetOwner, err)
	}

	csMember, closeMember := connectWithRole("member")
	defer closeMember()
	resSetMember, err := csMember.CallTool(ctx, &mcp.CallToolParams{
		Name: "octarq_vault__set_secret",
		Arguments: map[string]any{
			"keyName": "MCP_KEY_2",
			"name":    "MCP Key 2",
			"payload": map[string]any{"value": "nope"},
		},
	})
	if err == nil && (resSetMember == nil || !resSetMember.IsError) {
		t.Fatalf("expected member role to fail octarq_vault__set_secret, got %+v", resSetMember)
	}
}

func TestVault_ServiceTokenFolderBoundaryAndAudit(t *testing.T) {
	var auditActions []string
	var auditOrgIDs []uint
	var auditTargetIDs []uint
	p, db, mux := setupVaultTestCustom(
		t,
		func(*http.Request) uint { return 10 },
		func(r *http.Request, action, targetType string, targetID uint, meta map[string]any) {
			auditActions = append(auditActions, action)
			auditOrgIDs = append(auditOrgIDs, plugin.OrgIDFromContext(r.Context()))
			auditTargetIDs = append(auditTargetIDs, targetID)
		},
	)
	ctx := context.Background()

	puts := []struct {
		folder string
		key    string
	}{
		{"/app", "APP_ROOT"},
		{"/app/db", "APP_DB"},
		{"/apple", "APPLE_OTHER"},
		{"/app-prod", "APP_PROD_OTHER"},
		{"/", "ROOT_KEY"},
	}
	for _, item := range puts {
		if _, err := p.PutSecret(ctx, plugin.PutSecretRequest{
			OrgID:          1,
			Scope:          plugin.VaultScopeOrg,
			CollectionSlug: "api",
			Environment:    "prod",
			FolderPath:     item.folder,
			ItemType:       "kv_secret",
			KeyName:        item.key,
			Name:           item.key,
			Payload:        map[string]any{"value": "v-" + item.key},
		}); err != nil {
			t.Fatalf("PutSecret %s: %v", item.key, err)
		}
	}

	tok, rawToken, err := p.CreateServiceToken(ctx, 1, "app-ci", "api", "prod", "/app", "read", nil, 10)
	if err != nil {
		t.Fatalf("CreateServiceToken: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/vault/raw?format=json", nil)
	req.Header.Set("Authorization", "Bearer "+rawToken)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Environment string         `json:"environment"`
		Secrets     map[string]any `json:"secrets"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Secrets["APP_ROOT"] != "v-APP_ROOT" || resp.Secrets["APP_DB"] != "v-APP_DB" {
		t.Fatalf("expected /app and /app/db secrets, got %+v", resp.Secrets)
	}
	for _, forbidden := range []string{"APPLE_OTHER", "APP_PROD_OTHER", "ROOT_KEY"} {
		if _, exists := resp.Secrets[forbidden]; exists {
			t.Fatalf("service token leaked out-of-prefix secret %s: %+v", forbidden, resp.Secrets)
		}
	}

	if len(auditActions) != 1 || auditActions[0] != "vault.service_token.pulled" || auditOrgIDs[0] != 1 || auditTargetIDs[0] != tok.ID {
		t.Fatalf("unexpected audit callback emissions: actions=%v orgs=%v targets=%v", auditActions, auditOrgIDs, auditTargetIDs)
	}

	// Also verify direct audit_logs DB persistence when p.audit is nil.
	p.audit = nil
	req2 := httptest.NewRequest(http.MethodGet, "/api/vault/raw?format=env", nil)
	req2.Header.Set("Authorization", "Bearer "+rawToken)
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200 on second pull, got %d", rec2.Code)
	}
	var logRow models.AuditLog
	if err := db.Where("action = ?", "vault.service_token.pulled").First(&logRow).Error; err != nil {
		t.Fatalf("expected audit_logs row when p.audit is nil: %v", err)
	}
	if logRow.OrgID != tok.OrgID || logRow.TargetType != "vault_service_token" || logRow.TargetID != tok.ID {
		t.Fatalf("unexpected audit_logs row: %+v", logRow)
	}
}

func TestVault_DeleteInstanceSecretAndShred(t *testing.T) {
	p, _, _ := setupVaultTest(t)
	ctx := context.Background()

	meta, err := p.PutInstanceSecret(ctx, plugin.PutSecretRequest{
		ItemType: "instance_secret",
		KeyName:  "instance.test.shred",
		Name:     "Temp Instance Secret",
		Payload:  map[string]any{"token": "tok-123"},
	})
	if err != nil {
		t.Fatalf("PutInstanceSecret: %v", err)
	}
	if !p.host.Crypto().HasSubjectKey(0, itemSubject(meta.SecretID)) {
		t.Fatal("expected subject key before DeleteInstanceSecret")
	}
	if err := p.DeleteInstanceSecret(ctx, "  instance.test.shred  "); err != nil {
		t.Fatalf("DeleteInstanceSecret: %v", err)
	}
	if _, err := p.ResolveInstanceSecret(ctx, "instance.test.shred"); !errors.Is(err, plugin.ErrSecretNotFound) {
		t.Fatalf("expected ErrSecretNotFound after DeleteInstanceSecret, got %v", err)
	}
	if p.host.Crypto().HasSubjectKey(0, itemSubject(meta.SecretID)) {
		t.Fatal("expected subject key to be shredded after DeleteInstanceSecret")
	}
	if err := p.DeleteInstanceSecret(ctx, "instance.test.shred"); !errors.Is(err, plugin.ErrSecretNotFound) {
		t.Fatalf("expected ErrSecretNotFound on missing key, got %v", err)
	}
	if err := p.DeleteInstanceSecret(ctx, "   "); err == nil {
		t.Fatal("expected error for empty keyName")
	}
}

func TestVault_DatabaseSSRFBlocked(t *testing.T) {
	p, _, mux := setupVaultTest(t)
	ctx := context.Background()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen loopback: %v", err)
	}
	defer func() { _ = ln.Close() }()
	port := ln.Addr().(*net.TCPAddr).Port

	meta, err := p.PutSecret(ctx, plugin.PutSecretRequest{
		OrgID:    1,
		Scope:    plugin.VaultScopeOrg,
		ItemType: "database",
		Name:     "Loopback DB",
		Payload:  map[string]any{"host": "127.0.0.1", "port": float64(port), "password": "secret-pass"},
	})
	if err != nil {
		t.Fatalf("PutSecret database: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/vault/items/%d/test", meta.ID), bytes.NewReader(nil))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 SSRF block for 127.0.0.1, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "ssrf guard") {
		t.Fatalf("expected ssrf guard error in response, got %s", rec.Body.String())
	}
}

func TestVault_ListItemsQueryIsolation(t *testing.T) {
	p, _, _ := setupVaultTest(t)
	ctx := context.Background()

	// Org 1 matching across name, key_name, provider, and tags
	org1Match, err := p.PutSecret(ctx, plugin.PutSecretRequest{
		OrgID:          1,
		Scope:          plugin.VaultScopeOrg,
		CollectionSlug: "payments",
		Environment:    "prod",
		FolderPath:     "/billing",
		ItemType:       "integration",
		ProviderType:   "payment",
		Provider:       "stripe",
		KeyName:        "BILLING_GATEWAY",
		Name:           "Primary Gateway",
		Tags:           []string{"stripe-live"},
		Payload:        map[string]any{"secretKey": "sk_live_org1"},
	})
	if err != nil {
		t.Fatalf("PutSecret org1: %v", err)
	}
	// Org 1 non-matching item
	if _, err := p.PutSecret(ctx, plugin.PutSecretRequest{
		OrgID:          1,
		Scope:          plugin.VaultScopeOrg,
		CollectionSlug: "payments",
		Environment:    "prod",
		ItemType:       "kv_secret",
		KeyName:        "OTHER_KEY",
		Name:           "Unrelated Secret",
		Payload:        map[string]any{"value": "val"},
	}); err != nil {
		t.Fatalf("PutSecret org1 other: %v", err)
	}
	// Org 2 item with "stripe" in key_name, provider, name, tags (must NEVER leak to Org 1)
	if _, err := p.PutSecret(ctx, plugin.PutSecretRequest{
		OrgID:        2,
		Scope:        plugin.VaultScopeOrg,
		ItemType:     "integration",
		ProviderType: "payment",
		Provider:     "stripe",
		KeyName:      "STRIPE_SECRET_KEY",
		Name:         "Org 2 Stripe",
		Tags:         []string{"stripe"},
		Payload:      map[string]any{"secretKey": "sk_live_org2"},
	}); err != nil {
		t.Fatalf("PutSecret org2: %v", err)
	}
	// User 9 personal item in Org 1 with "stripe" (must NEVER leak to Org 1 org-scope query)
	if _, err := p.PutSecret(ctx, plugin.PutSecretRequest{
		OrgID:       1,
		Scope:       plugin.VaultScopeUser,
		OwnerUserID: 9,
		KeyName:     "STRIPE_PERSONAL",
		Name:        "User 9 Stripe",
		Tags:        []string{"stripe"},
		Payload:     map[string]any{"value": "sk_user9"},
	}); err != nil {
		t.Fatalf("PutSecret user9: %v", err)
	}
	// Instance item with "stripe" (must NEVER leak to Org 1 org-scope query)
	if _, err := p.PutInstanceSecret(ctx, plugin.PutSecretRequest{
		ItemType: "instance_secret",
		KeyName:  "instance.stripe.webhook",
		Name:     "Instance Stripe Webhook",
		Tags:     []string{"stripe"},
		Payload:  map[string]any{"secret": "whsec_inst"},
	}); err != nil {
		t.Fatalf("PutInstanceSecret: %v", err)
	}

	got, err := p.ListItems(ctx, 1, plugin.VaultFilter{
		Scope: plugin.VaultScopeOrg,
		Query: "stripe",
	})
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if len(got) != 1 || got[0].ID != org1Match.ID {
		t.Fatalf("expected only Org 1 item %d, got %+v", org1Match.ID, got)
	}

	// Verify all filter branches & validation errors
	filtered, err := p.ListItems(ctx, 1, plugin.VaultFilter{
		Scope:          plugin.VaultScopeOrg,
		CollectionSlug: "payments",
		Environment:    "prod",
		FolderPath:     "/billing",
		ItemType:       "integration",
		ProviderType:   "payment",
		Provider:       "stripe",
		KeyName:        "BILLING_GATEWAY",
		Query:          "primary",
	})
	if err != nil || len(filtered) != 1 {
		t.Fatalf("expected 1 item with full filters, got %+v err=%v", filtered, err)
	}
	emptyCol, err := p.ListItems(ctx, 1, plugin.VaultFilter{Scope: plugin.VaultScopeOrg, CollectionSlug: "missing-col"})
	if err != nil || len(emptyCol) != 0 {
		t.Fatalf("expected empty slice for missing collection, got %+v err=%v", emptyCol, err)
	}
	if _, err := p.ListItems(ctx, 0, plugin.VaultFilter{Scope: plugin.VaultScopeOrg}); err == nil {
		t.Fatal("expected error for orgID=0")
	}
	if _, err := p.ListItems(ctx, 1, plugin.VaultFilter{Scope: plugin.VaultScopeUser}); err == nil {
		t.Fatal("expected error for missing OwnerUserID on user scope")
	}
}

func roleAtLeast(actual, min string) bool {
	rank := map[string]int{"member": 1, "admin": 2, "owner": 3}
	return rank[strings.ToLower(actual)] >= rank[strings.ToLower(min)]
}

func setupVaultTestRBAC(t *testing.T, state *struct {
	uid             uint
	orgID           uint
	role            string
	isInstanceAdmin bool
}) (*Plugin, *gorm.DB, *http.ServeMux) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "vault.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.Setting{}, &models.AuditLog{}); err != nil {
		t.Fatalf("migrate settings/audit: %v", err)
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
	humaAPI := humago.New(mux, huma.DefaultConfig("Vault RBAC Test", "1.0.0"))
	th := &plugin.TestHost{
		CryptoMock: realCipherVault{c: c},
		SessionMock: &plugin.TestSession{
			UserIDFn:      func(*http.Request) uint { return state.uid },
			OrgIDFn:       func(*http.Request) uint { return state.orgID },
			OrgRoleFn:     func(*http.Request) string { return state.role },
			RequireRoleFn: func(_ *http.Request, min string) bool { return state.isInstanceAdmin || roleAtLeast(state.role, min) },
			RequirePermFn: func(_ *http.Request, _, min string) bool {
				return state.isInstanceAdmin || roleAtLeast(state.role, min)
			},
			IsInstanceAdminFn: func(*http.Request) bool { return state.isInstanceAdmin },
		},
	}
	p.Mount(mux, &plugin.Context{DB: db, Huma: humaAPI, Host: th})
	return p, db, mux
}

func doJSONReq(t *testing.T, mux *http.ServeMux, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestVault_HTTP_RBACAndPersonalScope(t *testing.T) {
	state := &struct {
		uid             uint
		orgID           uint
		role            string
		isInstanceAdmin bool
	}{uid: 10, orgID: 1, role: "owner", isInstanceAdmin: false}
	p, _, mux := setupVaultTestRBAC(t, state)

	// Schemas endpoint
	if rec := doJSONReq(t, mux, http.MethodGet, "/api/vault/schemas", nil); rec.Code != http.StatusOK {
		t.Fatalf("GET /api/vault/schemas: %d %s", rec.Code, rec.Body.String())
	}

	// Unauthenticated (uid == 0) -> 401; Missing workspace (orgID == 0) -> 400
	state.uid = 0
	if rec := doJSONReq(t, mux, http.MethodGet, "/api/vault/items", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when uid=0, got %d", rec.Code)
	}
	state.uid = 10
	state.orgID = 0
	if rec := doJSONReq(t, mux, http.MethodGet, "/api/vault/items", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when orgID=0, got %d", rec.Code)
	}
	state.orgID = 1

	// Collections RBAC: member cannot create, admin can create, owner can delete empty only
	state.role = "member"
	if rec := doJSONReq(t, mux, http.MethodGet, "/api/vault/collections", nil); rec.Code != http.StatusOK {
		t.Fatalf("member listCollections: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doJSONReq(t, mux, http.MethodPost, "/api/vault/collections", map[string]any{"name": "Team"}); rec.Code != http.StatusForbidden {
		t.Fatalf("member createCollection expected 403, got %d", rec.Code)
	}
	state.role = "admin"
	if rec := doJSONReq(t, mux, http.MethodPost, "/api/vault/collections", map[string]any{"name": "   "}); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty collection name expected 400, got %d", rec.Code)
	}
	colRec := doJSONReq(t, mux, http.MethodPost, "/api/vault/collections", map[string]any{"name": "Backend", "slug": "backend"})
	if colRec.Code != http.StatusCreated {
		t.Fatalf("admin createCollection expected 201, got %d: %s", colRec.Code, colRec.Body.String())
	}
	var col VaultCollection
	_ = json.Unmarshal(colRec.Body.Bytes(), &col)
	if rec := doJSONReq(t, mux, http.MethodPost, "/api/vault/collections", map[string]any{"name": "Backend", "slug": "backend"}); rec.Code != http.StatusConflict {
		t.Fatalf("duplicate collection expected 409, got %d", rec.Code)
	}
	if rec := doJSONReq(t, mux, http.MethodDelete, fmt.Sprintf("/api/vault/collections/%d", col.ID), nil); rec.Code != http.StatusForbidden {
		t.Fatalf("admin deleteCollection expected 403, got %d", rec.Code)
	}

	// Admin creates org integration & TOTP item
	createRec := doJSONReq(t, mux, http.MethodPost, "/api/vault/items", map[string]any{
		"collectionSlug": "backend",
		"environment":    "prod",
		"itemType":       "integration",
		"providerType":   "payment",
		"provider":       "stripe",
		"keyName":        "STRIPE_PROD",
		"name":           "Stripe Prod",
		"payload":        map[string]any{"secretKey": "sk_prod_1"},
	})
	if createRec.Code != http.StatusCreated {
		t.Fatalf("admin createItem expected 201, got %d: %s", createRec.Code, createRec.Body.String())
	}
	var orgItem plugin.VaultItemMeta
	_ = json.Unmarshal(createRec.Body.Bytes(), &orgItem)

	totpRec := doJSONReq(t, mux, http.MethodPost, "/api/vault/items", map[string]any{
		"itemType": "twofa_totp",
		"name":     "Org TOTP",
		"payload":  map[string]any{"secret": "JBSWY3DPEHPK3PXP"},
	})
	if totpRec.Code != http.StatusCreated {
		t.Fatalf("create org TOTP: %d", totpRec.Code)
	}
	var orgTotp plugin.VaultItemMeta
	_ = json.Unmarshal(totpRec.Body.Bytes(), &orgTotp)

	// Invalid scope checks
	if rec := doJSONReq(t, mux, http.MethodGet, "/api/vault/items?scope=instance", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("listItems invalid scope expected 400, got %d", rec.Code)
	}
	if rec := doJSONReq(t, mux, http.MethodPost, "/api/vault/items", map[string]any{"scope": "instance", "name": "Bad", "payload": map[string]any{"value": "x"}}); rec.Code != http.StatusBadRequest {
		t.Fatalf("createItem invalid scope expected 400, got %d", rec.Code)
	}

	// Admin updates -> v2, reveals, rolls back, sets default, cannot delete or export
	if rec := doJSONReq(t, mux, http.MethodPut, fmt.Sprintf("/api/vault/items/%d", orgItem.ID), map[string]any{"payload": map[string]any{"secretKey": "sk_prod_2"}}); rec.Code != http.StatusOK {
		t.Fatalf("admin updateItem expected 200, got %d", rec.Code)
	}
	if rec := doJSONReq(t, mux, http.MethodPost, fmt.Sprintf("/api/vault/items/%d/reveal", orgItem.ID), nil); rec.Code != http.StatusOK {
		t.Fatalf("admin revealItem expected 200, got %d", rec.Code)
	}
	if rec := doJSONReq(t, mux, http.MethodPost, fmt.Sprintf("/api/vault/items/%d/rollback/1", orgItem.ID), nil); rec.Code != http.StatusOK {
		t.Fatalf("admin rollbackItem expected 200, got %d", rec.Code)
	}
	if rec := doJSONReq(t, mux, http.MethodPost, fmt.Sprintf("/api/vault/items/%d/default", orgItem.ID), nil); rec.Code != http.StatusOK {
		t.Fatalf("admin setDefaultItem expected 200, got %d", rec.Code)
	}
	if rec := doJSONReq(t, mux, http.MethodDelete, fmt.Sprintf("/api/vault/items/%d", orgItem.ID), nil); rec.Code != http.StatusForbidden {
		t.Fatalf("admin deleteItem expected 403, got %d", rec.Code)
	}
	if rec := doJSONReq(t, mux, http.MethodGet, "/api/vault/export?format=env", nil); rec.Code != http.StatusForbidden {
		t.Fatalf("admin export expected 403, got %d", rec.Code)
	}

	// Member on org items: read/versions/test/totp allowed, write/reveal/rollback/default/delete forbidden
	state.role = "member"
	if rec := doJSONReq(t, mux, http.MethodGet, "/api/vault/items?scope=org", nil); rec.Code != http.StatusOK {
		t.Fatalf("member listItems: %d", rec.Code)
	}
	if rec := doJSONReq(t, mux, http.MethodGet, fmt.Sprintf("/api/vault/items/%d", orgItem.ID), nil); rec.Code != http.StatusOK {
		t.Fatalf("member getItem: %d", rec.Code)
	}
	if rec := doJSONReq(t, mux, http.MethodGet, fmt.Sprintf("/api/vault/items/%d/versions", orgItem.ID), nil); rec.Code != http.StatusOK {
		t.Fatalf("member listItemVersions: %d", rec.Code)
	}
	if rec := doJSONReq(t, mux, http.MethodPost, fmt.Sprintf("/api/vault/items/%d/test", orgItem.ID), nil); rec.Code != http.StatusOK {
		t.Fatalf("member testItem: %d", rec.Code)
	}
	if rec := doJSONReq(t, mux, http.MethodGet, fmt.Sprintf("/api/vault/items/%d/totp", orgTotp.ID), nil); rec.Code != http.StatusOK {
		t.Fatalf("member getItemTOTP: %d", rec.Code)
	}
	for _, tc := range []struct {
		m, p string
		body any
	}{
		{http.MethodPost, "/api/vault/items", map[string]any{"name": "X", "payload": map[string]any{"value": "y"}}},
		{http.MethodPut, fmt.Sprintf("/api/vault/items/%d", orgItem.ID), map[string]any{"name": "X"}},
		{http.MethodPost, fmt.Sprintf("/api/vault/items/%d/reveal", orgItem.ID), nil},
		{http.MethodPost, fmt.Sprintf("/api/vault/items/%d/rollback/1", orgItem.ID), nil},
		{http.MethodPost, fmt.Sprintf("/api/vault/items/%d/default", orgItem.ID), nil},
		{http.MethodDelete, fmt.Sprintf("/api/vault/items/%d", orgItem.ID), nil},
	} {
		if rec := doJSONReq(t, mux, tc.m, tc.p, tc.body); rec.Code != http.StatusForbidden {
			t.Fatalf("member %s %s expected 403, got %d: %s", tc.m, tc.p, rec.Code, rec.Body.String())
		}
	}

	// Member Personal (scope = "user") self-service: full CRUD + reveal + rollback + totp + delete on OWN items
	uCreate := doJSONReq(t, mux, http.MethodPost, "/api/vault/items", map[string]any{
		"scope":     "user",
		"itemType":  "twofa_totp",
		"keyName":   "MY_GITHUB_2FA",
		"name":      "My GitHub 2FA",
		"isDefault": true, // must be forced false on personal scope
		"payload":   map[string]any{"secret": "JBSWY3DPEHPK3PXP"},
	})
	if uCreate.Code != http.StatusCreated {
		t.Fatalf("member personal createItem expected 201, got %d: %s", uCreate.Code, uCreate.Body.String())
	}
	var uItem plugin.VaultItemMeta
	_ = json.Unmarshal(uCreate.Body.Bytes(), &uItem)
	if uItem.Scope != plugin.VaultScopeUser || uItem.OwnerUserID != 10 || uItem.CollectionSlug != "default" || uItem.IsDefault {
		t.Fatalf("unexpected personal item metadata: %+v", uItem)
	}
	if rec := doJSONReq(t, mux, http.MethodGet, "/api/vault/items?scope=user", nil); rec.Code != http.StatusOK {
		t.Fatalf("member list user items: %d", rec.Code)
	}
	if rec := doJSONReq(t, mux, http.MethodPut, fmt.Sprintf("/api/vault/items/%d", uItem.ID), map[string]any{"payload": map[string]any{"secret": "JBSWY3DPEHPK3PXQ"}}); rec.Code != http.StatusOK {
		t.Fatalf("member update own personal item: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doJSONReq(t, mux, http.MethodPut, fmt.Sprintf("/api/vault/items/%d", uItem.ID), map[string]any{"isDefault": true}); rec.Code != http.StatusBadRequest {
		t.Fatalf("setting isDefault on personal item expected 400, got %d", rec.Code)
	}
	if rec := doJSONReq(t, mux, http.MethodPost, fmt.Sprintf("/api/vault/items/%d/default", uItem.ID), nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("setDefault on personal item expected 400, got %d", rec.Code)
	}
	if rec := doJSONReq(t, mux, http.MethodPost, fmt.Sprintf("/api/vault/items/%d/reveal", uItem.ID), nil); rec.Code != http.StatusOK {
		t.Fatalf("member reveal own personal item: %d", rec.Code)
	}
	if rec := doJSONReq(t, mux, http.MethodPost, fmt.Sprintf("/api/vault/items/%d/rollback/1", uItem.ID), nil); rec.Code != http.StatusOK {
		t.Fatalf("member rollback own personal item: %d", rec.Code)
	}
	if rec := doJSONReq(t, mux, http.MethodGet, fmt.Sprintf("/api/vault/items/%d/totp", uItem.ID), nil); rec.Code != http.StatusOK {
		t.Fatalf("member TOTP own personal item: %d", rec.Code)
	}

	// Owner (different user uid=20) cannot access User 10's personal item (404 on every endpoint)
	state.uid = 20
	state.role = "owner"
	for _, tc := range []struct {
		m, p string
		body any
	}{
		{http.MethodGet, fmt.Sprintf("/api/vault/items/%d", uItem.ID), nil},
		{http.MethodPut, fmt.Sprintf("/api/vault/items/%d", uItem.ID), map[string]any{"name": "Hijack"}},
		{http.MethodPost, fmt.Sprintf("/api/vault/items/%d/reveal", uItem.ID), nil},
		{http.MethodGet, fmt.Sprintf("/api/vault/items/%d/versions", uItem.ID), nil},
		{http.MethodPost, fmt.Sprintf("/api/vault/items/%d/rollback/1", uItem.ID), nil},
		{http.MethodPost, fmt.Sprintf("/api/vault/items/%d/test", uItem.ID), nil},
		{http.MethodGet, fmt.Sprintf("/api/vault/items/%d/totp", uItem.ID), nil},
		{http.MethodDelete, fmt.Sprintf("/api/vault/items/%d", uItem.ID), nil},
	} {
		if rec := doJSONReq(t, mux, tc.m, tc.p, tc.body); rec.Code != http.StatusNotFound {
			t.Fatalf("other user %s %s expected 404, got %d", tc.m, tc.p, rec.Code)
		}
	}

	// User 10 deletes their own personal item as member
	state.uid = 10
	state.role = "member"
	if rec := doJSONReq(t, mux, http.MethodDelete, fmt.Sprintf("/api/vault/items/%d", uItem.ID), nil); rec.Code != http.StatusOK {
		t.Fatalf("member delete own personal item expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Owner deletes org item and collection (non-empty -> 409, empty -> 200, missing -> 404)
	state.role = "owner"
	if rec := doJSONReq(t, mux, http.MethodDelete, fmt.Sprintf("/api/vault/collections/%d", col.ID), nil); rec.Code != http.StatusConflict {
		t.Fatalf("non-empty collection delete expected 409, got %d", rec.Code)
	}
	if rec := doJSONReq(t, mux, http.MethodDelete, fmt.Sprintf("/api/vault/items/%d", orgItem.ID), nil); rec.Code != http.StatusOK {
		t.Fatalf("owner delete org item expected 200, got %d", rec.Code)
	}
	if rec := doJSONReq(t, mux, http.MethodDelete, fmt.Sprintf("/api/vault/collections/%d", col.ID), nil); rec.Code != http.StatusOK {
		t.Fatalf("owner delete empty collection expected 200, got %d", rec.Code)
	}
	if rec := doJSONReq(t, mux, http.MethodDelete, "/api/vault/collections/99999", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("missing collection delete expected 404, got %d", rec.Code)
	}
	_ = p
}

func TestVault_HTTP_ImportExportTokensAndInstance(t *testing.T) {
	state := &struct {
		uid             uint
		orgID           uint
		role            string
		isInstanceAdmin bool
	}{uid: 10, orgID: 1, role: "owner", isInstanceAdmin: false}
	p, _, mux := setupVaultTestRBAC(t, state)

	// Bulk import .env & json + invalid format/line
	if rec := doJSONReq(t, mux, http.MethodPost, "/api/vault/import", map[string]any{
		"collectionSlug": "app",
		"environment":    "prod",
		"format":         "env",
		"dotEnv":         "# comment\nexport FOO=\"bar\\n\\\"baz\\\"\"\nBAR='qux'\nFOO=final\n",
	}); rec.Code != http.StatusOK {
		t.Fatalf("import env failed: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doJSONReq(t, mux, http.MethodPost, "/api/vault/import", map[string]any{
		"collectionSlug": "app",
		"environment":    "prod",
		"format":         "json",
		"jsonItems":      map[string]any{"JSON_STR": "hello", "JSON_OBJ": map[string]any{"value": "pw", "host": "db.internal"}},
	}); rec.Code != http.StatusOK {
		t.Fatalf("import json failed: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doJSONReq(t, mux, http.MethodPost, "/api/vault/import", map[string]any{"format": "xml"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("unsupported format expected 400, got %d", rec.Code)
	}
	if rec := doJSONReq(t, mux, http.MethodPost, "/api/vault/import", map[string]any{"format": "env", "dotEnv": "INVALID_LINE"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid dotenv line expected 400, got %d", rec.Code)
	}

	// Owner export env & json
	if rec := doJSONReq(t, mux, http.MethodGet, "/api/vault/export?collection=app&environment=prod&format=env", nil); rec.Code != http.StatusOK {
		t.Fatalf("export env failed: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doJSONReq(t, mux, http.MethodGet, "/api/vault/export?collection=app&environment=prod&format=json", nil); rec.Code != http.StatusOK {
		t.Fatalf("export json failed: %d %s", rec.Code, rec.Body.String())
	}

	// Service tokens HTTP CRUD + read-only enforcement
	if rec := doJSONReq(t, mux, http.MethodPost, "/api/vault/tokens", map[string]any{"name": "Bad Perm", "permissions": "read_write"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("read_write token expected 400, got %d", rec.Code)
	}
	tokCreate := doJSONReq(t, mux, http.MethodPost, "/api/vault/tokens", map[string]any{
		"name":           "CI Reader",
		"collectionSlug": "app",
		"environment":    "prod",
		"permissions":    "read",
	})
	if tokCreate.Code != http.StatusCreated {
		t.Fatalf("create token failed: %d %s", tokCreate.Code, tokCreate.Body.String())
	}
	var tokOut struct {
		Token     VaultServiceToken `json:"token"`
		Plaintext string            `json:"plaintext"`
	}
	_ = json.Unmarshal(tokCreate.Body.Bytes(), &tokOut)

	if rec := doJSONReq(t, mux, http.MethodGet, "/api/vault/tokens", nil); rec.Code != http.StatusOK {
		t.Fatalf("list tokens failed: %d", rec.Code)
	}
	if rec := doJSONReq(t, mux, http.MethodDelete, fmt.Sprintf("/api/vault/tokens/%d", tokOut.Token.ID), nil); rec.Code != http.StatusOK {
		t.Fatalf("revoke token failed: %d", rec.Code)
	}
	if rec := doJSONReq(t, mux, http.MethodDelete, fmt.Sprintf("/api/vault/tokens/%d", tokOut.Token.ID), nil); rec.Code != http.StatusNotFound {
		t.Fatalf("second revoke expected 404, got %d", rec.Code)
	}

	// Revoked token & expired token rejected on /api/vault/raw
	rawReq := httptest.NewRequest(http.MethodGet, "/api/vault/raw", nil)
	rawReq.Header.Set("Authorization", "Bearer "+tokOut.Plaintext)
	rawRec := httptest.NewRecorder()
	mux.ServeHTTP(rawRec, rawReq)
	if rawRec.Code != http.StatusUnauthorized {
		t.Fatalf("revoked token expected 401, got %d", rawRec.Code)
	}
	past := time.Now().Add(-time.Hour)
	_, expRaw, err := p.CreateServiceToken(context.Background(), 1, "Expired", "app", "prod", "/", "", &past, 10)
	if err != nil {
		t.Fatalf("create expired token: %v", err)
	}
	if _, err := p.AuthenticateServiceToken(context.Background(), "Bearer "+expRaw); err == nil {
		t.Fatal("expected expired token error")
	}
	if _, err := p.AuthenticateServiceToken(context.Background(), "Bearer bad_prefix"); err == nil {
		t.Fatal("expected bad prefix error")
	}
	if _, err := p.AuthenticateServiceToken(context.Background(), "Bearer octvq_missing"); err == nil {
		t.Fatal("expected missing token error")
	}

	// Instance vault routes: non-instance-admin forbidden, uid=0 unauthorized, instance-admin full CRUD
	if rec := doJSONReq(t, mux, http.MethodGet, "/api/instance/vault/items", nil); rec.Code != http.StatusForbidden {
		t.Fatalf("non-instance-admin expected 403, got %d", rec.Code)
	}
	state.uid = 0
	if rec := doJSONReq(t, mux, http.MethodGet, "/api/instance/vault/items", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated instance vault expected 401, got %d", rec.Code)
	}
	state.uid = 10
	state.isInstanceAdmin = true

	instPut := doJSONReq(t, mux, http.MethodPost, "/api/instance/vault/items", map[string]any{
		"itemType": "instance_secret",
		"keyName":  "instance.metrics_token",
		"name":     "Metrics Token",
		"payload":  map[string]any{"token": "metrics-xyz"},
	})
	if instPut.Code != http.StatusOK {
		t.Fatalf("instance put expected 200, got %d: %s", instPut.Code, instPut.Body.String())
	}
	var instMeta plugin.VaultItemMeta
	_ = json.Unmarshal(instPut.Body.Bytes(), &instMeta)

	if rec := doJSONReq(t, mux, http.MethodGet, "/api/instance/vault/items", nil); rec.Code != http.StatusOK {
		t.Fatalf("instance list expected 200, got %d", rec.Code)
	}
	if rec := doJSONReq(t, mux, http.MethodPost, fmt.Sprintf("/api/instance/vault/items/%d/reveal", instMeta.ID), nil); rec.Code != http.StatusOK {
		t.Fatalf("instance reveal expected 200, got %d", rec.Code)
	}
	if rec := doJSONReq(t, mux, http.MethodDelete, fmt.Sprintf("/api/instance/vault/items/%d", instMeta.ID), nil); rec.Code != http.StatusOK {
		t.Fatalf("instance delete expected 200, got %d", rec.Code)
	}
	if rec := doJSONReq(t, mux, http.MethodPost, fmt.Sprintf("/api/instance/vault/items/%d/reveal", instMeta.ID), nil); rec.Code != http.StatusNotFound {
		t.Fatalf("missing instance reveal expected 404, got %d", rec.Code)
	}
	if rec := doJSONReq(t, mux, http.MethodDelete, fmt.Sprintf("/api/instance/vault/items/%d", instMeta.ID), nil); rec.Code != http.StatusNotFound {
		t.Fatalf("missing instance delete expected 404, got %d", rec.Code)
	}
}

func TestVault_TestConnEnvTotpRefsPurgeAndValidate(t *testing.T) {
	p, _, _ := setupVaultTest(t)
	ctx := context.Background()

	// Metadata, HelpDocsFS, MCPTier
	if p.Name() != "vault" || p.Describe().Title == "" || !p.Describe().Core || p.MCPTier() != "" || p.HelpDocsFS() == nil {
		t.Fatalf("unexpected plugin descriptor: name=%s desc=%+v", p.Name(), p.Describe())
	}
	hasEnc, err := p.HasEncryptedData()
	if err != nil || hasEnc {
		t.Fatalf("expected HasEncryptedData=false on empty DB, got %v err=%v", hasEnc, err)
	}

	// Valid SSH key & X.509 certificates (valid and expired)
	_, edPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519 keygen: %v", err)
	}
	sshPEM, err := ssh.MarshalPrivateKey(edPriv, "")
	if err != nil {
		t.Fatalf("marshal ssh key: %v", err)
	}
	validSSH := string(pem.EncodeToMemory(sshPEM))

	rsaKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	makeCert := func(notAfter time.Time) string {
		tmpl := &x509.Certificate{
			SerialNumber: big.NewInt(1),
			Subject:      pkix.Name{CommonName: "vault.test"},
			NotBefore:    time.Now().Add(-24 * time.Hour),
			NotAfter:     notAfter,
		}
		der, _ := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &rsaKey.PublicKey, rsaKey)
		return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	}

	cases := []struct {
		item   VaultItem
		pay    map[string]any
		wantOK bool
	}{
		{VaultItem{ItemType: "ssh_key"}, map[string]any{}, false},
		{VaultItem{ItemType: "ssh_key"}, map[string]any{"privateKey": "not-a-key"}, false},
		{VaultItem{ItemType: "ssh_key"}, map[string]any{"privateKey": validSSH}, true},
		{VaultItem{ItemType: "certificate"}, map[string]any{"certPem": "bad"}, false},
		{VaultItem{ItemType: "certificate"}, map[string]any{"certPem": string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("bad")}))}, false},
		{VaultItem{ItemType: "certificate"}, map[string]any{"certPem": makeCert(time.Now().Add(-time.Hour))}, false},
		{VaultItem{ItemType: "certificate"}, map[string]any{"certPem": makeCert(time.Now().Add(24 * time.Hour))}, true},
		{VaultItem{ItemType: "twofa_totp"}, map[string]any{}, false},
		{VaultItem{ItemType: "twofa_totp"}, map[string]any{"secret": "JBSWY3DPEHPK3PXP"}, true},
		{VaultItem{ItemType: "database"}, map[string]any{}, false},
		{VaultItem{ProviderType: "llm", Provider: "openai"}, map[string]any{}, false},
		{VaultItem{ProviderType: "llm", Provider: "openai"}, map[string]any{"apiKey": "sk-1"}, true},
		{VaultItem{ProviderType: "llm", Provider: "ollama"}, map[string]any{"baseUrl": "http://127.0.0.1:11434"}, false},
		{VaultItem{ProviderType: "payment", Provider: "stripe"}, map[string]any{}, false},
		{VaultItem{ProviderType: "payment", Provider: "polar"}, map[string]any{}, false},
		{VaultItem{ProviderType: "payment", Provider: "wechat"}, map[string]any{"mchId": "m1"}, false},
		{VaultItem{ProviderType: "payment", Provider: "alipay"}, map[string]any{"appId": "a1"}, false},
		{VaultItem{ProviderType: "payment", Provider: "stripe"}, map[string]any{"secretKey": "sk_1"}, true},
		{VaultItem{ProviderType: "storage", Provider: "s3"}, map[string]any{"bucket": "b"}, false},
		{VaultItem{ProviderType: "storage", Provider: "s3"}, map[string]any{"bucket": "b", "apiKey": "k", "secretKey": "s"}, true},
		{VaultItem{ProviderType: "communication", Provider: "telegram"}, map[string]any{}, false},
		{VaultItem{ProviderType: "communication", Provider: "slack"}, map[string]any{}, false},
		{VaultItem{ProviderType: "communication", Provider: "slack"}, map[string]any{"webhookUrl": "https://hooks.slack.com/x"}, true},
		{VaultItem{ProviderType: "dns", Provider: "cloudflare"}, map[string]any{}, false},
		{VaultItem{ProviderType: "dns", Provider: "cloudflare"}, map[string]any{"apiToken": "tok"}, true},
		{VaultItem{ItemType: "kv_secret"}, map[string]any{"value": "abc"}, true},
	}
	for i, tc := range cases {
		ok, _ := p.testVaultItem(ctx, tc.item, tc.pay)
		if ok != tc.wantOK {
			t.Fatalf("case %d (%+v): got ok=%v, want %v", i, tc.item, ok, tc.wantOK)
		}
	}

	// TOTP otpauth URI + period branches
	if code, rem, err := computeTOTPCode(map[string]any{
		"otpauthUri": "otpauth://totp/Octarq:ops@example.com?secret=JBSWY3DPEHPK3PXP&period=60&digits=6",
	}, time.Now()); err != nil || len(code) != 6 || rem <= 0 {
		t.Fatalf("otpauth TOTP failed: code=%q rem=%d err=%v", code, rem, err)
	}
	if _, _, err := computeTOTPCode(map[string]any{"otpauthUri": "otpauth://totp/%zz"}, time.Now()); err == nil {
		t.Fatal("expected invalid otpauth error")
	}
	if _, _, err := computeTOTPCode(map[string]any{"secret": "NOT_BASE32!"}, time.Now()); err == nil {
		t.Fatal("expected invalid base32 secret error")
	}
	if code, _, err := computeTOTPCode(map[string]any{"secret": "JBSWY3DPEHPK3PXP", "period": float64(45)}, time.Now()); err != nil || len(code) != 6 {
		t.Fatalf("custom period TOTP failed: %v", err)
	}

	// parseVaultRef error paths + formatDotEnv multi-field escaping
	for _, badRef := range []string{"http://x", "vault://id/", "vault://id/abc", "vault://onlycol", "vault://col/prod/"} {
		if _, err := parseVaultRef(badRef); err == nil {
			t.Fatalf("expected parseVaultRef error for %q", badRef)
		}
	}
	envStr := formatDotEnv([]plugin.VaultSecret{{
		Meta:    plugin.VaultItemMeta{ID: 1, Name: "DB Config"},
		Payload: map[string]any{"host": "localhost", "pass": "a b\"c\nd"},
	}})
	if !strings.Contains(envStr, "DB_CONFIG_HOST=localhost") || !strings.Contains(envStr, `DB_CONFIG_PASS="a b\"c\nd"`) {
		t.Fatalf("unexpected formatDotEnv output: %q", envStr)
	}

	// Put secret -> HasEncryptedData=true -> exportData -> MCP list/meta/test -> purge -> Validate failure check
	item, err := p.PutSecret(ctx, plugin.PutSecretRequest{
		OrgID:          1,
		Scope:          plugin.VaultScopeOrg,
		CollectionSlug: "ops",
		Environment:    "prod",
		ItemType:       "kv_secret",
		KeyName:        "API_TOKEN",
		Name:           "Ops API Token",
		Payload:        map[string]any{"value": "super-secret"},
	})
	if err != nil {
		t.Fatalf("PutSecret: %v", err)
	}
	hasEnc, err = p.HasEncryptedData()
	if err != nil || !hasEnc {
		t.Fatalf("expected HasEncryptedData=true, got %v err=%v", hasEnc, err)
	}

	exported := p.exportData(1)
	itemsAny, ok := exported["items"].([]plugin.VaultItemMeta)
	if !ok || len(itemsAny) == 0 {
		t.Fatalf("expected exported metadata items, got %+v", exported)
	}

	// MCP list_items, get_item_meta, test_integration
	srv := mcp.NewServer(&mcp.Implementation{Name: "vault-test", Version: "1"}, nil)
	p.RegisterMCP(srv)
	c := plugin.WithTokenRole(plugin.WithOrgID(ctx, 1), "member")
	sT, cT := mcp.NewInMemoryTransports()
	ss, _ := srv.Connect(c, sT, nil)
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1"}, nil)
	cs, _ := client.Connect(c, cT, nil)
	if res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "octarq_vault__list_items", Arguments: map[string]any{}}); err != nil || res.IsError {
		t.Fatalf("MCP list_items failed: res=%+v err=%v", res, err)
	}
	if res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "octarq_vault__get_item_meta", Arguments: map[string]any{"itemId": item.ID}}); err != nil || res.IsError {
		t.Fatalf("MCP get_item_meta failed: res=%+v err=%v", res, err)
	}
	if res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "octarq_vault__test_integration", Arguments: map[string]any{"itemId": item.ID}}); err != nil || res.IsError {
		t.Fatalf("MCP test_integration failed: res=%+v err=%v", res, err)
	}
	_ = cs.Close()
	_ = ss.Close()

	// Validate fails closed when DEK is missing
	_ = p.shredItemKey(1, item.SecretID)
	if err := p.Validate(ctx, p.host); err == nil {
		t.Fatal("expected Validate to fail when subject DEK is missing")
	}

	// Purge removes all items/collections/tokens
	if err := p.purge(1); err != nil {
		t.Fatalf("purge: %v", err)
	}
	remaining, err := p.ListItems(ctx, 1, plugin.VaultFilter{Scope: plugin.VaultScopeOrg})
	if err != nil || len(remaining) != 0 {
		t.Fatalf("expected 0 items after purge, got %+v err=%v", remaining, err)
	}
}

func TestVault_StoreErrorPathsAndEdgeCases(t *testing.T) {
	ctx := context.Background()

	// Nil DB / unmounted plugin guards
	pNil := New()
	pNil.Mount(nil, nil)
	if enc, err := pNil.HasEncryptedData(); err != nil || enc {
		t.Fatalf("nil DB HasEncryptedData: enc=%v err=%v", enc, err)
	}
	if err := pNil.Validate(ctx, nil); err == nil {
		t.Fatal("expected Validate error when crypto unavailable")
	}
	if _, err := pNil.PutSecret(ctx, plugin.PutSecretRequest{OrgID: 1, Name: "X"}); err == nil {
		t.Fatal("expected PutSecret error on nil DB")
	}
	if _, err := pNil.PutInstanceSecret(ctx, plugin.PutSecretRequest{Name: "X"}); err == nil {
		t.Fatal("expected PutInstanceSecret error on nil DB")
	}
	if _, err := pNil.UpdateSecret(ctx, 1, 1, nil, nil, nil, nil, "", 0); err == nil {
		t.Fatal("expected UpdateSecret error on nil DB")
	}
	if _, err := pNil.RollbackVersion(ctx, 1, 1, 1, 0); err == nil {
		t.Fatal("expected RollbackVersion error on nil DB")
	}
	if _, err := pNil.GetMeta(ctx, 1, 1); err == nil {
		t.Fatal("expected GetMeta error on nil DB")
	}
	if _, err := pNil.GetSecret(ctx, 1, 1); err == nil {
		t.Fatal("expected GetSecret error on nil DB")
	}
	if _, err := pNil.ResolveByRef(ctx, 1, "vault://id/1"); err == nil {
		t.Fatal("expected ResolveByRef error on nil DB")
	}
	if _, err := pNil.ResolveInstanceSecret(ctx, "k"); err == nil {
		t.Fatal("expected ResolveInstanceSecret error on nil DB")
	}
	if _, err := pNil.ListItems(ctx, 1, plugin.VaultFilter{}); err == nil {
		t.Fatal("expected ListItems error on nil DB")
	}
	if err := pNil.DeleteSecret(ctx, 1, 1); err == nil {
		t.Fatal("expected DeleteSecret error on nil DB")
	}
	if err := pNil.DeleteInstanceSecret(ctx, "k"); err == nil {
		t.Fatal("expected DeleteInstanceSecret error on nil DB")
	}
	if _, err := pNil.LookupCredential(ctx, 1, plugin.CredentialQuery{}); err == nil {
		t.Fatal("expected LookupCredential error on nil DB")
	}
	if _, _, err := pNil.CreateServiceToken(ctx, 1, "t", "default", "prod", "/", "read", nil, 0); err == nil {
		t.Fatal("expected CreateServiceToken error on nil DB")
	}
	if _, err := pNil.AuthenticateServiceToken(ctx, "Bearer octvq_123"); err == nil {
		t.Fatal("expected AuthenticateServiceToken error on nil DB")
	}
	if err := pNil.purge(1); err != nil {
		t.Fatalf("purge on nil DB should be nil, got %v", err)
	}
	if got := pNil.exportData(0); len(got["items"].([]plugin.VaultItemMeta)) != 0 {
		t.Fatalf("exportData(0) expected empty, got %+v", got)
	}

	// Mounted plugin with Provide callback and Store edge cases
	p, db, mux := setupVaultTest(t)
	provided := map[string]any{}
	p.Mount(nil, &plugin.Context{
		DB:   db,
		Host: p.host,
		Provide: func(name string, svc any) {
			provided[name] = svc
		},
	})
	if provided[plugin.ServiceVault] == nil || provided[plugin.ServiceCredentialResolver] == nil {
		t.Fatalf("expected Mount to provide vault services, got %+v", provided)
	}

	// PutSecret validation errors
	if _, err := p.PutSecret(ctx, plugin.PutSecretRequest{OrgID: 1, Scope: plugin.VaultScopeInstance, Name: "X"}); err == nil {
		t.Fatal("PutSecret must reject instance scope")
	}
	if _, err := p.PutSecret(ctx, plugin.PutSecretRequest{OrgID: 0, Name: "X"}); err == nil {
		t.Fatal("PutSecret must reject orgID=0")
	}
	if _, err := p.PutSecret(ctx, plugin.PutSecretRequest{OrgID: 1}); err == nil {
		t.Fatal("PutSecret must reject empty name and keyName")
	}
	if _, err := p.PutSecret(ctx, plugin.PutSecretRequest{
		OrgID: 1, ItemType: "integration", ProviderType: "payment", Provider: "stripe", Name: "Bad Stripe",
		Payload: map[string]any{},
	}); err == nil {
		t.Fatal("expected validatePayload error for missing required stripe secretKey")
	}

	// KeyName collision creates v2 and honors IsDefault
	v1, err := p.PutSecret(ctx, plugin.PutSecretRequest{
		OrgID: 1, ItemType: "integration", ProviderType: "llm", Provider: "openai",
		KeyName: "COLLIDE_OPENAI", Name: "OpenAI 1",
		Payload: map[string]any{"apiKey": "sk-1", "model": "gpt-4o"},
	})
	if err != nil {
		t.Fatalf("PutSecret v1: %v", err)
	}
	v2, err := p.PutSecret(ctx, plugin.PutSecretRequest{
		OrgID: 1, ItemType: "integration", ProviderType: "llm", Provider: "openai",
		KeyName: "COLLIDE_OPENAI", Name: "OpenAI 2", IsDefault: true,
		Payload: map[string]any{"apiKey": "sk-2", "model": "gpt-4.1"},
	})
	if err != nil || v2.CurrentVersion != 2 || !v2.IsDefault {
		t.Fatalf("expected keyName collision to update v2 default, got %+v err=%v", v2, err)
	}

	// UpdateSecret: keep masked field ("••••••••"), clear field (""), non-string field, reject secret-looking metadata key
	upd, err := p.UpdateSecret(ctx, 1, v1.ID, nil,
		map[string]any{"apiKey": "leaked", "region": "us-east"},
		map[string]any{"apiKey": "••••••••", "model": "", "maxRetries": 3},
		nil, "partial update", 10,
	)
	if err != nil || upd.CurrentVersion != 3 {
		t.Fatalf("UpdateSecret partial failed: %+v err=%v", upd, err)
	}
	secUpd, err := p.GetSecret(ctx, 1, v1.ID)
	if err != nil || secUpd.Payload["apiKey"] != "sk-2" || secUpd.Payload["model"] != nil {
		t.Fatalf("unexpected merged payload: %+v err=%v", secUpd, err)
	}
	if _, hasLeak := upd.Metadata["apiKey"]; hasLeak {
		t.Fatalf("secret-looking key leaked into metadata: %+v", upd.Metadata)
	}

	// Not-found & invalid-param branches across store methods
	if _, err := p.UpdateSecret(ctx, 1, 99999, nil, nil, nil, nil, "", 0); !errors.Is(err, plugin.ErrSecretNotFound) {
		t.Fatalf("expected ErrSecretNotFound on UpdateSecret, got %v", err)
	}
	if _, err := p.RollbackVersion(ctx, 1, 99999, 1, 0); !errors.Is(err, plugin.ErrSecretNotFound) {
		t.Fatalf("expected ErrSecretNotFound on RollbackVersion, got %v", err)
	}
	if _, err := p.RollbackVersion(ctx, 1, v1.ID, 999, 0); err == nil {
		t.Fatal("expected missing target version error")
	}
	if _, err := p.GetMeta(ctx, 1, 99999); !errors.Is(err, plugin.ErrSecretNotFound) {
		t.Fatalf("expected ErrSecretNotFound on GetMeta, got %v", err)
	}
	if err := p.DeleteSecret(ctx, 1, 99999); !errors.Is(err, plugin.ErrSecretNotFound) {
		t.Fatalf("expected ErrSecretNotFound on DeleteSecret, got %v", err)
	}
	if _, _, err := p.ComputeTOTP(ctx, 1, 99999); !errors.Is(err, plugin.ErrSecretNotFound) {
		t.Fatalf("expected ErrSecretNotFound on ComputeTOTP, got %v", err)
	}
	if _, err := p.ResolveByRef(ctx, 1, "vault://id/99999"); !errors.Is(err, plugin.ErrSecretNotFound) {
		t.Fatalf("expected ErrSecretNotFound on missing id ref, got %v", err)
	}
	if _, err := p.ResolveByRef(ctx, 1, "vault://no-such-col/prod/KEY"); !errors.Is(err, plugin.ErrSecretNotFound) {
		t.Fatalf("expected ErrSecretNotFound on missing collection ref, got %v", err)
	}
	if _, err := p.ResolveByRef(ctx, 1, "vault://integrations/prod/MISSING_KEY"); !errors.Is(err, plugin.ErrSecretNotFound) {
		t.Fatalf("expected ErrSecretNotFound on missing key ref, got %v", err)
	}
	if _, err := p.ResolveInstanceSecret(ctx, "   "); err == nil {
		t.Fatal("expected error for empty instance keyName")
	}

	// LookupCredential by ID, by ProviderType only, and ErrCredentialNotFound
	byID, err := p.LookupCredential(ctx, 1, plugin.CredentialQuery{ID: v1.ID})
	if err != nil || byID.ID != v1.ID {
		t.Fatalf("LookupCredential by ID: %+v err=%v", byID, err)
	}
	if _, err := p.LookupCredential(ctx, 0, plugin.CredentialQuery{}); err == nil {
		t.Fatal("expected error for orgID=0 on LookupCredential")
	}
	if _, err := p.LookupCredential(ctx, 1, plugin.CredentialQuery{ProviderType: "dns"}); !errors.Is(err, plugin.ErrCredentialNotFound) {
		t.Fatalf("expected ErrCredentialNotFound, got %v", err)
	}

	// CreateServiceToken error branches
	if _, _, err := p.CreateServiceToken(ctx, 0, "t", "default", "prod", "/", "read", nil, 0); err == nil {
		t.Fatal("expected error for orgID=0")
	}
	if _, _, err := p.CreateServiceToken(ctx, 1, "   ", "default", "prod", "/", "read", nil, 0); err == nil {
		t.Fatal("expected error for empty token name")
	}
	if _, _, err := p.CreateServiceToken(ctx, 1, "t", "missing-col", "prod", "/", "read", nil, 0); err == nil {
		t.Fatal("expected error for missing collection")
	}

	// MCP error paths (missing orgID, missing item, invalid payload)
	srv := mcp.NewServer(&mcp.Implementation{Name: "vault-err-test", Version: "1"}, nil)
	p.RegisterMCP(srv)
	sT, cT := mcp.NewInMemoryTransports()
	ss, _ := srv.Connect(ctx, sT, nil)
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1"}, nil)
	cs, _ := client.Connect(ctx, cT, nil)
	for _, tc := range []struct {
		name string
		args map[string]any
	}{
		{"octarq_vault__list_items", map[string]any{}},
		{"octarq_vault__get_item_meta", map[string]any{"itemId": 1}},
		{"octarq_vault__set_secret", map[string]any{"name": "X", "payload": map[string]any{}}},
		{"octarq_vault__test_integration", map[string]any{"itemId": 1}},
		{"octarq_vault__get_totp", map[string]any{"itemId": 1}},
	} {
		if res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: tc.name, Arguments: tc.args}); err == nil && !res.IsError {
			t.Fatalf("expected MCP %s without orgID to fail", tc.name)
		}
	}
	_ = cs.Close()
	_ = ss.Close()

	// Auth errors on HTTP routes when host/session is nil
	pNoHost := &Plugin{db: db}
	muxNoHost := http.NewServeMux()
	humaNoHost := humago.New(muxNoHost, huma.DefaultConfig("NoHost", "1.0.0"))
	pNoHost.registerRoutes(muxNoHost, &plugin.Context{DB: db, Huma: humaNoHost})
	if rec := doJSONReq(t, muxNoHost, http.MethodGet, "/api/vault/items", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with nil host, got %d", rec.Code)
	}
	if rec := doJSONReq(t, muxNoHost, http.MethodGet, "/api/instance/vault/items", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 on instance vault with nil host, got %d", rec.Code)
	}
	rawReq := httptest.NewRequest(http.MethodGet, "/api/vault/raw", nil)
	rawRec := httptest.NewRecorder()
	muxNoHost.ServeHTTP(rawRec, rawReq)
	if rawRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 on /api/vault/raw without bearer, got %d", rawRec.Code)
	}

	// schemas.go & extractNonSecretMetadata edge branches
	if err := validatePayload("kv_secret", "", "", nil); err == nil {
		t.Fatal("expected error for nil payload on required kv_secret")
	}
	if err := validatePayload("integration", "payment", "stripe", map[string]any{"secretKey": "   "}); err == nil {
		t.Fatal("expected error for whitespace required secretKey")
	}
	metaOut := extractNonSecretMetadata("integration", "llm", "openai", map[string]any{"model": "gpt-4o", "apiKey": "sk-1"}, map[string]any{"secretToken": "no", "label": "prod"})
	if metaOut["model"] != "gpt-4o" || metaOut["label"] != "prod" {
		t.Fatalf("unexpected extracted metadata: %+v", metaOut)
	}
	if _, hasSec := metaOut["secretToken"]; hasSec {
		t.Fatalf("secretToken leaked into metadata: %+v", metaOut)
	}

	// Create item via HTTP with missing required field -> 400, update missing item -> 404
	if rec := doJSONReq(t, mux, http.MethodPost, "/api/vault/items", map[string]any{
		"name": "Missing Payload", "itemType": "integration", "providerType": "payment", "provider": "stripe", "payload": map[string]any{},
	}); rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing stripe secretKey, got %d", rec.Code)
	}
	if rec := doJSONReq(t, mux, http.MethodPut, "/api/vault/items/99999", map[string]any{"name": "Missing"}); rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 updating missing item, got %d", rec.Code)
	}

	// Crypto seal/open error paths (corrupted ciphertext, empty ciphertext, failing SubjectVault)
	itemEnc, err := p.PutSecret(ctx, plugin.PutSecretRequest{
		OrgID: 1, Scope: plugin.VaultScopeOrg, Name: "Enc Test", KeyName: "ENC_TEST",
		Payload: map[string]any{"value": "abc"},
	})
	if err != nil {
		t.Fatalf("PutSecret Enc Test: %v", err)
	}
	if decEmpty, err := p.openPayload(1, itemEnc.SecretID, 1, ""); err != nil || len(decEmpty) != 0 {
		t.Fatalf("openPayload empty: %+v err=%v", decEmpty, err)
	}
	if _, err := p.openPayload(1, itemEnc.SecretID, 1, "tampered-ciphertext"); err == nil {
		t.Fatal("expected openPayload error on tampered ciphertext")
	}
	// Corrupt stored ciphertext in version 1 and verify GetSecret / UpdateSecret / RollbackVersion fail closed
	db.Model(&VaultItemVersion{}).Where("item_id = ? AND version = 1", itemEnc.ID).Update("ciphertext", "corrupted")
	if _, err := p.GetSecret(ctx, 1, itemEnc.ID); err == nil {
		t.Fatal("expected GetSecret failure on corrupted ciphertext")
	}
	if _, err := p.UpdateSecret(ctx, 1, itemEnc.ID, nil, nil, map[string]any{"value": "new"}, nil, "", 10); err == nil {
		t.Fatal("expected UpdateSecret failure when current version ciphertext is corrupted")
	}
	if _, err := p.RollbackVersion(ctx, 1, itemEnc.ID, 1, 10); err == nil {
		t.Fatal("expected RollbackVersion failure when target version ciphertext is corrupted")
	}
	// Missing active version row
	db.Where("item_id = ?", itemEnc.ID).Delete(&VaultItemVersion{})
	if _, err := p.GetSecret(ctx, 1, itemEnc.ID); err == nil {
		t.Fatal("expected GetSecret error when active version row is missing")
	}

	// Unparseable JSON in openPayload + nil crypto in seal/open/shred
	pNoCrypto := &Plugin{db: db}
	if _, err := pNoCrypto.sealPayload(1, "s", 1, map[string]any{"a": 1}); err == nil {
		t.Fatal("expected sealPayload error without crypto")
	}
	if _, err := pNoCrypto.openPayload(1, "s", 1, "x"); err == nil {
		t.Fatal("expected openPayload error without crypto")
	}
	if err := pNoCrypto.shredItemKey(1, "s"); err == nil {
		t.Fatal("expected shredItemKey error without crypto")
	}
}
