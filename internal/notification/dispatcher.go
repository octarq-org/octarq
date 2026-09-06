package notification

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/octarq-org/octarq/plugin"
)

// Dispatcher manages asynchronous delivery to notification channels with exponential backoff retries.
type Dispatcher struct {
	maxRetries  int
	baseBackoff time.Duration
	wg          sync.WaitGroup
	mu          sync.RWMutex
	onFailure   func(err error, ch plugin.NotificationChannel, recipient plugin.NotificationRecipient, payload plugin.NotificationPayload)
	sleepFunc   func(d time.Duration)
}

// NewDispatcher initializes a Dispatcher with default retry parameters (max 3 attempts, 50ms initial backoff).
func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		maxRetries:  3,
		baseBackoff: 50 * time.Millisecond,
		sleepFunc:   time.Sleep,
	}
}

// SetMaxRetries updates the maximum delivery attempts (minimum 1).
func (d *Dispatcher) SetMaxRetries(n int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if n < 1 {
		n = 1
	}
	d.maxRetries = n
}

// SetBaseBackoff updates the initial exponential backoff duration.
func (d *Dispatcher) SetBaseBackoff(b time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if b < 0 {
		b = 0
	}
	d.baseBackoff = b
}

// SetOnFailure registers a callback invoked when a notification fails after all retry attempts.
func (d *Dispatcher) SetOnFailure(fn func(err error, ch plugin.NotificationChannel, recipient plugin.NotificationRecipient, payload plugin.NotificationPayload)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.onFailure = fn
}

// Wait blocks until all in-flight asynchronous dispatch goroutines complete.
func (d *Dispatcher) Wait() {
	d.wg.Wait()
}

// Dispatch kicks off asynchronous delivery in a goroutine with exponential backoff retries up to maxRetries.
func (d *Dispatcher) Dispatch(ctx context.Context, ch plugin.NotificationChannel, recipient plugin.NotificationRecipient, payload plugin.NotificationPayload) {
	if ch == nil {
		return
	}
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		_ = d.DispatchSync(ctx, ch, recipient, payload)
	}()
}

// DispatchSync delivers a notification synchronously, retrying up to maxRetries on failure using exponential backoff.
func (d *Dispatcher) DispatchSync(ctx context.Context, ch plugin.NotificationChannel, recipient plugin.NotificationRecipient, payload plugin.NotificationPayload) error {
	if ch == nil {
		return errors.New("notification: nil channel")
	}

	d.mu.RLock()
	maxAttempts := d.maxRetries
	base := d.baseBackoff
	sleep := d.sleepFunc
	onFail := d.onFailure
	d.mu.RUnlock()

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err := ch.Send(ctx, recipient, payload)
		if err == nil {
			return nil
		}
		lastErr = err

		if attempt < maxAttempts {
			backoff := base * time.Duration(1<<(attempt-1))
			if backoff > 0 && sleep != nil {
				sleep(backoff)
			}
		}
	}

	if onFail != nil {
		onFail(lastErr, ch, recipient, payload)
	} else {
		log.Printf("[notification] delivery failed permanently after %d attempts to channel=%s user=%s: %v",
			maxAttempts, ch.Name(), recipient.UserID, lastErr)
	}

	return lastErr
}
