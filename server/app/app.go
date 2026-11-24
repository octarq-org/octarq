package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/octarq-org/octarq/server/config"
	"github.com/octarq-org/octarq/server/idempotency"
	"github.com/octarq-org/octarq/server/internal/api"
	"github.com/octarq-org/octarq/server/internal/auth"
	"github.com/octarq-org/octarq/server/internal/cache"
	"github.com/octarq-org/octarq/server/internal/cron"
	"github.com/octarq-org/octarq/server/internal/crypto"
	"github.com/octarq-org/octarq/server/internal/endpoint"
	"github.com/octarq-org/octarq/server/internal/eventbus"
	"github.com/octarq-org/octarq/server/internal/geo"
	"github.com/octarq-org/octarq/server/internal/mcp"
	"github.com/octarq-org/octarq/server/internal/monitor"
	"github.com/octarq-org/octarq/server/internal/notification"
	"github.com/octarq-org/octarq/server/internal/queue"
	"github.com/octarq-org/octarq/server/internal/server"
	"github.com/octarq-org/octarq/server/origin"
	"github.com/octarq-org/octarq/server/pkg/telemetry"
	"github.com/octarq-org/octarq/server/plugin"
	"github.com/octarq-org/octarq/server/webembed"
	"gorm.io/gorm"
)

type App struct {
	cfg             *config.Config
	gdb             *gorm.DB
	cipher          *crypto.Cipher
	auth            *auth.Manager
	geo             *geo.Resolver
	plugins         []plugin.Plugin
	services        *plugin.Registry
	healthCollector atomic.Pointer[monitor.Collector]
	webFS           fs.FS
	telemetry       *telemetry.Telemetry
	mcpTierGate     plugin.TierGate
}

func (a *App) HealthCollector() *monitor.Collector {
	return a.healthCollector.Load()
}

