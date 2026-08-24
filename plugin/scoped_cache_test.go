package plugin_test

import (
	"context"
	"testing"
	"time"

	"github.com/octarq-org/octarq/internal/cache"
	"github.com/octarq-org/octarq/plugin"
)

func TestScopedCache_Isolation(t *testing.T) {
	ctx := context.Background()
	l1 := cache.NewMemoryScoped()
	l2 := cache.NewMemoryScoped()

	linksCache := plugin.NewScopedCache("links", l1, l2)
	mailCache := plugin.NewScopedCache("mail", l1, l2)

	if err := linksCache.Set(ctx, "item", "links_data", 0); err != nil {
		t.Fatalf("linksCache.Set failed: %v", err)
	}
	if err := mailCache.Set(ctx, "item", "mail_data", 0); err != nil {
		t.Fatalf("mailCache.Set failed: %v", err)
	}

	var linksVal, mailVal string
	found, err := linksCache.Get(ctx, "item", &linksVal)
	if err != nil || !found || linksVal != "links_data" {
		t.Fatalf("linksCache.Get got (%v, %v, %q), want (true, nil, \"links_data\")", found, err, linksVal)
	}

	found, err = mailCache.Get(ctx, "item", &mailVal)
	if err != nil || !found || mailVal != "mail_data" {
		t.Fatalf("mailCache.Get got (%v, %v, %q), want (true, nil, \"mail_data\")", found, err, mailVal)
	}

	// Delete from linksCache only
	if err := linksCache.Delete(ctx, "item"); err != nil {
		t.Fatalf("linksCache.Delete failed: %v", err)
	}

	found, _ = linksCache.Get(ctx, "item", &linksVal)
	if found {
		t.Errorf("expected linksCache item to be deleted, but found it")
	}

	found, err = mailCache.Get(ctx, "item", &mailVal)
	if !found || err != nil || mailVal != "mail_data" {
		t.Errorf("mailCache item should not be affected by linksCache delete, got (%v, %v, %q)", found, err, mailVal)
	}
}

func TestScopedCache_TwoLevel_Fill(t *testing.T) {
	ctx := context.Background()
	l1 := cache.NewMemoryScoped()
	l2 := cache.NewMemoryScoped()

	sc := plugin.NewScopedCache("svc", l1, l2)

	// Pre-populate L2 directly
	if err := l2.Set(ctx, "svc:hot_key", "from_l2", 0); err != nil {
		t.Fatalf("l2.Set failed: %v", err)
	}

	// Verify L1 is empty initially
	var v1 string
	found, _ := l1.Get(ctx, "svc:hot_key", &v1)
	if found {
		t.Fatalf("expected l1 to miss initially")
	}

	// Get via ScopedCache: should find in L2 and backfill L1
	var dest string
	found, err := sc.Get(ctx, "hot_key", &dest)
	if err != nil || !found || dest != "from_l2" {
		t.Fatalf("sc.Get got (%v, %v, %q), want (true, nil, \"from_l2\")", found, err, dest)
	}

	// Verify L1 has been backfilled
	var backfilled string
	found, _ = l1.Get(ctx, "svc:hot_key", &backfilled)
	if !found || backfilled != "from_l2" {
		t.Fatalf("l1 backfill check got (%v, %q), want (true, \"from_l2\")", found, backfilled)
	}

	// Now delete from L2 to ensure subsequent Get is served from L1
	if err := l2.Delete(ctx, "svc:hot_key"); err != nil {
		t.Fatalf("l2.Delete failed: %v", err)
	}

	var dest2 string
	found, err = sc.Get(ctx, "hot_key", &dest2)
	if err != nil || !found || dest2 != "from_l2" {
		t.Fatalf("sc.Get after L2 deletion got (%v, %v, %q), want (true, nil, \"from_l2\")", found, err, dest2)
	}
}

func TestScopedCache_Delete(t *testing.T) {
	ctx := context.Background()
	l1 := cache.NewMemoryScoped()
	l2 := cache.NewMemoryScoped()

	sc := plugin.NewScopedCache("mod", l1, l2)

	if err := sc.Set(ctx, "target", "val123", time.Minute); err != nil {
		t.Fatalf("sc.Set failed: %v", err)
	}

	var v string
	found, err := sc.Get(ctx, "target", &v)
	if err != nil || !found || v != "val123" {
		t.Fatalf("sc.Get got (%v, %v, %q), want (true, nil, \"val123\")", found, err, v)
	}

	if err := sc.Delete(ctx, "target"); err != nil {
		t.Fatalf("sc.Delete failed: %v", err)
	}

	found, _ = sc.Get(ctx, "target", &v)
	if found {
		t.Errorf("sc.Get after Delete returned found=true")
	}

	found, _ = l1.Get(ctx, "mod:target", &v)
	if found {
		t.Errorf("l1.Get after Delete returned found=true")
	}

	found, _ = l2.Get(ctx, "mod:target", &v)
	if found {
		t.Errorf("l2.Get after Delete returned found=true")
	}
}

func TestScopedCache_NilL2(t *testing.T) {
	ctx := context.Background()
	l1 := cache.NewMemoryScoped()

	sc := plugin.NewScopedCache("single", l1, nil)

	if err := sc.Set(ctx, "alpha", "omega", 0); err != nil {
		t.Fatalf("sc.Set failed: %v", err)
	}

	var val string
	found, err := sc.Get(ctx, "alpha", &val)
	if err != nil || !found || val != "omega" {
		t.Fatalf("sc.Get got (%v, %v, %q), want (true, nil, \"omega\")", found, err, val)
	}

	if err := sc.Delete(ctx, "alpha"); err != nil {
		t.Fatalf("sc.Delete failed: %v", err)
	}

	found, _ = sc.Get(ctx, "alpha", &val)
	if found {
		t.Errorf("expected alpha to be deleted from single-tier cache")
	}

	if err := sc.InvalidateTag(ctx, "any_tag"); err != nil {
		t.Errorf("InvalidateTag failed on nil L2: %v", err)
	}
}

func TestScopedCache_InvalidateTag(t *testing.T) {
	ctx := context.Background()
	l1 := cache.NewMemoryScoped()
	l2 := cache.NewMemoryScoped()

	sc := plugin.NewScopedCache("mod", l1, l2)

	if err := sc.InvalidateTag(ctx, "tag1"); err != nil {
		t.Fatalf("InvalidateTag failed: %v", err)
	}
}

func TestTieredCache_KeyAndPrefix(t *testing.T) {
	l1 := cache.NewMemoryScoped()
	c := plugin.NewScopedCache("myplugin", l1, nil)
	if got := c.Prefix(); got != "myplugin" {
		t.Errorf("Prefix = %q want myplugin", got)
	}
	if got := c.Key("foo"); got != "myplugin:foo" {
		t.Errorf("Key = %q want myplugin:foo", got)
	}
	empty := plugin.NewScopedCache("", l1, nil)
	if got := empty.Key("bar"); got != "bar" {
		t.Errorf("empty prefix Key = %q want bar", got)
	}
	if got := empty.Prefix(); got != "" {
		t.Errorf("empty Prefix = %q want empty", got)
	}
}
