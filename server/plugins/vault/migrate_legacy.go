package vault

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

// MigrateLegacyCoreSecrets deterministically migrates pre-Vault encrypted Core
// secrets (llmgateway, webhooks, notification_channels, instance settings)
// into vault_items + vault_item_versions under per-item SubjectVault DEKs.
//
// It is idempotent and fail-closed: any decryption failure aborts migration and
// leaves legacy data intact.
func (p *Plugin) MigrateLegacyCoreSecrets(ctx context.Context) error {
	if p.db == nil {
		return nil
	}
	c, err := p.requireCrypto()
	if err != nil {
		return err
	}

	if err := p.migrateLegacyLLMProviders(ctx, c); err != nil {
		return err
	}
	if err := p.migrateLegacyWebhooks(ctx, c); err != nil {
		return err
	}
	if err := p.migrateLegacyNotifyChannels(ctx, c); err != nil {
		return err
	}
	if err := p.migrateLegacyInstanceSettings(ctx, c); err != nil {
		return err
	}
	return nil
}

func (p *Plugin) tableHasColumns(table string, cols ...string) bool {
	if p.db == nil || !p.db.Migrator().HasTable(table) {
		return false
	}
	colTypes, err := p.db.Migrator().ColumnTypes(table)
	if err != nil {
		return false
	}
	present := make(map[string]bool, len(colTypes))
	for _, ct := range colTypes {
		present[strings.ToLower(ct.Name())] = true
	}
	for _, c := range cols {
		if !present[strings.ToLower(c)] {
			return false
		}
	}
	return true
}

func (p *Plugin) migrateLegacyLLMProviders(ctx context.Context, c plugin.CryptoVault) error {
	if !p.tableHasColumns("llm_gateway_providers", "api_key_encr", "vault_item_id") {
		return nil
	}
	type legacyLLMRow struct {
		ID          uint   `gorm:"primaryKey"`
		OrgID       uint   `gorm:"column:org_id"`
		Name        string `gorm:"column:name"`
		Backend     string `gorm:"column:backend"`
		BaseURL     string `gorm:"column:base_url"`
		Model       string `gorm:"column:model"`
		CheapModel  string `gorm:"column:cheap_model"`
		IsDefault   bool   `gorm:"column:is_default"`
		APIKeyEncr  string `gorm:"column:api_key_encr"`
		VaultItemID uint   `gorm:"column:vault_item_id"`
	}
	var rows []legacyLLMRow
	if err := p.db.WithContext(ctx).Table("llm_gateway_providers").
		Where("vault_item_id = 0 AND api_key_encr <> ''").
		Find(&rows).Error; err != nil {
		return fmt.Errorf("vault migrate llmgateway: query: %w", err)
	}
	for _, r := range rows {
		plain, err := c.Decrypt(r.APIKeyEncr)
		if err != nil {
			return fmt.Errorf("vault migrate llmgateway %d: decrypt: %w", r.ID, err)
		}
		meta, err := p.PutSecret(ctx, plugin.PutSecretRequest{
			OrgID:          r.OrgID,
			Scope:          plugin.VaultScopeOrg,
			CollectionSlug: "integrations",
			ItemType:       "integration",
			ProviderType:   "llm",
			Provider:       r.Backend,
			Name:           r.Name,
			IsDefault:      r.IsDefault,
			Payload: map[string]any{
				"apiKey":     string(plain),
				"baseUrl":    r.BaseURL,
				"model":      r.Model,
				"cheapModel": r.CheapModel,
			},
			ChangeNote: "Migrated from legacy llmgateway provider",
		})
		if err != nil {
			return fmt.Errorf("vault migrate llmgateway %d: store: %w", r.ID, err)
		}
		if err := p.db.WithContext(ctx).Table("llm_gateway_providers").
			Where("id = ?", r.ID).
			Updates(map[string]any{"vault_item_id": meta.ID, "api_key_encr": ""}).Error; err != nil {
			return fmt.Errorf("vault migrate llmgateway %d: link: %w", r.ID, err)
		}
	}
	return nil
}

func (p *Plugin) migrateLegacyWebhooks(ctx context.Context, c plugin.CryptoVault) error {
	if !p.tableHasColumns("webhooks", "secret", "vault_item_id") {
		return nil
	}
	type legacyWebhookRow struct {
		ID          uint   `gorm:"primaryKey"`
		OrgID       uint   `gorm:"column:owner_id"`
		Name        string `gorm:"column:name"`
		URL         string `gorm:"column:url"`
		Secret      string `gorm:"column:secret"`
		VaultItemID uint   `gorm:"column:vault_item_id"`
	}
	var rows []legacyWebhookRow
	if err := p.db.WithContext(ctx).Table("webhooks").
		Where("vault_item_id = 0 AND secret <> ''").
		Find(&rows).Error; err != nil {
		return fmt.Errorf("vault migrate webhooks: query: %w", err)
	}
	for _, r := range rows {
		plain, err := c.Decrypt(r.Secret)
		if err != nil {
			return fmt.Errorf("vault migrate webhook %d: decrypt: %w", r.ID, err)
		}
		meta, err := p.PutSecret(ctx, plugin.PutSecretRequest{
			OrgID:      r.OrgID,
			Scope:      plugin.VaultScopeOrg,
			ItemType:   "webhook",
			Name:       r.Name,
			Payload:    map[string]any{"signingSecret": string(plain), "targetUrl": r.URL},
			ChangeNote: "Migrated from legacy webhook",
		})
		if err != nil {
			return fmt.Errorf("vault migrate webhook %d: store: %w", r.ID, err)
		}
		if err := p.db.WithContext(ctx).Table("webhooks").
			Where("id = ?", r.ID).
			Updates(map[string]any{"vault_item_id": meta.ID, "secret": ""}).Error; err != nil {
			return fmt.Errorf("vault migrate webhook %d: link: %w", r.ID, err)
		}
	}
	return nil
}