func (a *App) RunMCP(ctx context.Context) error {
	if err := a.prepareMigrationAndCipher(); err != nil {
		return err
	}

	taskQueue := queue.New(a.cfg.RedisURL)
	go taskQueue.Start(ctx)
	apiHandler := api.New(a.cfg, a.gdb, a.cipher, a.auth, a.geo, taskQueue)
	if err := apiHandler.InitErr(); err != nil {
		return fmt.Errorf("rate limiting is unavailable: %w", err)
	}
	apiHandler.SetPlugins(a.plugins)
	throwaway := apiHandler.Routes()
	services := plugin.NewRegistry()
	a.services = services
	apiHandler.SetServiceLookup(services.Lookup)
	if apiHandler.Storage() != nil {
		services.Provide(plugin.ServiceStorage, plugin.StorageService(apiHandler.Storage()))
	}
	services.Provide(plugin.ServiceExportSignedURL, plugin.ExportSignedURLFunc(apiHandler.GenerateExportSignedURL))
	cronEngine := cron.New(a.cfg.RedisURL)
	apiHandler.SetCronService(cronEngine)
	services.Provide(plugin.ServiceCron, plugin.CronService(cronEngine))
	var eventMu sync.Mutex
	var deferredEvents []func(string, json.RawMessage)
	endpointEngine := endpoint.NewEngine()
	notifRouter := notification.NewRouter(a.gdb)
	if err := notifRouter.RegisterChannel(notification.NewEmailChannel(a.gdb, func(ctx context.Context, orgID uint, to, subject, htmlBody, textBody string) error {
		return a.sendMail(orgID, to, subject, htmlBody, textBody)
	})); err != nil {
		slog.Warn("app: failed to register email notification channel in MCP mode", "error", err)
	}
	notification.SetDefaultRouter(notifRouter)
	pctx := a.buildPluginContext(pluginContextParams{
		apiHandler:      apiHandler,
		gdb:             a.gdb,
		cipher:          a.cipher,
		auth:            a.auth,
		notifRouter:     notifRouter,
		services:        services,
		geo:             a.geo,
		endpointEngine:  endpointEngine,
		taskQueue:       taskQueue,
		cronEngine:      cronEngine,
		sendMail:        a.sendMail,
		loginByEmail:    a.loginByEmail,
		loginByIdentity: a.loginByIdentity,
		onEventMu:       &eventMu,
		deferredEvents:  &deferredEvents,
		handleRoot:      func(h http.Handler) {},
		handleStatic:    func(prefix string, fsys fs.FS) {},
		httpMode:        false,
		plugins:         a.plugins,
	})
	apiHandler.SetEndpointSource(endpointEngine)
	services.Provide(idempotency.ServiceName, idempotency.New(a.gdb).Middleware(func(r *http.Request) uint {
		return a.auth.OrgID(r)
	}))
	services.Provide(plugin.ServiceNotificationRouter, notifRouter)
	unscoped, err := newUnscopedMatcher(a.plugins)
	if err != nil {
		return err
	}
	enabled := a.pluginGate(apiHandler, unscoped)
	routes := newRouteRegistry()
	for _, p := range a.plugins {
		pctxCopy := *pctx
		pInfo := plugin.Describe(p)
		pName := p.Name()
		pctxCopy.Cache = cache.NewScoped(a.auth.Cache(), pName)
		pctxCopy.RegisterNotificationChannel = func(ch plugin.NotificationChannel) {
			if notifRouter != nil {
				if err := notifRouter.RegisterChannelWithDescriptor(ch, notification.Descriptor{
					Type:        ch.Name(),
					Title:       pInfo.Title,
					Description: pInfo.Description,
					Icon:        pInfo.Icon,
					PluginName:  pName,
				}); err != nil {
					slog.Warn("app: failed to register notification channel with descriptor in MCP mode", "plugin", pName, "error", err)
				}
			}
		}
		pctxCopy.RegisterNotifier = func(typ string, send func(ctx context.Context, cfgJSON, text string) error) {
			pctxCopy.RegisterNotificationChannel(notification.NewLegacyNotifierAdapter(typ, pInfo.Title, send))
		}
		thirdParty := pluginIsThirdParty(p)
		recorder := &recordingMux{real: throwaway, routes: routes, plugin: pName, thirdParty: thirdParty}
		if plugin.FeatureIsCore(a.plugins, plugin.FeatureKey(p)) {
			pctxCopy.Huma = &recordingAPI{
				API:      apiHandler.Huma(),
				rAdapter: &recordingAdapter{Adapter: apiHandler.Huma().Adapter(), routes: routes, owner: pName},
			}
			p.Mount(recorder, &pctxCopy)
		} else {
			pctxCopy.Huma = &gatedAPI{
				API: apiHandler.Huma(),
				gAdapter: &gatedAdapter{
					Adapter:    apiHandler.Huma().Adapter(),
					plugin:     plugin.FeatureKey(p),
					enabled:    enabled,
					routes:     routes,
					owner:      pName,
					thirdParty: thirdParty,
				},
			}
			p.Mount(&gatedMux{real: recorder, plugin: plugin.FeatureKey(p), enabled: enabled}, &pctxCopy)
		}
	}
	if err := preflightUnscopedClaims(a.plugins, routes); err != nil {
		return err
	}
	if err := routes.Err(); err != nil {
		return err
	}
	if err := services.Err(); err != nil {
		return err
	}
	if vs, ok := plugin.LookupServiceAs[plugin.VaultService](services.Lookup, plugin.ServiceVault); ok {
		eventbus.SetVaultService(vs)
		notifRouter.SetVaultService(vs)
		apiHandler.SetVaultService(vs)
	}
	if err := a.verifyNoOrphanedPluginData(); err != nil {
		return err
	}
	valCtx, valCancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	err = plugin.ValidateAll(valCtx, a.plugins, mcp.NewStdioPluginHost(nil, pctx.Host))
	valCancel()
	if err != nil {
		return err
	}
	if eventsService, ok := plugin.LookupServiceAs[plugin.EventDispatcher](services.Lookup, plugin.ServiceEventsDispatch); ok {
		eventMu.Lock()
		handlers := deferredEvents
		deferredEvents = nil
		eventMu.Unlock()
		for _, handler := range handlers {
			eventsService(handler)
		}
	}

	return mcp.RunStdio(ctx, a.gdb, a.plugins, pctx.Host, a.mcpTierGate)
}

