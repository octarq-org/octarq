package cache

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/textproto"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNew_MemoryDefault(t *testing.T) {
	c := New("")
	if c.IsRedis() {
		t.Errorf("expected MemoryCache to return false for IsRedis")
	}

	ctx := context.Background()
	var dst string
	if c.Get(ctx, "key", &dst) {
		t.Errorf("expected Get missing key to return false")
	}

	if err := c.Set(ctx, "key", "val", 0); err != nil {
		t.Errorf("expected Set to succeed, got %v", err)
	}

	if !c.Get(ctx, "key", &dst) || dst != "val" {
		t.Errorf("expected Get to find 'val', got %v, %q", c.Get(ctx, "key", &dst), dst)
	}

	if err := c.Delete(ctx, "key"); err != nil {
		t.Errorf("expected Delete to succeed, got %v", err)
	}

	if c.Get(ctx, "key", &dst) {
		t.Errorf("expected key to be deleted")
	}
}

func TestNew_InvalidURL(t *testing.T) {
	c := New("::invalid-url::")
	if c.IsRedis() {
		t.Errorf("expected fallback MemoryCache to return false for IsRedis")
	}
}

func TestMemoryCache_TTL(t *testing.T) {
	c := NewMemoryCache(100)
	ctx := context.Background()

	_ = c.Set(ctx, "short", "temp", 50*time.Millisecond)
	var dst string
	if !c.Get(ctx, "short", &dst) || dst != "temp" {
		t.Fatalf("expected to get unexpired key")
	}

	time.Sleep(70 * time.Millisecond)
	if c.Get(ctx, "short", &dst) {
		t.Fatalf("expected key to be expired")
	}
}

func TestMemoryCache_Capacity(t *testing.T) {
	c := NewMemoryCache(3)
	ctx := context.Background()

	_ = c.Set(ctx, "k1", "v1", 0)
	_ = c.Set(ctx, "k2", "v2", 0)
	_ = c.Set(ctx, "k3", "v3", 0)
	_ = c.Set(ctx, "k4", "v4", 0) // Should evict one

	var dst string
	count := 0
	for _, k := range []string{"k1", "k2", "k3", "k4"} {
		if c.Get(ctx, k, &dst) {
			count++
		}
	}
	if count > 3 {
		t.Errorf("expected at most 3 keys, found %d", count)
	}
}

func TestScopedCache_PrefixIsolation(t *testing.T) {
	backend := NewMemoryCache(100)
	ctx := context.Background()

	linksCache := NewScoped(backend, "links")
	mailCache := NewScoped(backend, "mail")

	_ = linksCache.Set(ctx, "item", "links_data", 0)
	_ = mailCache.Set(ctx, "item", "mail_data", 0)

	var linksVal, mailVal string
	found, err := linksCache.Get(ctx, "item", &linksVal)
	if !found || err != nil || linksVal != "links_data" {
		t.Errorf("linksCache got %v, %v, %q", found, err, linksVal)
	}

	found, err = mailCache.Get(ctx, "item", &mailVal)
	if !found || err != nil || mailVal != "mail_data" {
		t.Errorf("mailCache got %v, %v, %q", found, err, mailVal)
	}

	_ = linksCache.Delete(ctx, "item")
	found, _ = linksCache.Get(ctx, "item", &linksVal)
	if found {
		t.Errorf("expected linksCache item to be deleted")
	}

	found, _ = mailCache.Get(ctx, "item", &mailVal)
	if !found || mailVal != "mail_data" {
		t.Errorf("mailCache item should not have been affected by linksCache delete")
	}
}

func TestScopedCache_Concurrency(t *testing.T) {
	backend := NewMemoryCache(1000)
	sc := NewScoped(backend, "test")
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := fmt.Sprintf("k%d", id%5)
			_ = sc.Set(ctx, key, id, time.Minute)
			var v int
			_, _ = sc.Get(ctx, key, &v)
			if id%3 == 0 {
				_ = sc.Delete(ctx, key)
			}
		}(i)
	}
	wg.Wait()
}

