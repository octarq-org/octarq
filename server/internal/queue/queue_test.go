package queue

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNew_InMemory(t *testing.T) {
	q, err := New("")
	if err != nil {
		t.Fatalf("New(\"\") = %v", err)
	}
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
	if _, err := New("::invalid-url::"); err == nil {
		t.Fatal("an invalid redis URL must return an error, not an in-memory queue")
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

func TestInMemoryQueue_FullRejects(t *testing.T) {
	q := newInMemoryQueue()
	for i := 0; i < cap(q.ch); i++ {
		if err := q.Enqueue(context.Background(), "unregistered", []byte("fill")); err != nil {
			t.Fatalf("fill enqueue failed at %d: %v", i, err)
		}
	}

	ranCh := make(chan struct{}, 1)
	q.Register("overflow_task", func(ctx context.Context, payload []byte) error {
		ranCh <- struct{}{}
		return nil
	})

	err := q.Enqueue(context.Background(), "overflow_task", []byte("overflow"))
	if err == nil {
		t.Fatal("expected an error when the buffer is full")
	}
	if !strings.Contains(err.Error(), "queue: full") {
		t.Fatalf("expected error containing 'queue: full', got %v", err)
	}

	select {
	case <-ranCh:
		t.Fatal("handler ran inline; a full buffer must not execute the task")
	case <-time.After(200 * time.Millisecond):
	}
}

func TestAsynqQueue_EnqueueFailureIsReturned(t *testing.T) {
	q, err := New("redis://127.0.0.1:58999/0")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	asynqQ, ok := q.(*AsynqQueue)
	if !ok {
		t.Fatalf("expected *AsynqQueue")
	}

	ranCh := make(chan struct{}, 1)
	asynqQ.Register("task1", func(ctx context.Context, payload []byte) error {
		ranCh <- struct{}{}
		return nil
	})

	err = asynqQ.Enqueue(context.Background(), "task1", []byte("p1"))
	if err == nil {
		t.Fatal("expected an error when Redis is unreachable")
	}
	if !strings.Contains(err.Error(), "Redis") {
		t.Fatalf("expected error naming Redis, got %v", err)
	}

	select {
	case <-ranCh:
		t.Fatal("handler ran inline; a failed Redis enqueue must surface the error")
	case <-time.After(200 * time.Millisecond):
	}
}

func TestAsynqQueue_EnqueueWithoutHandlerStillFails(t *testing.T) {
	q, err := New("redis://127.0.0.1:58999/0")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	asynqQ, ok := q.(*AsynqQueue)
	if !ok {
		t.Fatalf("expected *AsynqQueue")
	}

	if err := asynqQ.Enqueue(context.Background(), "unregistered", []byte("p1")); err == nil {
		t.Fatal("expected an error when Redis is unreachable")
	}
}

func TestAsynqQueue_StartAndShutdown(t *testing.T) {
	q, err := New("redis://127.0.0.1:58999/0")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
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

	cancel()
	asynqQ.server.Shutdown()
}
