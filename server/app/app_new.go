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
	"github.com/octarq-org/octarq/server/internal/notification"
	"github.com/octarq-org/octarq/server/pkg/telemetry"
	"github.com/octarq-org/octarq/server/plugin/safehttp"
)

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
	notification.SetConfigDecryptor(func(stored string) (string, bool) {
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
	// Composition is the caller's job: New() mounts no feature plugins. Each
	// entry point (octarq/main.go, a downstream distribution's main, a trimmed edition) Uses the
	// plugins it wants — Core plugins the same way extra plugins are added, via
	// a.Use. The OSS default set is plugins/builtin.Default(); see
	// website/src/content/docs/architecture/overview.md.
	return a, nil
}
