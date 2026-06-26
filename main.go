// Command led is a single-binary domain / short-link / email management
// service (link · email · domain). It serves an embedded React dashboard,
// a JSON API, and a short-link redirector from one process.
package main

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
	"github.com/jungley/led/internal/server"
	"github.com/jungley/led/internal/shortlink"
	"github.com/jungley/led/webembed"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	gdb, err := db.Open(cfg)
	if err != nil {
		log.Fatalf("db: %v", err)
	}

	cipher := crypto.New(cfg.SecretKey)
	authMgr := auth.New(cfg, cipher).WithDB(gdb)

	geoResolver, err := geo.Open(cfg.GeoIPDB)
	if err != nil {
		log.Printf("geoip disabled: %v", err)
		geoResolver, _ = geo.Open("")
	}
	defer geoResolver.Close()

	short := shortlink.New(gdb, geoResolver)
	apiHandler := api.New(cfg, gdb, cipher, authMgr, geoResolver).Routes()

	webFS, err := webembed.FS()
	if err != nil {
		log.Fatalf("web assets: %v", err)
	}

	srv, err := server.New(cfg, apiHandler, short, webFS)
	if err != nil {
		log.Fatalf("server: %v", err)
	}

	httpSrv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           srv,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("led listening on %s (db=%s)", cfg.Listen, cfg.DBDriver)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	log.Println("shutting down...")
	_ = httpSrv.Shutdown(ctx)
}