func TestRedisCache_Unreachable(t *testing.T) {
	c := New("redis://127.0.0.1:58999/0")
	// Since connection fails, it falls back to memory cache
	if c.IsRedis() {
		t.Fatalf("expected fallback to MemoryCache when unreachable")
	}

	var dst string
	if c.Get(context.Background(), "k", &dst) {
		t.Errorf("Get on empty cache expected false")
	}
}

// Start a minimal fake Redis TCP server for testing RedisCache paths.
func startFakeRedis(t *testing.T) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go handleFakeRedisConn(conn)
		}
	}()

	return fmt.Sprintf("redis://%s/0", ln.Addr().String()), func() {
		_ = ln.Close()
		<-done
	}
}

func handleFakeRedisConn(conn net.Conn) {
	defer conn.Close()
	r := textproto.NewReader(bufio.NewReader(conn))
	w := textproto.NewWriter(bufio.NewWriter(conn))

	for {
		line, err := r.ReadLine()
		if err != nil {
			return
		}
		if strings.HasPrefix(line, "*") {
			var numElements int
			_, _ = fmt.Sscanf(line, "*%d", &numElements)
			cmd := ""
			var args []string
			for i := 0; i < numElements; i++ {
				_, _ = r.ReadLine()
				arg, _ := r.ReadLine()
				if i == 0 {
					cmd = strings.ToUpper(arg)
				} else {
					args = append(args, arg)
				}
			}
			switch cmd {
			case "HELLO":
				_ = w.PrintfLine("%%0")
			case "PING":
				_ = w.PrintfLine("+PONG")
			case "SET":
				_ = w.PrintfLine("+OK")
			case "DEL":
				_ = w.PrintfLine(":1")
			case "GET":
				if len(args) > 0 && args[0] == "badjson" {
					_ = w.PrintfLine("$7\r\nnotjson")
				} else if len(args) > 0 && args[0] == "goodjson" {
					_ = w.PrintfLine("$7\r\n\"hello\"")
				} else {
					_ = w.PrintfLine("$-1")
				}
			default:
				_ = w.PrintfLine("+OK")
			}
		}
	}
}

func TestRedisCache_WithFakeServer(t *testing.T) {
	redisURL, cleanup := startFakeRedis(t)
	defer cleanup()

	c := New(redisURL)
	if !c.IsRedis() {
		t.Fatalf("expected RedisCache")
	}

	ctx := context.Background()

	var val string
	if c.Get(ctx, "missing", &val) {
		t.Errorf("Get missing key expected false")
	}

	if err := c.Set(ctx, "k", "hello", time.Minute); err != nil {
		t.Errorf("Set expected nil, got %v", err)
	}

	if !c.Get(ctx, "goodjson", &val) || val != "hello" {
		t.Errorf("Get goodjson expected true and 'hello', got %v, %q", c.Get(ctx, "goodjson", &val), val)
	}

	if c.Get(ctx, "badjson", &val) {
		t.Errorf("Get badjson expected false due to unmarshal error")
	}

	if err := c.Delete(ctx, "k"); err != nil {
		t.Errorf("Delete expected nil, got %v", err)
	}

	unmarshallable := make(chan int)
	if err := c.Set(ctx, "chan", unmarshallable, time.Minute); err == nil {
		t.Errorf("expected Set with unmarshallable value on RedisCache to error")
	}
}

func TestNoopCache(t *testing.T) {
	t.Parallel()
	var nc NoopCache
	ctx := context.Background()
	if nc.IsRedis() {
		t.Errorf("expected NoopCache.IsRedis to be false")
	}
	var dst string
	if nc.Get(ctx, "any", &dst) {
		t.Errorf("expected NoopCache.Get to be false")
	}
	if err := nc.Set(ctx, "any", "val", time.Hour); err != nil {
		t.Errorf("expected NoopCache.Set to be nil, got %v", err)
	}
	if err := nc.Delete(ctx, "any"); err != nil {
		t.Errorf("expected NoopCache.Delete to be nil, got %v", err)
	}
}

