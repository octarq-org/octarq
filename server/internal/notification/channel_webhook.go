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

// WebhookChannel delivers notifications via HTTP POST payloads.
type WebhookChannel struct {
	db      *gorm.DB
	timeout time.Duration
}

var _ plugin.NotificationChannel = (*WebhookChannel)(nil)

// NewWebhookChannel constructs a Webhook notification channel driver.
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

// Name returns the channel identifier "webhook".
func (c *WebhookChannel) Name() string {
	return "webhook"
}

// DisplayName returns the human-readable channel name.
func (c *WebhookChannel) DisplayName() string {
	return "Webhook"
}

// ConfigSchema returns the JSON Schema for user configuration.
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

// Send delivers a notification via HTTP POST to the configured webhook URL.
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

	// Fallback to database lookup if org-scoped notification channel is configured
	if webhookURL == "" && c.db != nil && recipient.OrgID != "" {
		if orgNum, err := strconv.ParseUint(recipient.OrgID, 10, 64); err == nil && orgNum > 0 {
			var ch models.NotificationChannel
			if err := c.db.WithContext(ctx).Where("owner_id = ? AND type = ? AND enabled = ?", orgNum, "webhook", true).First(&ch).Error; err == nil && ch.Config != "" {
				plain, decErr := ConfigPlaintext(ch.Config)
				if decErr == nil {
					var m struct {
						URL string `json:"url"`
					}
					if json.Unmarshal([]byte(plain), &m) == nil {
						webhookURL = strings.TrimSpace(m.URL)
					}
				}
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
