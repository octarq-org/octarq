package eventbus_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/internal/eventbus"
	"github.com/octarq-org/octarq/server/plugin"
)

func TestReactor_EntityKeySerialization(t *testing.T) {
	reg := eventbus.NewReactorRegistry()

	var (
		activeKeyMu sync.Mutex
		activeKeys  = make(map[string]bool)
		violations  int64
	)

	r := &testReactor{
		events: []string{"link.serial"},
		reactFn: func(ctx context.Context, env plugin.Envelope) error {
			key := env.EntityKey
			activeKeyMu.Lock()
			if activeKeys[key] {
				atomic.AddInt64(&violations, 1)
			}
			activeKeys[key] = true
			activeKeyMu.Unlock()

			// Sleep briefly to expand concurrency race window if sharding failed
			time.Sleep(200 * time.Microsecond)

			activeKeyMu.Lock()
			activeKeys[key] = false
			activeKeyMu.Unlock()
			return nil
		},
	}

	if err := reg.Register(r); err != nil {
		t.Fatalf("register: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stop := reg.Start(ctx, 8)
	defer stop()

	const totalEvents = 100
	var pubWg sync.WaitGroup
	// Concurrently publish 100 events across 3 distinct entity keys
	for i := 0; i < totalEvents; i++ {
		pubWg.Add(1)
		go func(idx int) {
			defer pubWg.Done()
			var key string
			switch idx % 3 {
			case 1:
				key = "entity:B"
			case 2:
				key = "entity:C"
			default:
				key = "entity:A"
			}
			eventbus.PublishEnvelope(eventbus.Envelope{
				OrgID:     1,
				Key:       "link.serial",
				EntityKey: key,
			})
		}(i)
	}
	pubWg.Wait()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&r.count) == totalEvents {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if got := atomic.LoadInt64(&r.count); got != totalEvents {
		t.Fatalf("expected %d events, got %d", totalEvents, got)
	}

	if v := atomic.LoadInt64(&violations); v != 0 {
		t.Fatalf("expected 0 concurrency violations on same EntityKey, got %d", v)
	}
}

func TestReactor_CustomEntityKeyReactor(t *testing.T) {
	reg := eventbus.NewReactorRegistry()
	r := &customEntityReactor{
		testReactor: testReactor{events: []string{"custom.event"}},
		customKeyFn: func(env plugin.Envelope) string {
			return "custom:" + env.ID
		},
	}
	if err := reg.Register(r); err != nil {
		t.Fatalf("register: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stop := reg.Start(ctx, 4)
	defer stop()

	eventbus.PublishEnvelope(eventbus.Envelope{
		OrgID:     1,
		Key:       "custom.event",
		EntityKey: "ignored-key",
		ID:        "custom-id-99",
	})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&r.count) == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	if got := atomic.LoadInt64(&r.count); got != 1 {
		t.Fatalf("expected 1 event, got %d", got)
	}
}

func TestReactor_GracefulShutdown(t *testing.T) {
	reg := eventbus.NewReactorRegistry()
	r := &testReactor{events: []string{"shutdown.event"}}
	if err := reg.Register(r); err != nil {
		t.Fatalf("register: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	stop := reg.Start(ctx, 4)

	// Trigger cancel
	cancel()
	stop()

	// After stop, publishing should not panic or deliver to stopped runner
	eventbus.PublishEnvelope(eventbus.Envelope{
		OrgID: 1,
		Key:   "shutdown.event",
	})
}

func TestReactor_ErrorAndPanicResilience(t *testing.T) {
	reg := eventbus.NewReactorRegistry()
	var (
		errCount   int64
		panicCount int64
		okCount    int64
	)
	r := &testReactor{
		events: []string{"fault.event"},
		reactFn: func(ctx context.Context, env plugin.Envelope) error {
			if env.EntityKey == "err" {
				atomic.AddInt64(&errCount, 1)
				return errors.New("simulated error")
			}
			if env.EntityKey == "panic" {
				atomic.AddInt64(&panicCount, 1)
				panic("simulated panic")
			}
			atomic.AddInt64(&okCount, 1)
			return nil
		},
	}
	if err := reg.Register(r); err != nil {
		t.Fatalf("register: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stop := reg.Start(ctx, 4)
	defer stop()

	eventbus.PublishEnvelope(eventbus.Envelope{OrgID: 1, Key: "fault.event", EntityKey: "err"})
	eventbus.PublishEnvelope(eventbus.Envelope{OrgID: 1, Key: "fault.event", EntityKey: "panic"})
	eventbus.PublishEnvelope(eventbus.Envelope{OrgID: 1, Key: "fault.event", EntityKey: "ok"})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&okCount) == 1 && atomic.LoadInt64(&errCount) == 1 && atomic.LoadInt64(&panicCount) == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	if atomic.LoadInt64(&errCount) != 1 {
		t.Errorf("expected 1 err, got %d", atomic.LoadInt64(&errCount))
	}
	if atomic.LoadInt64(&panicCount) != 1 {
		t.Errorf("expected 1 panic, got %d", atomic.LoadInt64(&panicCount))
	}
	if atomic.LoadInt64(&okCount) != 1 {
		t.Errorf("expected 1 ok, got %d", atomic.LoadInt64(&okCount))
	}
}

func TestReactor_DefaultRegistryHelpers(t *testing.T) {
	eventbus.ResetReactors()
	r := &testReactor{events: []string{"default.test"}}
	if err := eventbus.RegisterReactor(r); err != nil {
		t.Fatalf("RegisterReactor helper failed: %v", err)
	}
	defer eventbus.ResetReactors()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stop := eventbus.StartReactors(ctx, 2)
	defer stop()

	eventbus.PublishEnvelope(eventbus.Envelope{OrgID: 1, Key: "default.test"})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&r.count) == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	if got := atomic.LoadInt64(&r.count); got != 1 {
		t.Errorf("expected 1 event on default registry, got %d", got)
	}
}

func TestReactor_ShardQueueFullDrop(t *testing.T) {
	reg := eventbus.NewReactorRegistry()
	blockCh := make(chan struct{})
	startedCh := make(chan struct{}, 1)

	r := &testReactor{
		events: []string{"queue.overflow"},
		reactFn: func(ctx context.Context, env plugin.Envelope) error {
			select {
			case startedCh <- struct{}{}:
			default:
			}
			<-blockCh
			return nil
		},
	}

	if err := reg.Register(r); err != nil {
		t.Fatalf("Register: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	beforeDropped := eventbus.DroppedTotal()
	stop := reg.Start(ctx, 1) // 1 worker shard with 256 buffer
	defer stop()

	// Publish 1st event to block worker 0
	eventbus.PublishEnvelope(eventbus.Envelope{
		OrgID:     1,
		Key:       "queue.overflow",
		EntityKey: "entity-block",
	})

	// Wait until worker starts executing the 1st event
	select {
	case <-startedCh:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not start executing first event in time")
	}

	// Now worker 0 is blocked. Shard buffer is 256.
	// Publish 300 more events: 256 should buffer, remaining should drop.
	const overflowCount = 300
	for i := 0; i < overflowCount; i++ {
		eventbus.PublishEnvelope(eventbus.Envelope{
			OrgID:     1,
			Key:       "queue.overflow",
			EntityKey: "entity-block",
		})
	}

	// Allow dispatcher to process incoming events from spine and drop overflow
	time.Sleep(50 * time.Millisecond)

	afterDropped := eventbus.DroppedTotal()
	if afterDropped <= beforeDropped {
		t.Errorf("expected DroppedTotal to increase due to full shard queue, before=%d, after=%d", beforeDropped, afterDropped)
	}

	// Unblock worker to drain remaining tasks
	close(blockCh)
}