func TestNewMemory_And_NewMemoryScoped(t *testing.T) {
	t.Parallel()
	mem := NewMemory()
	if mem == nil || mem.IsRedis() {
		t.Errorf("expected valid non-redis MemoryCache")
	}

	scoped := NewMemoryScoped()
	if scoped == nil {
		t.Fatalf("expected valid ScopedCache")
	}

	ctx := context.Background()
	_ = scoped.Set(ctx, "item", "val", time.Minute)
	var dst string
	ok, err := scoped.Get(ctx, "item", &dst)
	if !ok || err != nil || dst != "val" {
		t.Errorf("expected item=val, got ok=%v, err=%v, dst=%q", ok, err, dst)
	}
}

func TestScopedCache_InvalidateTag_And_NilBackend(t *testing.T) {
	t.Parallel()
	backend := NewMemoryCache(100)
	sc := NewScoped(backend, "test-scope")
	ctx := context.Background()

	// Tag invalidation when no tags registered
	if err := sc.InvalidateTag(ctx, "nonexistent"); err != nil {
		t.Errorf("expected nil error on nonexistent tag, got %v", err)
	}

	// Tag invalidation with populated tag
	if concrete, ok := sc.(*ScopedCache); ok {
		concrete.mu.Lock()
		concrete.tags["my-tag"] = map[string]struct{}{"test-scope:key1": {}, "test-scope:key2": {}}
		concrete.mu.Unlock()
		_ = backend.Set(ctx, "test-scope:key1", "val1", time.Minute)
		_ = backend.Set(ctx, "test-scope:key2", "val2", time.Minute)

		if err := sc.InvalidateTag(ctx, "my-tag"); err != nil {
			t.Errorf("expected InvalidateTag to succeed, got %v", err)
		}
		var dst string
		if backend.Get(ctx, "test-scope:key1", &dst) || backend.Get(ctx, "test-scope:key2", &dst) {
			t.Errorf("expected tagged keys to be invalidated")
		}
	}

	// Nil backend checks
	nilScoped := NewScoped(nil, "nil-scope")
	var dst string
	ok, err := nilScoped.Get(ctx, "k", &dst)
	if ok || err != nil {
		t.Errorf("nil backend Get expected false, nil; got %v, %v", ok, err)
	}
	if err := nilScoped.Set(ctx, "k", "v", time.Minute); err != nil {
		t.Errorf("nil backend Set expected nil, got %v", err)
	}
	if err := nilScoped.Delete(ctx, "k"); err != nil {
		t.Errorf("nil backend Delete expected nil, got %v", err)
	}
	if err := nilScoped.InvalidateTag(ctx, "tag1"); err != nil {
		t.Errorf("nil backend InvalidateTag expected nil, got %v", err)
	}
}

func TestMemoryCache_EdgeCases(t *testing.T) {
	t.Parallel()
	mc := NewMemoryCache(0) // should fallback to 10000
	ctx := context.Background()

	// Test dst == nil
	_ = mc.Set(ctx, "key1", "val1", time.Minute)
	if !mc.Get(ctx, "key1", nil) {
		t.Errorf("expected Get with dst=nil to return true for existing key")
	}

	// Test unmarshal failure
	var intDst int
	if mc.Get(ctx, "key1", &intDst) {
		t.Errorf("expected unmarshal failure to return false")
	}

	// Test Set marshal error
	unmarshallable := make(chan int)
	if err := mc.Set(ctx, "chan", unmarshallable, time.Minute); err == nil {
		t.Errorf("expected Set with unmarshallable value to return error")
	}
}
