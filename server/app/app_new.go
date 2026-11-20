package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/octarq-org/octarq/server/config"
	"github.com/octarq-org/octarq/server/internal/auth"
	"github.com/octarq-org/octarq/server/internal/buildinfo"
	"github.com/octarq-org/octarq/server/internal/cache"
	"github.com/octarq-org/octarq/server/internal/crypto"
	"github.com/octarq-org/octarq/server/internal/db"
	"github.com/octarq-org/octarq/server/internal/eventbus"
	"github.com/octarq-org/octarq/server/internal/geo"
	"github.com/octarq-org/octarq/server/pkg/telemetry"
	"github.com/octarq-org/octarq/server/plugin/safehttp"
)

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
	safehttp.SetAllowPrivateWebhooks(cfg.AllowPrivateWebhooks)
	safehttp.SetAllowPrivateSMTP(cfg.AllowPrivateSMTP)

	cipher := crypto.New(cfg.SecretKey)
	cacheLayer := cache.New(cfg.RedisURL)
	authMgr := auth.New(cfg, cipher).WithDB(gdb).WithCache(cacheLayer)

	geoResolver, err := geo.Open(cfg.GeoIPDB)
	if err != nil {
		return nil, fmt.Errorf("open geoip database %q: %w", cfg.GeoIPDB, err)
	}

	telCfg := telemetry.ConfigFromEnv(config.DefaultAppName, buildinfo.Get().Version)
	tel, err := telemetry.Init(context.Background(), telCfg)
	if err != nil {
		slog.Warn("telemetry init failed", "err", err)
	}

	a := &App{
		cfg:       cfg,
		gdb:       gdb,
		cipher:    cipher,
		auth:      authMgr,
		geo:       geoResolver,
		telemetry: tel,
	}
	return a, nil
}
