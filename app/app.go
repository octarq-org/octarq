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
	"strings"
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
	"github.com/octarq-org/octarq/internal/mail"
	"github.com/octarq-org/octarq/internal/mcp"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/internal/notify"
	"github.com/octarq-org/octarq/internal/queue"
	"github.com/octarq-org/octarq/internal/safehttp"
	"github.com/octarq-org/octarq/internal/server"
	"github.com/octarq-org/octarq/internal/shortlink"
	"github.com/octarq-org/octarq/plugin"
	"github.com/octarq-org/octarq/plugins/dns"
	"github.com/octarq-org/octarq/webembed"
	"gorm.io/gorm"
)

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
		h.ServeHTTP(w, r)
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
		handler(ctx)
	})
}

// App holds the wired core dependencies and any registered plugins.
type App struct {
	cfg     *config.Config
	gdb     *gorm.DB
	cipher  *crypto.Cipher
	auth    *auth.Manager
	geo     *geo.Resolver
	plugins []plugin.Plugin
	webFS   fs.FS // overrides the embedded OSS dashboard when set (see WithWebFS)
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

	// Secure session cookies over a non-HTTPS base URL are silently dropped by
	// browsers — a common "login works but every next request is 401" trap in
	// local/HTTP setups. Warn loudly with the escape hatch.
	if cfg.SecureCookies && !strings.HasPrefix(strings.ToLower(cfg.BaseURL), "https://") {
		slog.Warn("secure cookies are on but the base URL is not https — browsers will drop the session cookie over plain HTTP; set OCTARQ_SECURE_COOKIES=false for local HTTP development")
	}

	gdb, err := db.Open(cfg)
	if err != nil {
		return nil, err
	}

	eventbus.Init(gdb)
	// Opt webhook/notification delivery into private targets only when the operator
	// has explicitly allowed it (trusted internal receivers); default stays strict.
	safehttp.SetAllowPrivateWebhooks(cfg.AllowPrivateWebhooks)

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
	// Built-in Core features, extracted from the monolithic API handler into
	// default-on plugins (docs/CORE-PLUGIN-EXTRACTION.md). They mount ungated like
	// any Core plugin, so every binary — open-core and Pro — gets them.
	a.Use(dns.New())
	return a, nil
}

// DB exposes the shared database handle (useful for plugin construction).
func (a *App) DB() *gorm.DB { return a.gdb }

// Notify delivers a notification via a configured channel type ("telegram", "webhook").
func (a *App) Notify(ctx context.Context, typ, cfgJSON, text string) error {
	return notify.Send(ctx, typ, cfgJSON, text)
}

// sendMail is the implementation behind plugin.Context.SendMail. It resolves the
// org's first configured SMTP sender, decrypts its password, and relays the
// message — mirroring internal/api.Handler.sendEmail so plugins can send
// transactional mail without importing octarq's internal packages.
func (a *App) sendMail(orgID uint, to, subject, htmlBody, textBody string) error {
	var s models.SMTPSender
	if err := a.gdb.Where("owner_id = ? ", orgID).Order("id").First(&s).Error; err != nil {
		return fmt.Errorf("no SMTP sender configured for org %d", orgID)
	}
	pass, err := a.cipher.Decrypt(s.Pass)
	if err != nil {
		return err
	}
	sender := mail.NewCustomSender(s.Host, fmt.Sprint(s.Port), s.User, string(pass), s.FromEmail)
	return sender.Send(mail.Message{From: s.FromEmail, To: []string{to}, Subject: subject, HTML: htmlBody, Text: textBody})
}

// Use registers a plugin. All plugins must be registered before Run so their
// models are migrated and their routes mounted.
func (a *App) Use(p plugin.Plugin) { a.plugins = append(a.plugins, p) }

// lazyDNSManager resolves the plugin.DNSManager that the dns Core plugin
// provides under dns.ServiceDNSManager, on each call. ctx.DNS used to be the API
// Handler's own manager; with domain/DNS extracted into the dns plugin
// (docs/CORE-PLUGIN-EXTRACTION.md) the plugin provides the seam during Mount and
// consumers (Pro infra / ai MCP tools) resolve it here at request time — after
// all plugins have mounted, so the lookup always succeeds in practice.
type lazyDNSManager struct {
	lookup func(name string) (any, bool)
}

var _ plugin.DNSManager = (*lazyDNSManager)(nil)

func (l *lazyDNSManager) resolve() (plugin.DNSManager, error) {
	if v, ok := l.lookup(dns.ServiceDNSManager); ok {
		if m, ok := v.(plugin.DNSManager); ok {
			return m, nil
		}
	}
	return nil, errors.New("dns manager unavailable: the dns plugin is not mounted")
}

