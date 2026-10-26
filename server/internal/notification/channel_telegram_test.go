package notification

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

func TestTelegramChannel_Basics(t *testing.T) {
	ch := NewTelegramChannel()
	if ch.Name() != "telegram" {
		t.Errorf("Name() = %q, want telegram", ch.Name())
	}
	if ch.DisplayName() != "Telegram" {
		t.Errorf("DisplayName() = %q, want Telegram", ch.DisplayName())
	}
	schema := ch.ConfigSchema()
	if len(schema) == 0 || !strings.Contains(string(schema), "botToken") {
		t.Errorf("unexpected ConfigSchema: %s", string(schema))
	}
}

func TestTelegramChannel_ValidationAndErrors(t *testing.T) {
	ch := NewTelegramChannel()

	// 1. Missing credentials
	err := ch.Send(context.Background(), plugin.NotificationRecipient{}, plugin.NotificationPayload{Body: "hello"})
	if err == nil || !strings.Contains(err.Error(), "missing telegram credentials") {
		t.Errorf("expected missing credentials error, got %v", err)
	}

	// 2. Empty botToken in config
	err = ch.Send(context.Background(), plugin.NotificationRecipient{
		Config: map[string]any{"botToken": "", "chatId": "12345"},
	}, plugin.NotificationPayload{Body: "hello"})
	if err == nil || !strings.Contains(err.Error(), "missing telegram credentials") {
		t.Errorf("expected missing credentials error, got %v", err)
	}

	// 3. Empty chatId in config
	err = ch.Send(context.Background(), plugin.NotificationRecipient{
		Config: map[string]any{"botToken": "token", "chatId": ""},
	}, plugin.NotificationPayload{Body: "hello"})
	if err == nil || !strings.Contains(err.Error(), "missing telegram credentials") {
		t.Errorf("expected missing credentials error, got %v", err)
	}

	// 4. Invalid JSON in _raw_config
	err = ch.Send(context.Background(), plugin.NotificationRecipient{
		Config: map[string]any{"_raw_config": "invalid-json"},
	}, plugin.NotificationPayload{Body: "hello"})
	if err == nil {
		t.Error("expected error for malformed _raw_config, got nil")
	}

	// 5. Invalid chatId (not a number)
	err = ch.Send(context.Background(), plugin.NotificationRecipient{
		Config: map[string]any{"botToken": "token", "chatId": "not-a-number"},
	}, plugin.NotificationPayload{Body: "hello"})
	if err == nil || !strings.Contains(err.Error(), "invalid telegram chatId") {
		t.Errorf("expected invalid chatId error, got %v", err)
	}

	// 6. Invalid bot token format triggers initialization error
	err = ch.Send(context.Background(), plugin.NotificationRecipient{
		Config: map[string]any{"botToken": "invalid-token", "chatId": "12345"},
	}, plugin.NotificationPayload{Body: "hello"})
	if err == nil || !strings.Contains(err.Error(), "failed to initialize telegram notifier") {
		t.Errorf("expected failed to initialize error, got %v", err)
	}

	// 7. Valid token format with non-existent token triggers API error
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err = ch.Send(ctx, plugin.NotificationRecipient{
		Config: map[string]any{
			"botToken": "123456789:ABCdefGHIjklMNOpqrsTUVwxyz_12345678",
			"chatId":   "123456",
		},
	}, plugin.NotificationPayload{Title: "Only Title"})
	if err == nil {
		t.Error("expected error from API with fake token, got nil")
	}

	// 8. Test with body instead of title
	ctx2, cancel2 := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel2()
	_ = ch.Send(ctx2, plugin.NotificationRecipient{
		Config: map[string]any{
			"botToken": "123456789:ABCdefGHIjklMNOpqrsTUVwxyz_12345678",
			"chatId":   "123456",
		},
	}, plugin.NotificationPayload{Body: "Body message"})
}

func TestTelegramChannel_ConfigFormats(t *testing.T) {
	ch := NewTelegramChannel()

	// Test different types for chatId: float64, int64, int
	types := []any{float64(12345), int64(12345), int(12345)}
	for _, cid := range types {
		err := ch.Send(context.Background(), plugin.NotificationRecipient{
			Config: map[string]any{
				"botToken": "invalid-token",
				"chatId":   cid,
			},
		}, plugin.NotificationPayload{Body: "hello"})
		if err == nil || !strings.Contains(err.Error(), "failed to initialize telegram notifier") {
			t.Errorf("chatId %T %v: expected initialization error, got %v", cid, cid, err)
		}
	}

	// Test _raw_config with float64, int, int64
	rawFormats := []string{
		`{"botToken":"invalid-token","chatId":12345}`,
		`{"botToken":"invalid-token","chatId":"12345"}`,
	}
	for _, raw := range rawFormats {
		err := ch.Send(context.Background(), plugin.NotificationRecipient{
			Config: map[string]any{"_raw_config": raw},
		}, plugin.NotificationPayload{Body: "hello"})
		if err == nil || !strings.Contains(err.Error(), "failed to initialize telegram notifier") {
			t.Errorf("raw %s: expected initialization error, got %v", raw, err)
		}
	}
}

