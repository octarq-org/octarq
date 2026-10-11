package eventbus_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/internal/eventbus"
	"github.com/octarq-org/octarq/server/plugin"
)

type testReactor struct {
	events   []string
	reactFn  func(ctx context.Context, env plugin.Envelope) error
	count    int64
	received []plugin.Envelope
	mu       sync.Mutex
}

func (r *testReactor) Events() []string {
	return r.events
}

func (r *testReactor) React(ctx context.Context, env plugin.Envelope) error {
	atomic.AddInt64(&r.count, 1)
	r.mu.Lock()
	r.received = append(r.received, env)
	r.mu.Unlock()
	if r.reactFn != nil {
		return r.reactFn(ctx, env)
	}
	return nil
}

type debounceTestReactor struct {
	testReactor
	minInterval time.Duration
}

func (d *debounceTestReactor) MinInterval() time.Duration {
	return d.minInterval
}

type customEntityReactor struct {
	testReactor
	customKeyFn func(env plugin.Envelope) string
}

func (c *customEntityReactor) EntityKey(env plugin.Envelope) string {
	if c.customKeyFn != nil {
		return c.customKeyFn(env)
	}
	return env.EntityKey
}

func TestReactor_RegistrationValidation(t *testing.T) {
	reg := eventbus.NewReactorRegistry()

	// Nil reactor
	if err := reg.Register(nil); err == nil {
		t.Fatal("expected error for nil reactor, got nil")
	}

	// Empty events
	rEmpty := &testReactor{events: []string{}}
	if err := reg.Register(rEmpty); err == nil {
		t.Fatal("expected error for empty events, got nil")
	}

	// Whitespace event key
	rSpace := &testReactor{events: []string{"   "}}
	if err := reg.Register(rSpace); err == nil {
		t.Fatal("expected error for whitespace event key, got nil")
	}

	// Valid registration
	rValid := &testReactor{events: []string{"link.click"}}
	if err := reg.Register(rValid); err != nil {
		t.Fatalf("unexpected error registering valid reactor: %v", err)
	}

	// Duplicate registration
	if err := reg.Register(rValid); err == nil {
		t.Fatal("expected error for duplicate reactor registration, got nil")
	}

	reactors := reg.Reactors()
	if len(reactors) != 1 {
		t.Fatalf("expected 1 reactor, got %d", len(reactors))
	}

	reg.Reset()
	if len(reg.Reactors()) != 0 {
		t.Fatalf("expected 0 reactors after Reset, got %d", len(reg.Reactors()))
	}
}

func TestReactor_MultiReactorFanout(t *testing.T) {
	reg := eventbus.NewReactorRegistry()
	r1 := &testReactor{events: []string{"link.click"}}
	r2 := &testReactor{events: []string{"link.click", "link.created"}}

	if err := reg.Register(r1); err != nil {
		t.Fatalf("register r1: %v", err)
	}
	if err := reg.Register(r2); err != nil {
		t.Fatalf("register r2: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stop := reg.Start(ctx, 4)
	defer stop()

	eventbus.PublishEnvelope(eventbus.Envelope{
		OrgID:     1,
		Key:       "link.click",
		EntityKey: "link:100",
	})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&r1.count) == 1 && atomic.LoadInt64(&r2.count) == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	if c1 := atomic.LoadInt64(&r1.count); c1 != 1 {
		t.Errorf("r1 expected 1 event, got %d", c1)
	}
	if c2 := atomic.LoadInt64(&r2.count); c2 != 1 {
		t.Errorf("r2 expected 1 event, got %d", c2)
	}
}

func TestReactor_MinIntervalDebounce(t *testing.T) {
	reg := eventbus.NewReactorRegistry()
	r := &debounceTestReactor{
		testReactor: testReactor{events: []string{"link.click"}},
		minInterval: 80 * time.Millisecond,
	}
	if err := reg.Register(r); err != nil {
		t.Fatalf("register: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	beforeDropped := eventbus.DroppedTotal()
	stop := reg.Start(ctx, 4)
	defer stop()

	// Send 3 events with same EntityKey immediately
	eventbus.PublishEnvelope(eventbus.Envelope{OrgID: 1, Key: "link.click", EntityKey: "link:debounce-1"})
	eventbus.PublishEnvelope(eventbus.Envelope{OrgID: 1, Key: "link.click", EntityKey: "link:debounce-1"})
	eventbus.PublishEnvelope(eventbus.Envelope{OrgID: 1, Key: "link.click", EntityKey: "link:debounce-1"})

	// Send 1 event with a different EntityKey
	eventbus.PublishEnvelope(eventbus.Envelope{OrgID: 1, Key: "link.click", EntityKey: "link:debounce-2"})

	// Give time to process
	time.Sleep(40 * time.Millisecond)

	// Wait past minInterval and send link:debounce-1 again
	time.Sleep(100 * time.Millisecond)
	eventbus.PublishEnvelope(eventbus.Envelope{OrgID: 1, Key: "link.click", EntityKey: "link:debounce-1"})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&r.count) >= 3 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	// link:debounce-1 should fire 2 times, link:debounce-2 should fire 1 time -> total 3 fires
	if got := atomic.LoadInt64(&r.count); got != 3 {
		t.Errorf("expected 3 executed events, got %d", got)
	}

	afterDropped := eventbus.DroppedTotal()
	if afterDropped-beforeDropped < 2 {
		t.Errorf("expected at least 2 dropped events in DroppedTotal, got diff %d", afterDropped-beforeDropped)
	}
}

func TestReactor_DebouncePerReactorIsolation(t *testing.T) {
	reg := eventbus.NewReactorRegistry()
	// Reactor A has debounce 200ms
	rA := &debounceTestReactor{
		testReactor: testReactor{events: []string{"link.anomaly"}},
		minInterval: 200 * time.Millisecond,
	}
	// Reactor B has NO debounce
	rB := &testReactor{events: []string{"link.anomaly"}}

	if err := reg.Register(rA); err != nil {
		t.Fatalf("register rA: %v", err)
	}
	if err := reg.Register(rB); err != nil {
		t.Fatalf("register rB: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stop := reg.Start(ctx, 4)
	defer stop()

	// Send 2 rapid events with same EntityKey
	eventbus.PublishEnvelope(eventbus.Envelope{OrgID: 1, Key: "link.anomaly", EntityKey: "link:iso-1"})
	eventbus.PublishEnvelope(eventbus.Envelope{OrgID: 1, Key: "link.anomaly", EntityKey: "link:iso-1"})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&rB.count) == 2 && atomic.LoadInt64(&rA.count) == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	// rA debounced 1 event -> count 1
	if gotA := atomic.LoadInt64(&rA.count); gotA != 1 {
		t.Errorf("rA expected 1 event, got %d", gotA)
	}
	// rB has no debounce -> count 2
	if gotB := atomic.LoadInt64(&rB.count); gotB != 2 {
		t.Errorf("rB expected 2 events (must not be affected by rA debounce), got %d", gotB)
	}
}
