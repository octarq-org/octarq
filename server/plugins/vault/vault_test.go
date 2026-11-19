package vault

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/glebarez/sqlite"
	"github.com/modelcontextprotocol/go-sdk/mcp"
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
