package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/octarq-org/octarq/server/plugin"
)

// LegacyProviderFunc matches the signature of the legacy notify.Provider callback.
type LegacyProviderFunc func(ctx context.Context, cfgJSON, text string) error

// LegacyNotifierAdapter adapts a legacy notify.Provider or ctx.RegisterNotifier callback
// into a plugin.NotificationChannel SPI implementation.
type LegacyNotifierAdapter struct {
	ChannelName  string
	ChannelTitle string
	SendFunc     LegacyProviderFunc
}

var _ plugin.NotificationChannel = (*LegacyNotifierAdapter)(nil)

// NewLegacyNotifierAdapter wraps a legacy provider function into a plugin.NotificationChannel.
func NewLegacyNotifierAdapter(name, title string, fn LegacyProviderFunc) *LegacyNotifierAdapter {
	name = strings.ToLower(strings.TrimSpace(name))
	if title == "" {
		title = name
		if len(title) > 0 {
			title = strings.ToUpper(title[:1]) + title[1:]
		}
	}
	return &LegacyNotifierAdapter{
		ChannelName:  name,
		ChannelTitle: title,
		SendFunc:     fn,
	}
}

func (a *LegacyNotifierAdapter) Name() string {
	return a.ChannelName
}

func (a *LegacyNotifierAdapter) DisplayName() string {
	if a.ChannelTitle != "" {
		return a.ChannelTitle
	}
	return a.ChannelName
}

func (a *LegacyNotifierAdapter) ConfigSchema() json.RawMessage {
	return nil
}

func (a *LegacyNotifierAdapter) Send(ctx context.Context, recipient plugin.NotificationRecipient, payload plugin.NotificationPayload) error {
	if a.SendFunc == nil {
		return fmt.Errorf("notification: legacy provider function is nil for channel %q", a.ChannelName)
	}

	cfgJSON := "{}"
	if recipient.Config != nil {
		if raw, ok := recipient.Config["_raw_config"].(string); ok && strings.TrimSpace(raw) != "" {
			cfgJSON = raw
		} else {
			b, err := json.Marshal(recipient.Config)
			if err == nil {
				cfgJSON = string(b)
			}
		}
	}

	text := payload.Body
	if text == "" && payload.Title != "" {
		text = payload.Title
	}

	return a.SendFunc(ctx, cfgJSON, text)
}
