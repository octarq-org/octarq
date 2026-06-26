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
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jungley/led/config"
	"github.com/jungley/led/internal/api"
	"github.com/jungley/led/internal/auth"
	"github.com/jungley/led/internal/crypto"
	"github.com/jungley/led/internal/db"
	"github.com/jungley/led/internal/geo"
	"github.com/jungley/led/internal/notify"
	"github.com/jungley/led/internal/server"
	"github.com/jungley/led/internal/shortlink"
	"github.com/jungley/led/internal/vpschecker"
	"github.com/jungley/led/plugin"
	"github.com/jungley/led/webembed"
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

// Use registers a plugin. All plugins must be registered before Run so their
// models are migrated and their routes mounted.
func (a *App) Use(p plugin.Plugin) { a.plugins = append(a.plugins, p) }

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

	// 2. Core API mux, then let plugins mount their own routes onto it.
	mux := api.New(a.cfg, a.gdb, a.cipher, a.auth, a.geo).Routes()
	pctx := &plugin.Context{DB: a.gdb, Guard: a.auth.Require, Notify: notify.Send}
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

	checker := vpschecker.New(a.gdb, a.cipher)
	go checker.Start(ctx)

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
