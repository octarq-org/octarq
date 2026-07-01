// Package app is the public composition root for led. It wires config, the
// database, auth, the core API, the short-link redirector and the embedded
// dashboard into one HTTP server — and lets external (Pro) modules extend it
// through the plugin package without forking.
//
// This is the importable seam of the Core-as-Library split: the open-core
// binary (cmd in this repo) calls New().Run() with no plugins; the private
// led-core consumer calls Use() for each Pro plugin before Run().
//
// AutoMigrate timing: New() opens the database but does NOT migrate. Run()
// collects core models plus every registered plugin's Models() and migrates
// them together, exactly once, before any request is served.
package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Jungley8/led/config"
	"github.com/Jungley8/led/internal/api"
	"github.com/Jungley8/led/internal/auth"
	"github.com/Jungley8/led/internal/cleanup"
	"github.com/Jungley8/led/internal/crypto"
	"github.com/Jungley8/led/internal/db"
	"github.com/Jungley8/led/internal/eventbus"
	"github.com/Jungley8/led/internal/geo"
	"github.com/Jungley8/led/internal/mail"
	"github.com/Jungley8/led/internal/mcp"
	"github.com/Jungley8/led/internal/models"
	"github.com/Jungley8/led/internal/notify"
	"github.com/Jungley8/led/internal/server"
	"github.com/Jungley8/led/internal/shortlink"
	"github.com/Jungley8/led/plugin"
	"github.com/Jungley8/led/webembed"
	"gorm.io/gorm"
)

// App holds the wired core dependencies and any registered plugins.
type App struct {
	cfg     *config.Config
	gdb     *gorm.DB
	cipher  *crypto.Cipher
	auth    *auth.Manager
	geo     *geo.Resolver
	plugins []plugin.Plugin
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

	cipher := crypto.New(cfg.SecretKey)
	authMgr := auth.New(cfg, cipher).WithDB(gdb)

	geoResolver, err := geo.Open(cfg.GeoIPDB)
	if err != nil {
		log.Printf("geoip disabled: %v", err)
		geoResolver, _ = geo.Open("")
	}

	return &App{
		cfg:    cfg,
		gdb:    gdb,
		cipher: cipher,
		auth:   authMgr,
		geo:    geoResolver,
	}, nil
}

// DB exposes the shared database handle (useful for plugin construction).
func (a *App) DB() *gorm.DB { return a.gdb }

// sendMail is the implementation behind plugin.Context.SendMail. It resolves the
// org's first configured SMTP sender, decrypts its password, and relays the
// message — mirroring internal/api.Handler.sendEmail so plugins can send
// transactional mail without importing led's internal packages.
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

// RunMCP runs the MCP server with the registered plugins over stdio. It mounts
// the plugins with the same plugin.Context the HTTP server uses, so their MCP
// tool handlers have their dependencies wired (DB, DNS manager, Encrypt/Decrypt)
// — without this, plugin MCP tools would run with nil dependencies. The HTTP mux
// they mount onto is discarded; only the captured context matters here.
func (a *App) RunMCP(ctx context.Context) error {
	var extra []any
	for _, p := range a.plugins {
		extra = append(extra, p.Models()...)
	}
	if err := db.Migrate(a.gdb, extra...); err != nil {
		return err
	}
	if err := a.cipher.EnableEnvelope(settingsStore{a.gdb}, a.cfg.OldSecretKeys...); err != nil {
		return err
	}

	apiHandler := api.New(a.cfg, a.gdb, a.cipher, a.auth, a.geo)
	apiHandler.SetPlugins(a.plugins)
	pctx := &plugin.Context{
		DB:       a.gdb,
		Guard:    a.auth.Require,
		Notify:   notify.Send,
		UserID:   a.auth.UserID,
		OrgID:    a.auth.OrgID,
		Audit:    apiHandler.Audit,
		Encrypt:  a.cipher.Encrypt,
		Decrypt:  a.cipher.Decrypt,
		OnEmail:  apiHandler.OnEmail,
		DNS:      apiHandler.DNSManager(),
		SendMail: a.sendMail,
	}
	throwaway := http.NewServeMux()
	for _, p := range a.plugins {
		p.Mount(throwaway, pctx)
	}

	return mcp.RunWithPlugins(ctx, a.plugins)
}

// Run migrates the schema (core + plugin models), builds the HTTP server, and
// serves until interrupted.
func (a *App) Run(ctx context.Context) error {
	defer a.geo.Close()

	// 1. Migrate AFTER every plugin is registered, so plugin models join the
	//    core schema in a single AutoMigrate pass.
	var extra []any
	for _, p := range a.plugins {
		extra = append(extra, p.Models()...)
	}
	if err := db.Migrate(a.gdb, extra...); err != nil {
		return err
	}

	// 1b. Upgrade the cipher to envelope mode now that the settings table exists
	//     (loads or creates the DEK; re-wraps it under a rotated key if needed).
	if err := a.cipher.EnableEnvelope(settingsStore{a.gdb}, a.cfg.OldSecretKeys...); err != nil {
		return err
	}

	// 2. Core API mux, then let plugins mount their own routes onto it.
	auth.InitGothStore(a.cfg.SecretKey)
	apiHandler := api.New(a.cfg, a.gdb, a.cipher, a.auth, a.geo)
	apiHandler.SetPlugins(a.plugins)
	mux := apiHandler.Routes()
	pctx := &plugin.Context{
		DB:       a.gdb,
		Guard:    a.auth.Require,
		Notify:   notify.Send,
		UserID:   a.auth.UserID,
		OrgID:    a.auth.OrgID,
		Audit:    apiHandler.Audit,
		Encrypt:  a.cipher.Encrypt,
		Decrypt:  a.cipher.Decrypt,
		OnEmail:  apiHandler.OnEmail,
		DNS:      apiHandler.DNSManager(),
		SendMail: a.sendMail,
	}
	for _, p := range a.plugins {
		p.Mount(mux, pctx)
		log.Printf("plugin mounted: %s", p.Name())
		if s, ok := p.(plugin.Starter); ok {
			go s.Start(ctx)
		}
	}

	short := shortlink.New(a.gdb, a.geo)
	webFS, err := webembed.FS()
	if err != nil {
		return err
	}
	srv, err := server.New(a.cfg, mux, short, webFS)
	if err != nil {
		return err
	}

	httpSrv := &http.Server{
		Addr:              a.cfg.Listen,
		Handler:           srv,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go cleanup.Start(ctx, a.gdb, apiHandler.DataRetentionDays)

	go func() {
		log.Printf("led listening on %s (db=%s)", a.cfg.Listen, a.cfg.DBDriver)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
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
	log.Println("shutting down...")
	return httpSrv.Shutdown(shutCtx)
}
