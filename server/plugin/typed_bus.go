package plugin

import (
	"context"
	"log"
	"sync"
)

type TypedBus[T any] struct {
	mu       sync.RWMutex
	handlers []func(T) error
}

func NewTypedBus[T any]() *TypedBus[T] {
	return &TypedBus[T]{}
}

func (b *TypedBus[T]) Subscribe(h func(T) error) {
	if h == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers = append(b.handlers, h)
}

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
					if r := recover(); r != nil {
						log.Printf("[typed_bus] recovered panic in async handler for %T: %v", e, r)
					}
				}()
				if err := fn(e); err != nil {
					log.Printf("[typed_bus] error in async handler for %T: %v", e, err)
				}
			}(h)
		}
	}()
}
