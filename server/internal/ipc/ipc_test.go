package ipc

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testSocket(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "run.sock")
}

func TestSocketPathDefault(t *testing.T) {
	t.Setenv("OCTARQ_IPC_SOCKET", "")
	got := SocketPath()
	if got == "" || len(got) < len("run.sock") {
		t.Errorf("socket path = %q", got)
	}
}

func TestSocketPathOverride(t *testing.T) {
	t.Setenv("OCTARQ_IPC_SOCKET", "/tmp/custom-octarq.sock")
	if got := SocketPath(); got != "/tmp/custom-octarq.sock" {
		t.Errorf("socket path = %q", got)
	}
}

func TestListenPermissions(t *testing.T) {
	path := testSocket(t)
	lis, err := Listen(path)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer lis.Close()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("socket perm = %o, want 600", info.Mode().Perm())
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if dirInfo.Mode().Perm()&0o077 != 0 {
		t.Errorf("socket dir perm = %o, want 0700", dirInfo.Mode().Perm())
	}
}

func TestStatusRoundTrip(t *testing.T) {
	path := testSocket(t)
	lis, err := Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	defer lis.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d := &Daemon{Version: "test-1", Started: time.Now().Add(-90 * time.Second)}
	go func() { _ = d.Serve(ctx, lis) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := net.DialTimeout("unix", path, time.Second); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("daemon did not accept")
		}
		time.Sleep(20 * time.Millisecond)
	}

	c := NewClient(path)
	reply, err := c.Status(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if reply.Version != "test-1" {
		t.Errorf("version = %q", reply.Version)
	}
	if reply.PID != os.Getpid() {
		t.Errorf("pid = %d, want %d", reply.PID, os.Getpid())
	}
	if reply.UptimeSecs < 80 {
		t.Errorf("uptime = %d, want >= 80", reply.UptimeSecs)
	}
	if reply.Goroutines <= 0 || reply.MemAlloc == 0 {
		t.Errorf("vitals missing: %+v", reply)
	}
}

func TestReloadAppliesHook(t *testing.T) {
	path := testSocket(t)
	lis, err := Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	defer lis.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d := &Daemon{
		Version: "test-1",
		Started: time.Now(),
		OnReload: func(ctx context.Context) ([]string, error) {
			return []string{"log.level"}, nil
		},
	}
	go func() { _ = d.Serve(ctx, lis) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := net.DialTimeout("unix", path, time.Second); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("daemon did not accept")
		}
		time.Sleep(20 * time.Millisecond)
	}

	c := NewClient(path)
	reply, err := c.Reload(context.Background())
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(reply.Applied) != 1 || reply.Applied[0] != "log.level" {
		t.Errorf("applied = %v", reply.Applied)
	}
}

func TestClientAgainstDeadDaemonFails(t *testing.T) {
	c := NewClient(filepath.Join(t.TempDir(), "nobody.sock"))
	if _, err := c.Status(context.Background()); err == nil {
		t.Error("dead daemon must be an error, never fabricated vitals")
	}
}

func TestMetricsRoundTrip(t *testing.T) {
	path := testSocket(t)
	lis, err := Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	defer lis.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d := &Daemon{
		Version: "test-1",
		Started: time.Now(),
		OnMetrics: func(ctx context.Context) (*MetricsReply, error) {
			return &MetricsReply{
				Overall:   "healthy",
				Providers: []ProviderMetric{{Name: "db", Status: "healthy"}},
				CheckedAt: time.Now().UTC().Format(time.RFC3339),
			}, nil
		},
	}
	go func() { _ = d.Serve(ctx, lis) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := net.DialTimeout("unix", path, time.Second); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("daemon did not accept")
		}
		time.Sleep(20 * time.Millisecond)
	}

	c := NewClient(path)
	rep, err := c.Metrics(context.Background())
	if err != nil {
		t.Fatalf("metrics: %v", err)
	}
	if rep.Overall != "healthy" || len(rep.Providers) != 1 || rep.Providers[0].Name != "db" {
		t.Errorf("metrics = %+v", rep)
	}
}
