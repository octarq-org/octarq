// Package app is the public composition root for octarq. It wires config, the
// database, auth, the core API, the short-link redirector and the embedded
// dashboard into one HTTP server — and lets external (Pro) modules extend it
// through the plugin package without forking.
//
// This is the importable seam of the Core-as-Library split: the open-core
// binary (cmd in this repo) calls New().Run() with no plugins; the private
// octarq-core consumer calls Use() for each Pro plugin before Run().
//
// AutoMigrate timing: New() opens the database but does NOT migrate. Run()
// collects core models plus every registered plugin's Models() and migrates
// them together, exactly once, before any request is served.
package app

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/octarq-org/octarq/config"
	"github.com/octarq-org/octarq/idempotency"
	"github.com/octarq-org/octarq/internal/api"
	"github.com/octarq-org/octarq/internal/auth"
	"github.com/octarq-org/octarq/internal/cache"
	"github.com/octarq-org/octarq/internal/cleanup"
	"github.com/octarq-org/octarq/internal/crypto"
	"github.com/octarq-org/octarq/internal/db"
	"github.com/octarq-org/octarq/internal/eventbus"
	"github.com/octarq-org/octarq/internal/geo"
	"github.com/octarq-org/octarq/internal/mcp"
	"github.com/octarq-org/octarq/internal/notify"
	"github.com/octarq-org/octarq/internal/queue"
	"github.com/octarq-org/octarq/internal/server"
	"github.com/octarq-org/octarq/origin"
	"github.com/octarq-org/octarq/plugin"
	"github.com/octarq-org/octarq/plugin/safehttp"
	"github.com/octarq-org/octarq/webembed"
	"gorm.io/gorm"
)

// App holds the wired core dependencies and any registered plugins.
type App struct {
	cfg      *config.Config
	gdb      *gorm.DB
	cipher   *crypto.Cipher
	auth     *auth.Manager
	geo      *geo.Resolver
	plugins  []plugin.Plugin
	services *plugin.Registry
	webFS    fs.FS // overrides the embedded OSS dashboard when set (see WithWebFS)
}

// New loads configuration and opens the database (without migrating). Call
// Use to register plugins, then Run.
func New() (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	gdb, err := db.Open(cfg)
	if err != nil {
		return nil, err
	}

	eventbus.Init(gdb)
	// Opt webhook/notification delivery into private targets only when the operator
	// has explicitly allowed it (trusted internal receivers); default stays strict.
	safehttp.SetAllowPrivateWebhooks(cfg.AllowPrivateWebhooks)
	safehttp.SetAllowPrivateSMTP(cfg.AllowPrivateSMTP)

	cipher := crypto.New(cfg.SecretKey)
	// Webhook signing secrets are AES-GCM encrypted at rest; teach the eventbus how
	// to unwrap them before HMAC-signing a delivery. (Envelope mode is enabled in
	// Run before any delivery fires.)
	eventbus.SetSecretDecryptor(func(stored string) (string, bool) {
		b, err := cipher.Decrypt(stored)
		if err != nil {
			return "", false
		}
		return string(b), true
	})
	notify.SetConfigDecryptor(func(stored string) (string, bool) {
		b, err := cipher.Decrypt(stored)
		if err != nil {
			return "", false
		}
		return string(b), true
	})
	cacheLayer := cache.New(cfg.RedisURL)
	authMgr := auth.New(cfg, cipher).WithDB(gdb).WithCache(cacheLayer)

	geoResolver, err := geo.Open(cfg.GeoIPDB)
	if err != nil {
		slog.Warn("geoip disabled", "err", err)
		geoResolver, _ = geo.Open("")
	}

	a := &App{
		cfg:    cfg,
		gdb:    gdb,
		cipher: cipher,
		auth:   authMgr,
		geo:    geoResolver,
	}
	// Composition is the caller's job: New() mounts no feature plugins. Each
	// entry point (octarq/main.go, octarq-pro's main, a trimmed edition) Uses the
	// plugins it wants — Core plugins the same way Pro plugins are added, via
	// a.Use. The OSS default set is plugins/builtin.Default(); see
	// website/src/content/docs/architecture/overview.md.
	return a, nil
}