func (a *App) Run(ctx context.Context) error {
	defer a.geo.Close()

	if err := a.prepareMigrationAndCipher(); err != nil {
		return err
	}

	auth.InitGothStore(a.cfg.SecretKey)
	taskQueue := queue.New(a.cfg.RedisURL)
	go func() {
		if err := taskQueue.Start(ctx); err != nil {
			slog.Error("queue start failed", "err", err)
		}
	}()
	apiHandler := api.New(a.cfg, a.gdb, a.cipher, a.auth, a.geo, taskQueue)
	if err := apiHandler.InitErr(); err != nil {
		return fmt.Errorf("rate limiting is unavailable: %w", err)
	}
	apiHandler.SetPlugins(a.plugins)
	mux := apiHandler.Routes()
	services := plugin.NewRegistry()
	a.services = services
	apiHandler.SetServiceLookup(services.Lookup)
	if apiHandler.Storage() != nil {
		services.Provide(plugin.ServiceStorage, plugin.StorageService(apiHandler.Storage()))
	}
	services.Provide(plugin.ServiceExportSignedURL, plugin.ExportSignedURLFunc(apiHandler.GenerateExportSignedURL))
	cronEngine := cron.New(a.cfg.RedisURL)
	apiHandler.SetCronService(cronEngine)
	services.Provide(plugin.ServiceCron, plugin.CronService(cronEngine))
	var rootHandler http.Handler
	var staticMounts []server.StaticMount
	var runEventMu sync.Mutex
	var runDeferredEvents []func(string, json.RawMessage)
	endpointEngine := endpoint.NewEngine()
	notifRouter := notification.NewRouter(a.gdb)
	if err := notifRouter.RegisterChannel(notification.NewEmailChannel(a.gdb, func(ctx context.Context, orgID uint, to, subject, htmlBody, textBody string) error {
		return a.sendMail(orgID, to, subject, htmlBody, textBody)
	})); err != nil {
		slog.Warn("app: failed to register email notification channel in Run mode", "error", err)
	}
	notification.SetDefaultRouter(notifRouter)
	pctx := a.buildPluginContext(pluginContextParams{
		apiHandler:      apiHandler,
		gdb:             a.gdb,
		cipher:          a.cipher,
		auth:            a.auth,
		notifRouter:     notifRouter,
		services:        services,
		geo:             a.geo,
		endpointEngine:  endpointEngine,
		taskQueue:       taskQueue,
		cronEngine:      cronEngine,
		sendMail:        a.sendMail,
		loginByEmail:    a.loginByEmail,
		loginByIdentity: a.loginByIdentity,
		onEventMu:       &runEventMu,
		deferredEvents:  &runDeferredEvents,
		handleRoot:      func(h http.Handler) { rootHandler = h },
		handleStatic: func(prefix string, fsys fs.FS) {
			staticMounts = append(staticMounts, server.StaticMount{Prefix: prefix, FS: fsys})
		},
		httpMode: true,
		plugins:  a.plugins,
	})
	apiHandler.SetEndpointSource(endpointEngine)
	apiHandler.SetHost(pctx.Host)
	a.Setup(pctx, apiHandler)
	healthCollector := monitor.NewCollector(30*time.Second,
		monitor.NewRuntimeProvider(),
		monitor.NewDBProvider(a.gdb),
		monitor.NewDiskProvider(""),
	)
	healthCollector.SetProviderSource(func() []plugin.HealthProvider {
		return services.HealthProviders()
	})
	a.healthCollector.Store(healthCollector)
	monitor.RegisterRoutes(apiHandler.Huma(), healthCollector)
	services.Provide(idempotency.ServiceName, idempotency.New(a.gdb).Middleware(func(r *http.Request) uint {
		return a.auth.OrgID(r)
	}))
	services.Provide(plugin.ServiceNotificationRouter, notifRouter)

	unscoped, err := newUnscopedMatcher(a.plugins)
	if err != nil {
		return err
	}
	enabled := a.pluginGate(apiHandler, unscoped)
	routes := newRouteRegistry()
	for _, p := range a.plugins {
		pctxCopy := *pctx
		pInfo := plugin.Describe(p)
		pName := p.Name()
		pctxCopy.Cache = cache.NewScoped(a.auth.Cache(), pName)
		pctxCopy.RegisterNotificationChannel = func(ch plugin.NotificationChannel) {
			if notifRouter != nil {
				if err := notifRouter.RegisterChannelWithDescriptor(ch, notification.Descriptor{
					Type:        ch.Name(),
					Title:       pInfo.Title,
					Description: pInfo.Description,
					Icon:        pInfo.Icon,
					PluginName:  pName,
				}); err != nil {
					slog.Warn("app: failed to register notification channel with descriptor", "plugin", pName, "error", err)
				}
			}
		}
		pctxCopy.RegisterNotifier = func(typ string, send func(ctx context.Context, cfgJSON, text string) error) {
			pctxCopy.RegisterNotificationChannel(notification.NewLegacyNotifierAdapter(typ, pInfo.Title, send))
		}
		thirdParty := pluginIsThirdParty(p)
		recorder := &recordingMux{real: mux, routes: routes, plugin: pName, thirdParty: thirdParty}
		if plugin.FeatureIsCore(a.plugins, plugin.FeatureKey(p)) {
			pctxCopy.Huma = &recordingAPI{
				API:      apiHandler.Huma(),
				rAdapter: &recordingAdapter{Adapter: apiHandler.Huma().Adapter(), routes: routes, owner: pName},
			}
			p.Mount(recorder, &pctxCopy)
		} else {
			pctxCopy.Huma = &gatedAPI{
				API: apiHandler.Huma(),
				gAdapter: &gatedAdapter{
					Adapter:    apiHandler.Huma().Adapter(),
					plugin:     plugin.FeatureKey(p),
					enabled:    enabled,
					routes:     routes,
					owner:      pName,
					thirdParty: thirdParty,
				},
			}
			p.Mount(&gatedMux{real: recorder, plugin: plugin.FeatureKey(p), enabled: enabled}, &pctxCopy)
		}
		slog.Info("plugin mounted", "name", p.Name())
	}
	mountEndpointHTTP(apiHandler, endpointEngine)
	if err := preflightUnscopedClaims(a.plugins, routes); err != nil {
		return err
	}
	if err := routes.Err(); err != nil {
		return err
	}
	if err := services.Err(); err != nil {
		return err
	}
	if vs, ok := plugin.LookupServiceAs[plugin.VaultService](services.Lookup, plugin.ServiceVault); ok {
		eventbus.SetVaultService(vs)
		notifRouter.SetVaultService(vs)
		apiHandler.SetVaultService(vs)
	}
	if err := a.verifyNoOrphanedPluginData(); err != nil {
		return err
	}
	if fn, ok := plugin.LookupServiceAs[plugin.TrustProxyProvider](services.Lookup, plugin.ServiceTrustProxy); ok {
		fn(a.cfg.TrustProxy)
	}
	if eventsService, ok := plugin.LookupServiceAs[plugin.EventDispatcher](services.Lookup, plugin.ServiceEventsDispatch); ok {
		runEventMu.Lock()
		handlers := runDeferredEvents
		runDeferredEvents = nil
		runEventMu.Unlock()
		for _, handler := range handlers {
			eventsService(handler)
		}
	}

	valCtx, valCancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	err = plugin.ValidateAll(valCtx, a.plugins, mcp.NewStdioPluginHost(nil, pctx.Host))
	valCancel()
	if err != nil {
		return err
	}

	pluginCtx, stopPluginContext := context.WithCancel(ctx)
	defer stopPluginContext()

	for _, p := range a.plugins {
		if s, ok := p.(plugin.Starter); ok {
			go s.Start(pluginCtx)
		}
	}
	eventbus.StartReactors(pluginCtx, 0)
	if err := cron.RegisterBackupJob(cronEngine, a.cfg); err != nil {
		slog.Error("cron backup registration failed", "err", err)
	}
	go func() {
		if err := cronEngine.Start(ctx); err != nil {
			slog.Error("cron start failed", "err", err)
		}
	}()

	domainsRegistered := pctx.Host.Hosts().Any() || origin.HasSharedHosts(a.gdb)
	mailReady := false
	if fn, ok := plugin.LookupServiceAs[plugin.MessagingReady](services.Lookup, plugin.ServiceMessagingReady); ok {
		mailReady = fn()
	}
	for _, line := range readinessReport(a.cfg, mailReady, domainsRegistered, apiHandler.RequireEmailVerification()) {
		slog.Info("readiness", "status", string(line.Status), "check", line.Subject, "detail", line.Detail)
	}
	if err := enforceSecretKeyFloor(a.cfg, domainsRegistered); err != nil {
		return err
	}

	webFS := a.webFS
	if webFS == nil {
		embedded, err := webembed.FS()
		if err != nil {
			if a.cfg.DevWebProxy == "" {
				return err
			}
		} else {
			webFS = embedded
		}
	}
	if a.cfg.DevWebProxy != "" {
		slog.Info("dev web proxy enabled", "target", a.cfg.DevWebProxy)
	}
	return a.serve(ctx, serverRunParams{
		mux:               mux,
		rootHandler:       rootHandler,
		webFS:             webFS,
		staticMounts:      staticMounts,
		apiHandler:        apiHandler,
		services:          services,
		cronEngine:        cronEngine,
		healthCollector:   healthCollector,
		stopPluginContext: stopPluginContext,
	})
}
