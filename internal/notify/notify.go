// Package notify delivers best-effort notifications about octarq events.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
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

// Descriptor describes a notification channel type (built-in or plugin-contributed).
type Descriptor struct {
	Type        string `json:"type"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	PluginName  string `json:"pluginName,omitempty"`
}

// providers holds channel types contributed at runtime (e.g. by Pro plugins
// via plugin.Context.RegisterNotifier). Registration happens at startup (Mount)
// and Send runs per-event, so the map is guarded for the concurrent read.
var (
	providersMu   sync.RWMutex
	providers     = map[string]Provider{}
	descriptorsMu sync.RWMutex
	descriptors   = map[string]Descriptor{
		"telegram": {
			Type:        "telegram",
			Title:       "Telegram",
			Description: "Deliver notifications via Telegram bot",
			Icon:        "send",
		},
		"webhook": {
			Type:        "webhook",
			Title:       "Webhook",
			Description: "Custom HTTP POST payload to any URL",
			Icon:        "webhook",
		},
	}
)

// Register adds (or replaces) a notification channel provider for typ. Plugins
// call this during Mount to add a new channel type — e.g. "slack", "sms" — that
// Send, core event dispatch, and the plugin Notify hook can then deliver to.
func Register(typ string, p Provider) {
	title := typ
	if len(typ) > 0 {
		title = strings.ToUpper(typ[:1]) + typ[1:]
	}
	RegisterWithDescriptor(Descriptor{
		Type:        typ,
		Title:       title,
		Description: fmt.Sprintf("Deliver notifications via %s", title),
		Icon:        "bell",
	}, p)
}

// RegisterWithDescriptor registers a provider along with its full metadata descriptor.
func RegisterWithDescriptor(desc Descriptor, p Provider) {
	if desc.Type == "" || p == nil {
		return
	}
	typ := strings.ToLower(strings.TrimSpace(desc.Type))
	desc.Type = typ
	providersMu.Lock()
	providers[typ] = p
	providersMu.Unlock()

	descriptorsMu.Lock()
	descriptors[typ] = desc
	descriptorsMu.Unlock()
}

// Descriptors returns all registered notification channel type descriptors sorted by type.
func Descriptors() []Descriptor {
	descriptorsMu.RLock()
	defer descriptorsMu.RUnlock()
	list := make([]Descriptor, 0, len(descriptors))
	for _, d := range descriptors {
		list = append(list, d)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Type < list[j].Type
	})
	return list
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
