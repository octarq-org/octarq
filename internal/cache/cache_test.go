package cache

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/textproto"
	"strings"
	"testing"
	"time"
)

func TestNew_Noop(t *testing.T) {
	c := New("")
	if c.IsRedis() {
		t.Errorf("expected NoopCache to return false for IsRedis")
	}

	var dst string
	if c.Get(context.Background(), "key", &dst) {
		t.Errorf("NoopCache.Get expected false")
	}

	if err := c.Set(context.Background(), "key", "val", 0); err != nil {
		t.Errorf("NoopCache.Set expected nil, got %v", err)
	}

	if err := c.Delete(context.Background(), "key"); err != nil {
		t.Errorf("NoopCache.Delete expected nil, got %v", err)
	}
}

func TestNew_InvalidURL(t *testing.T) {
	c := New("::invalid-url::")
	if c.IsRedis() {
		t.Errorf("expected fallback NoopCache to return false for IsRedis")
	}
}

func TestRedisCache_Unreachable(t *testing.T) {
	c := New("redis://127.0.0.1:58999/0")
	if !c.IsRedis() {
		t.Fatalf("expected RedisCache")
	}

	var dst string
	if c.Get(context.Background(), "k", &dst) {
		t.Errorf("Get on unreachable redis expected false")
	}

	if err := c.Set(context.Background(), "k", "v", time.Second); err == nil {
		t.Errorf("Set on unreachable redis expected error")
	}

	if err := c.Delete(context.Background(), "k"); err == nil {
		t.Errorf("Delete on unreachable redis expected error")
	}

	// JSON marshal error
	if err := c.Set(context.Background(), "k", make(chan int), time.Second); err == nil {
		t.Errorf("Set with unmarshalable value expected error")
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
			// Array header in RESP, read line by line
			var numElements int
			_, _ = fmt.Sscanf(line, "*%d", &numElements)
			cmd := ""
			var args []string
			for i := 0; i < numElements; i++ {
				_, _ = r.ReadLine() // length header $N
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
					// Return non-JSON string
					_ = w.PrintfLine("$7\r\nnotjson")
				} else if len(args) > 0 && args[0] == "goodjson" {
					// Return JSON string `"hello"`
					_ = w.PrintfLine("$7\r\n\"hello\"")
				} else {
					// Key not found
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

	// 1. Get non-existent key
	var val string
	if c.Get(ctx, "missing", &val) {
		t.Errorf("Get missing key expected false")
	}

	// 2. Set key
	if err := c.Set(ctx, "k", "hello", time.Minute); err != nil {
		t.Errorf("Set expected nil, got %v", err)
	}

	// 3. Get good json
	if !c.Get(ctx, "goodjson", &val) || val != "hello" {
		t.Errorf("Get goodjson expected true and 'hello', got %v, %q", c.Get(ctx, "goodjson", &val), val)
	}

	// 4. Get bad json (should fail unmarshal)
	if c.Get(ctx, "badjson", &val) {
		t.Errorf("Get badjson expected false due to unmarshal error")
	}

	// 5. Delete key
	if err := c.Delete(ctx, "k"); err != nil {
		t.Errorf("Delete expected nil, got %v", err)
	}
}
