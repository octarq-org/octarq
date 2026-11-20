package eventbus

import (
	"encoding/json"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func resetSpine(t *testing.T) {
	t.Helper()
	spMu.Lock()
	spSubs = nil
	spMu.Unlock()
	atomic.StoreUint64(&droppedCnt, 0)
}

func TestSpine_DefaultBuffer(t *testing.T) {
	resetSpine(t)

	ch, cancel := Subscribe(SubscribeOpts{})
	defer cancel()

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

func TestSpine_KeysFilter(t *testing.T) {
	resetSpine(t)

	ch, cancel := Subscribe(SubscribeOpts{Keys: []string{"link.click"}})
	defer cancel()

	PublishEnvelope(Envelope{OrgID: 1, Key: "link.click"})
	PublishEnvelope(Envelope{OrgID: 1, Key: "member.join"})

	time.Sleep(20 * time.Millisecond)

	select {
	case env := <-ch:
		if env.Key != "link.click" {
			t.Errorf("expected link.click, got %q", env.Key)
		}
	default:
		t.Fatal("expected at least one envelope")
	}

	select {
	case extra := <-ch:
		t.Errorf("unexpected envelope with key %q", extra.Key)
	default:
	}
}

func TestSpine_BackpressureDropOldestPerEntityKey(t *testing.T) {
	resetSpine(t)

	const bufSize = 4
	ch, cancel := Subscribe(SubscribeOpts{BufferSize: bufSize})
	defer cancel()

	publish := func(seq int, entity string) {
		PublishEnvelope(Envelope{
			OrgID:     1,
			Key:       "link.click",
			EntityKey: entity,
			Payload:   json.RawMessage(`{"seq":` + itoa(seq) + `}`),
		})
	}
	publish(0, "other")
	for i := 1; i < bufSize; i++ {
		publish(i, "link:1")
	}

	beforeDrop := DroppedTotal()
	publish(4, "link:1")
	if DroppedTotal() <= beforeDrop {
		t.Errorf("expected droppedCnt to increase, still %d", DroppedTotal())
	}

	got := make([]int, 0, bufSize)
	drain := time.After(500 * time.Millisecond)
drainLoop:
	for len(got) < bufSize {
		select {
		case env := <-ch:
			var m map[string]int
			if err := json.Unmarshal(env.Payload, &m); err == nil {
				got = append(got, m["seq"])
			}
		case <-drain:
			break drainLoop
		}
	}
	found := map[int]bool{}
	for _, seq := range got {
		if seq == 1 {
			t.Errorf("seq 1 (oldest same EntityKey) should have been dropped, got %v", got)
		}
		found[seq] = true
	}
	if !found[0] {
		t.Errorf("seq 0 (different entity, global head) must be retained under per-entity drop, got %v", got)
	}
	if !found[4] {
		t.Errorf("newest envelope (seq 4) should be delivered, got %v", got)
	}
}

func TestSpine_BackpressureFallbackDropHead(t *testing.T) {
	resetSpine(t)

	const bufSize = 4
	ch, cancel := Subscribe(SubscribeOpts{BufferSize: bufSize})
	defer cancel()

	for i := 0; i < bufSize; i++ {
		PublishEnvelope(Envelope{
			OrgID:     1,
			Key:       "link.click",
			EntityKey: "link:" + itoa(i),
			Payload:   json.RawMessage(`{"seq":` + itoa(i) + `}`),
		})
	}

	before := DroppedTotal()
	PublishEnvelope(Envelope{
		OrgID:     1,
		Key:       "link.click",
		EntityKey: "link:99",
		Payload:   json.RawMessage(`{"seq":99}`),
	})
	if DroppedTotal() <= before {
		t.Errorf("expected droppedCnt to increase")
	}

	got := make([]int, 0, bufSize)
	drain := time.After(500 * time.Millisecond)
drainLoop:
	for len(got) < bufSize {
		select {
		case env := <-ch:
			var m map[string]int
			if err := json.Unmarshal(env.Payload, &m); err == nil {
				got = append(got, m["seq"])
			}
		case <-drain:
			break drainLoop
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

func TestSpine_CancelIdempotent(t *testing.T) {
	resetSpine(t)

	goroutinesBefore := runtime.NumGoroutine()

	ch, cancel := Subscribe(SubscribeOpts{BufferSize: 8})

	cancel()
	cancel()
	cancel()

	select {
	case _, ok := <-ch:
		if ok {
			t.Error("channel should be closed after cancel")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("channel was not closed after cancel")
	}

	spMu.RLock()
	count := len(spSubs)
	spMu.RUnlock()
	if count != 0 {
		t.Errorf("expected 0 subscribers after cancel, got %d", count)
	}

	runtime.Gosched()
	time.Sleep(20 * time.Millisecond)
	goroutinesAfter := runtime.NumGoroutine()
	if goroutinesAfter > goroutinesBefore+2 {
		t.Errorf("goroutine leak: before=%d after=%d", goroutinesBefore, goroutinesAfter)
	}
}

func TestSpine_CancelWhilePublishing(t *testing.T) {
	resetSpine(t)

	_, cancel := Subscribe(SubscribeOpts{BufferSize: 8})

	var wg sync.WaitGroup
	stop := make(chan struct{})

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
	cancel()
	close(stop)
	wg.Wait()
}

func TestSpine_PublishBridgesToEnvelope(t *testing.T) {
	resetSpine(t)
	Init(nil)

	ch, cancel := Subscribe(SubscribeOpts{Keys: []string{"link.click"}})
	defer cancel()

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
}

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
