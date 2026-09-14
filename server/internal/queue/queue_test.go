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
		return
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
		return
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
	successCh := make(chan struct{})
	failCh := make(chan struct{})

	q.Register("test_task", func(ctx context.Context, payload []byte) error {
		if string(payload) == "fail" {
			close(failCh)
			return errors.New("handler error")
		}
		count.Add(1)
		close(successCh)
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

	select {
	case <-successCh:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for success task")
	}

	select {
	case <-failCh:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for fail task")
	}

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
	ranCh := make(chan struct{})
	q.Register("overflow_task", func(ctx context.Context, payload []byte) error {
		close(ranCh)
		return nil
	})
	err = q.Enqueue(context.Background(), "overflow_task", []byte("overflow"))
	if err != nil {
		t.Errorf("expected no error when fallback instant execution succeeds, got %v", err)
	}

	select {
	case <-ranCh:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for fallback task execution")
	}

	// Fallback error branch
	ranFailCh := make(chan struct{})
	q.Register("overflow_fail_task", func(ctx context.Context, payload []byte) error {
		close(ranFailCh)
		return errors.New("fallback error")
	})
	err = q.Enqueue(context.Background(), "overflow_fail_task", []byte("overflow_fail"))
	if err != nil {
		t.Errorf("expected no error from Enqueue even when fallback handler errors, got %v", err)
	}

	select {
	case <-ranFailCh:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for fallback failure task")
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
	ranCh := make(chan struct{})
	asynqQ.Register("task1", func(ctx context.Context, payload []byte) error {
		close(ranCh)
		return nil
	})

	err = asynqQ.Enqueue(context.Background(), "task1", []byte("p1"))
	if err != nil {
		t.Errorf("expected fallback success when handler registered, got %v", err)
	}

	select {
	case <-ranCh:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for fallback execution")
	}

	// Fallback with error
	ranFailCh := make(chan struct{})
	asynqQ.Register("task_fail", func(ctx context.Context, payload []byte) error {
		close(ranFailCh)
		return errors.New("asynq fallback handler error")
	})
	err = asynqQ.Enqueue(context.Background(), "task_fail", []byte("p_fail"))
	if err != nil {
		t.Errorf("expected no enqueue error for fallback task, got %v", err)
	}
	select {
	case <-ranFailCh:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for asynq fallback error execution")
	}
}

func TestAsynqQueue_StartAndShutdown(t *testing.T) {
	q := New("redis://127.0.0.1:58999/0")
	asynqQ, ok := q.(*AsynqQueue)
	if !ok {
		t.Fatalf("expected *AsynqQueue")
	}

	asynqQ.Register("asynq_task_a", func(ctx context.Context, payload []byte) error {
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	if err := asynqQ.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Cancel context to trigger shutdown
	cancel()
	// Shutdown takes a moment; give the goroutine a moment to finish srv.Shutdown()
	asynqQ.server.Shutdown()
}
