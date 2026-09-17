package notification

import (
	"context"
	"encoding/json"
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
	db *gorm.DB
}

var _ plugin.NotificationChannel = (*TelegramChannel)(nil)

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

	if recipient.Config != nil {
		if raw, ok := recipient.Config["_raw_config"].(string); ok && strings.TrimSpace(raw) != "" {
			var m struct {
				BotToken string `json:"botToken"`
				ChatID   any    `json:"chatId"`
			}
			if err := json.Unmarshal([]byte(raw), &m); err != nil {
				return err
			}
			botToken = m.BotToken
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

	// Fallback to database lookup if org-scoped notification channel is configured
	if (botToken == "" || chatIDStr == "") && c.db != nil && targetOrg != "" {
		if orgNum, err := strconv.ParseUint(targetOrg, 10, 64); err == nil && orgNum > 0 {
			var ch models.NotificationChannel
			if err := c.db.WithContext(ctx).Where("owner_id = ? AND type = ? AND enabled = ?", orgNum, "telegram", true).First(&ch).Error; err == nil && ch.Config != "" {
				plain, decErr := ConfigPlaintext(ch.Config)
				if decErr == nil {
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
