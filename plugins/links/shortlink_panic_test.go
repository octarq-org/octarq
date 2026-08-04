package links

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/plugin"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// testDB opens a throwaway in-memory SQLite database with the tables
// the click worker needs (links + link_events). It migrates on open
// so the worker's flushBatch can do real transactions.
func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Discard,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Link{}, &LinkEvent{}); err != nil {
		t.Fatal(err)
	}
	return db
}

// testEngine builds a minimal Engine backed by an in-memory SQLite DB.
// The supplied publishEvent callback runs inside flushBatch's event-
// publishing phase; it can panic to exercise the recovery path.
func testEngine(t *testing.T, publishEvent func(uint, string, any)) *Engine {
	t.Helper()
	ctx := &plugin.Context{
		PublishEvent: publishEvent,
	}
	e := &Engine{
		db:          testDB(t),
		ctx:         ctx,
		queue:       make(chan clickItem, 100),
		rateLimiter: newIPRateLimiter(300, time.Minute),
	}
	e.wg.Add(1)
	go e.worker()
	return e
}

// TestWorkerSurvivesPanic verifies that the click-worker continues consuming
// items after a panic in PublishEvent. If the safego.Recover in worker() is
// removed, this test fails because the post-panic item never reaches the
// callback.
func TestWorkerSurvivesPanic(t *testing.T) {
	var panicOnce sync.Once
	received := make(chan uint, 10)

	e := testEngine(t, func(orgID uint, _ string, _ any) {
		// Panic on the first call, succeed on the second.
		panicOnce.Do(func() {
			panic("simulated PublishEvent panic")
		})
		received <- orgID
	})

	// Seed links so the click worker can write events with a valid link_id.
	e.db.Create(&Link{ID: 1, Slug: "a", Enabled: true})
	e.db.Create(&Link{ID: 2, Slug: "b", Enabled: true})

	// First item triggers the panic inside PublishEvent.
	e.queue <- clickItem{orgID: 1, linkID: 1, createdAt: time.Now()}

	// Give the worker time to flush, panic, recover, and restart its loop.
	time.Sleep(500 * time.Millisecond)

	// Second item should be consumed normally after recovery.
	e.queue <- clickItem{orgID: 2, linkID: 2, createdAt: time.Now()}

	select {
	case id := <-received:
		if id != 2 {
			t.Fatalf("expected orgID 2, got %d", id)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not resume after panic — second item was never consumed")
	}

	// Clean shutdown must still work.
	e.Close()
}

// TestWorkerCloseAfterPanic verifies that Close() returns promptly after
// the worker has recovered from a panic. If the restart logic treats a
// normal channel close as an exception and re-loops, Close() blocks and
// this test times out.
func TestWorkerCloseAfterPanic(t *testing.T) {
	var calls atomic.Int32

	e := testEngine(t, func(_ uint, _ string, _ any) {
		if calls.Add(1) == 1 {
			panic("boom")
		}
	})

	e.db.Create(&Link{ID: 1, Slug: "c", Enabled: true})

	// Trigger a panic.
	e.queue <- clickItem{orgID: 1, linkID: 1, createdAt: time.Now()}
	time.Sleep(500 * time.Millisecond)

	// Close must return within a reasonable time.
	done := make(chan struct{})
	go func() {
		e.Close()
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(5 * time.Second):
		t.Fatal("Close() blocked — the worker restart loop did not honor channel close")
	}
}
