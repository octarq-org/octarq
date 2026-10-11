package queue

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestCharacterization_MemoryQueueProcessing locks in-memory queue dispatch,
// registration, and task execution.
func TestCharacterization_MemoryQueueProcessing(t *testing.T) {
	q := New("")

	var processed int32
	q.Register("test_task", func(ctx context.Context, payload []byte) error {
		if string(payload) == "hello" {
			atomic.AddInt32(&processed, 1)
		}
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := q.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if err := q.Enqueue(ctx, "test_task", []byte("hello")); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	// Wait briefly for worker processing
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&processed) == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if atomic.LoadInt32(&processed) != 1 {
		t.Fatalf("expected 1 task processed, got %d", processed)
	}
}

// TestCharacterization_QueueFullBackpressure locks the backpressure error
// when queue buffer is full.
func TestCharacterization_QueueFullBackpressure(t *testing.T) {
	q := newInMemoryQueue()
	// Fill buffer completely
	ctx := context.Background()
	for i := 0; i < cap(q.ch); i++ {
		if err := q.Enqueue(ctx, "fill", []byte("data")); err != nil {
			t.Fatalf("fill enqueue failed at %d: %v", i, err)
		}
	}

	// Next enqueue must return error queue full
	err := q.Enqueue(ctx, "overflow", []byte("data"))
	if err == nil {
		t.Fatal("expected error on full queue, got nil")
	}
	if !strings.Contains(err.Error(), "queue: full") {
		t.Fatalf("expected error containing 'queue: full', got %v", err)
	}
}

// TestCharacterization_UnregisteredTaskHandling locks unregistered task behavior.
func TestCharacterization_UnregisteredTaskHandling(t *testing.T) {
	q := newInMemoryQueue()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := q.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Enqueueing an unregistered task should succeed at enqueue time
	if err := q.Enqueue(ctx, "unregistered_type", []byte("payload")); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}
}
