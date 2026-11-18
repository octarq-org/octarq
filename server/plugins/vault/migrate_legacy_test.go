package vault

import (
	"context"
	"testing"

	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
)

func TestMigrateLegacyCoreSecrets(t *testing.T) {
	p, db, _ := setupVaultTest(t)
	ctx := context.Background()

	// Create legacy schemas/columns as they existed before unified vault cutover.
	if err := db.Exec(`CREATE TABLE llm_gateway_providers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		org_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		backend TEXT NOT NULL,
		api_key_encr TEXT DEFAULT '',
		vault_item_id INTEGER DEFAULT 0,
		base_url TEXT DEFAULT '',
		model TEXT DEFAULT '',
		cheap_model TEXT DEFAULT '',
		is_default BOOLEAN DEFAULT FALSE
	)`).Error; err != nil {
		t.Fatalf("create legacy llm table: %v", err)
	}
	if err := db.Exec(`CREATE TABLE webhooks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		owner_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		url TEXT NOT NULL,
		secret TEXT DEFAULT '',
		vault_item_id INTEGER DEFAULT 0
	)`).Error; err != nil {
		t.Fatalf("create legacy webhooks table: %v", err)
	}
	if err := db.Exec(`CREATE TABLE notification_channels (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		owner_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		type TEXT NOT NULL,
		config TEXT DEFAULT '',
		vault_item_id INTEGER DEFAULT 0
	)`).Error; err != nil {
		t.Fatalf("create legacy notify table: %v", err)
	}

	encLLM, _ := p.host.Crypto().Encrypt([]byte("sk-legacy-llm"))
	encHook, _ := p.host.Crypto().Encrypt([]byte("whsec-legacy"))
	encChan, _ := p.host.Crypto().Encrypt([]byte(`{"botToken":"123:legacy","chatId":"999"}`))
	encGoogle, _ := p.host.Crypto().Encrypt([]byte("google-legacy-sec"))

	db.Exec(`INSERT INTO llm_gateway_providers (org_id, name, backend, api_key_encr, base_url, model, is_default) VALUES (1, 'Legacy OpenAI', 'openai', ?, 'https://api.openai.com/v1', 'gpt-4o', 1)`, encLLM)
	db.Exec(`INSERT INTO webhooks (owner_id, name, url, secret) VALUES (1, 'Legacy Hook', 'https://example.com/hook', ?)`, encHook)
	db.Exec(`INSERT INTO notification_channels (owner_id, name, type, config) VALUES (1, 'Legacy TG', 'telegram', ?)`, encChan)
	db.Create(&models.Setting{Key: "google_client_secret", Value: encGoogle})

	if err := p.MigrateLegacyCoreSecrets(ctx); err != nil {
		t.Fatalf("MigrateLegacyCoreSecrets: %v", err)
	}
	// Run a second time to assert idempotency.
	if err := p.MigrateLegacyCoreSecrets(ctx); err != nil {
		t.Fatalf("MigrateLegacyCoreSecrets second run: %v", err)
	}

	var llmVaultID uint
	db.Raw(`SELECT vault_item_id FROM llm_gateway_providers WHERE id = 1`).Scan(&llmVaultID)
	secLLM, err := p.GetSecret(ctx, 1, llmVaultID)
	if err != nil || secLLM.Payload["apiKey"] != "sk-legacy-llm" {
		t.Fatalf("migrated llm secret mismatch: %+v, err=%v", secLLM, err)
	}

	var whVaultID uint
	db.Raw(`SELECT vault_item_id FROM webhooks WHERE id = 1`).Scan(&whVaultID)
	secWH, err := p.GetSecret(ctx, 1, whVaultID)
	if err != nil || secWH.Payload["signingSecret"] != "whsec-legacy" {
		t.Fatalf("migrated webhook secret mismatch: %+v, err=%v", secWH, err)
	}

	var chVaultID uint
	var chConfig string
	db.Raw(`SELECT vault_item_id, config FROM notification_channels WHERE id = 1`).Row().Scan(&chVaultID, &chConfig)
	secCH, err := p.GetSecret(ctx, 1, chVaultID)
	if err != nil || secCH.Payload["botToken"] != "123:legacy" || chConfig != `{"chatId":"999"}` {
		t.Fatalf("migrated channel mismatch: sec=%+v cfg=%q err=%v", secCH, chConfig, err)
	}

	instSec, err := p.ResolveInstanceSecret(ctx, "instance.oauth.google")
	if err != nil || instSec.Payload["clientSecret"] != "google-legacy-sec" {
		t.Fatalf("migrated instance oauth mismatch: %+v, err=%v", instSec, err)
	}
	if !p.host.Crypto().HasSubjectKey(1, itemSubject(secLLM.Meta.SecretID)) {
		t.Fatal("expected per-item SubjectVault key for migrated item")
	}
	_ = plugin.VaultScopeOrg
}
