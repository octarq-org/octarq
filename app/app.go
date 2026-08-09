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
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"sync"
	"syscall"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/config"
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
	"github.com/octarq-org/octarq/internal/safehttp"
	"github.com/octarq-org/octarq/internal/server"
	"github.com/octarq-org/octarq/origin"
	"github.com/octarq-org/octarq/plugin"
	"github.com/octarq-org/octarq/webembed"
	"gorm.io/gorm"
)

// pluginGate builds the per-workspace feature check the gated mux and adapter
// share. Non-core plugin routes answer 404 when the caller's workspace has the
// feature disabled.
//
// The org-0 case returns scoped=false, i.e. "not a workspace-scoped request, do
// not gate it" — the route is then served. That is deliberate and load-bearing:
// public plugin routes (payment webhooks, the buyer portal, license activation)
// carry no session, so there is no workspace whose toggle could be consulted.
// Returning scoped=true here to look "fail-closed" would 404 every incoming
// Stripe webhook and every buyer — payments would stop, quietly, and the failure
// would surface as missing revenue rather than as an error.
//
// What makes it safe is that those routes authenticate themselves (webhook
// signature, customer session cookie). That is an implicit contract with every
// public plugin route, so it is pinned by TestPluginGateDoesNotGateAnonymous.
func (a *App) pluginGate(apiHandler interface{ PluginEnabled(uint, string) bool }) func(*http.Request, string) (allowed, scoped bool) {
	return func(r *http.Request, featureKey string) (allowed, scoped bool) {
		oid := a.auth.OrgID(r)
		if oid == 0 {
			return false, false // no workspace in session (webhooks, portal) → not gated
		}
		return apiHandler.PluginEnabled(oid, featureKey), true
	}
}

// recoverWriter wraps http.ResponseWriter to track whether response headers
// have already been written. This lets the panic-recovery path avoid a
// superfluous WriteHeader call (and the warning log it would produce) when a
// handler panics after partially writing the response.
type recoverWriter struct {
	http.ResponseWriter
	wroteHeader bool
}

func (rw *recoverWriter) WriteHeader(code int) {
	rw.wroteHeader = true
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *recoverWriter) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		rw.wroteHeader = true
	}
	return rw.ResponseWriter.Write(b)
}

// Flush forwards to the underlying ResponseWriter if it implements
// http.Flusher, keeping SSE / streaming responses working.
func (rw *recoverWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// gatedMux wraps the shared API mux so every route a plugin registers is guarded
// by a per-workspace "plugin enabled" check. It satisfies plugin.Mux; when the
// caller's workspace has the plugin disabled, the wrapped handler answers 404
// before running. Requests with no workspace in session (public plugin routes
// such as payment webhooks and the customer portal) pass through unchanged —
// they aren't workspace-scoped and can't be org-gated here.
type gatedMux struct {
	real    *http.ServeMux
	plugin  string
	enabled func(r *http.Request, plugin string) (allowed, scoped bool)
}

func (g *gatedMux) Handle(pattern string, h http.Handler) {
	g.real.Handle(pattern, g.wrap(h))
}

func (g *gatedMux) HandleFunc(pattern string, h func(http.ResponseWriter, *http.Request)) {
	g.real.Handle(pattern, g.wrap(http.HandlerFunc(h)))
}

func (g *gatedMux) wrap(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if allowed, scoped := g.enabled(r, g.plugin); scoped && !allowed {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"plugin not enabled for this workspace"}`))
			return
		}
		rw := &recoverWriter{ResponseWriter: w}
		defer func() {
			if rec := recover(); rec != nil {
				if rec == http.ErrAbortHandler {
					panic(rec)
				}
				slog.Error("plugin handler panic",
					"plugin", g.plugin,
					"path", r.URL.Path,
					"panic", rec,
					"stack", string(debug.Stack()),
				)
				if !rw.wroteHeader {
					rw.ResponseWriter.Header().Set("Content-Type", "application/json")
					rw.ResponseWriter.WriteHeader(http.StatusInternalServerError)
					_, _ = rw.ResponseWriter.Write([]byte(`{"error":"internal server error"}`))
				}
			}
		}()
		h.ServeHTTP(rw, r)
	})
}

type gatedAPI struct {
	huma.API
	gAdapter huma.Adapter
}

func (g *gatedAPI) Adapter() huma.Adapter {
	return g.gAdapter
}

type gatedAdapter struct {
	huma.Adapter
	plugin  string
	enabled func(r *http.Request, plugin string) (allowed, scoped bool)
}

func (g *gatedAdapter) Handle(op *huma.Operation, handler func(ctx huma.Context)) {
	g.Adapter.Handle(op, func(ctx huma.Context) {
		r, w := humago.Unwrap(ctx)
		if allowed, scoped := g.enabled(r, g.plugin); scoped && !allowed {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"plugin not enabled for this workspace"}`))
			return
		}
		defer func() {
			if rec := recover(); rec != nil {
				if rec == http.ErrAbortHandler {
					panic(rec)
				}
				slog.Error("plugin huma handler panic",
					"plugin", g.plugin,
					"path", r.URL.Path,
					"panic", rec,
					"stack", string(debug.Stack()),
				)
				// Best-effort 500 response. If the handler already wrote
				// headers through the huma context, WriteHeader is a no-op
				// with a harmless "superfluous" log — acceptable on a panic
				// path that is already exceptional.
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":"internal server error"}`))
			}
		}()
		handler(ctx)
	})
}

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

