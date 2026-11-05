package api

import (
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/config"
	"github.com/octarq-org/octarq/server/internal/auth"
	"github.com/octarq-org/octarq/server/internal/crypto"
	"github.com/octarq-org/octarq/server/internal/geo"
	"github.com/octarq-org/octarq/server/internal/queue"
)

// TestHandlerReportsAnUnreachableCounterStore pins the boot-time refusal: a
// deployment that configures Redis and cannot reach it learns at startup,
// instead of quietly enforcing every budget per process.
func TestHandlerReportsAnUnreachableCounterStore(t *testing.T) {
	db := newTestDB(t)

	broken := &config.Config{AdminUser: "admin", AdminPassword: "pw", SecretKey: "secret", RedisURL: "redis://127.0.0.1:65534"}
	cipher := crypto.New(broken.SecretKey)
	if err := cipher.EnableEnvelope(apiEnvStore{db}); err != nil {
		t.Fatalf("EnableEnvelope: %v", err)
	}
	h := New(broken, db, cipher, auth.New(broken, cipher).WithDB(db), mustGeo(t), queue.New(""))
	err := h.InitErr()
	if err == nil {
		t.Fatal("unreachable Redis: want an init error, got nil")
	}
	if !strings.Contains(err.Error(), "Redis") {
		t.Fatalf("unreachable Redis: error must name the cause, got %q", err)
	}
	if h.loginLimiter != nil {
		t.Fatal("unreachable Redis: the limiter must not be built")
	}

	healthy := &config.Config{AdminUser: "admin", AdminPassword: "pw", SecretKey: "secret"}
	if err := New(healthy, db, cipher, auth.New(healthy, cipher).WithDB(db), mustGeo(t), queue.New("")).InitErr(); err != nil {
		t.Fatalf("no Redis configured: want nil, got %v", err)
	}
}

func mustGeo(t *testing.T) *geo.Resolver {
	t.Helper()
	g, err := geo.Open("")
	if err != nil {
		t.Fatalf("geo.Open: %v", err)
	}
	return g
}
