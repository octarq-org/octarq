package plugin

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestTypedBus_PublishSubscribe(t *testing.T) {
	bus := NewTypedBus[string]()

	var received string
	bus.Subscribe(func(msg string) error {
		received = msg
		return nil
	})

	errs := bus.Publish("hello world")
	if len(errs) != 0 {
		t.Fatalf("expected 0 errors, got %d", len(errs))
	}
	if received != "hello world" {
		t.Fatalf("expected 'hello world', got '%s'", received)
	}
}

func TestTypedBus_MultipleSubscribers(t *testing.T) {
	bus := NewTypedBus[int]()

	var called1, called2 bool
	bus.Subscribe(func(n int) error {
		if n == 42 {
			called1 = true
		}
		return nil
	})
	bus.Subscribe(func(n int) error {
		if n == 42 {
			called2 = true
		}
		return errors.New("handler 2 error")
	})

	errs := bus.Publish(42)
	if !called1 {
		t.Fatalf("expected handler 1 to be called")
	}
	if !called2 {
		t.Fatalf("expected handler 2 to be called")
	}
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if errs[0].Error() != "handler 2 error" {
		t.Fatalf("unexpected error: %v", errs[0])
	}
}

func TestTypedBus_PublishAsync(t *testing.T) {
	bus := NewTypedBus[string]()
	ch := make(chan string, 1)

	bus.Subscribe(func(msg string) error {
		ch <- msg
		return nil
	})

	start := time.Now()
	bus.PublishAsync(context.Background(), "async message")
	// Verify non-blocking
	if time.Since(start) > 50*time.Millisecond {
		t.Fatalf("PublishAsync blocked the caller")
	}

	select {
	case msg := <-ch:
		if msg != "async message" {
			t.Fatalf("expected 'async message', got '%s'", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for async event")
	}
}

func TestTypedBus_PublishAsync_PanicRecovery(t *testing.T) {
	bus := NewTypedBus[string]()
	var wg sync.WaitGroup
	wg.Add(1)

	var secondCalled bool
	bus.Subscribe(func(msg string) error {
		panic("boom")
	})
	bus.Subscribe(func(msg string) error {
		defer wg.Done()
		secondCalled = true
		return nil
	})

	bus.PublishAsync(context.Background(), "test-panic")

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		if !secondCalled {
			t.Fatalf("expected second handler to be called after first handler panicked")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for handlers after panic")
	}
}

func TestTypedBus_Isolation(t *testing.T) {
	intBus := NewTypedBus[int]()
	strBus := NewTypedBus[string]()

	var receivedInt int
	var receivedStr string

	intBus.Subscribe(func(val int) error {
		receivedInt = val
		return nil
	})
	strBus.Subscribe(func(val string) error {
		receivedStr = val
		return nil
	})

	intBus.Publish(100)
	strBus.Publish("isolated")

	if receivedInt != 100 {
		t.Fatalf("expected int 100, got %d", receivedInt)
	}
	if receivedStr != "isolated" {
		t.Fatalf("expected str 'isolated', got '%s'", receivedStr)
	}
}
