package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
	"github.com/octarq-org/octarq/server/plugin/safehttp"
	"gorm.io/gorm"
)

// SlackChannel delivers notifications to Slack via incoming webhooks or tenant integrations.
type SlackChannel struct {
	db      *gorm.DB
	client  *http.Client
	timeout time.Duration
}

var _ plugin.NotificationChannel = (*SlackChannel)(nil)

// NewSlackChannel constructs a Slack notification channel driver.
func NewSlackChannel(db ...*gorm.DB) *SlackChannel {
	var gdb *gorm.DB
	if len(db) > 0 {
		gdb = db[0]
	}
	return &SlackChannel{
		db:      gdb,
		timeout: 10 * time.Second,
	}
}

// Name returns the channel identifier "slack".
func (c *SlackChannel) Name() string {
	return "slack"
}

// DisplayName returns the human-readable channel name.
func (c *SlackChannel) DisplayName() string {
	return "Slack"
}

// ConfigSchema returns the JSON Schema for user configuration.
func (c *SlackChannel) ConfigSchema() json.RawMessage {
	return json.RawMessage(`{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"type": "object",
		"title": "Slack Notification Settings",
		"properties": {
			"webhook_url": {
				"type": "string",
				"description": "Slack incoming webhook URL (https://hooks.slack.com/...)"
			},
			"integration_id": {
				"type": "integer",
				"description": "ID of centralized communication integration"
			}
		}
	}`)
}

