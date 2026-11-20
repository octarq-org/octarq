package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
	"github.com/octarq-org/octarq/server/plugin/safehttp"
	"gorm.io/gorm"
)

type WebhookChannel struct {
	db      *gorm.DB
	timeout time.Duration
	vault   plugin.VaultService
}

var _ plugin.NotificationChannel = (*WebhookChannel)(nil)

func (c *WebhookChannel) SetVaultService(vs plugin.VaultService) {
	c.vault = vs
}

func NewWebhookChannel(db ...*gorm.DB) *WebhookChannel {
	var gdb *gorm.DB
	if len(db) > 0 {
		gdb = db[0]
	}
	return &WebhookChannel{
		db:      gdb,
		timeout: 10 * time.Second,
	}
}

func (c *WebhookChannel) Name() string {
	return "webhook"
}

func (c *WebhookChannel) DisplayName() string {
	return "Webhook"
}

func (c *WebhookChannel) ConfigSchema() json.RawMessage {
	return json.RawMessage(`{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"type": "object",
		"title": "Webhook Notification Settings",
		"properties": {
			"url": {
				"type": "string",
				"format": "uri",
				"description": "Destination Webhook HTTP/HTTPS URL"
			}
		},
		"required": ["url"]
	}`)
}

func (c *WebhookChannel) Send(ctx context.Context, recipient plugin.NotificationRecipient, payload plugin.NotificationPayload) error {
	var webhookURL string

	if recipient.Config != nil {
		if raw, ok := recipient.Config["_raw_config"].(string); ok && strings.TrimSpace(raw) != "" {
			var m struct {
				URL string `json:"url"`
			}
			if err := json.Unmarshal([]byte(raw), &m); err != nil {
				return err
			}
			webhookURL = m.URL
		}

		if webhookURL == "" {
			if v, ok := recipient.Config["url"].(string); ok {
				webhookURL = v
			}
		}
	}

	webhookURL = strings.TrimSpace(webhookURL)

	targetOrg := strings.TrimSpace(recipient.OrgID)
	if targetOrg == "" {
		targetOrg = strings.TrimSpace(payload.OrgID)
	}
	if targetOrg == "" {
		if oid := plugin.OrgIDFromContext(ctx); oid > 0 {
			targetOrg = strconv.FormatUint(uint64(oid), 10)
		}
	}

	if webhookURL == "" && c.db != nil && targetOrg != "" {
		if orgNum, err := strconv.ParseUint(targetOrg, 10, 64); err == nil && orgNum > 0 {
			var ch models.NotificationChannel
			if err := c.db.WithContext(ctx).Where("owner_id = ? AND type = ? AND enabled = ?", orgNum, "webhook", true).First(&ch).Error; err == nil && (ch.Config != "" || ch.VaultItemID > 0) {
				cfgMap, decErr := ResolveChannelConfig(ctx, c.vault, ch)
				if decErr != nil {
					return fmt.Errorf("webhook: resolve channel config: %w", decErr)
				}
				if u, ok := cfgMap["url"].(string); ok {
					webhookURL = strings.TrimSpace(u)
				}
			}
		}
	}

	if webhookURL == "" && c.db != nil && (targetOrg == "0" || payload.EventType == "system" || strings.HasPrefix(payload.EventType, "system.")) {
		var ch models.NotificationChannel
		if err := c.db.WithContext(ctx).Where("owner_id = ? AND type = ? AND enabled = ?", 1, "webhook", true).First(&ch).Error; err == nil && (ch.Config != "" || ch.VaultItemID > 0) {
			cfgMap, decErr := ResolveChannelConfig(ctx, c.vault, ch)
			if decErr != nil {
				return fmt.Errorf("webhook: resolve channel config: %w", decErr)
			}
			if u, ok := cfgMap["url"].(string); ok {
				webhookURL = strings.TrimSpace(u)
			}
		}
	}

	if webhookURL == "" {
		return fmt.Errorf("missing webhook url")
	}

	text := payload.Body
	if text == "" && payload.Title != "" {
		text = payload.Title
	}

	body, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	if err := safehttp.ValidateScheme(req.URL.Scheme); err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	timeout := c.timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	hc := safehttp.NewWebhookClient(timeout)
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook: HTTP %d", resp.StatusCode)
	}
	return nil
}