// RunMCP runs the MCP server with the registered plugins over stdio. It mounts
// the plugins with the same plugin.Context the HTTP server uses, so their MCP
// tool handlers have their dependencies wired (DB, DNS manager, Encrypt/Decrypt)
// — without this, plugin MCP tools would run with nil dependencies. The HTTP mux
// they mount onto is discarded; only the captured context matters here.
func (a *App) RunMCP(ctx context.Context) error {
	if err := preflightNameCollisions(a.plugins); err != nil {
		return err
	}
	if err := preflightDependencies(a.plugins); err != nil {
		return err
	}
	if err := preflightTableCollisions(a.gdb.NamingStrategy, a.plugins); err != nil {
		return err
	}
	// The webhook delivery log and the idempotency ledger are core
	// infrastructure that no plugin owns, so they join the same single pass.
	extra := append(eventbus.Models(), idempotency.Models()...)
	for _, p := range a.plugins {
		extra = append(extra, p.Models()...)
	}
	if err := db.Migrate(a.gdb, extra...); err != nil {
		return err
	}
	if err := a.cipher.EnableEnvelope(settingsStore{a.gdb}); err != nil {
		return err
	}

	taskQueue := queue.New(a.cfg.RedisURL)
	go taskQueue.Start(ctx)
	apiHandler := api.New(a.cfg, a.gdb, a.cipher, a.auth, a.geo, taskQueue)
	apiHandler.SetPlugins(a.plugins)
	throwaway := apiHandler.Routes()
	services := plugin.NewRegistry()
	a.services = services
	apiHandler.SetServiceLookup(services.Lookup)
	var emailMu sync.Mutex
	var deferredOnEmail []func(plugin.EmailEvent)
	pctx := &plugin.Context{
		Huma:   apiHandler.Huma(),
		DB:     a.gdb,
		Guard:  a.auth.Require,
		Notify: notify.Send,
		RegisterNotifier: func(typ string, send func(ctx context.Context, cfgJSON, text string) error) {
			notify.Register(typ, send)
		},
		RevokeUserOrgSessions: a.auth.RevokeUserOrgSessions,
		UserID:                a.auth.UserID,
		OrgID:                 a.auth.OrgID,
		OrgRole:               apiHandler.OrgRole,
		RequireRole:           apiHandler.RequireRole,
		RequirePerm:           apiHandler.RequirePerm,
		IsInstanceAdmin:       apiHandler.IsInstanceAdmin,
		LoginByEmail:          a.loginByEmail,
		LoginByIdentity:       a.loginByIdentity,
		BindIdentity:          a.auth.BindIdentity,
		RegisterAuthMethod: func(m plugin.AuthMethod) {
			auth.Register(auth.AuthMethod{
				ID:        m.ID,
				Label:     m.Label,
				LoginURL:  m.LoginURL,
				IconKey:   m.IconKey,
				Available: m.Available,
			})
		},
		Audit:   apiHandler.Audit,
		Encrypt: a.cipher.Encrypt,
		Decrypt: a.cipher.Decrypt,
		OnEmail: func(handler func(plugin.EmailEvent)) {
			if handler == nil {
				return
			}
			if onEmailService, ok := plugin.LookupServiceAs[plugin.EmailDispatcher](services.Lookup, plugin.ServiceMailDispatcher); ok {
				onEmailService(handler)
				return
			}
			emailMu.Lock()
			deferredOnEmail = append(deferredOnEmail, handler)
			emailMu.Unlock()
		},
		DNS:                  &lazyDNSManager{lookup: services.Lookup},
		SendMail:             a.sendMail,
		SetLLMResolver:       apiHandler.SetLLMResolver,
		SetLLMResolverForOrg: apiHandler.SetLLMResolverForOrg,
		RecordUsage: func(orgID uint, metric string, n int64) {
			// Lazily resolved on every call: the provider (Pro's cloud module) may
			// mount after the plugin that meters, so a Mount-time Lookup would
			// silently never find it.
			if fn, ok := plugin.LookupServiceAs[plugin.UsageMeter](services.Lookup, plugin.ServiceCloudUsage); ok {
				fn(orgID, metric, n)
			}
		},
		GetWorkspaceSetting: apiHandler.GetWorkspaceSetting,
		GetGlobalSetting:    apiHandler.GetGlobalSetting,
		SetGlobalSetting:    apiHandler.SetGlobalSetting,
		SetWorkspaceSetting: apiHandler.SetWorkspaceSetting,
		Enqueue:             taskQueue.Enqueue,
		RegisterTask: func(taskType string, h func(ctx context.Context, payload []byte) error) {
			taskQueue.Register(taskType, h)
		},
		PublishEvent: eventbus.Publish,
		RegisterWebhookEvent: func(d plugin.WebhookEventDef) {
			eventbus.RegisterEventDef(eventbus.EventDef{Key: d.Key, Group: d.Group, Title: d.Title, Description: d.Description})
		},
		CacheGet:    a.auth.Cache().Get,
		CacheSet:    a.auth.Cache().Set,
		DeleteCache: a.auth.Cache().Delete,
		GeoLookup:   a.geo.Locate,
		ParseUA: func(ua string) (string, string, string) {
			info := geo.ParseUA(ua)
			return info.Device, info.Browser, info.OS
		},
		HandleRoot:   func(h http.Handler) { /* unused in MCP */ },
		HandleStatic: func(prefix string, fsys fs.FS) { /* unused in MCP */ },
		Provide:      services.Provide,
		Lookup:       services.Lookup,
	}
	// Same idempotency seam as the HTTP path — a plugin that resolves it in
	// Mount must find it in both compositions.
	services.Provide(idempotency.ServiceName, idempotency.New(a.gdb).Middleware(func(r *http.Request) uint {
		return a.auth.OrgID(r)
	}))
	enabled := a.pluginGate(apiHandler)
	// Route ownership guard: see routeRegistry. Declared before the mount loop
	// because every plugin registers through it.
	routes := newRouteRegistry()
	for _, p := range a.plugins {
		pctxCopy := *pctx
		pInfo := plugin.Describe(p)
		pName := p.Name()
		pctxCopy.RegisterNotifier = func(typ string, send func(ctx context.Context, cfgJSON, text string) error) {
			notify.RegisterWithDescriptor(notify.Descriptor{
				Type:        typ,
				Title:       pInfo.Title,
				Description: pInfo.Description,
				Icon:        pInfo.Icon,
				PluginName:  pName,
			}, send)
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
	// Two plugins claiming the same route, or the same service name, is a wiring
	// bug — refuse to serve, same as a table collision. Routes first: that one
	// would otherwise have been a ServeMux panic.
	if err := routes.Err(); err != nil {
		return err
	}
	if err := services.Err(); err != nil {
		return err
	}
	if onEmailService, ok := plugin.LookupServiceAs[plugin.EmailDispatcher](services.Lookup, plugin.ServiceMailDispatcher); ok {
		emailMu.Lock()
		handlers := deferredOnEmail
		deferredOnEmail = nil
		emailMu.Unlock()
		for _, handler := range handlers {
			onEmailService(handler)
		}
	}

	return mcp.RunWithPlugins(ctx, a.plugins)
}

// Run migrates the schema (core + plugin models), builds the HTTP server, and
// serves until interrupted.
func (a *App) Run(ctx context.Context) error {
	defer a.geo.Close()

	// 1. Migrate AFTER every plugin is registered, so plugin models join the
	//    core schema in a single AutoMigrate pass. First refuse startup if two
	//    different plugin model types would fight over the same table.
	if err := preflightNameCollisions(a.plugins); err != nil {
		return err
	}
	if err := preflightDependencies(a.plugins); err != nil {
		return err
	}
	if err := preflightTableCollisions(a.gdb.NamingStrategy, a.plugins); err != nil {
		return err
	}
	// The webhook delivery log and the idempotency ledger are core
	// infrastructure that no plugin owns, so they join the same single pass.
	extra := append(eventbus.Models(), idempotency.Models()...)
	for _, p := range a.plugins {
		extra = append(extra, p.Models()...)
	}
	if err := db.Migrate(a.gdb, extra...); err != nil {
		return err
	}

	// 1b. Upgrade the cipher to envelope mode now that the settings table exists
	//     (loads or creates the DEK; re-wraps it under a rotated key if needed).
	if err := a.cipher.EnableEnvelope(settingsStore{a.gdb}); err != nil {
		return err
	}

	// 2. Core API mux, then let plugins mount their own routes onto it.
	auth.InitGothStore(a.cfg.SecretKey)
	taskQueue := queue.New(a.cfg.RedisURL)
	go func() {
		if err := taskQueue.Start(ctx); err != nil {
			slog.Error("queue start failed", "err", err)
		}
	}()
	apiHandler := api.New(a.cfg, a.gdb, a.cipher, a.auth, a.geo, taskQueue)
	apiHandler.SetPlugins(a.plugins)
	mux := apiHandler.Routes()
	services := plugin.NewRegistry()
	a.services = services
	apiHandler.SetServiceLookup(services.Lookup)
	var rootHandler http.Handler
	var staticMounts []server.StaticMount
	var runEmailMu sync.Mutex
	var runDeferredOnEmail []func(plugin.EmailEvent)
	pctx := &plugin.Context{
		Huma:   apiHandler.Huma(),
		DB:     a.gdb,
		Guard:  a.auth.Require,
		Notify: notify.Send,
		RegisterNotifier: func(typ string, send func(ctx context.Context, cfgJSON, text string) error) {
			notify.Register(typ, send)
		},
		RevokeUserOrgSessions: a.auth.RevokeUserOrgSessions,
		UserID:                a.auth.UserID,
		OrgID:                 a.auth.OrgID,
		OrgRole:               apiHandler.OrgRole,
		RequireRole:           apiHandler.RequireRole,
		RequirePerm:           apiHandler.RequirePerm,
		IsInstanceAdmin:       apiHandler.IsInstanceAdmin,
		LoginByEmail:          a.loginByEmail,
		LoginByIdentity:       a.loginByIdentity,
		BindIdentity:          a.auth.BindIdentity,
		RegisterAuthMethod: func(m plugin.AuthMethod) {
			auth.Register(auth.AuthMethod{
				ID:        m.ID,
				Label:     m.Label,
				LoginURL:  m.LoginURL,
				IconKey:   m.IconKey,
				Available: m.Available,
			})
		},
		Audit:   apiHandler.Audit,
		Encrypt: a.cipher.Encrypt,
		Decrypt: a.cipher.Decrypt,
		OnEmail: func(handler func(plugin.EmailEvent)) {
			if handler == nil {
				return
			}
			if onEmailService, ok := plugin.LookupServiceAs[plugin.EmailDispatcher](services.Lookup, plugin.ServiceMailDispatcher); ok {
				onEmailService(handler)
				return
			}
			runEmailMu.Lock()
			runDeferredOnEmail = append(runDeferredOnEmail, handler)
			runEmailMu.Unlock()
		},
		DNS:                  &lazyDNSManager{lookup: services.Lookup},
		SendMail:             a.sendMail,
		SetLLMResolver:       apiHandler.SetLLMResolver,
		SetLLMResolverForOrg: apiHandler.SetLLMResolverForOrg,
		RecordUsage: func(orgID uint, metric string, n int64) {
			// Lazily resolved on every call: the provider (Pro's cloud module) may
			// mount after the plugin that meters, so a Mount-time Lookup would
			// silently never find it.
			if fn, ok := plugin.LookupServiceAs[plugin.UsageMeter](services.Lookup, plugin.ServiceCloudUsage); ok {
				fn(orgID, metric, n)
			}
		},
		GetWorkspaceSetting: apiHandler.GetWorkspaceSetting,
		GetGlobalSetting:    apiHandler.GetGlobalSetting,
		SetGlobalSetting:    apiHandler.SetGlobalSetting,
		SetWorkspaceSetting: apiHandler.SetWorkspaceSetting,
		Enqueue:             taskQueue.Enqueue,
		RegisterTask: func(taskType string, h func(ctx context.Context, payload []byte) error) {
			taskQueue.Register(taskType, h)
		},
		PublishEvent: eventbus.Publish,
		RegisterWebhookEvent: func(d plugin.WebhookEventDef) {
			eventbus.RegisterEventDef(eventbus.EventDef{Key: d.Key, Group: d.Group, Title: d.Title, Description: d.Description})
		},
		CacheGet:    a.auth.Cache().Get,
		CacheSet:    a.auth.Cache().Set,
		DeleteCache: a.auth.Cache().Delete,
		GeoLookup:   a.geo.Locate,
		ParseUA: func(ua string) (string, string, string) {
			info := geo.ParseUA(ua)
			return info.Device, info.Browser, info.OS
		},
		PluginActive: func(orgID uint, p plugin.Plugin) bool {
			key := plugin.FeatureKey(p)
			if plugin.FeatureIsCore(a.plugins, key) {
				return true
			}
			return apiHandler.PluginEnabled(orgID, key)
		},
		FeatureActive: func(orgID uint, featureKey string) bool {
			if plugin.FeatureIsCore(a.plugins, featureKey) {
				return true
			}
			// Unlike the route gate, this answers for content that is listed
			// before a workspace is chosen (help docs). PluginEnabled fails
			// closed at orgID 0 by design, so ask for the declared default
			// instead of inheriting a "disabled" that only means "no org yet".
			if orgID == 0 {
				return apiHandler.FeatureDefaultEnabled(featureKey)
			}
			return apiHandler.PluginEnabled(orgID, featureKey)
		},
		ActivePlugins: func() []plugin.Plugin {
			return a.plugins
		},
		HandleRoot: func(h http.Handler) {
			rootHandler = h
		},
		HandleStatic: func(prefix string, fsys fs.FS) {
			staticMounts = append(staticMounts, server.StaticMount{Prefix: prefix, FS: fsys})
		},
		Provide: services.Provide,
		Lookup:  services.Lookup,
	}
	// Idempotency-Key support is offered to plugin routes through the service
	// registry rather than a new plugin.Context field, so a plugin adopts it
	// per route (see idempotency.ServiceName) without every route paying for it.
	services.Provide(idempotency.ServiceName, idempotency.New(a.gdb).Middleware(func(r *http.Request) uint {
		return a.auth.OrgID(r)
	}))
	// Non-core plugin routes are gated by a per-workspace feature toggle: when the
	// caller's workspace has the feature disabled, the app answers 404 before the
	// handler runs. Core plumbing (license activation) mounts ungated — it must
	// always work.
	enabled := a.pluginGate(apiHandler)
	// Route ownership guard: see routeRegistry. Declared before the mount loop
	// because every plugin registers through it.
	routes := newRouteRegistry()
	for _, p := range a.plugins {
		pctxCopy := *pctx
		pInfo := plugin.Describe(p)
		pName := p.Name()
		pctxCopy.RegisterNotifier = func(typ string, send func(ctx context.Context, cfgJSON, text string) error) {
			notify.RegisterWithDescriptor(notify.Descriptor{
				Type:        typ,
				Title:       pInfo.Title,
				Description: pInfo.Description,
				Icon:        pInfo.Icon,
				PluginName:  pName,
			}, send)
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
	// Two plugins claiming the same route, or the same service name, is a wiring
	// bug — refuse to serve, same as a table collision. Routes first: that one
	// would otherwise have been a ServeMux panic.
	if err := routes.Err(); err != nil {
		return err
	}
	if err := services.Err(); err != nil {
		return err
	}
	// Propagate cfg.TrustProxy to plugins that extract client IPs.
	// The seams were provided at mount time; this is the assembly-layer
	// wiring that was previously missing — see links/plugin.go:182.
	if fn, ok := plugin.LookupServiceAs[func(bool)](services.Lookup, "links.trust_proxy"); ok {
		fn(a.cfg.TrustProxy)
	}
	if fn, ok := plugin.LookupServiceAs[func(bool)](services.Lookup, "dns.trust_proxy"); ok {
		fn(a.cfg.TrustProxy)
	}
	// Launch Starters only after EVERY plugin has mounted (and Provided): this
	// is the ordering guarantee that makes Start-time Lookup of another
	// plugin's services safe regardless of registration order.
	if onEmailService, ok := plugin.LookupServiceAs[plugin.EmailDispatcher](services.Lookup, plugin.ServiceMailDispatcher); ok {
		runEmailMu.Lock()
		handlers := runDeferredOnEmail
		runDeferredOnEmail = nil
		runEmailMu.Unlock()
		for _, handler := range handlers {
			onEmailService(handler)
		}
	}

	for _, p := range a.plugins {
		if s, ok := p.(plugin.Starter); ok {
			go s.Start(ctx)
		}
	}

	// Tell the operator, once and before anything is served, which capabilities
	// this instance actually has. Several of them fail silently otherwise — see
	// readinessReport.
	domainsRegistered := origin.AnyRegistered(a.gdb)
	mailReady := false
	if fn, ok := plugin.LookupServiceAs[plugin.MailReady](services.Lookup, plugin.ServiceMailReady); ok {
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
			return err
		}
		webFS = embedded
	}
	// CSRFGuard wraps the fully-assembled mux (core + plugin routes) to block
	// cross-site state-changing requests and validate double-submit tokens.
	// It runs outside huma to protect standard library routes too.
	srv, err := server.New(a.cfg, a.gdb, api.CSRFGuard(a.cfg.SecretKey, mux), rootHandler, webFS, staticMounts, server.RuntimeSettings{
		MetricsToken: apiHandler.MetricsToken,
		RateLimits:   apiHandler.RateLimits,
		PublicGET:    api.PublicGETMatcher(apiHandler.Huma()),
		CORSOrigins:  apiHandler.CORSOrigins,
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
	for _, p := range a.plugins {
		if fn, ok := plugin.LookupServiceAs[plugin.CleanupFunc](services.Lookup, plugin.CleanupServiceName(p.Name())); ok {
			cleanups = append(cleanups, fn)
		}
	}
	go cleanup.Start(ctx, apiHandler.DataRetentionDays, cleanups...)
	go cleanup.StartSessionCleanup(ctx, a.gdb, apiHandler.DataRetentionDays)

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
	return httpSrv.Shutdown(shutCtx)
}
