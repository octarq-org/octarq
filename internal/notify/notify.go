// Package notify delivers best-effort notifications about led events.
//
// Today it ships a Telegram notifier driven by the Bot API. Notifications are
// always advisory: callers fire them asynchronously and ignore errors so a
// failing notifier never blocks or fails the originating request.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/jungley/led/config"
)

// Notifier sends a short text message somewhere out-of-band.
type Notifier interface {
	Notify(ctx context.Context, text string) error
}

const telegramAPIBase = "https://api.telegram.org"

// Telegram posts messages to a chat via the Bot API sendMessage method.
type Telegram struct {
	token  string
	chatID string
	base   string // overridable for tests
	hc     *http.Client
}

// NewTelegram builds a Telegram notifier from config, or returns nil if either
// the bot token or chat id is unconfigured. A nil *Telegram is a safe no-op.
func NewTelegram(cfg *config.Config) *Telegram {
	if cfg.TelegramBotToken == "" || cfg.TelegramChatID == "" {
		return nil
	}
	return &Telegram{
		token:  cfg.TelegramBotToken,
		chatID: cfg.TelegramChatID,
		base:   telegramAPIBase,
		hc:     &http.Client{Timeout: 10 * time.Second},
	}
}

// Notify sends text to the configured chat. A nil notifier is a no-op.
func (t *Telegram) Notify(ctx context.Context, text string) error {
	if t == nil {
		return nil
	}
	body, err := json.Marshal(map[string]any{
		"chat_id": t.chatID,
		"text":    text,
	})
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/bot%s/sendMessage", t.base, t.token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram: sendMessage HTTP %d", resp.StatusCode)
	}
	return nil
}