// WithWebFS overrides the embedded open-source dashboard with a caller-supplied
// filesystem. The commercial build (octarq-pro) uses this to serve a dashboard
// built with its plugin pages injected (VITE_OCTARQ_PLUGINS=pro) instead of the
// core's OSS bundle, whose empty plugin registry 404-degrades those pages. Pass
// an fs.FS rooted where the core's webembed.FS() would be (index.html at root).
func (a *App) WithWebFS(f fs.FS) *App { a.webFS = f; return a }

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
	// docs/PLUGIN-ARCHITECTURE.md.
	return a, nil
}

// DB exposes the shared database handle (useful for plugin construction).
func (a *App) DB() *gorm.DB { return a.gdb }

// loginByEmail backs plugin.Context.LoginByEmail. It delegates to the auth
// manager and translates the internal registration-disabled sentinel into the
// plugin-package one so identity plugins can errors.Is against it without
// importing internal/auth.
func (a *App) loginByEmail(w http.ResponseWriter, r *http.Request, email string) (uint, error) {
	uid, err := a.auth.LoginByEmail(w, r, email)
	if errors.Is(err, auth.ErrRegistrationDisabled) {
		return uid, plugin.ErrLoginRegistrationDisabled
	}
	return uid, err
}

// loginByIdentity backs plugin.Context.LoginByIdentity, translating the
// internal sentinels into the plugin-package ones the same way loginByEmail
// does. The two refusals are deliberately distinct: "this instance won't create
// accounts" is something an admin can change, while "that email already has an
// account" tells the visitor to sign in the way they already can and link SSO
// afterwards.
func (a *App) loginByIdentity(w http.ResponseWriter, r *http.Request, id plugin.ExternalIdentity) (uint, error) {
	uid, err := a.auth.LoginByIdentity(w, r, id)
	switch {
	case errors.Is(err, auth.ErrRegistrationDisabled):
		return uid, plugin.ErrLoginRegistrationDisabled
	case errors.Is(err, auth.ErrAccountLinkRequired):
		return uid, plugin.ErrAccountLinkRequired
	}
	return uid, err
}

// Notify delivers a notification via a configured channel type ("telegram", "webhook").
func (a *App) Notify(ctx context.Context, typ, cfgJSON, text string) error {
	return notify.Send(ctx, typ, cfgJSON, text)
}

