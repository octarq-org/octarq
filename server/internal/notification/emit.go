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

// DefaultRouter returns the active process-wide notification router instance.
func DefaultRouter() *Router {
	defaultRouterMu.RLock()
	defer defaultRouterMu.RUnlock()
	return defaultRouter
}

// SetDefaultRouter replaces the process-wide notification router instance.
func SetDefaultRouter(r *Router) {
	defaultRouterMu.Lock()
	defer defaultRouterMu.Unlock()
	if r != nil {
		defaultRouter = r
	}
}

// Emit broadcasts or delivers a notification event using the process-wide DefaultRouter.
func Emit(ctx context.Context, payload plugin.NotificationPayload) error {
	return DefaultRouter().Emit(ctx, payload)
}

// EmitTo delivers a notification event directly to a recipient using the process-wide DefaultRouter.
func EmitTo(ctx context.Context, recipient plugin.NotificationRecipient, payload plugin.NotificationPayload) error {
	return DefaultRouter().EmitTo(ctx, recipient, payload)
}

// SendDirect delivers a notification directly via the process-wide DefaultRouter.
func SendDirect(ctx context.Context, typ, cfgJSON, text string) error {
	return DefaultRouter().SendDirect(ctx, typ, cfgJSON, text)
}

// SendDirectWithTenant delivers a notification directly via the process-wide DefaultRouter using an explicit tenant org ID.
func SendDirectWithTenant(ctx context.Context, orgID uint, typ, cfgJSON, text string) error {
	return DefaultRouter().SendDirectWithTenant(ctx, orgID, typ, cfgJSON, text)
}

// SendDirectTo delivers a notification directly to an explicit recipient using the process-wide DefaultRouter.
func SendDirectTo(ctx context.Context, typ string, recipient plugin.NotificationRecipient, payload plugin.NotificationPayload) error {
	return DefaultRouter().SendDirectTo(ctx, typ, recipient, payload)
}