func TestTelegramChannel_DBLookup(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	_ = db.AutoMigrate(&models.NotificationChannel{})

	SetConfigDecryptor(func(stored string) (string, bool) {
		return stored, true
	})
	t.Cleanup(func() { SetConfigDecryptor(nil) })

	// Insert active telegram channel for org 1
	db.Create(&models.NotificationChannel{
		OrgID:   1,
		Type:    "telegram",
		Config:  `{"botToken":"invalid-token","chatId":"12345"}`,
		Enabled: true,
	})

	ch := NewTelegramChannel(db)
	err = ch.Send(context.Background(), plugin.NotificationRecipient{
		OrgID: "1",
	}, plugin.NotificationPayload{Body: "hello"})
	if err == nil || !strings.Contains(err.Error(), "failed to initialize telegram notifier") {
		t.Errorf("expected initialization error after resolving DB config, got %v", err)
	}

	// Org 2 has no channel configured -> missing credentials
	err = ch.Send(context.Background(), plugin.NotificationRecipient{
		OrgID: "2",
	}, plugin.NotificationPayload{Body: "hello"})
	if err == nil || !strings.Contains(err.Error(), "missing telegram credentials") {
		t.Errorf("expected missing credentials error for org 2, got %v", err)
	}
}

func TestTelegramChannel_TenantIntegrationsLookup(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	_ = db.AutoMigrate(&models.NotificationChannel{})

	if err := db.Exec(`CREATE TABLE IF NOT EXISTS tenant_integrations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		org_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		provider_type TEXT NOT NULL,
		provider TEXT NOT NULL,
		encrypted_config TEXT NOT NULL,
		is_default BOOLEAN DEFAULT FALSE,
		created_at DATETIME,
		updated_at DATETIME
	)`).Error; err != nil {
		t.Fatalf("create table: %v", err)
	}

	SetConfigDecryptor(func(stored string) (string, bool) {
		return strings.TrimPrefix(stored, "enc_"), true
	})
	t.Cleanup(func() { SetConfigDecryptor(nil) })

	integConfig := `{"botToken":"invalid-token-from-integration"}`
	if err := db.Exec(`INSERT INTO tenant_integrations (id, org_id, name, provider_type, provider, encrypted_config, is_default) VALUES (55, 1, 'Org1 Telegram Bot', 'communication', 'telegram', ?, TRUE)`, "enc_"+integConfig).Error; err != nil {
		t.Fatalf("insert integration: %v", err)
	}

	ch := NewTelegramChannel(db)

	// 1. Send with explicit integrationId
	err = ch.Send(context.Background(), plugin.NotificationRecipient{
		OrgID: "1",
		Config: map[string]any{
			"integrationId": 55,
			"chatId":        "12345",
		},
	}, plugin.NotificationPayload{Body: "hello"})
	if err == nil || !strings.Contains(err.Error(), "failed to initialize telegram notifier") {
		t.Errorf("expected initialization error using token from integration 55, got %v", err)
	}

	// 2. Send without integrationId (resolves default integration)
	err = ch.Send(context.Background(), plugin.NotificationRecipient{
		OrgID: "1",
		Config: map[string]any{
			"chatId": "12345",
		},
	}, plugin.NotificationPayload{Body: "hello"})
	if err == nil || !strings.Contains(err.Error(), "failed to initialize telegram notifier") {
		t.Errorf("expected initialization error using default integration, got %v", err)
	}

	// 3. Send from org 2 (no integrations) -> missing credentials
	err = ch.Send(context.Background(), plugin.NotificationRecipient{
		OrgID: "2",
		Config: map[string]any{
			"chatId": "12345",
		},
	}, plugin.NotificationPayload{Body: "hello"})
	if err == nil || !strings.Contains(err.Error(), "missing telegram credentials") {
		t.Errorf("expected missing credentials error for org 2, got %v", err)
	}

	// 4. Send with _raw_config specifying integrationId
	err = ch.Send(context.Background(), plugin.NotificationRecipient{
		OrgID: "1",
		Config: map[string]any{
			"_raw_config": `{"chatId":"12345","integrationId":55}`,
		},
	}, plugin.NotificationPayload{Body: "hello"})
	if err == nil || !strings.Contains(err.Error(), "failed to initialize telegram notifier") {
		t.Errorf("expected initialization error using raw config with integration 55, got %v", err)
	}
}