// sendMail is the implementation behind plugin.Context.SendMail. It resolves the
// org's first configured SMTP sender, decrypts its password, and relays the
// message — mirroring internal/api.Handler.sendEmail so plugins can send
// transactional mail without importing octarq's internal packages.
func (a *App) sendMail(orgID uint, to, subject, htmlBody, textBody string) error {
	if a.services != nil {
		if v, ok := a.services.Lookup("mail.send"); ok {
			if fn, ok := v.(func(uint, string, string, string, string) error); ok {
				return fn(orgID, to, subject, htmlBody, textBody)
			}
		}
	}
	return fmt.Errorf("no mail plugin mounted to send email for org %d", orgID)
}

// Use registers a plugin. All plugins must be registered before Run so their
// models are migrated and their routes mounted.
func (a *App) Use(p plugin.Plugin) { a.plugins = append(a.plugins, p) }

// lazyDNSManager resolves the plugin.DNSManager provided by the dns Core plugin
// under plugin.ServiceDNSManager on each call. Resolution happens at request
// time, after all plugins have mounted.
type lazyDNSManager struct {
	lookup func(name string) (any, bool)
}

var _ plugin.DNSManager = (*lazyDNSManager)(nil)

func (l *lazyDNSManager) resolve() (plugin.DNSManager, error) {
	if v, ok := l.lookup(plugin.ServiceDNSManager); ok {
		if m, ok := v.(plugin.DNSManager); ok {
			return m, nil
		}
	}
	return nil, errors.New("dns manager unavailable: the dns plugin is not mounted")
}

func (l *lazyDNSManager) List(ctx context.Context, orgID, domainID uint) ([]plugin.DNSRecord, error) {
	m, err := l.resolve()
	if err != nil {
		return nil, err
	}
	return m.List(ctx, orgID, domainID)
}

func (l *lazyDNSManager) Set(ctx context.Context, orgID, domainID uint, r plugin.DNSRecord) (plugin.DNSRecord, error) {
	m, err := l.resolve()
	if err != nil {
		return plugin.DNSRecord{}, err
	}
	return m.Set(ctx, orgID, domainID, r)
}

func (l *lazyDNSManager) Delete(ctx context.Context, orgID, domainID uint, recordID string) error {
	m, err := l.resolve()
	if err != nil {
		return err
	}
	return m.Delete(ctx, orgID, domainID, recordID)
}

