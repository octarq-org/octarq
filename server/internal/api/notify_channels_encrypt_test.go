package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/internal/notification"
	"github.com/octarq-org/octarq/server/plugin"
)

type configCaptureChannel struct {
	send func(cfgJSON string)
}

func (c *configCaptureChannel) Name() string                  { return "custom" }
func (c *configCaptureChannel) DisplayName() string           { return "Custom" }
func (c *configCaptureChannel) ConfigSchema() json.RawMessage { return nil }

func (c *configCaptureChannel) Send(_ context.Context, recipient plugin.NotificationRecipient, _ plugin.NotificationPayload) error {
	cfgJSON := "{}"
	if recipient.Config != nil {
		if raw, ok := recipient.Config["_raw_config"].(string); ok && strings.TrimSpace(raw) != "" {
			cfgJSON = raw
		}
	}
	c.send(cfgJSON)
	return nil
}

func TestNotificationChannelConfigEncryptedInDB(t *testing.T) {
	h, srv, db := newTestHandlerRaw(t)

	const orgID = uint(101)
	uid := seedOrgMember(t, db, orgID, "admin@notifyenc.com", "admin")
	sess := sessionCookies(t, uid, orgID)

	plaintextConfig := `{"token":"my-super-secret-token","endpoint":"https://example.com"}`

	jsonConfig, err := json.Marshal(plaintextConfig)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	reqBody := `{"name":"Custom Channel","type":"custom","config":` + string(jsonConfig) + `}`
	rec := do(srv, "POST", "/api/notification-channels", sess, reqBody)
	if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
		t.Fatalf("expected HTTP 200/201 on create, got %d: %s", rec.Code, rec.Body.String())
	}

	respStr := rec.Body.String()
	if strings.Contains(respStr, "my-super-secret-token") {
		t.Errorf("expected secret token to be redacted in API response, got: %s", respStr)
	}
	if !strings.Contains(respStr, "[REDACTED]") {
		t.Errorf("expected [REDACTED] in API response, got: %s", respStr)
	}

	var storedCh models.NotificationChannel
	if err := db.Where("owner_id = ? AND name = ?", orgID, "Custom Channel").First(&storedCh).Error; err != nil {
		t.Fatalf("failed to find channel in DB: %v", err)
	}
	if strings.Contains(storedCh.Config, "my-super-secret-token") {
		t.Fatalf("DB config column stored raw secret instead of moving it to Vault!")
	}
	if storedCh.VaultItemID == 0 {
		t.Fatalf("expected VaultItemID > 0 for channel with secret token")
	}

	var receivedConfig string
	if err := notification.DefaultRouter().RegisterChannel(&configCaptureChannel{
		send: func(cfgJSON string) {
			receivedConfig = cfgJSON
		},
	}); err != nil {
		t.Fatalf("register custom channel: %v", err)
	}

	resolvedJSON, err := notification.ResolveChannelConfigJSON(context.Background(), h.vault(), storedCh)
	if err != nil {
		t.Fatalf("ResolveChannelConfigJSON failed: %v", err)
	}
	if err := notification.DefaultRouter().SendDirect(context.Background(), "custom", resolvedJSON, "hello test"); err != nil {
		t.Fatalf("SendDirect failed: %v", err)
	}
	var gotMap, wantMap map[string]any
	if err := json.Unmarshal([]byte(receivedConfig), &gotMap); err != nil {
		t.Fatalf("unmarshal receivedConfig: %v", err)
	}
	_ = json.Unmarshal([]byte(plaintextConfig), &wantMap)
	if gotMap["token"] != wantMap["token"] || gotMap["endpoint"] != wantMap["endpoint"] {
		t.Errorf("expected resolved config %v, got %v", wantMap, gotMap)
	}
}
