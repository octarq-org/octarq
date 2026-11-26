package api

import (
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/config"
	"github.com/octarq-org/octarq/server/internal/auth"
	"github.com/octarq-org/octarq/server/internal/crypto"
	"github.com/octarq-org/octarq/server/internal/geo"
	"github.com/octarq-org/octarq/server/plugin"
)

func TestLoginLimitersTakeTheConfiguredBudget(t *testing.T) {
	cfg := &config.Config{
		AdminUser:            "admin",
		AdminPassword:        "pw",
		SecretKey:            "secret",
		LoginMaxFails:        7,
		LoginAccountMaxFails: 21,
		LoginWindow:          2 * time.Minute,
	}
	gdb := newTestDB(t)
	cipher := crypto.New(cfg.SecretKey)
	if err := cipher.EnableEnvelope(apiEnvStore{gdb}); err != nil {
		t.Fatalf("EnableEnvelope: %v", err)
	}
	authMgr := auth.New(cfg, cipher).WithDB(gdb)
	geoResolver, _ := geo.Open("")
	handler := New(cfg, gdb, cipher, authMgr, geoResolver, testQueue())
	handler.SetPlugins([]plugin.Plugin{dummyPlugin{}})

	if got := handler.loginLimiter.rate; got.Limit != 7 || got.Period != 2*time.Minute {
		t.Fatalf("login limiter = %d per %v, want 7 per 2m", got.Limit, got.Period)
	}
	if got := handler.loginAccountLimiter.rate; got.Limit != 21 || got.Period != 2*time.Minute {
		t.Fatalf("account limiter = %d per %v, want 21 per 2m", got.Limit, got.Period)
	}
}
