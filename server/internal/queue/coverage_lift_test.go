package queue

import (
	"context"
	"errors"
	"testing"

	"github.com/octarq-org/octarq/server/pkg/telemetry"
)

func TestCoverageLift_RunHandlerWithTelemetry_UnwrappedAndMetrics(t *testing.T) {
	ctx := context.Background()
	tel, err := telemetry.Init(ctx, telemetry.Config{
		Enabled:         true,
		ServiceName:     "test-queue",
		MetricsExporter: "none",
		TracesExporter:  "none",
	})
	if err != nil {
		t.Fatalf("failed to init telemetry: %v", err)
	}
	t.Cleanup(func() {
		_ = tel.Shutdown(ctx)
	})

	var calledWith []byte
	handler := func(ctx context.Context, payload []byte) error {
		calledWith = payload
		return nil
	}

	raw := []byte("plain-unwrapped-payload-not-json")
	if err := runHandlerWithTelemetry("test.unwrapped", handler, raw); err != nil {
		t.Fatalf("runHandlerWithTelemetry failed: %v", err)
	}

	if string(calledWith) != string(raw) {
		t.Fatalf("expected payload %q, got %q", string(raw), string(calledWith))
	}

	// Also test error path with telemetry metrics enabled
	errHandler := func(ctx context.Context, payload []byte) error {
		return errors.New("handler failed")
	}
	if err := runHandlerWithTelemetry("test.error", errHandler, raw); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestCoverageLift_InMemoryQueue_ChannelClose(t *testing.T) {
	q := newInMemoryQueue()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := q.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Close channel to trigger the !ok worker exit path
	close(q.ch)
	q.wg.Wait()
}
