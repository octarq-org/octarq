package notify

import (
	"context"
	"log/slog"
	"strings"

	"github.com/octarq-org/octarq/server/internal/notification"
)

type Provider = notification.LegacyProviderFunc

type Descriptor = notification.Descriptor

func Register(typ string, p Provider) {
	if strings.TrimSpace(typ) == "" || p == nil {
		return
	}
	adapter := notification.NewLegacyNotifierAdapter(typ, "", p)
	if err := notification.DefaultRouter().RegisterChannel(adapter); err != nil {
		slog.Warn("notify: channel registration failed", "type", typ, "err", err)
	}
}

func RegisterWithDescriptor(desc Descriptor, p Provider) {
	if desc.Type == "" || p == nil {
		return
	}
	adapter := notification.NewLegacyNotifierAdapter(desc.Type, desc.Title, p)
	if err := notification.DefaultRouter().RegisterChannelWithDescriptor(adapter, desc); err != nil {
		slog.Warn("notify: channel registration failed", "type", desc.Type, "err", err)
	}
}

func Descriptors() []Descriptor {
	return notification.DefaultRouter().Descriptors()
}

func Send(ctx context.Context, typ, cfgJSON, text string) error {
	typ = strings.ToLower(strings.TrimSpace(typ))
	return notification.DefaultRouter().SendDirect(ctx, typ, cfgJSON, text)
}