// Send resolves the webhook URL and dispatches the notification payload to Slack.
func (c *SlackChannel) Send(ctx context.Context, recipient plugin.NotificationRecipient, payload plugin.NotificationPayload) error {
	webhookURL := ""

	// 1. Resolve from recipient.Config ("webhook_url", "webhookUrl", or "integration_id")
	if recipient.Config != nil {
		if v, ok := recipient.Config["webhook_url"].(string); ok && strings.TrimSpace(v) != "" {
			webhookURL = strings.TrimSpace(v)
		} else if v, ok := recipient.Config["webhookUrl"].(string); ok && strings.TrimSpace(v) != "" {
			webhookURL = strings.TrimSpace(v)
		} else if raw, ok := recipient.Config["_raw_config"].(string); ok && strings.TrimSpace(raw) != "" {
			var m struct {
				WebhookURL    string `json:"webhookUrl"`
				WebhookURLAlt string `json:"webhook_url"`
				IntegrationID uint   `json:"integrationId"`
			}
			if err := json.Unmarshal([]byte(raw), &m); err == nil {
				if m.WebhookURL != "" {
					webhookURL = strings.TrimSpace(m.WebhookURL)
				} else if m.WebhookURLAlt != "" {
					webhookURL = strings.TrimSpace(m.WebhookURLAlt)
				}
			}
		}
	}

	targetOrg := strings.TrimSpace(recipient.OrgID)
	if targetOrg == "" {
		targetOrg = strings.TrimSpace(payload.OrgID)
	}
	if targetOrg == "" {
		if oid := plugin.OrgIDFromContext(ctx); oid > 0 {
			targetOrg = strconv.FormatUint(uint64(oid), 10)
		}
	}

	// 2. Resolve from centralized tenant_integrations (provider_type = 'communication', provider = 'slack')
	if webhookURL == "" && c.db != nil && targetOrg != "" {
		if orgNum, err := strconv.ParseUint(targetOrg, 10, 64); err == nil && orgNum > 0 {
			type tenantInteg struct {
				EncryptedConfig string `gorm:"column:encrypted_config"`
			}
			var row tenantInteg
			err := c.db.WithContext(ctx).Table("tenant_integrations").
				Where("org_id = ? AND provider_type = ? AND provider = ?", orgNum, "communication", "slack").
				Order("is_default DESC, id DESC").Limit(1).Take(&row).Error
			if err == nil && row.EncryptedConfig != "" {
				plain, decErr := ConfigPlaintext(row.EncryptedConfig)
				if decErr == nil {
					var m struct {
						WebhookURL    string `json:"webhookUrl"`
						WebhookURLAlt string `json:"webhook_url"`
					}
					if json.Unmarshal([]byte(plain), &m) == nil {
						if m.WebhookURL != "" {
							webhookURL = strings.TrimSpace(m.WebhookURL)
						} else if m.WebhookURLAlt != "" {
							webhookURL = strings.TrimSpace(m.WebhookURLAlt)
						}
					}
				}
			}
		}
	}

	// 3. Fallback to notification_channels table
	if webhookURL == "" && c.db != nil && targetOrg != "" {
		if orgNum, err := strconv.ParseUint(targetOrg, 10, 64); err == nil && orgNum > 0 {
			var ch models.NotificationChannel
			if err := c.db.WithContext(ctx).Where("owner_id = ? AND type = ? AND enabled = ?", orgNum, "slack", true).First(&ch).Error; err == nil && ch.Config != "" {
				plain, decErr := ConfigPlaintext(ch.Config)
				if decErr == nil {
					var m struct {
						WebhookURL    string `json:"webhookUrl"`
						WebhookURLAlt string `json:"webhook_url"`
					}
					if json.Unmarshal([]byte(plain), &m) == nil {
						if m.WebhookURL != "" {
							webhookURL = strings.TrimSpace(m.WebhookURL)
						} else if m.WebhookURLAlt != "" {
							webhookURL = strings.TrimSpace(m.WebhookURLAlt)
						}
					}
				}
			}
		}
	}

	if webhookURL == "" {
		return errors.New("slack: missing webhook url")
	}

	parsedURL, err := url.Parse(webhookURL)
	if err != nil || parsedURL.Host == "" {
		return errors.New("slack: invalid webhook url")
	}
	if err := safehttp.ValidateScheme(parsedURL.Scheme); err != nil {
		return err
	}

	text := formatSlackMessage(payload)
	msgBody, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return fmt.Errorf("slack: marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(msgBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	hc := c.getClient()
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if _, err := io.Copy(io.Discard, io.LimitReader(resp.Body, 4096)); err != nil {
		slog.WarnContext(ctx, "slack: drain response body failed", "error", err)
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("slack: HTTP %d", resp.StatusCode)
	}
	return nil
}

func (c *SlackChannel) getClient() *http.Client {
	if c.client != nil {
		return c.client
	}
	timeout := c.timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return safehttp.NewWebhookClient(timeout)
}

// formatSlackMessage formats NotificationPayload into a Slack mrkdwn message text.
func formatSlackMessage(p plugin.NotificationPayload) string {
	var lines []string

	var headerParts []string
	if p.Priority != "" {
		switch strings.ToLower(p.Priority) {
		case "critical":
			headerParts = append(headerParts, "🚨 *[CRITICAL]*")
		case "high":
			headerParts = append(headerParts, "⚠️ *[HIGH]*")
		case "low":
			headerParts = append(headerParts, "ℹ️ *[LOW]*")
		default:
			headerParts = append(headerParts, fmt.Sprintf("*[%s]*", strings.ToUpper(p.Priority)))
		}
	}

	title := strings.TrimSpace(p.Title)
	eventType := strings.TrimSpace(p.EventType)

	if title != "" {
		headerParts = append(headerParts, fmt.Sprintf("*%s*", title))
	} else if eventType != "" {
		headerParts = append(headerParts, fmt.Sprintf("*%s*", eventType))
	}

	if len(headerParts) > 0 {
		lines = append(lines, strings.Join(headerParts, " "))
	}

	if eventType != "" && title != "" {
		lines = append(lines, fmt.Sprintf("• *Event:* `%s`", eventType))
	}

	if body := strings.TrimSpace(p.Body); body != "" {
		lines = append(lines, body)
	}

	if len(lines) == 0 {
		return "Notification received"
	}
	return strings.Join(lines, "\n")
}
