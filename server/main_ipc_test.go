package main

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/internal/monitor"
)

func TestNewIPCDaemonMetricsUnknownWithoutCollector(t *testing.T) {
	d := newIPCDaemon("test", func() *monitor.Collector { return nil })
	rep, err := d.OnMetrics(context.Background())
	if err != nil {
		t.Fatalf("metrics: %v", err)
	}
	if rep.Overall != "unknown" {
		t.Errorf("overall = %q, want unknown", rep.Overall)
	}
}

func TestNewIPCDaemonReloadAppliesLogLevel(t *testing.T) {
	t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
	t.Setenv("OCTARQ_DB_DSN", filepath.Join(t.TempDir(), "ipc_reload.db"))
	t.Setenv("OCTARQ_SECRET_KEY", "ipc-reload-secret-1234567890")
	t.Setenv("OCTARQ_ADMIN_PASSWORD", "ipc-reload-admin-pass")
	t.Setenv("OCTARQ_LOG_LEVEL", "debug")

	d := newIPCDaemon("test", func() *monitor.Collector { return nil })
	applied, err := d.OnReload(context.Background())
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(applied) != 1 || applied[0] != "log.level" {
		t.Errorf("applied = %v, want [log.level]", applied)
	}
}

func TestNewIPCDaemonReloadFailsClosed(t *testing.T) {
	t.Setenv("OCTARQ_DB_DRIVER", "oracle")
	d := newIPCDaemon("test", func() *monitor.Collector { return nil })
	if _, err := d.OnReload(context.Background()); err == nil {
		t.Error("bad config must fail reload")
	}
}

func TestApplyServerFlags(t *testing.T) {
	t.Setenv("OCTARQ_LISTEN", ":8080")
	if err := applyServerFlags("9090", "127.0.0.1", ""); err != nil {
		t.Fatalf("apply flags: %v", err)
	}
	if got := os.Getenv("OCTARQ_LISTEN"); got != "127.0.0.1:9090" {
		t.Errorf("OCTARQ_LISTEN = %q, want 127.0.0.1:9090", got)
	}

	t.Setenv("OCTARQ_LISTEN", ":8080")
	if err := applyServerFlags("", "", ""); err != nil {
		t.Fatalf("no flags: %v", err)
	}
	if got := os.Getenv("OCTARQ_LISTEN"); got != ":8080" {
		t.Errorf("OCTARQ_LISTEN = %q, want untouched :8080", got)
	}

	if err := applyServerFlags("", "", filepath.Join(t.TempDir(), "missing.env")); err != nil {
		t.Fatalf("missing --config tolerated, got: %v", err)
	}

	dir := t.TempDir()
	if err := applyServerFlags("", "", dir); err == nil {
		t.Error("directory --config must fail closed")
	}
}

func TestSplitListen(t *testing.T) {
	host, port := splitListen("127.0.0.1:9090")
	if host != "127.0.0.1" || port != "9090" {
		t.Errorf("split = %q/%q", host, port)
	}
	if host, port := splitListen(":8080"); host != "" || port != "8080" {
		t.Errorf("split = %q/%q", host, port)
	}
}

func TestRunServerSubcommandBootsAndShutsDown(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
	t.Setenv("OCTARQ_DB_DSN", filepath.Join(tempDir, "server_sub.db"))
	t.Setenv("OCTARQ_SECRET_KEY", "server-sub-secret-key-32-bytes-long!")
	t.Setenv("OCTARQ_ADMIN_PASSWORD", "server-sub-admin-pass")
	t.Setenv("OCTARQ_LISTEN", "127.0.0.1:0")
	t.Setenv("OCTARQ_IPC_SOCKET", filepath.Join(tempDir, "run.sock"))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out, errb bytes.Buffer
	if code := run(ctx, []string{"server", "--port", "0"}, &out, &errb); code != 0 {
		t.Fatalf("run server = %d, want 0: %s", code, errb.String())
	}
}

func TestStartIPCDaemonListenFailureIsBestEffort(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OCTARQ_IPC_SOCKET", filepath.Join(blocker, "run.sock"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startIPCDaemon(ctx, "test", func() *monitor.Collector { return nil })
}

func TestStartIPCDaemonServes(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "run.sock")
	t.Setenv("OCTARQ_IPC_SOCKET", socket)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startIPCDaemon(ctx, "test-ipc-main", func() *monitor.Collector { return nil })

	deadline := time.Now().Add(5 * time.Second)
	for {
		conn, err := net.DialTimeout("unix", socket, time.Second)
		if err == nil {
			conn.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("ipc daemon did not accept")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
