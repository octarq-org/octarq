package eventbus

import (
	"encoding/json"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// resetSpine clears global spine state between tests.
func resetSpine(t *testing.T) {
	t.Helper()
	spMu.Lock()
	spSubs = nil
	spMu.Unlock()
	atomic.StoreUint64(&droppedCnt, 0)
}

// ── Test 1: default buffer size is 64 ────────────────────────────────────────

func TestSpine_DefaultBuffer(t *testing.T) {
	resetSpine(t)

	ch, cancel := Subscribe(SubscribeOpts{})
	defer cancel()

	// Should be able to send 64 items without blocking.
	for i := 0; i < defaultSpineBuffer; i++ {
		PublishEnvelope(Envelope{OrgID: 1, Key: "test.event"})
	}

	got := 0
	timeout := time.After(1 * time.Second)
	for got < defaultSpineBuffer {
		select {
		case <-ch:
			got++
		case <-timeout:
			t.Fatalf("timed out after receiving %d/%d envelopes", got, defaultSpineBuffer)
		}
	}
	if got != defaultSpineBuffer {
		t.Errorf("expected %d envelopes, got %d", defaultSpineBuffer, got)
	}
}

// ── Test 2: Keys filter works ─────────────────────────────────────────────────

func TestSpine_KeysFilter(t *testing.T) {
	resetSpine(t)

	ch, cancel := Subscribe(SubscribeOpts{Keys: []string{"link.click"}})
	defer cancel()

	// Send two different event types.
	PublishEnvelope(Envelope{OrgID: 1, Key: "link.click"})
	PublishEnvelope(Envelope{OrgID: 1, Key: "member.join"}) // should be filtered out

	// Give publisher goroutine time to finish (PublishEnvelope is synchronous but just in case).
	time.Sleep(20 * time.Millisecond)

	select {
	case env := <-ch:
		if env.Key != "link.click" {
			t.Errorf("expected link.click, got %q", env.Key)
		}
	default:
		t.Fatal("expected at least one envelope")
	}

	// No second envelope should have arrived.
	select {
	case extra := <-ch:
		t.Errorf("unexpected envelope with key %q", extra.Key)
	default:
	}
}

// ── Test 3: Full buffer — drop-oldest-per-EntityKey, then fallback ────────────

func TestSpine_BackpressureDropOldestPerEntityKey(t *testing.T) {
	resetSpine(t)

	const bufSize = 4
	ch, cancel := Subscribe(SubscribeOpts{BufferSize: bufSize})
	defer cancel()

	// Fill the buffer completely with entity "link:1".
	for i := 0; i < bufSize; i++ {
		PublishEnvelope(Envelope{
			OrgID:     1,
			Key:       "link.click",
			EntityKey: "link:1",
			Payload:   json.RawMessage(`{"seq":` + itoa(i) + `}`),
		})
	}
	// Channel is now full with seq 0,1,2,3.

	// Publish one more for entity "link:1" — should drop seq 0 (oldest same entity).
	beforeDrop := DroppedTotal()
	PublishEnvelope(Envelope{
		OrgID:     1,
		Key:       "link.click",
		EntityKey: "link:1",
		Payload:   json.RawMessage(`{"seq":4}`),
	})
	if DroppedTotal() <= beforeDrop {
		t.Errorf("expected droppedCnt to increase, still %d", DroppedTotal())
	}

	// Drain channel and verify seq 0 was dropped (seq 1,2,3,4 remain).
	got := make([]int, 0, bufSize)
	drain := time.After(500 * time.Millisecond)
	for len(got) < bufSize {
		select {
		case env := <-ch:
			var m map[string]int
			if err := json.Unmarshal(env.Payload, &m); err == nil {
				got = append(got, m["seq"])
			}
		case <-drain:
			break
		}
	}
	for _, seq := range got {
		if seq == 0 {
			t.Errorf("seq 0 should have been dropped (oldest same EntityKey)")
		}
	}
	found4 := false
	for _, seq := range got {
		if seq == 4 {
			found4 = true
		}
	}
	if !found4 {
		t.Errorf("newest envelope (seq 4) should be in channel, got %v", got)
	}
}

func TestSpine_BackpressureFallbackDropHead(t *testing.T) {
	resetSpine(t)

	const bufSize = 4
	ch, cancel := Subscribe(SubscribeOpts{BufferSize: bufSize})
	defer cancel()

	// Fill with mixed entity keys so no match for the incoming entity.
	for i := 0; i < bufSize; i++ {
		PublishEnvelope(Envelope{
			OrgID:     1,
			Key:       "link.click",
			EntityKey: "link:" + itoa(i), // all different
			Payload:   json.RawMessage(`{"seq":` + itoa(i) + `}`),
		})
	}

	before := DroppedTotal()
	// Send with a new EntityKey — fallback: drop the overall head (seq 0).
	PublishEnvelope(Envelope{
		OrgID:     1,
		Key:       "link.click",
		EntityKey: "link:99",
		Payload:   json.RawMessage(`{"seq":99}`),
	})
	if DroppedTotal() <= before {
		t.Errorf("expected droppedCnt to increase")
	}

	// Drain and verify seq 0 is gone and seq 99 is present.
	got := make([]int, 0, bufSize)
	drain := time.After(500 * time.Millisecond)
	for len(got) < bufSize {
		select {
		case env := <-ch:
			var m map[string]int
			if err := json.Unmarshal(env.Payload, &m); err == nil {
				got = append(got, m["seq"])
			}
		case <-drain:
			break
		}
	}
	for _, seq := range got {
		if seq == 0 {
			t.Errorf("seq 0 (head) should have been dropped")
		}
	}
	found99 := false
	for _, seq := range got {
		if seq == 99 {
			found99 = true
		}
	}
	if !found99 {
		t.Errorf("newest envelope (seq 99) should be in channel, got %v", got)
	}
}

// ── Test 4: cancel is idempotent, no panic, no goroutine leak ─────────────────

func TestSpine_CancelIdempotent(t *testing.T) {
	resetSpine(t)

	goroutinesBefore := runtime.NumGoroutine()

	ch, cancel := Subscribe(SubscribeOpts{BufferSize: 8})

	// Double-cancel must not panic.
	cancel()
	cancel()
	cancel()

	// Channel must be closed.
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("channel should be closed after cancel")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("channel was not closed after cancel")
	}

	// Subscriber must be removed from the registry.
	spMu.RLock()
	count := len(spSubs)
	spMu.RUnlock()
	if count != 0 {
		t.Errorf("expected 0 subscribers after cancel, got %d", count)
	}

	// Goroutine count should not grow (allow small scheduling slack).
	runtime.Gosched()
	time.Sleep(20 * time.Millisecond)
	goroutinesAfter := runtime.NumGoroutine()
	if goroutinesAfter > goroutinesBefore+2 {
		t.Errorf("goroutine leak: before=%d after=%d", goroutinesBefore, goroutinesAfter)
	}
}

