package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/internal/cache"
	"github.com/octarq-org/octarq/server/plugin"
)

func TestCacheCharacterizationTenantScoping(t *testing.T) {
	mem := cache.NewMemoryCache(100)
	scoped := cache.NewScoped(mem, "testplugin")

	ctx := plugin.WithOrgID(context.Background(), 42)

	// Set value with tenant context
	err := scoped.Set(ctx, "hello", "world", time.Minute)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Verify tenant-isolated retrieval
	var val string
	found, err := scoped.Get(ctx, "hello", &val)
	if err != nil || !found || val != "world" {
		t.Fatalf("Get with tenant failed: found=%v, val=%q, err=%v", found, val, err)
	}

	// Different tenant must NOT see the key
	ctxOther := plugin.WithOrgID(context.Background(), 99)
	var valOther string
	found, err = scoped.Get(ctxOther, "hello", &valOther)
	if err != nil || found {
		t.Fatalf("cross-tenant leak detected: found=%v, val=%q", found, valOther)
	}
}

func TestCacheCharacterizationMemoryExpiration(t *testing.T) {
	mem := cache.NewMemoryCache(10)
	ctx := context.Background()

	err := mem.Set(ctx, "expiring_key", "expiring_val", 10*time.Millisecond)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	time.Sleep(30 * time.Millisecond)

	var val string
	found := mem.Get(ctx, "expiring_key", &val)
	if found {
		t.Fatal("expected expired key to not be found")
	}
}
