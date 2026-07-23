// Package notify delivers best-effort notifications about octarq events.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nikoksr/notify"
	"github.com/nikoksr/notify/service/telegram"
	"github.com/octarq-org/octarq/internal/safehttp"
)

// webhookClient is SSRF-hardened: notification webhook URLs are user-supplied,
// so a channel pointed at an internal/metadata address must be refused.
var webhookClient = safehttp.NewWebhookClient(10 * time.Second)

// Provider delivers a notification for one channel type. cfgJSON is the
// channel's stored JSON config; text is the message body.
type Provider func(ctx context.Context, cfgJSON, text string) error

// providers holds channel types contributed at runtime (e.g. by Pro plugins
// via plugin.Context.RegisterNotifier). Registration happens at startup (Mount)
// and Send runs per-event, so the map is guarded for the concurrent read.
var (
	providersMu sync.RWMutex
	providers   = map[string]Provider{}
)

// Register adds (or replaces) a notification channel provider for typ. Plugins
// call this during Mount to add a new channel type — e.g. "slack", "sms" — that
// Send, core event dispatch, and the plugin Notify hook can then deliver to.
func Register(typ string, p Provider) {
	if typ == "" || p == nil {
		return
	}
	providersMu.Lock()
	providers[strings.ToLower(typ)] = p
	providersMu.Unlock()
}

func lookup(typ string) Provider {
	providersMu.RLock()
	defer providersMu.RUnlock()
	return providers[typ]
}

// Send dispatches a notification via the specified channel type. Built-in types
// (telegram, webhook) are handled directly; any other type is resolved from the
// plugin-contributed provider registry. An unregistered type is an error.
func Send(ctx context.Context, typ, cfgJSON, text string) error {
	typ = strings.ToLower(strings.TrimSpace(typ))
	switch typ {
	case "telegram":
		return sendTelegram(ctx, cfgJSON, text)
	case "webhook":
		return sendWebhook(ctx, cfgJSON, text)
	}
	if p := lookup(typ); p != nil {
		return p(ctx, cfgJSON, text)
	}
	return fmt.Errorf("unknown notification channel type: %s", typ)
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

	chatID, err := strconv.ParseInt(cfg.ChatID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid telegram chatId: %w", err)
	}

	tg, err := telegram.New(cfg.BotToken)
	if err != nil {
		return fmt.Errorf("failed to initialize telegram notifier: %w", err)
	}
	tg.AddReceivers(chatID)

	notifier := notify.New()
	notifier.UseServices(tg)

	return notifier.Send(ctx, "", text)
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
	if err := safehttp.ValidateScheme(req.URL.Scheme); err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	// SSRF-hardened: a notification channel's webhook URL is user-supplied.
	hc := safehttp.NewWebhookClient(10 * time.Second)
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
