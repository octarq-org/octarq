package app

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/octarq-org/octarq/server/internal/api"
	"github.com/octarq-org/octarq/server/internal/cleanup"
	"github.com/octarq-org/octarq/server/internal/cron"
	"github.com/octarq-org/octarq/server/internal/endpoint"
	"github.com/octarq-org/octarq/server/internal/monitor"
	"github.com/octarq-org/octarq/server/internal/server"
	"github.com/octarq-org/octarq/server/plugin"
)

func mountEndpointHTTP(apiHandler *api.Handler, endpointEngine *endpoint.Engine) {
	if err := endpointEngine.MountHTTP(apiHandler.Huma(), endpoint.HTTPOptions{
		RequireAuth: func(ctx context.Context) (uint, error) {
			orgID := plugin.OrgIDFromContext(ctx)
			if orgID == 0 {
				return 0, huma.Error401Unauthorized("Authentication required. Sign in to continue.")
			}
			return orgID, nil
		},
		RequireRole: func(ctx context.Context, roles []string) error {
			if len(roles) == 0 {
				return nil
			}
			r, err := http.NewRequestWithContext(ctx, http.MethodGet, "/", nil)
			if err != nil {
				return huma.Error403Forbidden("Insufficient permissions. Contact a workspace admin for access.")
			}
			for _, role := range roles {
				if apiHandler.RequireRole(r, role) {
					return nil
				}
			}
			return huma.Error403Forbidden("Insufficient permissions. Contact a workspace admin for access.")
		},
	}); err != nil {
		slog.Warn("app: failed to mount endpoint HTTP routes", "error", err)
	}
}

type serverRunParams struct {
	mux             http.Handler
	rootHandler     http.Handler
	webFS           fs.FS
	staticMounts    []server.StaticMount
	apiHandler      *api.Handler
	services        *plugin.Registry
	cronEngine      *cron.Engine
	healthCollector *monitor.Collector
}

func (a *App) serve(ctx context.Context, p serverRunParams) error {
	srv, err := server.New(a.cfg, a.gdb, api.CSRFGuard(a.cfg.SecretKey, p.mux), p.rootHandler, p.webFS, p.staticMounts, server.RuntimeSettings{
		MetricsToken: p.apiHandler.MetricsToken,
		RateLimits:   p.apiHandler.RateLimits,
		PublicGET:    api.PublicGETMatcher(p.apiHandler.Huma()),
		CORSOrigins:  p.apiHandler.CORSOrigins,
	})
	if err != nil {
		return err
	}

	httpSrv := &http.Server{
		Addr:              a.cfg.Listen,
		Handler:           srv,
		ReadHeaderTimeout: 10 * time.Second,
	}

	var cleanups []func(context.Context, int)
	for _, pl := range a.plugins {
		if fn, ok := plugin.LookupServiceAs[plugin.CleanupFunc](p.services.Lookup, plugin.CleanupServiceName(pl.Name())); ok {
			cleanups = append(cleanups, fn)
		}
	}
	go cleanup.Start(ctx, p.apiHandler.DataRetentionDays, cleanups...)
	go cleanup.StartSessionCleanup(ctx, a.gdb, p.apiHandler.DataRetentionDays)
	p.healthCollector.Start(ctx)
	defer p.healthCollector.Stop()

	go func() {
		slog.Info("octarq listening", "addr", a.cfg.Listen, "db", a.cfg.DBDriver)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("listen failed", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	select {
	case <-stop:
	case <-ctx.Done():
	}

	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	slog.Info("shutting down")
	if err := p.cronEngine.Stop(shutCtx); err != nil {
		slog.Warn("cron shutdown failed", "err", err)
	}
	if a.telemetry != nil {
		if err := a.telemetry.Shutdown(shutCtx); err != nil {
			slog.Warn("app: telemetry shutdown failed", "error", err)
		}
	}
	return httpSrv.Shutdown(shutCtx)
}
