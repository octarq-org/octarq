package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/octarq-org/octarq/server/internal/api"
	"github.com/octarq-org/octarq/server/internal/cleanup"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
)

// Setup sets up built-in background services, including cron jobs.
func (a *App) Setup(pctx *plugin.Context, apiHandler *api.Handler) {
	a.setupBuiltinCron(pctx, apiHandler)
}

func (a *App) setupBuiltinCron(pctx *plugin.Context, apiHandler *api.Handler) {
	if pctx == nil || pctx.RegisterCron == nil {
		return
	}
	if err := pctx.RegisterCron("session_cleanup", "0 * * * *", func(ctx context.Context) error {
		now := time.Now()
		if err := a.gdb.WithContext(ctx).Where("expires_at < ?", now).Delete(&models.Session{}).Error; err != nil {
			return err
		}
		if err := a.gdb.WithContext(ctx).Where("expires_at IS NOT NULL AND expires_at < ?", now).Delete(&models.Token{}).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		slog.Warn("app: failed to register session_cleanup cron", "error", err)
	}

	if err := pctx.RegisterCron("log_rotation", "0 0 * * *", func(ctx context.Context) error {
		if apiHandler == nil {
			return nil
		}
		days := apiHandler.DataRetentionDays()
		if days <= 0 {
			return nil
		}
		_, err := cleanup.PruneAuditLogs(a.gdb, days)
		return err
	}); err != nil {
		slog.Warn("app: failed to register log_rotation cron", "error", err)
	}
}
