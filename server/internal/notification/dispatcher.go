package notification

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"sync"
	"time"

	"github.com/octarq-org/octarq/server/plugin"
)

type Dispatcher struct {
	maxRetries  int
	baseBackoff time.Duration
	wg          sync.WaitGroup
	mu          sync.RWMutex
	onFailure   func(err error, ch plugin.NotificationChannel, recipient plugin.NotificationRecipient, payload plugin.NotificationPayload)
	sleepFunc   func(d time.Duration)
}

func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		maxRetries:  3,
		baseBackoff: 50 * time.Millisecond,
		sleepFunc:   time.Sleep,
	}
}

func (d *Dispatcher) SetMaxRetries(n int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if n < 1 {
		n = 1
	}
	d.maxRetries = n
}

func (d *Dispatcher) SetBaseBackoff(b time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if b < 0 {
		b = 0
	}
	d.baseBackoff = b
}

func (d *Dispatcher) SetOnFailure(fn func(err error, ch plugin.NotificationChannel, recipient plugin.NotificationRecipient, payload plugin.NotificationPayload)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.onFailure = fn
}

func (d *Dispatcher) Wait() {
	d.wg.Wait()
}

func (d *Dispatcher) Dispatch(ctx context.Context, ch plugin.NotificationChannel, recipient plugin.NotificationRecipient, payload plugin.NotificationPayload) {
	if ch == nil {
		return
	}
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		if err := d.DispatchSync(ctx, ch, recipient, payload); err != nil {
			slog.Warn("notification: async dispatch delivery failed", "channel", ch.Name(), "error", err)
		}
	}()
}

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
