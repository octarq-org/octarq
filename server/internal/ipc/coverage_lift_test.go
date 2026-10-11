package ipc

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"testing"
	"time"
)

func TestCoverageLift_JSONCodec_MarshalError(t *testing.T) {
	codec := jsonCodec{}
	// Channel types cannot be marshaled into JSON
	_, err := codec.MarshalAppend(nil, make(chan int))
	if err == nil {
		t.Fatal("expected error when marshaling chan int, got nil")
	}
}

func TestCoverageLift_SocketPath_Fallback(t *testing.T) {
	t.Setenv("OCTARQ_IPC_SOCKET", "")
	t.Setenv("HOME", "")
	got := SocketPath()
	if filepath.Base(got) != "octarq-run.sock" {
		t.Fatalf("expected octarq-run.sock fallback, got %q", got)
	}
}

func TestCoverageLift_Listen_Errors(t *testing.T) {
	if _, err := Listen(""); err == nil {
		t.Fatal("expected error for empty socket path")
	}
	if _, err := Listen("/dev/null/impossible/octarq.sock"); err == nil {
		t.Fatal("expected error when mkdir fails")
	}
}

func TestCoverageLift_IPCErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s")
	lis, err := Listen(path)
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	defer lis.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	d := &Daemon{
		Version: "test-lift",
		Started: time.Now(),
		OnReload: func(ctx context.Context) ([]string, error) {
			return nil, errors.New("reload failed")
		},
		OnMetrics: nil, // Test nil OnMetrics
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
		time.Sleep(10 * time.Millisecond)
	}

	c := NewClient(path)

	// 1. Reload error branch
	_, err = c.Reload(context.Background())
	if err == nil {
		t.Fatal("expected error from failed OnReload, got nil")
	}

	// 2. Metrics with nil OnMetrics (returns overall: "unknown")
	rep, err := c.Metrics(context.Background())
	if err != nil {
		t.Fatalf("unexpected error when OnMetrics is nil: %v", err)
	}
	if rep.Overall != "unknown" {
		t.Fatalf("expected overall: unknown, got %q", rep.Overall)
	}

	// 3. Metrics with error OnMetrics
	d.OnMetrics = func(ctx context.Context) (*MetricsReply, error) {
		return nil, errors.New("metrics provider failed")
	}
	_, err = c.Metrics(context.Background())
	if err == nil {
		t.Fatal("expected error from failed OnMetrics, got nil")
	}
}