func TestSpine_CancelWhilePublishing(t *testing.T) {
	// Race between cancel and concurrent PublishEnvelope must not panic.
	resetSpine(t)

	_, cancel := Subscribe(SubscribeOpts{BufferSize: 8})

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Publisher goroutine.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				PublishEnvelope(Envelope{OrgID: 1, Key: "test.event"})
			}
		}
	}()

	time.Sleep(5 * time.Millisecond)
	cancel() // cancel while publisher is running — must not panic
	close(stop)
	wg.Wait()
}

// ── Test 5: old Publish → subscribers receive equivalent Envelope + webhook OK ─

func TestSpine_PublishBridgesToEnvelope(t *testing.T) {
	resetSpine(t)
	// Use nil DB so only the spine path runs (no webhook fan-out).
	Init(nil)

	ch, cancel := Subscribe(SubscribeOpts{Keys: []string{"link.click"}})
	defer cancel()

	// Publish via the legacy API.
	data := map[string]any{"url": "https://example.com"}
	Publish(1, "link.click", data)

	select {
	case env := <-ch:
		if env.OrgID != 1 {
			t.Errorf("expected OrgID 1, got %d", env.OrgID)
		}
		if env.Key != "link.click" {
			t.Errorf("expected key link.click, got %q", env.Key)
		}
		if len(env.Payload) == 0 {
			t.Error("expected non-empty Payload")
		}
		// Verify Payload round-trips.
		var m map[string]any
		if err := json.Unmarshal(env.Payload, &m); err != nil {
			t.Errorf("Payload is not valid JSON: %v", err)
		}
		if m["url"] != "https://example.com" {
			t.Errorf("unexpected payload content: %v", m)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for envelope from legacy Publish")
	}
}

// ── Concurrency / race detector pass ─────────────────────────────────────────

func TestSpine_ConcurrentPublish(t *testing.T) {
	resetSpine(t)

	const numSubs = 5
	const numPublishers = 8
	const eventsEach = 200

	cancels := make([]func(), numSubs)
	for i := 0; i < numSubs; i++ {
		_, c := Subscribe(SubscribeOpts{BufferSize: 32})
		cancels[i] = c
	}
	defer func() {
		for _, c := range cancels {
			c()
		}
	}()

	var wg sync.WaitGroup
	for p := 0; p < numPublishers; p++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < eventsEach; i++ {
				PublishEnvelope(Envelope{
					OrgID:     1,
					Key:       "link.click",
					EntityKey: "link:1",
				})
			}
		}()
	}
	wg.Wait()
	// No assertions beyond "it didn't panic and the race detector is happy".
}

// ── helpers ───────────────────────────────────────────────────────────────────

// itoa converts an int to its decimal string representation without importing
// strconv (keeps the test file dependency-light).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	buf := [20]byte{}
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
