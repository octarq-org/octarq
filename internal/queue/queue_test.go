package queue

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestNew_InMemory(t *testing.T) {
	q := New("")
	if q == nil {
		t.Fatal("expected non-nil Queue")
	}
	_, ok := q.(*InMemoryQueue)
	if !ok {
		t.Errorf("expected *InMemoryQueue for empty redis URL")
	}
}

func TestNew_InvalidRedisURL(t *testing.T) {
	q := New("::invalid-url::")
	if q == nil {
		t.Fatal("expected non-nil Queue")
	}
	_, ok := q.(*InMemoryQueue)
	if !ok {
		t.Errorf("expected *InMemoryQueue fallback for invalid redis URL")
	}
}

func TestInMemoryQueue_Execution(t *testing.T) {
	q := newInMemoryQueue()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var count atomic.Int32
	q.Register("test_task", func(ctx context.Context, payload []byte) error {
		if string(payload) == "fail" {
			return errors.New("handler error")
		}
		count.Add(1)
		return nil
	})

	if err := q.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if err := q.Enqueue(ctx, "test_task", []byte("hello")); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	if err := q.Enqueue(ctx, "test_task", []byte("fail")); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	if err := q.Enqueue(ctx, "unknown_task", []byte("xyz")); err != nil {
		t.Fatalf("Enqueue unknown task failed: %v", err)
	}

	// Wait for worker processing
	time.Sleep(100 * time.Millisecond)

	if count.Load() != 1 {
		t.Errorf("expected count 1, got %d", count.Load())
	}
}

func TestInMemoryQueue_FullFallback(t *testing.T) {
	q := newInMemoryQueue()
	// Unregistered task when full
	for i := 0; i < 1000; i++ {
		_ = q.Enqueue(context.Background(), "unregistered", []byte("fill"))
	}
	err := q.Enqueue(context.Background(), "unregistered", []byte("overflow"))
	if err == nil {
		t.Errorf("expected error when queue full and no handler registered")
	}

	// Registered task when full
	var ran atomic.Bool
	q.Register("overflow_task", func(ctx context.Context, payload []byte) error {
		ran.Store(true)
		return nil
	})
	err = q.Enqueue(context.Background(), "overflow_task", []byte("overflow"))
	if err != nil {
		t.Errorf("expected no error when fallback instant execution succeeds, got %v", err)
	}

	time.Sleep(100 * time.Millisecond)
	if !ran.Load() {
		t.Errorf("expected fallback instant execution to run task")
	}
}

func TestAsynqQueue_Fallback(t *testing.T) {
	// Connect to non-existent Redis port
	q := New("redis://127.0.0.1:58999/0")
	asynqQ, ok := q.(*AsynqQueue)
	if !ok {
		t.Fatalf("expected *AsynqQueue")
	}

	// Enqueue without handler registered
	err := asynqQ.Enqueue(context.Background(), "task1", []byte("p1"))
	if err == nil {
		t.Errorf("expected error when redis down and no handler registered")
	}

	// Enqueue with handler registered -> triggers fallback
	var ran atomic.Bool
	asynqQ.Register("task1", func(ctx context.Context, payload []byte) error {
		ran.Store(true)
		return nil
	})

	err = asynqQ.Enqueue(context.Background(), "task1", []byte("p1"))
	if err != nil {
		t.Errorf("expected fallback success when handler registered, got %v", err)
	}

	time.Sleep(100 * time.Millisecond)
	if !ran.Load() {
		t.Errorf("expected fallback execution when Redis is unreachable")
	}
}
