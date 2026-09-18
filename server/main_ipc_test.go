package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

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
