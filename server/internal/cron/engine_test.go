package cron

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type rejectingGuard struct{}

func (r *rejectingGuard) Acquire(ctx context.Context, jobName string, ttl time.Duration) (func(), bool, error) {
	return nil, false, nil
}

func TestEngine_RegisterUnregisterList(t *testing.T) {
	engine := NewEngine(nil)

	// Validation errors
	if err := engine.Register("", "* * * * *", func(ctx context.Context) error { return nil }); err == nil {
		t.Fatal("expected error for empty job name")
	}
	if err := engine.Register("test", "* * * * *", nil); err == nil {
		t.Fatal("expected error for nil handler")
	}
	if err := engine.Register("test", "invalid spec", func(ctx context.Context) error { return nil }); err == nil {
		t.Fatal("expected error for invalid spec")
	}

	// Register 2 jobs
	err := engine.Register("b_job", "0 * * * *", func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected register error: %v", err)
	}

	err = engine.Register("a_job", "0 0 * * *", func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected register error: %v", err)
	}

	// List should be sorted alphabetically: a_job, b_job
	jobs := engine.List()
	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(jobs))
	}
	if jobs[0].Name != "a_job" || jobs[1].Name != "b_job" {
		t.Fatalf("expected jobs sorted, got %s, %s", jobs[0].Name, jobs[1].Name)
	}

	// Unregister
	if err := engine.Unregister("nonexistent"); err == nil {
		t.Fatal("expected error unregistering nonexistent job")
	}
	if err := engine.Unregister("a_job"); err != nil {
		t.Fatalf("unexpected unregister error: %v", err)
	}
	if len(engine.List()) != 1 {
		t.Fatalf("expected 1 job left, got %d", len(engine.List()))
	}
}

func TestEngine_LifecycleAndExecution(t *testing.T) {
	engine := New("")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var counter int64
	err := engine.Register("fast_job", "@every 100ms", func(ctx context.Context) error {
		atomic.AddInt64(&counter, 1)
		return nil
	})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	// Double start error
	if err := engine.Start(ctx); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	if err := engine.Start(ctx); err == nil {
		t.Fatal("expected error on duplicate start")
	}

	// Wait for job to run at least once
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&counter) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if atomic.LoadInt64(&counter) == 0 {
		t.Fatal("expected job to run at least once")
	}

	jobs := engine.List()
	if len(jobs) != 1 || jobs[0].LastRun.IsZero() {
		t.Fatalf("expected last run updated, got %+v", jobs)
	}

	// Stop engine
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	if err := engine.Stop(stopCtx); err != nil {
		t.Fatalf("stop failed: %v", err)
	}

	// Stop when not running is a no-op
	if err := engine.Stop(stopCtx); err != nil {
		t.Fatalf("expected nil when stopping idle engine: %v", err)
	}
}

func TestEngine_ExecutionFailureAndPanic(t *testing.T) {
	engine := NewEngine(nil)
	ctx := context.Background()

	// Error job
	_ = engine.Register("err_job", "* * * * *", func(ctx context.Context) error {
		return errors.New("custom job failure")
	})
	if err := engine.RunJob(ctx, "err_job"); err == nil || err.Error() != "custom job failure" {
		t.Fatalf("expected 'custom job failure', got %v", err)
	}
	jobs := engine.List()
	if len(jobs) != 1 || jobs[0].Status != "error" || jobs[0].LastErr != "custom job failure" {
		t.Fatalf("expected error status and lastErr recorded, got %+v", jobs)
	}

	// Panic job
	_ = engine.Register("panic_job", "* * * * *", func(ctx context.Context) error {
		panic("kaboom")
	})
	if err := engine.RunJob(ctx, "panic_job"); err == nil || err.Error() != "panic: kaboom" {
		t.Fatalf("expected panic error, got %v", err)
	}

	// Non-existent job
	if err := engine.RunJob(ctx, "unknown"); err == nil {
		t.Fatal("expected error for nonexistent job")
	}
}

func TestEngine_GuardRejection(t *testing.T) {
	engine := NewEngine(&rejectingGuard{})
	ctx := context.Background()

	var ran bool
	_ = engine.Register("blocked_job", "* * * * *", func(ctx context.Context) error {
		ran = true
		return nil
	})

	_ = engine.RunJob(ctx, "blocked_job")
	if ran {
		t.Fatal("expected job not to run when guard rejects acquire")
	}
}

func TestEngine_StopTimeout(t *testing.T) {
	engine := NewEngine(nil)
	ctx := context.Background()

	started := make(chan struct{})
	blocker := make(chan struct{})

	_ = engine.Register("slow_job", "@every 50ms", func(ctx context.Context) error {
		close(started)
		<-blocker
		return nil
	})

	_ = engine.Start(ctx)
	<-started

	// Attempt stop with already expired context
	expiredCtx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	time.Sleep(10 * time.Millisecond)
	cancel()

	err := engine.Stop(expiredCtx)
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context timeout on Stop, got %v", err)
	}

	// Unblock to allow clean exit
	close(blocker)
	_ = engine.Stop(context.Background())
}
