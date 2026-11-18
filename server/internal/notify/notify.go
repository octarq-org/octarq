package notify

import (
	"context"
	"log/slog"
	"strings"

	"github.com/octarq-org/octarq/server/internal/notification"
)

// Provider delivers a notification for one channel type. cfgJSON is the
// channel's JSON config; text is the message body.
type Provider = notification.LegacyProviderFunc

// Descriptor describes a notification channel type (built-in or plugin-contributed).
type Descriptor = notification.Descriptor

// Register adds (or replaces) a notification channel provider for typ.
func Register(typ string, p Provider) {
	if strings.TrimSpace(typ) == "" || p == nil {
		return
	}
	adapter := notification.NewLegacyNotifierAdapter(typ, "", p)
	if err := notification.DefaultRouter().RegisterChannel(adapter); err != nil {
		slog.Warn("notify: channel registration failed", "type", typ, "err", err)
	}
}

// RegisterWithDescriptor registers a provider along with its full metadata descriptor.
func RegisterWithDescriptor(desc Descriptor, p Provider) {
	if desc.Type == "" || p == nil {
		return
	}
	adapter := notification.NewLegacyNotifierAdapter(desc.Type, desc.Title, p)
	if err := notification.DefaultRouter().RegisterChannelWithDescriptor(adapter, desc); err != nil {
		slog.Warn("notify: channel registration failed", "type", desc.Type, "err", err)
	}
}

// Descriptors returns all registered notification channel type descriptors sorted by type.
func Descriptors() []Descriptor {
	return notification.DefaultRouter().Descriptors()
}

// Send dispatches a notification via the specified channel type.
func Send(ctx context.Context, typ, cfgJSON, text string) error {
	typ = strings.ToLower(strings.TrimSpace(typ))
	return notification.DefaultRouter().SendDirect(ctx, typ, cfgJSON, text)
}
