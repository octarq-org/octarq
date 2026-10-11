package safego_test

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/octarq-org/octarq/server/internal/safego"
)

func TestSafegoHappyPath(t *testing.T) {
	var executed atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)

	safego.Go("happy-path-worker", func() {
		defer wg.Done()
		executed.Store(true)
	})

	wg.Wait()
	if !executed.Load() {
		t.Fatal("expected function to execute cleanly")
	}
}

func TestSafegoConcurrentPanicResilience(t *testing.T) {
	const count = 50
	var wg sync.WaitGroup
	wg.Add(count)

	for i := 0; i < count; i++ {
		idx := i
		safego.Go("concurrent-worker", func() {
			defer wg.Done()
			if idx%2 == 0 {
				panic("simulated panic")
			}
		})
	}

	wg.Wait()
}
