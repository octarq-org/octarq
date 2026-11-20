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

type TelegramChannel struct {
	db    *gorm.DB
	host  plugin.Host
	vault plugin.VaultService
}

var _ plugin.NotificationChannel = (*TelegramChannel)(nil)

func (c *TelegramChannel) SetHost(h plugin.Host) {
	c.host = h
}

func (c *TelegramChannel) SetVaultService(vs plugin.VaultService) {
	c.vault = vs
}

func NewTelegramChannel(db ...*gorm.DB) *TelegramChannel {
	var gdb *gorm.DB
	if len(db) > 0 {
		gdb = db[0]
	}
	return &TelegramChannel{db: gdb}
}

func (c *TelegramChannel) Name() string {
	return "telegram"
}

func (c *TelegramChannel) DisplayName() string {
	return "Telegram"
}

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

	if reqIntegID > 0 {
		botToken = ""
	}
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

	if (botToken == "" || chatIDStr == "") && reqIntegID == 0 && c.db != nil && targetOrg != "" {
		if orgNum, err := strconv.ParseUint(targetOrg, 10, 64); err == nil && orgNum > 0 {
			var ch models.NotificationChannel
			if err := c.db.WithContext(ctx).Where("owner_id = ? AND type = ? AND enabled = ?", orgNum, "telegram", true).First(&ch).Error; err == nil && (ch.Config != "" || ch.VaultItemID > 0) {
				cfgMap, decErr := ResolveChannelConfig(ctx, c.vault, ch)
				if decErr != nil {
					return fmt.Errorf("telegram: resolve channel config: %w", decErr)
				}
				if botToken == "" {
					if v, ok := cfgMap["botToken"].(string); ok {
						botToken = strings.TrimSpace(v)
					}
				}
				if chatIDStr == "" && cfgMap["chatId"] != nil {
					switch val := cfgMap["chatId"].(type) {
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