func (l *lazyDNSManager) List(ctx context.Context, domainID uint) ([]plugin.DNSRecord, error) {
	m, err := l.resolve()
	if err != nil {
		return nil, err
	}
	return m.List(ctx, domainID)
}

func (l *lazyDNSManager) Set(ctx context.Context, domainID uint, r plugin.DNSRecord) (plugin.DNSRecord, error) {
	m, err := l.resolve()
	if err != nil {
		return plugin.DNSRecord{}, err
	}
	return m.Set(ctx, domainID, r)
}

func (l *lazyDNSManager) Delete(ctx context.Context, domainID uint, recordID string) error {
	m, err := l.resolve()
	if err != nil {
		return err
	}
	return m.Delete(ctx, domainID, recordID)
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
	pctx := &plugin.Context{
		Huma:                apiHandler.Huma(),
		DB:                  a.gdb,
		Guard:               a.auth.Require,
		Notify:              notify.Send,
		UserID:              a.auth.UserID,
		OrgID:               a.auth.OrgID,
		Audit:               apiHandler.Audit,
		Encrypt:             a.cipher.Encrypt,
		Decrypt:             a.cipher.Decrypt,
		OnEmail:             apiHandler.OnEmail,
		DNS:                 &lazyDNSManager{lookup: services.Lookup},
		SendMail:            a.sendMail,
		SetLLMResolver:      apiHandler.SetLLMResolver,
		GetWorkspaceSetting: apiHandler.GetWorkspaceSetting,
		SetWorkspaceSetting: apiHandler.SetWorkspaceSetting,
		Provide:             services.Provide,
		Lookup:              services.Lookup,
	}
	enabled := func(r *http.Request, featureKey string) (allowed, scoped bool) {
		oid := a.auth.OrgID(r)
		if oid == 0 {
			return false, false
		}
		return apiHandler.PluginEnabled(oid, featureKey), true
	}
	for _, p := range a.plugins {
		pctxCopy := *pctx
		if plugin.Describe(p).Core {
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

	return mcp.RunWithPlugins(ctx, a.plugins)
}

// Run migrates the schema (core + plugin models), builds the HTTP server, and
// serves until interrupted.
func (a *App) Run(ctx context.Context) error {
	defer a.geo.Close()

	// 1. Migrate AFTER every plugin is registered, so plugin models join the
	//    core schema in a single AutoMigrate pass. First refuse startup if two
	//    different plugin model types would fight over the same table.
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
	auth.InitGothStore(a.cfg.SecretKey, a.cfg.SecureCookies)
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
	pctx := &plugin.Context{
		Huma:           apiHandler.Huma(),
		DB:             a.gdb,
		Guard:          a.auth.Require,
		Notify:         notify.Send,
		UserID:         a.auth.UserID,
		OrgID:          a.auth.OrgID,
		Audit:          apiHandler.Audit,
		Encrypt:        a.cipher.Encrypt,
		Decrypt:        a.cipher.Decrypt,
		OnEmail:        apiHandler.OnEmail,
		DNS:            &lazyDNSManager{lookup: services.Lookup},
		SendMail:       a.sendMail,
		SetLLMResolver: apiHandler.SetLLMResolver,
		Provide:        services.Provide,
		Lookup:         services.Lookup,
	}
	// Non-core plugin routes are gated by a per-workspace feature toggle: when the
	// caller's workspace has the feature disabled, the app answers 404 before the
	// handler runs. Core plumbing (license activation, buyer identity) mounts
	// ungated — it must always work.
	enabled := func(r *http.Request, featureKey string) (allowed, scoped bool) {
		oid := a.auth.OrgID(r)
		if oid == 0 {
			return false, false // no workspace in session (webhooks, portal) → not gated
		}
		return apiHandler.PluginEnabled(oid, featureKey), true
	}
	for _, p := range a.plugins {
		pctxCopy := *pctx
		if plugin.Describe(p).Core {
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
	for _, p := range a.plugins {
		if s, ok := p.(plugin.Starter); ok {
			go s.Start(ctx)
		}
	}

	shortlink.SetTrustProxy(a.cfg.TrustProxy)
	short := shortlink.New(a.gdb, a.geo).WithCache(a.auth.Cache())
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
	srv, err := server.New(a.cfg, api.CSRFGuard(mux), short, webFS, server.RuntimeSettings{
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

	go cleanup.Start(ctx, a.gdb, apiHandler.DataRetentionDays)
	go cleanup.StartSessionCleanup(ctx, a.gdb)

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
