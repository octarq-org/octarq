package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/internal/notify"
)

func TestNotificationChannelConfigEncryptedInDB(t *testing.T) {
	h, srv, db := newTestHandlerRaw(t)

	// Set up notify decryptor to use h.cipher
	notify.SetConfigDecryptor(func(stored string) (string, bool) {
		b, err := h.cipher.Decrypt(stored)
		if err != nil {
			return "", false
		}
		return string(b), true
	})

	const orgID = uint(101)
	uid := seedOrgMember(t, db, orgID, "admin@notifyenc.com", "admin")
	sess := sessionCookies(t, uid, orgID)

	plaintextConfig := `{"token":"my-super-secret-token","endpoint":"https://example.com"}`

	// 1. Create a notification channel via API
	jsonConfig, err := json.Marshal(plaintextConfig)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	reqBody := `{"name":"Custom Channel","type":"custom","config":` + string(jsonConfig) + `}`
	rec := do(srv, "POST", "/api/v1/notification-channels", sess, reqBody)
	if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
		t.Fatalf("expected HTTP 200/201 on create, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify response has secrets redacted
	respStr := rec.Body.String()
	if strings.Contains(respStr, "my-super-secret-token") {
		t.Errorf("expected secret token to be redacted in API response, got: %s", respStr)
	}
	if !strings.Contains(respStr, "[REDACTED]") {
		t.Errorf("expected [REDACTED] in API response, got: %s", respStr)
	}

	// 2. Query DB directly: verify config column is encrypted (not equal to plaintext)
	var storedCh models.NotificationChannel
	if err := db.Where("owner_id = ? AND name = ?", orgID, "Custom Channel").First(&storedCh).Error; err != nil {
		t.Fatalf("failed to find channel in DB: %v", err)
	}
	if storedCh.Config == plaintextConfig {
		t.Fatalf("DB config column stored raw plaintext instead of ciphertext!")
	}

	// 3. Register mock provider and verify notify.Send receives decrypted plaintext
	var receivedConfig string
	notify.Register("custom", func(ctx context.Context, cfgJSON, text string) error {
		receivedConfig = cfgJSON
		return nil
	})

	// Test notify.Send end-to-end with the stored encrypted config
	if err := notify.Send(context.Background(), "custom", storedCh.Config, "hello test"); err != nil {
		t.Fatalf("notify.Send failed: %v", err)
	}
	if receivedConfig != plaintextConfig {
		t.Errorf("expected notify.Send to receive decrypted plaintext %q, got %q", plaintextConfig, receivedConfig)
	}

	// 4. Test legacy plaintext row in DB: inserting raw plaintext config directly into DB
	legacyConfig := `{"token":"legacy-token","endpoint":"https://legacy.com"}`
	legacyCh := models.NotificationChannel{
		OrgID:   orgID,
		Name:    "Legacy Channel",
		Type:    "custom",
		Config:  legacyConfig,
		Enabled: true,
	}
	db.Create(&legacyCh)

	// Assert end-to-end send for legacy plaintext row succeeds
	receivedConfig = ""
	if err := notify.Send(context.Background(), "custom", legacyCh.Config, "hello legacy"); err != nil {
		t.Fatalf("notify.Send failed for legacy row: %v", err)
	}
	if receivedConfig != legacyConfig {
		t.Errorf("expected legacy plaintext row to fall back to raw string %q, got %q", legacyConfig, receivedConfig)
	}
}