func (p *Plugin) migrateLegacyNotifyChannels(ctx context.Context, c plugin.CryptoVault) error {
	if !p.tableHasColumns("notification_channels", "config", "vault_item_id") {
		return nil
	}
	type legacyChanRow struct {
		ID          uint   `gorm:"primaryKey"`
		OrgID       uint   `gorm:"column:owner_id"`
		Name        string `gorm:"column:name"`
		Type        string `gorm:"column:type"`
		Config      string `gorm:"column:config"`
		VaultItemID uint   `gorm:"column:vault_item_id"`
	}
	var rows []legacyChanRow
	if err := p.db.WithContext(ctx).Table("notification_channels").
		Where("vault_item_id = 0 AND config <> ''").
		Find(&rows).Error; err != nil {
		return fmt.Errorf("vault migrate notify channels: query: %w", err)
	}
	for _, r := range rows {
		trimmed := strings.TrimSpace(r.Config)
		// Already non-secret JSON metadata (no migration needed).
		if strings.HasPrefix(trimmed, "{") {
			continue
		}
		plain, err := c.Decrypt(trimmed)
		if err != nil {
			return fmt.Errorf("vault migrate notify channel %d: decrypt: %w", r.ID, err)
		}
		var raw map[string]any
		if err := json.Unmarshal(plain, &raw); err != nil {
			return fmt.Errorf("vault migrate notify channel %d: json: %w", r.ID, err)
		}
		metaMap := map[string]any{}
		secretMap := map[string]any{}
		for k, v := range raw {
			if looksSecretKey(k) || strings.Contains(strings.ToLower(k), "webhook") {
				if v != nil && strings.TrimSpace(fmt.Sprint(v)) != "" {
					secretMap[k] = v
				}
			} else {
				metaMap[k] = v
			}
		}
		var vaultItemID uint
		if len(secretMap) > 0 {
			meta, err := p.PutSecret(ctx, plugin.PutSecretRequest{
				OrgID:        r.OrgID,
				Scope:        plugin.VaultScopeOrg,
				ItemType:     "notification_channel",
				ProviderType: "notification",
				Provider:     r.Type,
				Name:         r.Name,
				Payload:      secretMap,
				ChangeNote:   "Migrated from legacy notification channel",
			})
			if err != nil {
				return fmt.Errorf("vault migrate notify channel %d: store: %w", r.ID, err)
			}
			vaultItemID = meta.ID
		}
		metaBytes, _ := json.Marshal(metaMap)
		if err := p.db.WithContext(ctx).Table("notification_channels").
			Where("id = ?", r.ID).
			Updates(map[string]any{"vault_item_id": vaultItemID, "config": string(metaBytes)}).Error; err != nil {
			return fmt.Errorf("vault migrate notify channel %d: link: %w", r.ID, err)
		}
	}
	return nil
}

func quoteCol(db *gorm.DB, name string) string {
	if db == nil || db.Dialector == nil {
		return `"` + name + `"`
	}
	var b strings.Builder
	db.QuoteTo(&b, name)
	return b.String()
}

func (p *Plugin) migrateLegacyInstanceSettings(ctx context.Context, c plugin.CryptoVault) error {
	if !p.db.Migrator().HasTable("settings") {
		return nil
	}
	type settingRow struct {
		Key   string `gorm:"primaryKey"`
		Value string
	}
	mappings := []struct {
		settingKey string
		vaultKey   string
		itemType   string
		name       string
		field      string
	}{
		{"google_client_secret", "instance.oauth.google", "instance_secret", "Google OAuth", "clientSecret"},
		{"github_client_secret", "instance.oauth.github", "instance_secret", "GitHub OAuth", "clientSecret"},
		{"metrics_token", "instance.metrics_token", "instance_secret", "Metrics Bearer Token", "token"},
	}
	keyCol := quoteCol(p.db, "key")
	for _, m := range mappings {
		var row settingRow
		if err := p.db.WithContext(ctx).Table("settings").Where(keyCol+" = ? AND value <> ''", m.settingKey).First(&row).Error; err != nil {
			continue
		}
		plain, err := c.Decrypt(row.Value)
		if err != nil {
			return fmt.Errorf("vault migrate setting %s: decrypt: %w", m.settingKey, err)
		}
		if _, err := p.PutInstanceSecret(ctx, plugin.PutSecretRequest{
			ItemType:   m.itemType,
			KeyName:    m.vaultKey,
			Name:       m.name,
			Payload:    map[string]any{m.field: string(plain)},
			ChangeNote: "Migrated from legacy settings",
		}); err != nil {
			return fmt.Errorf("vault migrate setting %s: store: %w", m.settingKey, err)
		}
		if err := p.db.WithContext(ctx).Table("settings").Where(keyCol+" = ?", m.settingKey).Update("value", "").Error; err != nil {
			return fmt.Errorf("vault migrate setting %s: clear: %w", m.settingKey, err)
		}
	}
	return nil
}
