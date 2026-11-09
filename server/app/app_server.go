package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/octarq-org/octarq/server/config"
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
	// stopPluginContext cancels the context every plugin Start goroutine was
	// given. serve calls it before Stopper, so no plugin is still producing
	// work while others drain.
	stopPluginContext context.CancelFunc
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

	mainLn, err := net.Listen("tcp", a.cfg.Listen)
	if err != nil {
		return fmt.Errorf("main listen: %w", err)
	}

	var (
		webhookSrv *http.Server
		webhookLn  net.Listener
	)
	if a.cfg.WebhookListen != "" {
		if err := validateWebhookListen(a.cfg.Listen, a.cfg.WebhookListen); err != nil {
			_ = mainLn.Close()
			return err
		}
		var err error
		webhookLn, err = net.Listen("tcp", a.cfg.WebhookListen)
		if err != nil {
			_ = mainLn.Close()
			return fmt.Errorf("webhook listen: %w", err)
		}
		webhookSrv = &http.Server{
			Addr:              a.cfg.WebhookListen,
			Handler:           newWebhookDispatcher(srv),
			ReadHeaderTimeout: 10 * time.Second,
		}
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
		if err := httpSrv.Serve(mainLn); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("listen failed", "err", err)
			os.Exit(1)
		}
	}()

	if webhookSrv != nil {
		go func() {
			slog.Info("webhook listener started", "addr", a.cfg.WebhookListen)
			if err := webhookSrv.Serve(webhookLn); err != nil && !errors.Is(err, http.ErrServerClosed) {
				slog.Error("webhook listen failed", "err", err)
				os.Exit(1)
			}
		}()
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)
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

	var (
		shutErr error
		mu      sync.Mutex
		wg      sync.WaitGroup
	)
	if webhookSrv != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := webhookSrv.Shutdown(shutCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
				slog.Warn("webhook shutdown failed", "err", err)
				mu.Lock()
				shutErr = errors.Join(shutErr, err)
				mu.Unlock()
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := httpSrv.Shutdown(shutCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			mu.Lock()
			shutErr = errors.Join(shutErr, err)
			mu.Unlock()
		}
	}()
	wg.Wait()
	// HTTP stops accepting (and finishes in-flight) requests before the
	// plugins that serve them are torn down, so Stop never runs under a live
	// request handler.
	if err := stopPlugins(shutCtx, p.stopPluginContext, a.plugins); err != nil {
		slog.Warn("plugin shutdown reported errors", "err", err)
		shutErr = errors.Join(shutErr, err)
	}
	return shutErr
}

// newWebhookDispatcher wraps next and permits only exact paths:
// POST /api/connectors/webhook and GET /api/health.
// All other paths return HTTP 404 Fail-Closed.
func newWebhookDispatcher(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if (r.Method == http.MethodPost && r.URL.Path == "/api/connectors/webhook") ||
			(r.Method == http.MethodGet && r.URL.Path == "/api/health") {
			next.ServeHTTP(w, r)
			return
		}
		http.NotFound(w, r)
	})
}

// validateWebhookListen asserts that webhookListen is valid and does not
// conflict with listen.
func validateWebhookListen(listen, webhookListen string) error {
	return config.ValidateWebhookListen(listen, webhookListen)
}
