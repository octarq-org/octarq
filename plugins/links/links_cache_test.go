package links

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/internal/cache"
	"github.com/octarq-org/octarq/origin"
	"github.com/octarq-org/octarq/plugin"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestEngine_LookupCaching(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	if err := db.AutoMigrate(&Link{}, &LinkEvent{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	cacheBackend := cache.NewMemoryCache(100)
	scoped := cache.NewScoped(cacheBackend, "links")
	pctx := &plugin.Context{
		Cache:       scoped,
		CacheGet:    cacheBackend.Get,
		CacheSet:    cacheBackend.Set,
		DeleteCache: cacheBackend.Delete,
	}

	resolver := origin.NewResolver(db)
	_ = resolver
	engine := NewEngine(db, pctx)
	defer engine.Close()

	// 1. Create a link
	link := Link{
		OrgID:     1,
		Host:      "",
		Slug:      "test-cache",
		Target:    "https://example.com/target",
		Enabled:   true,
		CreatedAt: time.Now(),
	}
	if err := db.Create(&link).Error; err != nil {
		t.Fatalf("create link: %v", err)
	}

	// 2. First lookup (Cache Miss -> DB)
	l1, ok := engine.Lookup("", "test-cache")
	if !ok || l1 == nil {
		t.Fatalf("expected Lookup to succeed")
	}
	if l1.Target != "https://example.com/target" {
		t.Errorf("expected target https://example.com/target, got %s", l1.Target)
	}

	// Verify cached
	var cached Link
	if !cacheBackend.Get(context.Background(), "link:redirect::test-cache", &cached) {
		t.Fatalf("expected redirect to be cached in backend")
	}
	if cached.Slug != "test-cache" {
		t.Errorf("expected cached slug test-cache, got %s", cached.Slug)
	}

	// 3. Update DB directly and verify cache still returns original until invalidation
	db.Model(&Link{}).Where("id = ?", link.ID).Update("target", "https://example.com/updated")

	l2, ok := engine.Lookup("", "test-cache")
	if !ok || l2.Target != "https://example.com/target" {
		t.Errorf("expected cached target before invalidation, got %s", l2.Target)
	}

	// 4. Invalidate cache
	_ = pctx.DeleteCache(context.Background(), "link:redirect::test-cache")

	// 5. Lookup after cache invalidation (should fetch updated target)
	l3, ok := engine.Lookup("", "test-cache")
	if !ok || l3.Target != "https://example.com/updated" {
		t.Errorf("expected updated target after invalidation, got %s", l3.Target)
	}
}
