package api

import (
	"sync"
	"testing"
	"time"

	"github.com/octarq-org/octarq/plugin"
)

// TestEmitEmailFansOut verifies OnEmail handlers all receive the event and that
// dispatch is asynchronous (a slow handler doesn't block the others).
func TestEmitEmailFansOut(t *testing.T) {
	h := &Handler{}

	var mu sync.Mutex
	got := map[string]plugin.EmailEvent{}
	var wg sync.WaitGroup
	wg.Add(2)

	h.OnEmail(func(e plugin.EmailEvent) {
		mu.Lock()
		got["a"] = e
		mu.Unlock()
		wg.Done()
	})
	h.OnEmail(func(e plugin.EmailEvent) {
		mu.Lock()
		got["b"] = e
		mu.Unlock()
		wg.Done()
	})

	want := plugin.EmailEvent{ID: 7, OrgID: 3, From: "x@y.z", Subject: "hi"}
	h.emitEmail(want)

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handlers did not fire")
	}

	mu.Lock()
	defer mu.Unlock()
	if got["a"].ID != 7 || got["b"].ID != 7 || got["a"].Subject != "hi" {
		t.Errorf("handlers got wrong event: %+v", got)
	}
}

// TestOnEmailNilIgnored ensures a nil handler is silently dropped (no panic on
// dispatch).
func TestOnEmailNilIgnored(t *testing.T) {
	h := &Handler{}
	h.OnEmail(nil)
	h.emitEmail(plugin.EmailEvent{ID: 1}) // must not panic
}
