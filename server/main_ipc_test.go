package main

import (
	"context"
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
