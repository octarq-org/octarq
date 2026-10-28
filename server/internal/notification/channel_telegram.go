package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/nikoksr/notify"
	"github.com/nikoksr/notify/service/telegram"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

// TelegramChannel delivers notifications via Telegram bot.
type TelegramChannel struct {
	db   *gorm.DB
	host plugin.Host
}

var _ plugin.NotificationChannel = (*TelegramChannel)(nil)

// SetHost injects the host runtime SPI for credential resolution.
func (c *TelegramChannel) SetHost(h plugin.Host) {
	c.host = h
}

// NewTelegramChannel constructs a Telegram notification channel driver.
func NewTelegramChannel(db ...*gorm.DB) *TelegramChannel {
	var gdb *gorm.DB
	if len(db) > 0 {
		gdb = db[0]
	}
	return &TelegramChannel{db: gdb}
}

// Name returns the channel identifier "telegram".
func (c *TelegramChannel) Name() string {
	return "telegram"
}

// DisplayName returns the human-readable channel name.
func (c *TelegramChannel) DisplayName() string {
	return "Telegram"
}

// ConfigSchema returns the JSON Schema for user configuration.
func (c *TelegramChannel) ConfigSchema() json.RawMessage {
	return json.RawMessage(`{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"type": "object",
		"title": "Telegram Notification Settings",
		"properties": {
			"botToken": {
				"type": "string",
				"description": "Telegram Bot Token from @BotFather"
			},
			"chatId": {
				"type": "string",
				"description": "Telegram Chat or Channel ID"
			}
		},
		"required": ["botToken", "chatId"]
	}`)
}

// Send delivers a notification via Telegram.
func (c *TelegramChannel) Send(ctx context.Context, recipient plugin.NotificationRecipient, payload plugin.NotificationPayload) error {
	var botToken, chatIDStr string
	var reqIntegID uint

	if recipient.Config != nil {
		if raw, ok := recipient.Config["_raw_config"].(string); ok && strings.TrimSpace(raw) != "" {
			var m struct {
				BotToken      string `json:"botToken"`
				ChatID        any    `json:"chatId"`
				IntegrationID uint   `json:"integrationId"`
			}
			if err := json.Unmarshal([]byte(raw), &m); err != nil {
				return err
			}
			botToken = m.BotToken
			reqIntegID = m.IntegrationID
			if m.ChatID != nil {
				switch val := m.ChatID.(type) {
				case string:
					chatIDStr = val
				case float64:
					chatIDStr = strconv.FormatInt(int64(val), 10)
				case int64:
					chatIDStr = strconv.FormatInt(val, 10)
				case int:
					chatIDStr = strconv.Itoa(val)
				}
			}
		}

		if botToken == "" {
			if v, ok := recipient.Config["botToken"].(string); ok {
				botToken = v
			}
		}
		if reqIntegID == 0 {
			switch v := recipient.Config["integrationId"].(type) {
			case float64:
				if v > 0 {
					reqIntegID = uint(v)
				}
			case int:
				if v > 0 {
					reqIntegID = uint(v)
				}
			case int64:
				if v > 0 {
					reqIntegID = uint(v)
				}
			case uint:
				reqIntegID = v
			}
		}
		if chatIDStr == "" {
			if v := recipient.Config["chatId"]; v != nil {
				switch val := v.(type) {
				case string:
					chatIDStr = val
				case float64:
					chatIDStr = strconv.FormatInt(int64(val), 10)
				case int64:
					chatIDStr = strconv.FormatInt(val, 10)
				case int:
					chatIDStr = strconv.Itoa(val)
				}
			}
		}
	}

	botToken = strings.TrimSpace(botToken)
	chatIDStr = strings.TrimSpace(chatIDStr)

	targetOrg := strings.TrimSpace(recipient.OrgID)
	if targetOrg == "" {
		targetOrg = strings.TrimSpace(payload.OrgID)
	}

	// 1. Resolve botToken via Host SPI (Centralized Integrations Hub). A config
	// that names an integration is answered by the Hub alone: falling back to
	// the local row would send through a credential the operator did not select.
	if botToken == "" && targetOrg != "" {
		if orgNum, err := strconv.ParseUint(targetOrg, 10, 64); err == nil && orgNum > 0 {
			host := c.host
			if host == nil {
				host = plugin.EnsureHost(nil)
			}
			cred, err := host.LookupCredential(ctx, uint(orgNum), plugin.CredentialQuery{
				ID:           reqIntegID,
				ProviderType: "communication",
				Provider:     "telegram",
			})
			switch {
			case err == nil:
				if cred == nil || cred.OrgID != uint(orgNum) {
					if reqIntegID > 0 {
						return fmt.Errorf("telegram: the selected integration does not exist")
					}
				} else if len(cred.ConfigJSON) > 0 {
					var m struct {
						BotToken string `json:"botToken"`
					}
					if json.Unmarshal(cred.ConfigJSON, &m) == nil && m.BotToken != "" {
						botToken = strings.TrimSpace(m.BotToken)
					}
				}
				if botToken == "" && reqIntegID > 0 {
					return fmt.Errorf("telegram: the selected integration has no bot token")
				}
			case errors.Is(err, plugin.ErrCredentialNotFound), errors.Is(err, plugin.ErrCredentialResolverUnavailable):
				if reqIntegID > 0 {
					return fmt.Errorf("telegram: the selected integration does not exist")
				}
			default:
				return fmt.Errorf("telegram: resolve integration credentials: %w", err)
			}
		}
	}

	// 2. Local channel config, only when no integration was named.
	if (botToken == "" || chatIDStr == "") && reqIntegID == 0 && c.db != nil && targetOrg != "" {
		if orgNum, err := strconv.ParseUint(targetOrg, 10, 64); err == nil && orgNum > 0 {
			var ch models.NotificationChannel
			if err := c.db.WithContext(ctx).Where("owner_id = ? AND type = ? AND enabled = ?", orgNum, "telegram", true).First(&ch).Error; err == nil && ch.Config != "" {
				plain, decErr := ConfigPlaintext(ch.Config)
				if decErr != nil {
					return fmt.Errorf("telegram: decrypt channel config: %w", decErr)
				}
				var m struct {
					BotToken string `json:"botToken"`
					ChatID   any    `json:"chatId"`
				}
				if json.Unmarshal([]byte(plain), &m) == nil {
					if botToken == "" {
						botToken = strings.TrimSpace(m.BotToken)
					}
					if chatIDStr == "" && m.ChatID != nil {
						switch val := m.ChatID.(type) {
						case string:
							chatIDStr = strings.TrimSpace(val)
						case float64:
							chatIDStr = strconv.FormatInt(int64(val), 10)
						case int64:
							chatIDStr = strconv.FormatInt(val, 10)
						case int:
							chatIDStr = strconv.Itoa(val)
						}
					}
				}
			}
		}
	}

	if botToken == "" || chatIDStr == "" {
		return fmt.Errorf("missing telegram credentials")
	}

	chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid telegram chatId: %w", err)
	}

	tg, err := telegram.New(botToken)
	if err != nil {
		return fmt.Errorf("failed to initialize telegram notifier: %w", err)
	}
	tg.AddReceivers(chatID)

	notifier := notify.New()
	notifier.UseServices(tg)

	text := payload.Body
	if text == "" && payload.Title != "" {
		text = payload.Title
	}

	return notifier.Send(ctx, "", text)
}
