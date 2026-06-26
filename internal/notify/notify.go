// Package notify delivers best-effort notifications about led events.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Send dispatches a notification via the specified channel type.
// If the type is unknown, it returns an error.
func Send(ctx context.Context, typ, cfgJSON, text string) error {
	switch typ {
	case "telegram":
		return sendTelegram(ctx, cfgJSON, text)
	case "webhook":
		return sendWebhook(ctx, cfgJSON, text)
	default:
		return fmt.Errorf("unknown notification channel type: %s", typ)
	}
}

func sendTelegram(ctx context.Context, cfgJSON, text string) error {
	var cfg struct {
		BotToken string `json:"botToken"`
		ChatID   string `json:"chatId"`
	}
	if err := json.Unmarshal([]byte(cfgJSON), &cfg); err != nil {
		return err
	}
	if cfg.BotToken == "" || cfg.ChatID == "" {
		return fmt.Errorf("missing telegram credentials")
	}

	body, err := json.Marshal(map[string]any{
		"chat_id": cfg.ChatID,
		"text":    text,
	})
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", cfg.BotToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	hc := &http.Client{Timeout: 10 * time.Second}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram: sendMessage HTTP %d", resp.StatusCode)
	}
	return nil
}

func sendWebhook(ctx context.Context, cfgJSON, text string) error {
	var cfg struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal([]byte(cfgJSON), &cfg); err != nil {
		return err
	}
	if cfg.URL == "" {
		return fmt.Errorf("missing webhook url")
	}

	body, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	hc := &http.Client{Timeout: 10 * time.Second}
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
