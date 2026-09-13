package plugin

import (
	"context"
	"sync"
)

// TypedBus provides a type-safe in-memory event bus for inter-plugin communication.
type TypedBus[T any] struct {
	mu       sync.RWMutex
	handlers []func(T) error
}

// NewTypedBus creates a new TypedBus instance for event type T.
func NewTypedBus[T any]() *TypedBus[T] {
	return &TypedBus[T]{}
}

// Subscribe registers an event handler.
func (b *TypedBus[T]) Subscribe(h func(T) error) {
	if h == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers = append(b.handlers, h)
}

// Publish synchronously invokes all registered handlers and collects errors.
func (b *TypedBus[T]) Publish(e T) []error {
	b.mu.RLock()
	handlers := make([]func(T) error, len(b.handlers))
	copy(handlers, b.handlers)
	b.mu.RUnlock()

	var errs []error
	for _, h := range handlers {
		if err := h(e); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

// PublishAsync asynchronously invokes handlers in a separate goroutine without blocking the caller.
// Each handler execution is guarded with recover to prevent panics from crashing the process.
func (b *TypedBus[T]) PublishAsync(ctx context.Context, e T) {
	b.mu.RLock()
	handlers := make([]func(T) error, len(b.handlers))
	copy(handlers, b.handlers)
	b.mu.RUnlock()

	go func() {
		for _, h := range handlers {
			if ctx != nil && ctx.Err() != nil {
				return
			}
			func(fn func(T) error) {
				defer func() {
					_ = recover()
				}()
				_ = fn(e)
			}(h)
		}
	}()
}