// Plugins returns the registered plugins.
func (a *App) Plugins() []plugin.Plugin {
	return a.plugins
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
	var extra []any
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
			if disp, ok := services.Lookup("mail.dispatcher"); ok {
				if onEmailService, ok := disp.(func(func(plugin.EmailEvent))); ok {
					onEmailService(handler)
					return
				}
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
			if v, ok := services.Lookup("cloud.usage"); ok {
				if fn, ok := v.(func(orgID uint, metric string, n int64)); ok {
					fn(orgID, metric, n)
				}
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
	enabled := a.pluginGate(apiHandler)
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
		if plugin.FeatureIsCore(a.plugins, plugin.FeatureKey(p)) {
			pctxCopy.Huma = apiHandler.Huma()
			p.Mount(throwaway, &pctxCopy)
		} else {
			pctxCopy.Huma = &gatedAPI{
				API: apiHandler.Huma(),
				gAdapter: &gatedAdapter{
					Adapter: apiHandler.Huma().Adapter(),
					plugin:  plugin.FeatureKey(p),
					enabled: enabled,
				},
			}
			p.Mount(&gatedMux{real: throwaway, plugin: plugin.FeatureKey(p), enabled: enabled}, &pctxCopy)
		}
	}
	// Two plugins Providing the same service name is a wiring bug — refuse to
	// serve, same as a table collision.
	if err := services.Err(); err != nil {
		return err
	}
	if disp, ok := services.Lookup("mail.dispatcher"); ok {
		if onEmailService, ok := disp.(func(func(plugin.EmailEvent))); ok {
			emailMu.Lock()
			handlers := deferredOnEmail
			deferredOnEmail = nil
			emailMu.Unlock()
			for _, handler := range handlers {
				onEmailService(handler)
			}
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
	var extra []any
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
			if disp, ok := services.Lookup("mail.dispatcher"); ok {
				if onEmailService, ok := disp.(func(func(plugin.EmailEvent))); ok {
					onEmailService(handler)
					return
				}
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
			if v, ok := services.Lookup("cloud.usage"); ok {
				if fn, ok := v.(func(orgID uint, metric string, n int64)); ok {
					fn(orgID, metric, n)
				}
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
	// Non-core plugin routes are gated by a per-workspace feature toggle: when the
	// caller's workspace has the feature disabled, the app answers 404 before the
	// handler runs. Core plumbing (license activation) mounts ungated — it must
	// always work.
	//
	// That is not what keeps public routes alive, and the two are easy to
	// conflate. Payment webhooks and the buyer portal reach the same gate, but
	// they survive because they carry no session: pluginGate reports org 0 as
	// "not workspace-scoped" and the route is served whether or not its plugin
	// is core. So a buyer-facing feature does not need Core to stay reachable —
	// see pluginGate for the org-0 contract and the test that pins it.
	enabled := a.pluginGate(apiHandler)
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
		if plugin.FeatureIsCore(a.plugins, plugin.FeatureKey(p)) {
			pctxCopy.Huma = apiHandler.Huma()
			p.Mount(mux, &pctxCopy)
		} else {
			pctxCopy.Huma = &gatedAPI{
				API: apiHandler.Huma(),
				gAdapter: &gatedAdapter{
					Adapter: apiHandler.Huma().Adapter(),
					plugin:  plugin.FeatureKey(p),
					enabled: enabled,
				},
			}
			p.Mount(&gatedMux{real: mux, plugin: plugin.FeatureKey(p), enabled: enabled}, &pctxCopy)
		}
		slog.Info("plugin mounted", "name", p.Name())
	}
	// Two plugins Providing the same service name is a wiring bug — refuse to
	// serve, same as a table collision.
	if err := services.Err(); err != nil {
		return err
	}
	// Launch Starters only after EVERY plugin has mounted (and Provided): this
	// is the ordering guarantee that makes Start-time Lookup of another
	// plugin's services safe regardless of registration order.
	if disp, ok := services.Lookup("mail.dispatcher"); ok {
		if onEmailService, ok := disp.(func(func(plugin.EmailEvent))); ok {
			runEmailMu.Lock()
			handlers := runDeferredOnEmail
			runDeferredOnEmail = nil
			runEmailMu.Unlock()
			for _, handler := range handlers {
				onEmailService(handler)
			}
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
	//
	// This has to run HERE, after the mount loop: "can this instance send mail"
	// is a lookup of the mail.send service in the plugin registry, and a plugin
	// only Provides it when it mounts. Asking earlier (in New, or before the
	// loop) would report "no mail" for every instance, including the ones that
	// have it — an answer that is wrong rather than merely early.
	domainsRegistered := origin.AnyRegistered(a.gdb)
	_, mailAvailable := services.Lookup("mail.send")
	for _, line := range readinessReport(a.cfg, mailAvailable, domainsRegistered) {
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
	// cross-site state-changing requests riding the session cookie; bearer/webhook
	// clients (no session cookie) pass through untouched.
	srv, err := server.New(a.cfg, a.gdb, api.CSRFGuard(mux), rootHandler, webFS, staticMounts, server.RuntimeSettings{
		MetricsToken: apiHandler.MetricsToken,
		RateLimits:   apiHandler.RateLimits,
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
		if v, ok := services.Lookup(p.Name() + ".cleanup"); ok {
			if fn, ok := v.(func(context.Context, int)); ok {
				cleanups = append(cleanups, fn)
			}
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
