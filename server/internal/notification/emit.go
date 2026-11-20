package notification

import (
	"context"
	"sync"

	"github.com/octarq-org/octarq/server/plugin"
)

var (
	defaultRouterMu sync.RWMutex
	defaultRouter   = NewRouter(nil)
)

func DefaultRouter() *Router {
	defaultRouterMu.RLock()
	defer defaultRouterMu.RUnlock()
	return defaultRouter
}

func SetDefaultRouter(r *Router) {
	defaultRouterMu.Lock()
	defer defaultRouterMu.Unlock()
	if r != nil {
		defaultRouter = r
	}
}

func Emit(ctx context.Context, payload plugin.NotificationPayload) error {
	return DefaultRouter().Emit(ctx, payload)
}

func EmitTo(ctx context.Context, recipient plugin.NotificationRecipient, payload plugin.NotificationPayload) error {
	return DefaultRouter().EmitTo(ctx, recipient, payload)
}

func SendDirect(ctx context.Context, typ, cfgJSON, text string) error {
	return DefaultRouter().SendDirect(ctx, typ, cfgJSON, text)
}

func SendDirectWithTenant(ctx context.Context, orgID uint, typ, cfgJSON, text string) error {
	return DefaultRouter().SendDirectWithTenant(ctx, orgID, typ, cfgJSON, text)
}

func SendDirectTo(ctx context.Context, typ string, recipient plugin.NotificationRecipient, payload plugin.NotificationPayload) error {
	return DefaultRouter().SendDirectTo(ctx, typ, recipient, payload)
}
