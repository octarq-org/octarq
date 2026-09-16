// Package notify delivers best-effort notifications about octarq events.
// It serves as a backward-compatible adapter delegating to internal/notification.Router.
package notify

import (
	"context"
	"strings"

	"github.com/octarq-org/octarq/server/internal/notification"
)

// Provider delivers a notification for one channel type. cfgJSON is the
// channel's stored JSON config; text is the message body.
type Provider = notification.LegacyProviderFunc

// Descriptor describes a notification channel type (built-in or plugin-contributed).
type Descriptor = notification.Descriptor

// Register adds (or replaces) a notification channel provider for typ.
func Register(typ string, p Provider) {
	if strings.TrimSpace(typ) == "" || p == nil {
		return
	}
	adapter := notification.NewLegacyNotifierAdapter(typ, "", p)
	_ = notification.DefaultRouter().RegisterChannel(adapter)
}

// RegisterWithDescriptor registers a provider along with its full metadata descriptor.
func RegisterWithDescriptor(desc Descriptor, p Provider) {
	if desc.Type == "" || p == nil {
		return
	}
	adapter := notification.NewLegacyNotifierAdapter(desc.Type, desc.Title, p)
	_ = notification.DefaultRouter().RegisterChannelWithDescriptor(adapter, desc)
}

// Descriptors returns all registered notification channel type descriptors sorted by type.
func Descriptors() []Descriptor {
	return notification.DefaultRouter().Descriptors()
}

// SetConfigDecryptor registers how a stored (encrypted) notification channel config
// is unwrapped before it is used for notification delivery.
func SetConfigDecryptor(fn func(string) (string, bool)) {
	notification.SetConfigDecryptor(fn)
}

// configPlaintext resolves the plaintext config for a stored value.
func configPlaintext(stored string) (string, error) {
	return notification.ConfigPlaintext(stored)
}

// Send dispatches a notification via the specified channel type.
func Send(ctx context.Context, typ, cfgJSON, text string) error {
	typ = strings.ToLower(strings.TrimSpace(typ))
	var err error
	cfgJSON, err = configPlaintext(cfgJSON)
	if err != nil {
		return err
	}
	return notification.DefaultRouter().SendDirect(ctx, typ, cfgJSON, text)
}
